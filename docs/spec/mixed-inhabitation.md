# Mixed values in materialized Descriptors

**Normative, 12 November 2026.** This defines when an
[ontos/wire](https://github.com/Bitspark/ontos/blob/v0.17.0/docs/design/0007-optional-wire-extension.md)
mixed value inhabits a type that Horos materializes with `Program.Descriptor`.

Horos specifies the judgment and supplies a corpus; it supplies no checker. A
binding backend implements the judgment
([bittype#194](https://github.com/Bitspark/bittype/issues/194), row 28). The
decision and its reasons are in [ADR 0012](../architecture/decisions/0012-mixed-inhabitation.md).
Ground `wf`, `conforms` and `diagnose`, `any`, and the
[data/hydration boundary](hydrated-types.md) are unchanged.

## 1. Inputs

A **mixed value** is an admitted value of the ontos/wire profile, revision 1:

```text
MixedValue = Atom(bytes) | Wire(handle) | Tuple(MixedValue*)
```

The handle is opaque. Inhabitation never inspects it and never compares two
handles. A mixed value with no `Wire` leaf is *wire-free*, and its *recovery* is
the ground ontos value with the same atoms and tuples (0007's `recover`).

A **materialized Descriptor** is a closed proper type expression returned by
`Program.Descriptor` (see [declarations](declarations.md#7-checked-values-and-the-ground-boundary)).
It is an ordinary ontos value. It may contain `horos/wire(E)`, which denotes a
sending capability for messages of type E (see [Wire types](wire-types.md)).

The judgment is defined only on these inputs. A value outside the profile, or an
expression that `Program.Descriptor` would not return, has no verdict, and a
backend refuses it before judging.

## 2. Where `wire` occurs

Formation admits `wire` at the root of a Descriptor and in exactly these type
positions:

| Form | Type positions that may hold `wire` |
| --- | --- |
| `tuple(T0, …)` | every child |
| `compound(label, T0, …)` | every child after the label |
| `list-of(T)` | the element type |
| `record(closed, field…)` | every field type |
| `union(T0, …)` | every alternative |
| `wire(E)` | the message type |

`seq`, `set-of`, `map-of`, `and`, `not`, a `rec` body and the field types of an
open record take Data operands, so they never contain `wire`. Literal operands
are not type positions: an `exact` value, a label atom, a field name or a
recursion name shaped like `horos/wire(…)` is data.

A descriptor node is **mixed** if it is a `wire` node, or one of the forms above
with a mixed node in one of those positions. Every other node is **ground**. The
judgment reaches a ground node only at the root or as a child of a mixed node,
and such a node is a well-formed, closed ground descriptor. No `ref` inside it
denotes a `rec` outside it, because a `rec` body is never mixed, so no `rec`
encloses a mixed node.

## 3. The judgment

v inhabits T is decided by the first rule that applies.

1. **A ground node T.** v inhabits T exactly when v is wire-free and its recovery
   conforms to T by the ground judgment of the [form contract](horos.md). That
   ground check is a separate top-level check of the recovered value against T,
   with its own profile and limits.
2. **`wire(E)`.** v inhabits T exactly when v is a `Wire` leaf. E is not consulted.
3. **`tuple(T0, …, Tn-1)`.** v is a tuple of arity n, and each item vi inhabits Ti.
4. **`compound(Atom(label), T0, …, Tn-1)`.** v is a tuple of arity n + 1 whose item
   0 is exactly that label atom, and each item vi+1 inhabits Ti.
5. **`list-of(E)`.** v is a tuple whose item 0 is the atom `list`, and every later
   item inhabits E.
6. **`record(closed, field…)`.** All of the following hold:
   - v is a tuple whose item 0 is the atom `map`;
   - every later item is a tuple of arity 2, a *key* and a *field value*;
   - every key is wire-free, and its recovery is a canonical `utf8-text`;
   - the keys' ontos-codec-v1 encodings are strictly ascending;
   - every required field's name is a key;
   - every key is a field name;
   - every field value inhabits the type of the field its key names.

   An optional field may be absent. An entry holding `null` is present, not absent.
7. **`union(T0, …)`.** At least one Ti is inhabited by v.

These are the ground rules of `tuple`, `compound`, `list-of`, closed `record` and
`union` with mixed children. The only new rule is rule 2.

Consequences that the corpus pins:

- No atom or tuple inhabits `wire(E)`, even if its bytes look like a reference,
  and a `Wire` leaf inhabits no position except `wire(E)`.
- `any` is data-only. A `Wire` leaf under `any`, in an open record's extra fields,
  or in any other ground node does not inhabit it.
- Aliasing changes nothing. `(w, w)` and `(w1, w2)` inhabit `tuple(wire(E), wire(E))` alike.
- A Descriptor without `wire` is a ground descriptor, and inhabitation there is
  ground conformance of the recovered value.

## 4. Outcomes

A verdict is `conforms`, `mismatch` or `exhausted`, combined as in
[ADR 0008](../architecture/decisions/0008-diagnostic-outcomes-kleene.md):

- **Product forms.** In `tuple`, `compound`, `list-of` and closed `record`, a
  mismatching part decides `mismatch` even if another part is exhausted.
  Otherwise any exhausted part makes the result `exhausted`.
  - The parts are the shape conditions and the children.
- **`union`.** A conforming alternative decides `conforms`; otherwise any
  exhausted alternative makes the result `exhausted`.
- **Where exhaustion comes from.** It comes from a delegated ground check (rule
  1), or from a budget the backend states for its own walk. Exhaustion
  establishes neither membership nor non-membership.

The mixed part of a Descriptor has no recursion, so the walk over it is bounded
by the sizes of T and v. A boolean form reports `true` exactly for `conforms`. As
with the ground boolean API, its `false` for an exhausted check is a bounded
answer, not a completed mismatch. This revision defines no diagnostic locations
for mixed values.

## 5. What inhabitation does not establish

Inhabitation does not establish:

- that a handle is recognized by a runtime, carries authority, is live or is in
  scope;
- which messages a handle accepts, or that they have type E;
- anything about a library's protocol;
- identity: it never compares handles.

It sends, exports and hydrates nothing. In the
[judgment table](wire-types.md#3-tooling-and-runtime-boundaries) it follows
ontos/wire's mixed structural admission and precedes capability recognition, and
it establishes neither. A backend may check more, for example a handle's message
type, as a separate judgment. That check must not change this verdict.

## 6. Corpus

[`vectors/mixed-inhabitation.json`](../../vectors/mixed-inhabitation.json) holds
cases authored from this specification. It is generated from
`vectors/mixed-inhabitation.py`, which encodes the inputs and checks their form.

```text
{"format": "horos-mixed-inhabitation-v1",
 "cases": [{"name", "note", "descriptor": "<ontos-codec-v1 hex>",
            "values": [{"name", "value": <tree>, "verdict": "conforms" | "mismatch" | "exhausted"}]}]}

tree = {"atom": "<lowercase hex>"} | {"tuple": [tree, …]} | {"wire": "<name>"}
```

The value tree is ontos's authoring notation for ground values plus the `wire`
leaf. A harness creates one test handle per distinct name within a value, so
equal names express aliasing. No verdict depends on a name. A backend that
claims this judgment replays every case and reports each verdict.

Horos's Go tests check the corpus's form:

- every descriptor is admitted by `Program.Proper` and returned unchanged by
  `Program.Descriptor`;
- for each descriptor without `wire`, every verdict equals the ground judgment;
- every position excluded in section 2 refuses `wire`.

They do not judge values against descriptors that contain `wire`.
