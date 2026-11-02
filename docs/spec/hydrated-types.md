# Data types and the boundary to hydration

**Owner decision, 2 November 2026: Ontos and Horos remain data-only.** Bittype
adds the type description for Wire above Horos; hydration remains above Horos
in the existing Wire/runtime layers. This replaces this page's earlier proposal
for a Horos declaration revision 2 and `horos/wire`. Neither was implemented.
There is no replacement Horos constructor, compatibility spelling or widened
Horos value domain. This change is documentation only.

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
runtime, authority and lifetime, so it is not an Ontos value. A name or encoded
reference can be ordinary data; interpreting it as a capability requires a
separate binding context. Horos may check the data representation, but does not
resolve that name, establish authority or recognize a live endpoint.

`any`, complements, open records, collections, exact literals and recursive
descriptors retain their existing ground interpretation. In particular,
`any()` does not start accepting Wires. Unknown constructors remain invalid.

## Structure does not require widening data

[Deixis's slot contract](https://github.com/Bitspark/deixis/blob/e0b0410d6f76d795992cc0a5792c507a19ef05ee/docs/SLOTS.md)
allows opaque contents, including live capabilities. Structural selection does
not interpret them or confer portable identity, equality or serialization.
Ontos is a ground data model; the more general Deixis carrier does not require
Ontos or Horos to adopt every possible slot domain. See the
[Ontos core](https://github.com/Bitspark/ontos/blob/a3f1d081f2e82f1e0ec39688c4060b5606b16c63/docs/spec/ontos-core.md).

Bitwire already defines a separate live domain:

```text
HydratedValue = ByteAtom | HydratedTuple(HydratedValue*) | Wire
```

Ground values embed unchanged. Wires are opaque sending capabilities recognized
by the runtime; hydrated tuples may contain them. The
[native hydrated contract](https://github.com/Bitspark/bitwire/blob/e2d2d641744a49f1d8098b6ea5f2724454d72b96/docs/wire/hydrated-native.md)
places this boundary above ground Wire without extending Ontos. Hydration
conveys sending faces through exports and proxies; receiving and closure remain
owner operations. None of this is Horos descriptor checking.

## Ownership and composition

| Component | Responsibility |
| --- | --- |
| Deixis | Generic structure over opaque contents |
| Ontos | Ground data values, structural identity and encoding |
| Horos | Data descriptors, data declarations, specialization and conformance |
| Bittype contract-language provider | Proposed Wire message type and composition of data types with capability types |
| Bittype agnostic kernel | Generic trees, semantic facts and provider registration; no Wire semantics |
| Bitwire / bitruntime | Existing capability contracts and hydration realization above Horos |
| Contract libraries | Function, Stream, Cell declarations, representations and behavioral laws |

The [Bittype proposal](https://github.com/Bitspark/bittype/blob/main/docs/hydrated-types.md)
owns the exact proposed Wire expression and illustrative library compositions.
It is a separate language layer, not a new Horos revision or an extension of
the Horos frontend. Horos has no dependency on Bittype, Wire or a live runtime.

A description of a Wire can itself be stored as an Ontos value. That describes
the syntax's storage, not its meaning: being encoded as Ontos does not make
every type-description language a Horos descriptor. Horos can describe the
encoded syntax as data without interpreting the capability that it denotes.

`Page<Person>` remains a Horos data application. `Page<Wire<Message>>` is not
an admissible application of that data declaration. If Bittype reuses Page's
structure over capabilities, it must own an explicit lifting interpretation,
preserve the source declaration and bindings, and qualify the supported forms.
Such a lift can be generated; it does not require hand-authoring an expanded
Page. It must not silently widen Horos's generic domain or pass a live result
to `WF`/`Conforms` as ordinary data.

## Scope and forbidden changes

Function, Stream and Cell remain ordinary library declarations above this
boundary. Their protocols do not become Horos constructors, Wire operations or
hydration cases. There is no structural fingerprint or name-based dispatch that
selects their behavior in a general generator, registry or runtime.

Do not add Wire to the Horos vocabulary, accept capabilities in Ontos containers,
widen ground checks, or introduce an implicit revision conversion. A Horos
analysis result establishes data declaration validity, not endpoint recognition
or remote protocol compliance. Loading a type description performs no sends,
binding, exporting or resource allocation.

Implemented declaration revision 1 and published 0.6.0 ground packages retain
their existing scope. Bittype's capability-type provider, its native mappings
and the illustrative library declarations remain proposals. This document adds
no implementation, dependency, runtime binding, test vector or package release.
