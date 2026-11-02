# Hydrated types and the Wire primitive

**2 November 2026. Proposed declaration revision 2; documentation only.** The
owner requested a type for the existing native Wire capability, with Function,
Stream and Cell composed in ordinary libraries. No checker, declaration API,
AST, generator, binding, dependency or released package implements this proposal.
The implemented [declaration revision 1](declarations.md) and [ground descriptor
contract](horos.md) remain unchanged.

## 1. Values and descriptions

The concrete missing case is a request containing an argument and a reply Wire.
It cannot be represented as a ground ontos value. Bitwire already defines and
bitruntime realizes the necessary live domain:

```text
GroundValue   = ByteAtom | Tuple(GroundValue*)
HydratedValue = ByteAtom | HydratedTuple(HydratedValue*) | Wire
```

The [native hydrated contract](https://github.com/Bitspark/bitwire/blob/e2d2d641744a49f1d8098b6ea5f2724454d72b96/docs/wire/hydrated-native.md)
owns capability recognition, sending authority, tuple capture and endpoint
lifetime. A Wire is an opaque live leaf, not an ontos byte atom, an endpoint
owner or an application object that happens to have a send method. The tuple
structure remains finite and acyclic even when communication relationships cycle.

**A type description is still ground data. A described value may contain Wires.**
For example, the description of a Wire carrying text contains a descriptor for
text; it contains no live endpoint. Module encoding and content pins therefore
continue to use ontos-codec-v1 and SHA-256 over ordinary ground values. No live
Wire is inserted into ontos, hashed as module content or assigned a new portable
reference encoding. Existing hydration supplies exports and proxies.

## 2. Exact Wire type format

Use the exact `A`, `T`, `I`, `L`, `M`, `H`, `D` and `Pin` value abbreviations from
[declaration revision 1](declarations.md#1-representation-and-notation).
The sole new primitive type expression is:

```text
Wire(E) = H("wire", E)
        = Tuple(Atom(UTF8("horos/wire")), E)
```

There is exactly one argument, the message type expression `E`. No argument may
be omitted; there is no implicit `any`, direction flag, result slot, library
name or protocol tag. The constructor denotes a **sending** capability. Receiving
and closing remain owner operations outside a conveyed Wire's authority.

The message argument is a type position. It admits descriptors, indexed generic
parameters and ordinary local/imported applications under the enclosing module's
rules. The authoritative schema walker must expose it at tuple path `[1]`.
The value checker treats the actual Wire as a leaf and never traverses a receiver,
sends a test message or creates an endpoint. Schema traversal and value traversal
are different operations.

The complete expression extension is:

```text
E2 = a v1 descriptor with E2 in each child-descriptor position
   | H("wire", E2)
   | D("param", I(index))
   | D("apply", target, L(E2, ...))
```

Targets retain the `local`/`imported` forms. Modes, labels, field names, bounds
and exact-literal contents remain literal positions; adding Wire does not permit
parameters or live values there.

The named generic spelling is an ordinary library declaration, not a reserved
name. This complete proposed module exports it:

```text
WireLibrary = D("module", I(2), T("example.wire"), M{}, M{
  "Wire": D("type", L(T("M")),
    H("wire", D("param", I(0))))
})
```

`example.wire.Wire<Text>` is an application of that declaration to a text type.
An application may import it under any alias. Horos knows the primitive tag
`horos/wire`; general AST carriers, registries and runtimes do not acquire a
catalogue of these library names. The example module name is not a built-in
import, a published package or a privileged binding identifier.

## 3. Version and domain selection

The proposed module envelope is the existing form with revision **2**:

```text
D("module", I(2), T(moduleName), imports, declarations)
```

The declaration forms, identifier grammar, explicit arguments, indexed binders,
literal boundaries and exact import pins otherwise retain revision-1 rules.
Revision 2 selects the hydrated interpretation below for the whole checked
program. The initial proposal permits only revision-2 imports in that program.
A revision-1 module is not silently widened when imported or supplied a Wire
argument. A ground-only library can be authored as a revision-2 module with its
own resulting pin. No cross-revision coercion or import fallback is specified.

Current tools reject revision 2; current ground checkers reject `horos/wire` as an
unknown constructor. Even a bare `any()` or `not(...)` must retain its selected
domain when leaving a revision-2 program. A future specialization result must
therefore retain the revision/profile alongside the specialized expression; it
must not be passed to today's ground `WF`/`Conforms` as though that domain were
implicit. This document does not add or rename a public API.

## 4. Structural meaning over hydrated values

The proposed hydrated shape judgment is bounded and reports completion separately
from mismatch, preserving exhaustion through complements and other combinations.
It performs no interaction. Its rules are:

| Type form | Hydrated shape rule |
| --- | --- |
| `wire(E)` | Match a sending Wire recognized by the supplied runtime boundary; preserve E as the declared message contract |
| `any()` | Match any hydrated value in this profile |
| Atoms, scalars, byte/text refinements | Retain their existing ground recognition; a Wire is not an atom or scalar |
| `exact(value)` | Compare only ground candidates with the ground literal using existing ontos identity; a live candidate never matches |
| `tuple`, `seq`, `compound` | Apply existing arity/label rules and recurse into hydrated children |
| `list-of(E)` | Retain the ontos list embedding's structure, with hydrated element positions checked against E |
| `record` | Retain the map embedding, canonical ground text keys and required/optional rules; field values may be hydrated; open extra fields admit any hydrated value |
| `map-of(K,V)` | Require canonically ordered ground keys satisfying K; values may be hydrated and must satisfy V |
| `set-of(E)` | Require canonically ordered ground elements satisfying E |
| `union`, `and`, `not` | Combine judgments over the selected hydrated universe, without converting exhaustion into a completed answer |
| `rec`, `ref` | Retain guarded structural recursion and the additional message boundary below |

The lifted collection layouts are explicit. Here v may be hydrated, while k and
g must be ground; tags are the existing raw atoms:

```text
List(v0, ...)       = HydratedTuple(A("list"), v0, ...)
Map(k0: v0, ...)    = HydratedTuple(A("map"), HydratedTuple(k0, v0), ...)
Set(g0, ...)        = Tuple(A("set"), g0, ...)
```

Maps require unique keys in codec-v1 byte order; sets require unique elements in
that order. Records use the same map layout with canonical text keys. A hydrated
tuple without any live leaf can use its ordinary ground tuple representation.
These are not live Wires stored inside an ordinary ontos value implementation.
Keys/elements that require canonical ordering use existing ground ordering;
there is no new equality, hash, serialization or ordering operation for Wires.
Consequently `set-of(wire(E))` admits only the empty set, and
`map-of(wire(E),V)` admits only the empty map. These are well-formed descriptions
of restricted sets of values, not a new nonempty-type check. A list or a map's
values can contain Wires. Literal tuples resembling `horos/wire` remain data
inside `exact`, just as parameter-shaped literals do in revision 1.

Every message expression E must be closed with respect to `rec`/`ref` binders
outside that message expression. It may introduce its own guarded recursion or
use enclosing declaration parameters. A Wire boundary is not structural descent
and cannot serve as a recursion guard. For example,
`wire(rec("n", list-of(ref("n"))))` is structurally guarded, whereas
`rec("n", wire(ref("n")))` is rejected. This is not a session-type or recursive
communication-protocol system. Concrete substitutions remain recursively closed;
named declaration applications and import graphs remain acyclic in this proposal.

### What the message argument establishes

`wire(Text)` and `wire(Integer)` carry different declared message contracts.
Recognizing the same live sending capability is not evidence that its receiver
accepts either contract. Shape admission establishes the Wire leaf only;
bindings can validate actual messages against E at their sending/receiving
boundaries. They cannot prove future remote behavior by inspecting the leaf.
No variance, assignability or protocol-equivalence rule is inferred here.

The runtime supplies capability recognition; a type checker must not simulate it
using a `kind` string, duck typing or a global registry of library contracts.
Generic arguments and declaration identities must survive export even when two
descriptions have the same local shape predicate. Shape equality never selects
a behavioral adapter. Horos's program-validity analysis establishes declaration
well-formedness and binding, not fulfillment of a remote protocol.

## 5. Library-owned Function, Cell and Stream

These are complete value equations for **illustrative proposed libraries**.
They do not replace any deployed protocol or claim compatibility with an existing
Function/Stream package. The chosen Function profile is deliberately success-only;
errors, timeouts, release and cancellation protocols need their owning library's
full specification before a production binding is implemented.

Three additional exact abbreviations keep applications readable:

```text
P(i)                  = D("param", I(i))
Use(alias, name, ...)  = D("apply", D("imported", T(alias), T(name)), L(...))
Local(name, ...)       = D("apply", D("local", T(name)), L(...))
```

The ellipsis denotes the ordered arguments supplied at that occurrence, including
an empty argument list. These equations abbreviate values; they are not new
canonical forms, executable constructors or a source grammar.

### Function

```text
FunctionLibrary = D("module", I(2), T("example.contracts.function"), M{
  "wire": D("import", T("example.wire"), Pin(WireLibrary))
}, M{
  "Unit": D("type", L(), H("tuple")),
  "Function": D("type", L(T("A"), T("B")),
    Use("wire", "Wire",
      H("tuple", P(0), Use("wire", "Wire", P(1)))))
})
```

Its explanatory meaning is:

```text
Unit = tuple()
Function<A,B> = Wire<tuple(A, Wire<B>)>
```

The Function value is a request Wire. Each invocation sends its argument and a
fresh reply Wire; the library's provider performs the operation and sends one B
through that reply Wire. Reply matching follows the conveyed capability, without
requiring a correlation field in this example. Freshness, one reply, completion,
resource cleanup and actual execution are library rules, not consequences of
the tuple shape. `Unit` is the singleton bare empty tuple, not null or an implicit
native `void` mapping. A function with several arguments can use a tuple as A.

### Cell

```text
CellLibrary = D("module", I(2), T("example.contracts.cell"), M{
  "fn": D("import", T("example.contracts.function"), Pin(FunctionLibrary))
}, M{
  "Cell": D("type", L(T("T")),
    H("record", A("closed"),
      H("field", T("get"), A("req"),
        Use("fn", "Function", Use("fn", "Unit"), P(0))),
      H("field", T("set"), A("req"),
        Use("fn", "Function", P(0), Use("fn", "Unit")))))
})
```

```text
Cell<T> = record closed {
  get: Function<Unit,T>
  set: Function<T,Unit>
}
```

This is a record whose two values are request Wires under the Function
representation. The Cell library implements storage, updates and any chosen
concurrency guarantees. A generic record binding can compose the two Function
bindings; no core Cell kind or custom Cell adapter is necessary for this
representation. A different custom representation would require an explicit
library-owned binding, not shape recognition by the runtime.

### Stream

```text
StreamLibrary = D("module", I(2), T("example.contracts.stream"), M{
  "fn": D("import", T("example.contracts.function"), Pin(FunctionLibrary))
}, M{
  "Step": D("type", L(T("T")),
    H("union", H("compound", A("item"), P(0)), H("compound", A("end")))),
  "Stream": D("type", L(T("T")),
    H("record", A("closed"),
      H("field", T("next"), A("req"),
        Use("fn", "Function", Use("fn", "Unit"), Local("Step", P(0)))),
      H("field", T("cancel"), A("req"),
        Use("fn", "Function", Use("fn", "Unit"), Use("fn", "Unit")))))
})
```

```text
Step<T> = union(compound("item", T), compound("end"))
Stream<T> = record closed {
  next: Function<Unit,Step<T>>
  cancel: Function<Unit,Unit>
}
```

This is a pull representation. The Stream library owns item order, demand,
concurrent-next policy, terminal behavior and cancellation races; those laws do
not follow from a record of functions. A push representation is another library
choice. Neither choice introduces a Stream protocol into Wire or hydration.

All four modules contain only ground type-description data. Each `Pin(...)` is
the exact 32-byte SHA-256 atom computed from the preceding expanded module's
canonical bytes, not a text placeholder or a package-version lookup. Function's
A/B and Cell/Stream's T remain separate indexed binders. Imports, aliases and
nested applications retain their ordinary resolved identities.

## 6. Ownership and future qualification

| Component | Responsibility |
| --- | --- |
| ontos | Unchanged ground values, structural identity and codec |
| bitwire / bitruntime | Existing hydrated value/capability contracts and their realization; no Function, Stream or Cell knowledge |
| horos | Proposed hydrated type profile, Wire message-type position and declaration semantics |
| bittype Horos frontend | Preserve the selected profile, declarations, operands and bindings in AST/semantic projections |
| General bittype kernel | Generic trees, analysis publication and registration; no concrete Wire or behavioral-contract semantics |
| Contract libraries | Named declarations, native representations, adapters and behavioral laws |
| Consuming application | Explicit imports and composition of selected library implementations |

The [bittype projection proposal](https://github.com/Bitspark/bittype/blob/main/docs/hydrated-types.md)
describes the tooling boundary. Adding a library contract must not require a
generator/runtime/Wire special kind, name switch, default import, central
registration or structural fingerprint. Association with a native representation
or adapter follows the library's resolved declaration identity; equal expanded
shapes do not establish ownership. Exact adapter metadata is future scoped work,
not a second declaration format specified here.

Before implementation is claimed, independent cases must cover exact arity and
revision rejection; open/nested message arguments; aliases and lexical scope;
ground literals resembling Wire expressions; profile-preserving specialization;
hydrated records/lists and ground key restrictions; recursion-message boundaries;
bounded/exhausted checks; and recognition without sending or exporting anything.
Ground conformance must remain qualified separately. Library behavioral tests
must establish request/reply, Cell and Stream laws separately from structural
typing. This change supplies documentation only, with no such new implementation
or conformance claim.
