# Data cores, optional Wire extensions and hydration

**Owner direction, 3 November 2026; alignment updated 4 November.**
The existing Ontos and Horos cores remain data-only. Optional `ontos/wire` and
`horos/wire` extensions own mixed values and their type/declaration
semantics. Bittype supplies tooling for those canonical definitions; hydration
remains in the existing Wire/runtime layers.

This replaces the 2 November proposal for a Bittype-owned
`bittype/contract/*` model. The earlier proposal for a Wire constructor inside
ground Horos declaration revision 2 also remains superseded. Neither proposal
was implemented, and neither becomes a compatibility format. The optional
[horos/wire profile](wire-types.md) has its own envelope, revision and
interpretation. No existing Horos constructor, generic domain or ground judgment
changes. The [static Go API](../../core/go/wire/README.md) implements the optional
declaration profile and closed ground embedding; live runtime integration remains separate.

## What Horos describes

The [form contract](horos.md) describes sets of finite, ground Ontos values.
The [declaration model](declarations.md) adds modules, imports, generic parameters
and applications around those data types. Specialization produces a ground
descriptor for the existing checker. A generic parameter is a slot for a data
type; it is not permission to substitute a live capability.

```text
GroundValue = ByteAtom | Tuple(GroundValue*)
```

Ontos values are immutable and context-free. A live endpoint depends on a
runtime, authority and lifetime, so it is not a ground Ontos value. A name or encoded
reference can be ordinary data; interpreting it as a capability requires a
separate binding context. Horos may check the data representation, but does not
resolve that name, establish authority or recognize a live endpoint.

`any`, complements, open records, collections, exact literals and recursive
descriptors retain their existing ground interpretation. In particular,
`any()` does not start accepting Wires. Unknown constructors remain invalid.

Declaration revision 1 retains its existing scope in Horos 0.8.0. The release
includes both Go declaration packages, while Rust and TypeScript retain their
ground-only implementations. Source identity, binder identity, application
resolution and explicit specialization remain as specified in
[declarations](declarations.md).

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
| Optional ontos/wire | Go/TypeScript mixed value profile, native tuple structure and exact ground correspondence |
| Horos core and data declarations | Data descriptors, declaration binding, specialization and ground conformance |
| Optional horos/wire | Static Go mixed type/declaration profile and explicit closed ground embedding |
| Bittype | AST/CST and semantic projections, provider composition, transformations and native type generation |
| Bitwire / bitruntime | Existing sending-capability contracts and hydration realization |
| Contract libraries | Function, Stream, Cell declarations, representations and behavioral laws |

The [horos/wire profile](wire-types.md) owns the canonical Wire expression,
module format and illustrative Function, Cell and Stream library compositions.
It is an optional semantic profile, not a widening of the ground profile.
Its static definitions must not depend on Bittype, Bitwire or a live runtime.
Bittype's [tooling projection](https://github.com/Bitspark/bittype/blob/main/docs/hydrated-types.md)
delegates those semantics rather than maintaining a second canonical algebra.

A description of a Wire can itself be stored as an Ontos value. That describes
the syntax's storage, not its meaning: being encoded as Ontos does not make
every type-description language a Horos descriptor. Horos can describe the
encoded syntax as data without interpreting the capability that it denotes.
Structural admission, runtime recognition, current operational validity and
remote protocol compliance are separate judgments; none follows merely from a
well-formed type description or from recognizing a Wire leaf.

A ground `Page<Person>` remains a Horos data application. To support both data
and capabilities, author `Page<T>` directly in horos/wire and apply that same
declaration to `Data(PersonDescriptor)`, `Wire(Message)` or a library-owned Cell.
The parameter already has the intended optional-profile domain; no copied
template or derived module is needed. See the [direct pagination example](wire-types.md#4-direct-authoring-and-ground-embedding).
A closed ground specialization can separately be embedded as `Data(G)`. That
operation creates no declaration and never widens an existing ground slot.
The owner removed template lifting on 4 November 2026; there is no compatibility
API or Bittype transformer for it.

Library-owned native representations, dynamic adapters, binding contexts,
resource ownership and static-versus-dynamic limits remain in Bittype's
[type representation](https://github.com/Bitspark/bittype/blob/main/docs/type-representations.md)
and [runtime value](https://github.com/Bitspark/bittype/blob/main/docs/runtime-values.md)
designs. Moving canonical semantics to optional Horos/Ontos profiles does not
remove those ideas or implement them.

## Scope and forbidden changes

Function, Stream and Cell remain ordinary library declarations above this
boundary. Their protocols do not become Horos constructors, Wire operations or
hydration cases. There is no structural fingerprint or name-based dispatch that
selects their behavior in a general generator, registry or runtime.

Do not add Wire to the ground Horos vocabulary, accept capabilities in ordinary
Ontos containers, widen ground checks, infer a semantic profile from Wire
occurrences, or introduce an implicit revision conversion. Do not select a
library behavior from a name or structural fingerprint. A static analysis
result does not prove endpoint recognition or protocol compliance. Loading a
description performs no sends, capability binding, exporting or resource allocation.

The optional Go profiles and Bittype frontends are implemented. Native mappings,
runtime bindings and the illustrative library protocols remain separate work. The
[cross-repository preservation map](https://github.com/Bitspark/bittype/blob/main/docs/wire-extension-preservation.md)
records the retained concepts and explicitly superseded ownership choices.
