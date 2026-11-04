# Wire types in unified Horos

**Owner decision, 4 November 2026.** Wire is a primitive static type expression
in the one [revision-2 declaration language](declarations.md), implemented by
[Go declarations](../../core/go/declarations/README.md). The former Horos/Wire
profile, package, envelope and Data wrapper are removed without aliases.
Ground value conformance remains the existing separate operation; no live value
checker or runtime binding is implemented here.

## 1. Primitive expression and bounds

Use the exact A/T/I/L/M/H/D/Pin notation from
[the declaration specification](declarations.md#1-representation-and-notation).
`H("wire", E)` denotes a sending capability accepting messages of type E.
The expression itself is ordinary ground Ontos data; it contains no endpoint.
Its message may be any admitted type. A Wire type is never Data, regardless of
its message. Description validity proves neither endpoint authority nor protocol
compliance, and loading it causes no sends, exports or allocation.

Tuple, compound, list-of, closed record and union can contain Wire types and
remain the same structural constructors used by data declarations. Their data
classification follows their operands. Other constructors retain the explicit
Data requirements in the declaration specification. `H("any")` still denotes
only data. A ground descriptor G is used directly, without a `Data(G)` wrapper.

The complete [pagination example](declarations.md#5-complete-pagination-example)
authors `Page<T>` once. The same identity and slot support `Page<Person>` and
`Page<Cell<Person>>`; `DataPage<D: Data> = Page<D>` restricts callers without
copying Page. Resolve arguments in caller scope, check bounds, then substitute.
`Specialize` returns a closed type expression; `DataDescriptor` additionally
requires data and returns the existing ground descriptor. Neither creates a
second language or module.

## 2. Library-owned Function, Cell and Stream

The following complete proposed module equations preserve ordinary library
composition. They specify no installed protocol or runtime implementation.
The abbreviations are:

```text
P(i)                 = D("param", I(i))
Use(alias, name, ...) = D("apply", D("imported", T(alias), T(name)), L(...))
Local(name, ...)      = D("apply", D("local", T(name)), L(...))

WireLibrary = D("module", I(2), T("example.wire"), M{}, M{
  "Wire": D("type", L(D("parameter", T("M"), A("type"))), H("wire", P(0)))
})

FunctionLibrary = D("module", I(2), T("example.contracts.function"), M{
  "wire": D("import", T("example.wire"), Pin(WireLibrary))
}, M{
  "Unit": D("type", L(), H("tuple")),
  "Function": D("type", L(D("parameter", T("A"), A("type")), D("parameter", T("B"), A("type"))),
    Use("wire", "Wire", H("tuple", P(0), Use("wire", "Wire", P(1)))))
})

CellLibrary = D("module", I(2), T("example.contracts.cell"), M{
  "fn": D("import", T("example.contracts.function"), Pin(FunctionLibrary))
}, M{
  "Cell": D("type", L(D("parameter", T("T"), A("type"))), H("record", A("closed"),
    H("field", T("get"), A("req"),
      Use("fn", "Function", Use("fn", "Unit"), P(0))),
    H("field", T("set"), A("req"),
      Use("fn", "Function", P(0), Use("fn", "Unit")))))
})

StreamLibrary = D("module", I(2), T("example.contracts.stream"), M{
  "fn": D("import", T("example.contracts.function"), Pin(FunctionLibrary))
}, M{
  "Step": D("type", L(D("parameter", T("T"), A("type"))),
    H("union", H("compound", A("item"), P(0)), H("compound", A("end")))),
  "Stream": D("type", L(D("parameter", T("T"), A("type"))), H("record", A("closed"),
    H("field", T("next"), A("req"),
      Use("fn", "Function", Use("fn", "Unit"), Local("Step", P(0)))),
    H("field", T("cancel"), A("req"),
      Use("fn", "Function", Use("fn", "Unit"), Use("fn", "Unit")))))
})
```

Each Pin is the exact SHA-256 atom of the expanded earlier module's canonical
Ontos bytes, not a placeholder string or package version. Library names are
illustrative declarations, not privileged registrations. Their readable meaning is:

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

H is the ground descriptor constructor defined above: `H("tuple")` is the
descriptor for the empty tuple, not the value itself.

Function is illustrative and success-only. Its library owns fresh reply
endpoints, matching, one-reply discipline, failures, completion and resource
management. Unit is an empty ground tuple, not automatically native void. Cell
owns state and mutation laws. The pull Stream owns demand, order, concurrent-next
policy and end/cancel races. These behaviors do not follow from the shapes;
production bindings need their own complete protocol and qualification.

Library-owned native type recipes are selected by exact resolved declaration
identity and target language/representation profile. They specify a
type-only native AST or external generic type, ordered parameter mapping,
locked imports/dependencies, supported arguments and representation limits.
They preserve aliases and origins. Ordinary structural generation may compose
these declarations without a custom recipe. An optional custom representation
does not become executable adapter code or proof of behavior. Bittype owns the
[native representation tooling proposal](https://github.com/Bitspark/bittype/blob/main/docs/type-representations.md);
it must not choose a library by its name or expanded structure, accept arbitrary
raw source, resolve ambiguity by registration order, or fall back silently.

## 3. Tooling, runtime and qualification

Bittype has one Horos AST/check/resolve/analyze family. It preserves explicit
bounds, binders, references, identities and diagnostics in provider-owned syntax
and semantic facts. The agnostic engine does not interpret Type, Data, Wire or
library names. Native transformers and representation recipes remain future work.

| Judgment | Meaning and authority |
| --- | --- |
| Declaration syntax | Exact tags, arities, bounds and revision in this grammar |
| Static semantics | Horos binding, bounds, specialization and data classification |
| Ground conformance | Existing WF/Conforms and diagnostics over ground Ontos data |
| Mixed structural admission | Ontos/wire finite positional structure and payload admission |
| Capability recognition | Runtime recognition of a particular sending face |
| Operational validity | Authority, scope, endpoint lifetime and transport state |
| Protocol compliance | Library-owned message, reply, state and cancellation laws |

None of these judgments silently establishes the next one. Static Horos has no
live Wire/runtime dependency. Ontos/wire owns optional mixed values; runtime
composition owns recognition and hydration. There is no import cycle from value
models to the runtime which consumes them. No general package dispatches on
Function, Stream, Cell or a structural fingerprint to select behavior.

[Bitwire 0.7.0](https://github.com/Bitspark/bitwire/blob/v0.7.0/docs/wire/hydrated-native.md)
and [Bitruntime 0.8.0](https://github.com/Bitspark/bitruntime/blob/v0.8.0/hydrated/README.md)
use Ontos 0.16.0's native mixed structure. Export references, proxies and endpoint
lifetimes remain there; they do not consult Horos to hydrate messages. Unifying
the declaration language changes neither that value model nor those protocols.

Resolution and specialization bound depth, node/work visits, bytes and supplied
modules. Cached symbolic declaration summaries prevent repeated DAG expansion;
actual specialization still charges repeated output occurrences. Exhaustion is
an incomplete request, not an invalid source or a usable partial model. Ground
diagnostics retain their existing limits. Canonical descriptions establish no
wire-size, queue, liveness, latency or cancellation guarantees.
