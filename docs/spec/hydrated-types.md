# One type language, distinct value domains

**Owner directions, 3-4 November 2026; unified declaration decision supersedes
the two-profile design.** Ontos ground values and Horos ground checks remain
data-only. Horos now has one declaration language for data and capability types,
with explicit Type/Data generic bounds. There is no separate Horos/Wire language.
Ontos/wire remains an optional *value* model, not a second declaration grammar.

A live capability depends on runtime authority and lifetime. Its description can
be stored as ordinary Ontos data without storing or validating an endpoint.
`any`, complement and all existing ground descriptors keep their meanings.
The unified `H("wire", E)` expression is not a ground descriptor passed to
WF/Conforms. `Program.DataDescriptor` explicitly establishes that boundary.

## Structure and optional value profiles

[Deixis's slot contract](https://github.com/Bitspark/deixis/blob/e0b0410d6f76d795992cc0a5792c507a19ef05ee/docs/SLOTS.md)
allows opaque contents, including live capabilities. Structural selection does
not interpret them or confer portable identity, equality or serialization.
The [ground Ontos projection](https://github.com/Bitspark/ontos/blob/a3f1d081f2e82f1e0ec39688c4060b5606b16c63/projection/deixis/PROJECTION.md)
uses the Deixis structure with optional byte contents. The
[ontos/wire extension](https://github.com/Bitspark/ontos/blob/v0.16.0/docs/design/0007-optional-wire-extension.md)
implements a separate recognized profile in Go and TypeScript with
byte-or-capability payloads: an atom leaf, an opaque capability leaf or a dense
positional tuple. Arbitrary Deixis nodes do not become mixed values. This uses Deixis as a value foundation, not as a
replacement for type semantics or ASTs.

The profile must preserve immutable bytes, captured tuple children and exact
ground embedding/recovery. It adds no portable equality, hashing or codec for
live capabilities. The more general Deixis carrier does not require the
[Ontos core](https://github.com/Bitspark/ontos/blob/a3f1d081f2e82f1e0ec39688c4060b5606b16c63/docs/spec/ontos-core.md)
or Horos's existing judgments to admit a live value.

Bitwire already defines a separate live domain:

```text
HydratedValue = ByteAtom | HydratedTuple(HydratedValue*) | Wire
```

Bitwire 0.7.0 specializes the Ontos 0.16.0 native mixed value model for its
sending contract. Bitruntime 0.8.0 uses the shared immutable tuple factory with
runtime-owned handle recognition. The
[native hydrated contract](https://github.com/Bitspark/bitwire/blob/v0.7.0/docs/wire/hydrated-native.md)
keeps raw Wire ground-only. The pure reference/frame codec remains in Bitwire;
exports, proxies, ownership, admission and lifetimes remain in Bitruntime.
Neither structural capture nor conversion sends, exports or tests liveness.
The explicit Deixis projection is not inserted into every send. Bitruntime does
not consult Horos to hydrate values. None of this supplies a public Horos
mixed-value checker or changes ground descriptor checking.

## Ownership and composition

| Component | Responsibility |
| --- | --- |
| Deixis | Generic structure over opaque contents |
| Ontos core | Ground data values, structural identity and encoding |
| Optional ontos/wire | Mixed value structure and exact ground embedding/recovery |
| Horos | Ground descriptors/conformance; unified declarations, bounds, resolution and specialization |
| Bittype | AST/CST and semantic projections, provider composition and future transformations |
| Bitwire / bitruntime | Sending capabilities and hydration realization |
| Contract libraries | Function, Stream, Cell declarations, representations and behavioral laws |

The [declaration model](declarations.md) authors `Page<T>` once, with a Type-bound
slot. `DataPage<D: Data> = Page<D>` restricts arguments through an ordinary alias.
Data arguments need no wrapper. Closed data specialization returns the existing
ground descriptor; mixed specialization stays a static type expression. The
[Wire examples](wire-types.md) show library-owned composition. No lifting,
embedding API or second declaration identity remains.

Library-owned native representations, dynamic adapters, binding contexts,
resource ownership and static-versus-dynamic limits remain in Bittype's
[type representation](https://github.com/Bitspark/bittype/blob/main/docs/type-representations.md)
and [runtime value](https://github.com/Bitspark/bittype/blob/main/docs/runtime-values.md)
designs. This work implements no generators or runtime bindings.

## Forbidden changes

Do not add Function, Stream or Cell kinds, name switches, built-in registrations
or structural fingerprints to general tooling, Wire, runtime or hydration.
Do not admit capabilities in ground Ontos values or widen Horos ground any.
Do not infer a language from Wire occurrences, accept retired envelopes, wrap
data schemas in a second profile or silently transform old modules. Preserve
literal values and the distinction between static validity, runtime recognition,
current authority and protocol compliance. Loading declarations has no effects.
