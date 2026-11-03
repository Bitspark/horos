# Optional horos/wire type and declaration profile

**Owner direction, 3 November 2026. Status: documentation proposal only.**
`horos/wire` is an optional type/declaration extension over the proposed
`ontos/wire` value profile. Its canonical meaning belongs to Horos; Bittype
supplies syntax/AST projections, analysis integration and transformations.
Neither extension, its APIs nor its package names are implemented by this
document. Existing ground descriptors and declaration revision 1 are unchanged.

This replaces the Bittype-owned `bittype/contract/*` model proposal. It does not
revive the earlier proposal to add `horos/wire` directly to ground declaration
revision 2. Those were unimplemented alternatives, not compatibility formats.
The [boundary decision](hydrated-types.md) records the explicit supersession.

## 1. Profile and value domains

The proposed semantic profile is the pair **(`horos/wire`, revision `1`)**.
Its module tag and revision below distinguish it from the implemented ground
declaration envelope `horos/decl/module`, revision 1. A profile identity fixes
syntax and interpretation; a module pin identifies particular canonical bytes.
Neither substitutes for the other. A package version, an equal-looking body or
the absence of a Wire occurrence does not select the ground profile.
The profile identity names the immutable contract specified here: owning
extension/revision, admitted domain, interpretation and recognition boundary,
ground embedding/recovery, and positional/encoding conventions. Matching a
profile's name without that semantic revision and domain contract is insufficient.

