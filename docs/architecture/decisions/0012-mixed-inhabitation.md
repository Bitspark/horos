# A mixed value inhabits a materialized Descriptor structurally

Status: accepted, 12 November 2026. Requested by Bittype research 0014, row 28
([bittype#194](https://github.com/Bitspark/bittype/issues/194)).

## Context

A binding backend receives values that mix data with sending capabilities, and it
must check them against contracts declared in Horos. A Cell is a closed record
whose `get` and `set` fields are capabilities. ontos declined to own the check:
ontos/wire admits a mixed value's own structure, but whether it fits a declared
type is type meaning, which ontos refuses
([ontos design record 0007](https://github.com/Bitspark/ontos/blob/v0.17.0/docs/design/0007-optional-wire-extension.md)).

Horos does not supply that check either. The
[data/hydration boundary](../../spec/hydrated-types.md) keeps ground checks
data-only and adds no live-value checker, and nothing in its "Forbidden changes"
is relaxed here. But Horos owns the meaning. `Program.Descriptor` materializes
closed proper types that may contain `wire(E)`, and until now no document said
which mixed values such a type describes. A backend would have to guess, and two
backends could guess differently.

Formation already narrows the question. In a materialized Descriptor, `wire` can
occur only at the root and in the type positions of `tuple`, `compound`,
`list-of`, a closed `record`, `union` and another `wire`. Every other type
position takes a Data operand: `seq`, `set-of`, `map-of`, `and`, `not`, a `rec`
body and an open record's fields. So the part of a Descriptor that can hold
capabilities is a finite tree without recursion, and everything below it is an
ordinary ground descriptor.

## Decision

1. **Horos specifies, the backend implements.** The judgment *v inhabits T* is
   defined for an admitted ontos/wire mixed value v (profile revision 1: atoms,
   capability leaves and tuples) and a materialized Descriptor T. It is normative
   in the [mixed inhabitation specification](../../spec/mixed-inhabitation.md),
   with a replayable corpus, `vectors/mixed-inhabitation.json`. Horos adds no
   checker, API, package or dependency for it. A binding backend implements it,
   and a backend that claims the judgment must agree with every case in the
   corpus.
2. **Positions without wire are ground conformance.** If a descriptor node
   contains no `wire` in its type positions, v inhabits it exactly when v holds
   no capability leaf and its ground recovery (0007's `recover`) conforms to that
   node by the unchanged ground judgment. That includes `any`, open records,
   refinements and `rec`/`ref`, with lexical scoping (ADR 0011). A backend can
   reuse Horos's published ground checker for these positions. Two definitions
   of the same ground descriptor could otherwise drift apart.
3. **The six mixed forms lift the ground rules unchanged.** `tuple`, `compound`,
   `list-of`, closed `record` and `union` keep their ground rules: arity, the
   label atom, the `list` label, the `map` label with canonical text keys in codec
   order, required, optional and closed fields, and at least one alternative.
   The only change is that a child may be a mixed value. `wire` is the sixth form.
4. **`wire(E)` admits exactly one capability leaf, and E is not consulted.** No
   atom or tuple inhabits a `wire` position, however much its bytes resemble a
   reference, and a capability leaf inhabits no other position.
   - **Why not E-level structure.** The leaf is opaque. 0007 gives it no
     structure, equality, identity or encoding that could be compared with E.
     What E constrains is the messages sent through the capability. That belongs
     to each message's own check and to protocol compliance, not to the value
     holding the capability. Checking a leaf against E would require recognizing
     what the capability is, which is the runtime's capability-recognition
     judgment.
   - **What a backend may add.** A backend whose runtime knows a handle's
     message type may check it in addition, as capability recognition. Doing so
     is not inhabitation and must not change the verdict this judgment reports.
5. **`any` and open records stay data-only.** A capability leaf under `any`, or
   in an open record's extra fields, does not inhabit. Formation already gives
   open records Data operands. Widening `any` is forbidden: a position authored
   as "any data" would quietly become a channel for authority.
   - A contract that carries an open-ended collection of capabilities says so,
     for example with `list-of(wire(E))` or named closed fields.
6. **Outcomes follow ADR 0008.** The verdicts are `conforms`, `mismatch` and
   `exhausted`.
   - **Product forms** (`tuple`, `compound`, `list-of`, closed `record`): a
     mismatching part decides `mismatch` even if another part is exhausted;
     otherwise exhaustion takes precedence over conformance.
   - **`union`:** a conforming alternative decides `conforms`; otherwise
     exhaustion takes precedence over mismatch.
   - **Where exhaustion comes from.** It comes from a delegated ground check,
     each of which is a separate top-level ground check with its own profile, or
     from a budget the backend states. It never establishes membership or
     non-membership.
   - **Inputs outside the domain.** A value that is not an admitted mixed value,
     and a descriptor that is not a materialized Descriptor, are outside the
     judgment. A backend refuses them before judging.
   - **No locations yet.** This decision specifies no diagnostic locations for
     mixed values.
7. **Nothing else follows from inhabitation.** It does not establish:
   - capability recognition, authority, liveness or scope;
   - handle identity: the judgment never compares two leaves, so aliased and
     distinct handles are judged alike;
   - message types or protocol compliance.

   It does not hydrate, export or send anything. The judgment table in
   [Wire types](../../spec/wire-types.md) places it after mixed structural
   admission and before capability recognition, and it establishes neither.
8. **The corpus writes mixed values in ontos's value-tree notation**,
   `{"atom": hex}` and `{"tuple": [...]}`, with one third leaf, `{"wire": name}`.
   - **Why a notation.** No codec exists for a capability (0007 defines none), so
     a mixed value has no bytes.
   - **Why not hex with placeholders.** A ground skeleton in hex, with
     placeholder atoms where capabilities go, would be ambiguous: 0007 forbids
     treating bytes that resemble a reference as a capability.
   - **How a harness reads it.** It reads atoms and tuples with the reader it
     already has, and turns each `wire` leaf into a test handle. Within one
     value, equal names denote one handle and different names distinct handles,
     so aliasing can be expressed. No verdict depends on a name.
   - **Descriptors stay hex.** They are ground values, so they are
     ontos-codec-v1 hex as in the other corpora.
9. **Evidence without a checker.** `vectors/mixed-inhabitation.py` encodes the
   authored inputs and checks their form; it computes no verdicts. Every verdict
   is written from the specification. Go tests check the corpus's form:
   - each descriptor is admitted by `Program.Proper` and returned unchanged by
     `Program.Descriptor`;
   - for each descriptor without `wire`, every verdict equals the ground
     judgment;
   - each position the specification excludes refuses `wire`.

## Consequences

A backend implements one small structural walk over the mixed part of a
Descriptor and hands every capability-free part to the ground checker it already
has. Bittype research 0014's row 28 has its owners: the binding backend for the
implementation, and Horos for the specification and the corpus.

What a capability position means can only become more specific by adding another
judgment, never by changing this one. Typed handle recognition, for example,
would be a separate judgment. If a later revision of ontos/wire changes its leaf
kinds, this decision is revisited. Horos's ground judgments, `any` and the
declaration language are unchanged.
