# Wire types in unified Horos

**Owner directions, 4 November 2026.** Wire is a primitive static type expression
in the one [revision-3 declaration language](declarations.md). Kinds and constructor
interfaces extend that language without adding another profile, envelope or Data
wrapper. The [Go declarations API](../../core/go/declarations/README.md) checks
static types. Ground conformance remains a separate operation; no live-value
checker or runtime binding is implemented here. Hosted qualification and
publication of the current source require their own release receipts.

## 1. Primitive expression and interfaces

Use the exact A/T/I/L/M/H/D/Pin notation from
[the declaration specification](declarations.md#1-representation-and-notation).
`H("wire", E)` denotes a sending capability accepting messages of proper type E.
The expression is ordinary Ontos data and contains no endpoint. Its message may
be any admitted proper type, but not an unapplied type constructor. Wire is never
Data, regardless of its message. Static validity proves neither endpoint
authority nor protocol compliance; loading a declaration sends nothing and
allocates no capability.

Tuple, compound, list-of, closed record and union can contain Wire types and
remain the same structural constructors used by data declarations. Their Data
classification follows their value-bearing operands. Other constructors retain
explicit Data requirements. `H("any")` still denotes data only. Use a ground
descriptor G directly, without a `Data(G)` wrapper.

Data is the refined kind of types established data-only. `wire<Person>` is a
proper Type but not Data; the bytes describing it are still Ontos data. The
[Page example](declarations.md#5-complete-pagination-example) authors one
`Page<T>`, used for `Page<Person>` and `Page<Cell<Person>>`.
`DataPage<D: Data> = Page<D>` restricts callers without copying Page.
A constructor parameter can promise a Data result, but merely accepting a Data
argument does not establish one: `Send<T> = wire<T>` never returns Data.

`Specialize` returns a closed proper type expression. `DataDescriptor` also
establishes Data and returns the existing ground descriptor. Both use checked
proper arguments and sealed named-constructor handles from the same Program.
Neither creates another language, module or runtime value model.

## 2. Library-owned Function, Cell and Stream

The complete module equations below illustrate ordinary library composition.
They specify no installed protocol or runtime implementation. Helpers are exact
value abbreviations, not executed source:

```text
P(i)                  = D("param", I(i))
Param(name)           = D("parameter", T(name), D("proper", A("type")))
Def(parameters, body) = D("type", parameters, A("type"), body)
Ref(alias, name)      = D("imported", T(alias), T(name))
Use(alias, name, ...) = D("apply", Ref(alias, name), L(...))
Local(name, ...)      = D("apply", D("local", T(name)), L(...))

WireLibrary = D("module", I(3), T("example.wire"), M{}, M{
  "Wire": Def(L(Param("M")), H("wire", P(0)))
})

FunctionLibrary = D("module", I(3), T("example.contracts.function"), M{
  "wire": D("import", T("example.wire"), Pin(WireLibrary))
}, M{
  "Unit": Def(L(), H("tuple")),
  "Function": Def(L(Param("A"), Param("B")),
    Use("wire", "Wire", H("tuple", P(0), Use("wire", "Wire", P(1)))))
})

CellLibrary = D("module", I(3), T("example.contracts.cell"), M{
  "fn": D("import", T("example.contracts.function"), Pin(FunctionLibrary))
}, M{
  "Cell": Def(L(Param("T")), H("record", A("closed"),
    H("field", T("get"), A("req"),
      Use("fn", "Function", Ref("fn", "Unit"), P(0))),
    H("field", T("set"), A("req"),
      Use("fn", "Function", P(0), Ref("fn", "Unit")))))
})

StreamLibrary = D("module", I(3), T("example.contracts.stream"), M{
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

## 3. Tooling, runtime and qualification

Bittype projects Horos's checked interfaces, kinds, summaries, references and
callee/argument structure into provider-owned facts. Its agnostic engine does
not interpret Type, Data, Wire or library names. Native mappings have separately
qualified supported profiles; canonical validity does not establish that a
native language or mapping supports residual higher-kinded parameters.

| Judgment | Meaning and authority |
| --- | --- |
| Declaration syntax | Exact tags, arities, interfaces, claims and revision |
| Static semantics | Horos binding, kinds, interfaces, specialization and Data proofs |
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
use Ontos 0.16.0's native mixed structure. Export references, proxies and endpoint
lifetimes remain there; they do not consult Horos to hydrate messages. Constructor
kinds change neither that value model nor those protocols.

Resolution, interface comparison, summary evaluation and specialization are
bounded. Caches retain exact bindings and constructor identity, while output
admission counts repeated occurrences. Exhaustion is an incomplete request,
not rejected source or a usable partial result. Data preservation supplies no
recursion guard, wire-size, queue, liveness, latency or cancellation guarantee.