The unchanged ground domain is finite, immutable Ontos atoms and tuples. The
proposed [optional value profile](https://github.com/Bitspark/ontos/blob/main/docs/design/0007-optional-wire-extension.md)
uses Deixis as its common structural foundation, with opaque byte-or-capability
payload alternatives. Its intended admitted shapes are an atom leaf, a Wire
leaf, or a tuple with dense canonical positional children. Arbitrary Deixis nodes
are not thereby admitted mixed values. The existing
[Ontos projection](https://github.com/Bitspark/ontos/blob/a3f1d081f2e82f1e0ec39688c4060b5606b16c63/projection/deixis/PROJECTION.md)
already supplies the ground correspondence; the optional mixed profile is new
work owned by Ontos, not a change to Deixis or Ontos L0.

Let `embed(v)` denote the mixed profile's total embedding of a ground value.
The profile must preserve ground atom bytes, tuple order, multiplicity and the
empty-atom/empty-tuple distinction. Ground recovery is exact on the admitted
wire-free subset. Shape immutability alone does not freeze opaque payloads:
the value profile must preserve immutable bytes and captured children while
retaining live capability handles. It must not clone a capability or invent
portable equality, hashing or an encoding for it.

Every type description in this document is itself ground Ontos data, even
when the type describes live instances. Encoding that description does not
serialize a live Wire. Type loading and analysis perform no sends, capability binding,
exports, proxy creation or resource acquisition.

## 2. Canonical expression format

Reuse the exact ground value notation from
[declaration revision 1](declarations.md#1-representation-and-notation):

```text
A(s)           = Atom(UTF8(s))
T(s)           = ontos/data utf8-text(s)
I(n)           = ontos/data int(n)
L(x, ...)      = ontos/data list(x, ...)
M{ k: v, ... } = ontos/data map with text keys T(k) and values v
H(c, x, ...)   = Tuple(A("horos/" + c), x, ...)
Pin(module)    = Atom(SHA256(ontos-codec-v1.encode(module)))

W(c, x, ...)   = Tuple(A("horos/wire/" + c), x, ...)
Data(G)        = W("data", G)
Wire(E)        = W("wire", E)
```

These are proposed exact value equations, not implemented builders or a readable
source grammar. Pins contain exactly 32 raw bytes. Canonical map ordering,
duplicate rejection and exact text/integer embeddings retain their ground
meaning. Descriptions use the existing Ontos codec; no live-value codec follows.

The initial closed expression vocabulary is:

```text
E = W("data", G)
  | W("wire", E)
  | W("tuple", E...)
  | W("compound", A(label), E...)
  | W("list-of", E)
  | W("record", A("closed"), W("field", T(name), A(presence), E)...)
  | W("union", E, E, ...)
  | W("param", I(index))
  | W("apply", target, L(E...))

target   = W("local", T(name)) | W("imported", T(alias), T(name))
presence = "req" | "opt"
```

Wire and list-of each take exactly one type operand at tuple path `[1]`.
Wire describes sending authority with message type E, not receiving, closure,
a result slot or an invocation protocol. The message type is a schema child;
the corresponding live capability remains a value leaf. Schema traversal never
enters a receiver or performs an interaction.

G must be a complete, closed, well-formed descriptor of the unchanged ground
Horos profile. This first wire-profile revision fixes the same v1 descriptor
vocabulary and Ontos 0.15.0 embeddings/codec v1 as ground declaration revision 1;
it does not inherit whatever a newer installed package happens to accept.
`Data(G)` admits exactly the values `embed(v)` for ground values
v that conform to G:

```text
instances(Data(G)) = { embed(v) | v |=ground G }
```

Here `v |=ground G` denotes ideal semantic ground membership, independent of an
implementation's evaluation budget. Existing boolean/checker APIs retain their
documented bounded behavior; diagnostic resource exhaustion is not a semantic
counterexample or proof of non-membership.

Horos's existing rules own G's entire interpretation:

- `Data(H("any"))` excludes values containing Wires.
- `Data(H("not", G))` complements within ground Ontos values only.
- `Data(H("exact", value))` keeps its whole literal opaque, even when the
  literal resembles a parameter, application or Wire expression.

Data is a separate language boundary. Upper parameters or applications cannot
be inserted into G's descriptor positions. Closed data applications may first
be specialized by the existing declarations tooling, with their source
identities and applications retained as provenance.

Tuple and compound take zero or more type children, union at least two, and
records zero or more fields with distinct literal text names. The mode is
explicitly closed. The mixed structural interpretation preserves the existing
Ontos tuple, labelled compound, list and text-keyed-map layouts while allowing
mixed values at the declared child positions. Explicitly, the value layouts are:

```text
tuple(v0, ...)       = MixedTuple(v0, ...)
compound(label, ...) = MixedTuple(A(label), ...)
list(v0, ...)        = MixedTuple(A("list"), v0, ...)
record(k0: v0, ...)  = MixedTuple(A("map"), MixedTuple(T(k0), v0), ...)
```

MixedTuple is the optional value profile's tuple constructor. Ground A/T operands
in these value equations denote their embedded images. This is not insertion of
capabilities into an ordinary ground Ontos tuple. Wire-free mixed instances
recover the exact ground layouts without reinterpreting their type profile.
Record keys remain ground text,
unique and ordered by their Ontos codec bytes; required/optional membership
retains its existing meaning. Absent and present-null remain different.

Other ground constructs, including sets, maps, open records, complements,
intersections and guarded recursion, remain available inside Data(G). The first
mixed profile adds no general top/complement, live map keys or set elements,
open-record extras or recursive named applications. Unsupported forms fail
explicitly instead of acquiring an invented lifted interpretation.

## 3. Modules, bindings and specialization

The proposed module envelope is:

```text
W("module", I(1), T(moduleName), importsMap, declarationsMap)
W("import", T(expectedModuleName), Pin(importedWireModule))
W("type", L(T(parameterName), ...), E)
```

Imports and declarations use canonical text-keyed maps. This profile adopts the
[ground declaration conventions](declarations.md#2-closed-declaration-vocabulary)
for identifier spelling, distinct ordered parameter names, explicit argument
arity, local/imported targets, forward references and exported declarations.
Unknown tags, wrong arities and unknown revisions fail. A package update does
not silently extend an existing document's grammar.

All imports in this first envelope target modules of this same `horos/wire`
profile. Ground Horos modules cannot be relabelled or imported as wire-profile
modules. A caller supplies the pinned module bundle; every transitive import,
including an unused one, must satisfy its exact pin and expected name. Import
and declaration-application graphs are acyclic in this revision. Retrieval and
package installation stay outside resolution.

Each parameter ranges over E and binds by its enclosing declaration and
zero-based slot. Nested applications do not introduce parameter scopes.
Resolve a target in its author's module and arguments in the caller's lexical
scope before substitution. Substitution visits declared upper type positions
only; it never scans literal labels, field names or Data(G) contents. There are
no implicit arguments, defaults, inference, partial applications or higher-kinded
slots. These are the existing binding principles applied to the separate domain.
The current Go Resolve/Specialize entry points remain ground-declaration APIs.
A future extension may share internal algorithms while enforcing its own
envelope and domain; it must not make those existing entry points accept W forms.

Open generics and applications remain in the checked model and native output.
Specialization is a separate operation; its extended result retains Data
boundaries and this profile even if it contains no Wire syntax. It is not a
ground descriptor accepted by the existing `WF`/`Conforms` APIs. A separately
specified exact ground recovery may be possible; no implicit conversion is
part of this proposal.

Declaration identity remains an exact module pin and declaration name. An
extended reference additionally qualifies that identity with its semantic profile;
this does not change the implemented ground Go `DeclarationID` structure.
A binder belongs to that declaration and slot index under its selected profile.
Derived declarations and binders have distinct identities from their sources:
preserve the exact original binder as provenance and record the ordered mapping
to the derived binder, rather than reusing a source identifier in the new domain.
Applications retain resolved targets and caller-bound arguments; aliases keep
their own declaration identities. Pins describe canonical document bytes, not
semantic equivalence, authority or behavioral compatibility. There is no union
sorting, alias expansion, binder renaming or shape-based coalescing before
pinning. Origins, native target-language naming preferences and derivation
manifests remain tooling metadata, not extra arguments in this closed canonical
grammar. Canonical local/imported target names and module/declaration names are
identity-bearing document data and therefore participate in the module pin.

## 4. Explicit embedding and partial template lifting

Horos owns the following proposed semantic mapping; Bittype implements its
future explicit AST transformation. The build selects the operation and roots.
An engine route does not decide to change the value domain, and a failed lift
does not retry as embedding or as a weaker type.

**Closed embedding** resolves and specializes a ground expression to a closed,
well-formed descriptor G, then produces Data(G). Preserve the original module,
declaration, applications and arguments as provenance. This does not widen any
source generic slot.

**Template lifting** derives a separately identified wire-profile declaration
while preserving its generic parameters. Let L(E) operate in the resolved source
lexical environment. Apply these rules in order:

| Source expression | Lift and obligation |
| --- | --- |
| Parameter at slot i | `W("param", I(i))` bound to the corresponding derived declaration; retain source binder provenance. |
| Resolved application of D to A | Apply the derived exact target L(D) to L(A), preserving argument order, arity, caller scope and source application. |
| Tuple | `W("tuple", L(E)...)`, preserving order and empty tuples. |
| Labelled compound | `W("compound", A(label), L(E)...)`; retain the literal label. |
| Typed list | `W("list-of", L(element))`. |
| Closed record | Closed W record with the same literal field names, required/optional membership and lifted field types. |
| Union with at least two alternatives | W union of lifted alternatives in the same order. |
| Singleton union | L(the alternative), retaining union provenance. |
| Empty union | `Data(H("union"))`, the empty ground type. |
| Any other independently specializable, closed, well-formed subtree G | Data(G), retaining source references without inspecting literal contents. |
| Anything else | Located unsupported-lift failure; no erased constraint, top substitution or usable partial result. |

The structural rules apply even when their children happen to be closed. This
preserves template structure before binding instead of opportunistically
collapsing it into Data. Closed means no free generic or recursion variable,
with all applications resolved and successfully specialized under Horos rules;
absence of a `param` token is insufficient.

Thus any/scalar/refinement/exact ground forms remain Data, and a closed
complement, set, map, open record or guarded recursive descriptor may embed.
`not(T)`, `set-of(T)` and `rec(...T...)` have no template lift in this first
profile. Their closed specializations may instead be explicitly embedded.

Validate the full original import closure before pruning generated output.
The derived closure follows applications retained by the ordered rules and
stops at each Data boundary. Translate a declaration once by resolved source
identity and lifting configuration, group results by source module, and pin
derived dependency modules before their importers. Preserve source pins,
selected roots, lifting profile/version, output naming choices and origins in
the derivation record. Reject missing imports, cycles and output naming conflicts
before publishing usable output. Source dependencies absorbed into Data still
remain validated provenance dependencies.

For a concrete boundary case, let `Other<T> = not(T)` and
`Root = not(Other<Person>)`. Root's closed whole body embeds as Data without
requiring the unsupported lift of Other. Selecting Other itself fails. An
`Alias = Other<Person>` retains its top-level application under the earlier
application rule and therefore also requires that unsupported lift. The caller
may explicitly select closed embedding for Alias. Traversal order and a failed
attempt must not change these outcomes.

For every admitted source template D and closed ground descriptor arguments A,
the future implementation must establish this preservation law:

```text
instances(specialize(L(D), Data(A)...))
    = { embed(v) | v |=ground specialize(D, A...) }
```

This is a denotational obligation preserving the ground layout, not equality
of ASTs, module pins or Deixis/Ontos codec bytes. It introduces no live-handle
equality. Qualification must respect the ground checker's existing bounded
diagnostic behavior instead of treating resource exhaustion as a type mismatch.
Lexical substitution coherence is a second qualification obligation: where both
partial routes are admitted, lifting an expression after bound substitution must
denote the same instances as substituting the corresponding lifted actuals into
its lift. Compare bindings in their original caller scopes, including nested
applications and shadowed names; do not require identical output ASTs or pins.
Neither obligation is an already-proven theorem or a change to the source APIs.

Pagination remains one authored template:

```text
Ground Page<T>  = record closed { items: list-of(T), next?: scalar(utf8-text) }
Derived Page<T> = record closed { items: list-of(T), next?: Data(scalar(utf8-text)) }
```

The [ground pagination module](declarations.md#5-complete-pagination-example)
is the authored input. For the selected output name `example.pagination.wire`,
the exact proposed generated module is:

```text
DerivedPagination = W("module", I(1), T("example.pagination.wire"), M{}, M{
  "Page": W("type", L(T("T")),
    W("record", A("closed"),
      W("field", T("items"), A("req"),
        W("list-of", W("param", I(0)))),
      W("field", T("next"), A("opt"),
        Data(H("scalar", A("utf8-text"))))))
})

WirePages = W("module", I(1), T("example.wirepages"), M{
  "pagination": W("import", T("example.pagination.wire"), Pin(DerivedPagination))
}, M{
  "WirePage": W("type", L(),
    W("apply", W("imported", T("pagination"), T("Page")), L(
      Wire(Data(H("scalar", A("utf8-text")))))))
})
```

This consumer applies the generated Page to sending faces for ground text
messages. Specialization yields a closed mixed record whose items list contains
Wire leaves; it does not make the ground Page accept those leaves. The derived
equation illustrates generated output, not a second declaration to maintain.

Ground `Page<Person>` stays a data application. The derived Page may accept a
Wire or a library-owned Cell type. No user authors another Page or an expanded
record per application, and no `LiftedPage` spelling is required. The derived
module/declaration identity remains distinct from the source. A library may
explicitly qualify a shared native representation for both identities; equal
shapes alone never authorize that association.

## 5. Library-owned Function, Cell and Stream

The following complete proposed module equations preserve ordinary library
composition. They specify no installed protocol or runtime implementation.
The abbreviations are:

```text
P(i)                 = W("param", I(i))
Use(alias, name, ...) = W("apply", W("imported", T(alias), T(name)), L(...))
Local(name, ...)      = W("apply", W("local", T(name)), L(...))

WireLibrary = W("module", I(1), T("example.wire"), M{}, M{
  "Wire": W("type", L(T("M")), W("wire", P(0)))
})

FunctionLibrary = W("module", I(1), T("example.contracts.function"), M{
  "wire": W("import", T("example.wire"), Pin(WireLibrary))
}, M{
  "Unit": W("type", L(), Data(H("tuple"))),
  "Function": W("type", L(T("A"), T("B")),
    Use("wire", "Wire", W("tuple", P(0), Use("wire", "Wire", P(1)))))
})

CellLibrary = W("module", I(1), T("example.contracts.cell"), M{
  "fn": W("import", T("example.contracts.function"), Pin(FunctionLibrary))
}, M{
  "Cell": W("type", L(T("T")), W("record", A("closed"),
    W("field", T("get"), A("req"),
      Use("fn", "Function", Use("fn", "Unit"), P(0))),
    W("field", T("set"), A("req"),
      Use("fn", "Function", P(0), Use("fn", "Unit")))))
})

StreamLibrary = W("module", I(1), T("example.contracts.stream"), M{
  "fn": W("import", T("example.contracts.function"), Pin(FunctionLibrary))
}, M{
  "Step": W("type", L(T("T")),
    W("union", W("compound", A("item"), P(0)), W("compound", A("end")))),
  "Stream": W("type", L(T("T")), W("record", A("closed"),
    W("field", T("next"), A("req"),
      Use("fn", "Function", Use("fn", "Unit"), Local("Step", P(0)))),
    W("field", T("cancel"), A("req"),
      Use("fn", "Function", Use("fn", "Unit"), Use("fn", "Unit")))))
})
```

Each Pin is the exact SHA-256 atom of the expanded earlier module's canonical
Ontos bytes, not a placeholder string or package version. Library names are
illustrative declarations, not privileged registrations. Their readable meaning is:

```text
Unit          = Data(H("tuple"))
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
identity/profile and target language/representation profile. They specify a
type-only native AST or external generic type, ordered parameter mapping,
locked imports/dependencies, supported arguments and representation limits.
They preserve aliases and origins. Ordinary structural generation may compose
these declarations without a custom recipe. An optional custom representation
does not become executable adapter code or proof of behavior. Bittype owns the
[native representation tooling proposal](https://github.com/Bitspark/bittype/blob/main/docs/type-representations.md);
it must not choose a library by its name or expanded structure, accept arbitrary
raw source, resolve ambiguity by registration order, or fall back silently.

## 6. Tooling, runtime and qualification

The canonical model and lifting rules live here. Bittype supplies language-owned
AST/check/resolve/analyze stages and explicit transformers over its existing
generic AST/CST and semantic infrastructure. Its provider must preserve the
profile identity, ground boundaries, binders, references, origins and diagnostic
distinctions. No generic kernel node, registry default or language-name switch
learns Wire, Function, Stream or Cell. Concrete registration IDs and Go marker
choices are tooling decisions with qualified profiles, not new canonical meaning.

The static model has no dependency on a live Wire/runtime implementation.
`ontos/wire` supplies the optional value profile; runtime composition supplies
capability recognition and hydration. Do not introduce an import cycle by
making the value profile depend on the runtime that consumes it. Loading an
extended descriptor or recognizing its syntax cannot establish endpoint authority,
liveness or a remote receiver's adherence to E. Future library bindings may check
actual messages; type analysis itself never invokes the capability.

Keep these judgments separate:

| Judgment | Meaning and authority |
| --- | --- |
| Canonical syntax | The tags, arities, literals and profile revision are admitted by this grammar. |
| Type/declaration semantics | Horos owns binding, resolution, specialization and the intended type interpretation; existing ground `WF`/`Conforms` still check only their ground domain. |
| Mixed structural admission | The proposed ontos/wire profile admits the value's tagged payload and finite positional structure. This does not authenticate an arbitrary payload as a usable endpoint. |
| Capability recognition | Runtime composition recognizes a particular sending face; a generic type or value package does not consult a global runtime to do so. |
| Operational validity | Current authority, scope, endpoint lifetime and transport state permit a particular operation. Recognition alone does not establish this. |
| Protocol compliance | The receiver and sender follow a library's message, reply, state and cancellation laws. Neither a Wire leaf nor its declared E proves that behavior. |

The current [Bitwire live contract](https://github.com/Bitspark/bitwire/blob/e2d2d641744a49f1d8098b6ea5f2724454d72b96/docs/wire/hydrated-native.md)
and [Bitruntime implementation](https://github.com/Bitspark/bitruntime/blob/64dbe709f2f002781a5010b08a6650c7eeb129df/hydrated/README.md)
already own atoms/tuples/Wires, export references, proxies and scope/endpoint
lifetimes. They do not currently use this proposed profile or Horos to hydrate
messages. Mapping their existing carrier to the Deixis-based profile is separate
future qualification, not a requirement to rewrite every native container now.

Finite inputs still require explicit resource bounds. Mixed structural
recognition, static loading/resolution, template lifting and specialization must
each have node, depth and byte budgets; module resolution also bounds its supplied
bundle, and expansion charges repeated parameters/literals in the actual output
tree. These are request limits, not weakened type meanings or new canonical
descriptor operands. Return a located exhausted-request diagnostic, distinct from
invalid syntax, unresolved context, unsupported lifting or a completed semantic
mismatch. Do not expose a usable partial model or silently choose a weaker
translation. Preserve the existing ground APIs' bounded behavior. Transport
message and lifetime limits remain the runtime's separate responsibility.

Before implementation is claimed, independently authored cases must cover:

- Separate profile/tag/revision rejection, exact module pins, open generics,
  literal boundaries, caller scope, imports and no usable partial results.
- Data(any), Data(complement), exact reference-like literals, empty atom versus
  empty tuple and wire-free extended types without implicit ground reinterpretation.
- The ordered lifting rules, full source closure versus pruned derived closure,
  unsupported open forms, reproducible identities, ground preservation and
  lexical substitution coherence.
- Optional-field absence versus present null; nested applications and transitive
  dependencies; parameters in each supported descriptor position; literal labels,
  scalar selectors and exact values resembling declaration syntax.
- Singleton unions and empty unions with no admitted value or invented bottom
  value; closed guarded recursion versus open generic recursion; unbound or
  captured recursion references at application boundaries.
- Pagination over ground and library capability types without another authored
  template; ordinary unfamiliar libraries without changes to general machinery.
- Static type/representation checks separated from runtime authority, protocol,
  capability aliasing, lifetime and transport evidence.
- Depth/node/byte and expansion exhaustion at each relevant stage, with preserved
  diagnostics and no usable partial output or semantic mismatch verdict.

No implementation, parser, builder, checker change, runtime adapter, dependency,
test vector or release is added now. Forbidden remain: widening ground APIs;
retaining the superseded Bittype namespace as a compatibility decoder; automatic
profile selection by Wire occurrence; named-contract kinds, switches, imports,
registrations or fingerprints in general machinery; and replacing hydration
with a type checker or generator.

This concrete Wire extension does not justify a universal capability-extension
framework. Generalization requires another demonstrated case with shared
semantics. The expert review and its evaluated recommendations are preserved in
Bittype's [consultation record](https://github.com/Bitspark/bittype/blob/main/research-docs/0003-optional-wire-extensions.md).
