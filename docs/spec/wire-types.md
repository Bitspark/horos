# Wire types in unified Horos

**Revision-4 contract, 4 November 2026; implementation in progress.** Wire is a
primitive static type expression in the one
[canonical declaration language](declarations.md). General structural kinds and
constructor contracts extend that language without adding another profile,
envelope or Data wrapper. The
[Go declarations API](../../core/go/declarations/README.md) specifies static type
operations. Ground conformance remains a separate operation; no live-value
checker or runtime binding is added here.

## 1. Primitive expression and ordinary kinds

Use the exact A/T/I/L/M/H/D/Pin notation from
[the declaration specification](declarations.md#1-representation-and-notation).
`H("wire", E)` denotes a sending capability accepting messages of proper type E.
The expression is ordinary Ontos data and contains no endpoint. Its message may
be any admitted proper type, but not an unapplied type constructor. A Wire type
does not belong to the ordinary bundled Data kind, regardless of its message.
Static validity proves neither endpoint
authority nor protocol compliance; loading a declaration sends nothing and
allocates no capability.

Tuple, compound, list-of, closed record and union can contain Wire types and
remain the same structural constructors used by data declarations. Their
membership in the ordinary Data kind follows the same structural rules as any
other kind query. Other constructors retain operand requirements expressed
through the bundled ordinary Data definition. `H("any")` still denotes data only. Use a ground
descriptor G directly, without a `Data(G)` wrapper.

Type and Data are ordinary definitions in the explicitly imported `horos.kinds`
module. Type is proper top; Data is a positive recursive union of data-shaped
forms. `wire<Person>` belongs to Type but not Data; the bytes describing it are
still Ontos data. A kind such as `Wire<Scalar>` can describe a narrower Wire
family using the same general machinery. The
[Page example](declarations.md#5-complete-pagination-example) authors one
`Page<T>`, used for `Page<Person>` and `Page<Cell<Person>>`.
`DataPage<D: Data> = Page<D>` restricts callers without copying Page.
A constructor parameter can promise a Data result, but merely accepting a Data
argument does not establish one: `Send<T> = wire<T>` never returns Data.

`Value`, `Proper` and `Apply` work with checked type values sealed to the same
Program. An application can return a constructor when its result contract says
so. `Descriptor` materializes a closed proper type expression; `GroundDescriptor`
additionally establishes ordinary ground-kind membership and unchanged ground WF
before returning the existing descriptor. These operations create no runtime
value model and do not recognize a live endpoint.

## 2. Library-owned Function, Cell and Stream

The complete module equations below illustrate ordinary library composition.
They specify no installed protocol or runtime implementation. Prelude denotes
the bundled canonical `horos.kinds` value exposed by `declarations.Prelude()`.
Helpers are value abbreviations:

```text
Kinds                 = D("import", T("horos.kinds"), Pin(Prelude))
KType                 = D("kind-imported", T("k"), T("Type"))
P(i)                  = D("param", I(i))
Param(name)           = D("parameter", T(name), KType)
Def(parameters, body) = D("type", parameters, KType, body)
Ref(alias, name)      = D("imported", T(alias), T(name))
Use(alias, name, ...) = D("apply", Ref(alias, name), L(...))
Local(name, ...)      = D("apply", D("local", T(name)), L(...))

WireLibrary = D("module", I(4), T("example.wire"), M{
  "k": Kinds
}, M{
  "Wire": Def(L(Param("M")), H("wire", P(0)))
})

FunctionLibrary = D("module", I(4), T("example.contracts.function"), M{
  "k": Kinds,
  "wire": D("import", T("example.wire"), Pin(WireLibrary))
}, M{
  "Unit": Def(L(), H("tuple")),
  "Function": Def(L(Param("A"), Param("B")),
    Use("wire", "Wire", H("tuple", P(0), Use("wire", "Wire", P(1)))))
})

CellLibrary = D("module", I(4), T("example.contracts.cell"), M{
  "k": Kinds,
  "fn": D("import", T("example.contracts.function"), Pin(FunctionLibrary))
}, M{
  "Cell": Def(L(Param("T")), H("record", A("closed"),
    H("field", T("get"), A("req"),
      Use("fn", "Function", Ref("fn", "Unit"), P(0))),
    H("field", T("set"), A("req"),
      Use("fn", "Function", P(0), Ref("fn", "Unit")))))
})

StreamLibrary = D("module", I(4), T("example.contracts.stream"), M{
  "k": Kinds,
  "fn": D("import", T("example.contracts.function"), Pin(FunctionLibrary))
}, M{
  "Step": Def(L(Param("T")),
    H("union", H("compound", A("item"), P(0)), H("compound", A("end")))),
  "Stream": Def(L(Param("T")), H("record", A("closed"),
    H("field", T("next"), A("req"),
      Use("fn", "Function", Ref("fn", "Unit"), Local("Step", P(0)))),
    H("field", T("cancel"), A("req"),
      Use("fn", "Function", Ref("fn", "Unit"), Ref("fn", "Unit")))))
})
```

Use and Local require nonempty argument lists. Unit has no parameters, so its
uses are references, not empty applications. Each Pin is the SHA-256 atom of
the expanded earlier module's canonical bytes, not a package version or
placeholder string. The library names are illustrative, never registrations.
Their readable meanings are:

```text
Unit          = H("tuple")
Function<A,B> = Wire<tuple(A, Wire<B>)>
Cell<T>       = record closed { get: Function<Unit,T>, set: Function<T,Unit> }
Step<T>       = compound("item", T) | compound("end")
Stream<T>     = record closed {
  next: Function<Unit,Step<T>>
  cancel: Function<Unit,Unit>
}
```

`H("tuple")` describes the empty tuple; it is not the value itself. Function is
illustrative and success-only. Its library owns fresh reply endpoints, matching,
one-reply discipline, failures, completion and resource management. Unit is not
automatically native void. Cell owns state and mutation laws. The pull Stream
owns demand, ordering, concurrent-next policy and end/cancel races. These behaviors
do not follow from the shapes; production bindings need their own protocol and
qualification.

Library-owned native recipes are selected by exact declaration identity and
target representation profile. They preserve parameter order, locked imports,
aliases and origins. An optional custom representation does not become executable
adapter code or proof of behavior. Bittype owns the
[native representation tooling proposal](https://github.com/Bitspark/bittype/blob/main/docs/type-representations.md).
It must not select a library by name or expanded shape, accept arbitrary raw
source, resolve ambiguity by registration order or silently fall back.

## 3. Tooling and runtime boundaries

Bittype projects Horos's sorts, declared contracts, symbolic bodies, individual
memberships with assumptions, references and callee/argument structure into
provider-owned facts. Its agnostic engine does not interpret Type, Data, Wire or
library names. Native mappings retain explicit supported profiles. Kind
declarations are compile-time definitions, not native type exports. The existing
types-only profile returns Unsupported for residual constructor parameters,
results and aliases; it does not silently specialize or prune them.

| Judgment | Meaning and authority |
| --- | --- |
| Declaration syntax | Exact tags, arities, sorts, kind contracts and revision |
| Static semantics | Horos binding, primitive formation, kind membership, constructor contracts and materialization |
| Ground conformance | Existing WF/Conforms and diagnostics over ground Ontos data |
| Mixed structural admission | Ontos/wire finite positional structure and payload admission |
| Capability recognition | Runtime recognition of a particular sending face |
| Operational validity | Authority, scope, endpoint lifetime and transport state |
| Protocol compliance | Library-owned message, reply, state and cancellation laws |

None silently establishes the next judgment. Static Horos has no live
Wire/runtime dependency. Ontos/wire owns optional mixed values; runtime
composition owns recognition and hydration. No general package dispatches on
Function, Stream, Cell or a structural fingerprint to select behavior.

[Bitwire 0.7.0](https://github.com/Bitspark/bitwire/blob/v0.7.0/docs/wire/hydrated-native.md)
and [Bitruntime 0.8.0](https://github.com/Bitspark/bitruntime/blob/v0.8.0/hydrated/README.md)
use Ontos 0.16.0's native mixed structure, which Ontos 0.17.0 (required by Horos)
leaves unchanged. Export references, proxies and endpoint lifetimes remain there;
they do not consult Horos to hydrate messages. Constructor
kinds change neither that value model nor those protocols.

Resolution, kind compilation, inclusion, constructor reasoning and materialization
are bounded. Caches retain exact bindings and constructor identity, while output
admission counts repeated occurrences. Closed proper membership is exact within
the admitted fragment; constructor reasoning is sound and incomplete. Unproved
is distinct from Refuted, Invalid and Exhausted. No failed operation exposes a
usable partial result. A structural kind guarantee supplies no ground recursion
guard, wire-size, queue, liveness, latency or cancellation guarantee.
