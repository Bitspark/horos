# Canonical declarations, revision 2

**Owner decision, 4 November 2026. Implemented by the Go declarations package
for Horos 0.9.0.** One declaration language contains data and capability type
expressions. Explicit Type/Data bounds constrain generic arguments without a
second language or duplicated declarations. This replaces both previous profiles
without aliases or old-format readers. Ground descriptor APIs retain their
existing data-only meanings. Rust and TypeScript declaration tooling, readable
source syntax, native generators and runtime bindings remain outside this work.

Horos owns canonical types and their semantics; Bittype owns AST/CST projections,
semantic facts, transformations and source generation. The declaration layer
adds names, modules, generic parameters and applications to the existing
[descriptor vocabulary](horos.md), plus the static [Wire type](wire-types.md).
It does not redefine Ontos data identity or ground conformance.

## 1. Representation and notation

A declaration document is an ordinary finite ontos value. Its variables are
encoded declaration syntax, not variables added to ontos. Storage uses the frozen
[ontos-codec-v1](https://github.com/Bitspark/ontos/blob/v0.16.0/docs/spec/ontos-codec.md).
No new codec or JSON value model is introduced.

The following abbreviations specify exact values throughout this document:

```text
A(s)           = Atom(UTF8(s))
T(s)           = ontos/data utf8-text(s)
I(n)           = ontos/data int(n)
L(x, ...)      = ontos/data list(x, ...)
M{ k: v, ... } = ontos/data map with text keys T(k) and values v
H(c, x, ...)   = Tuple(A("horos/" + c), x, ...)
D(c, x, ...)   = Tuple(A("horos/decl/" + c), x, ...)
Pin(module)    = Atom(SHA256(ontos-codec-v1.encode(module)))
```

`M` uses the existing canonical map ordering and rejects duplicate keys; source
listing order does not matter. `L` preserves order. `M{}` and `L()` are the
canonical empty map and list. Every string in this notation is an exact Unicode
string, with no implicit normalization. `A` is a raw atom; `T` is a text embedding.
They are not interchangeable. `Pin` contains exactly 32 raw bytes, not hex text.

These are value equations, not an implemented source language or functions a
consumer must write. Expanding them determines every tag, arity and embedding.
A readable source syntax and its parser belong to bittype and remain future work.
An on-disk declaration uses the canonical binary encoding of the expanded value;
whitespace and notation are not hashed.

## 2. Closed declaration vocabulary

| Form | Exact arguments after its `horos/decl/` tag |
| --- | --- |
| `module` | `I(2), T(moduleName), imports, declarations` |
| `import` | `T(expectedModuleName), Atom(32-byte SHA-256 digest)` |
| `type` | `L(D("parameter", T(parameterName), A(bound)), ...), body` |
| `parameter` | `T(parameterName), A("type")` or `T(parameterName), A("data")` |
| `param` | `I(zeroBasedParameterIndex)` |
| `apply` | `target, L(argument, ...)` |
| `local` | `T(declarationName)` |
| `imported` | `T(importAlias), T(declarationName)` |

`imports` is an `M` from local aliases to `import` values. `declarations` is an
`M` from declaration names to `type` values. All declarations are exported. Empty maps and parameter/argument lists are explicit;
omitted arguments and defaults do not exist. Unknown tags, extra arguments and
unknown module revisions are errors. `local` and `imported` are targets, not
standalone type expressions.

Declaration names, parameter names and import aliases match
`[A-Za-z_][A-Za-z0-9_]*`. Module names are one or more such segments separated
by a single dot. Names are case-sensitive. Parameters within one declaration
must have distinct names. Import aliases and declaration names occupy separate
namespaces, selected explicitly by the target form. Record field names retain
horos's existing text semantics; they are not restricted to these identifiers.

The module revision fixes this declaration grammar and the unified expression
vocabulary in this specification, using ontos 0.16.0 embeddings and codec v1.
A package version does not silently extend a document's admitted grammar.

## 3. Type expressions reuse descriptor positions

A body or argument is a type expression `E`:

```text
E = a horos descriptor, with each child-type position admitting E
  | H("wire", E)
  | D("param", I(index))
  | D("apply", target, L(E, ...))

target = D("local", T(name))
       | D("imported", T(alias), T(name))
```

Child-descriptor positions are exactly the `T`, `K` and `V` positions in the
existing descriptor specification, including a record field's final type and
the body of `rec`. Other operands remain literal values with their existing
checks. For example, a scalar kind, record mode, field name, recursion name,
integer bound or compound label cannot be a generic slot. This revision has
type parameters only, not value parameters or higher-kinded parameters.

In particular, `exact(value)` contains literal data, not another type
expression. This is a valid singleton descriptor with no free parameter:

```text
H("exact", D("param", I(0)))
```

Its one permitted value is the literal tuple tagged `horos/decl/param`.
No declaration traversal descends into that literal looking for parameters,
applications or references. Shape matching anywhere in an ontos tree is not a
substitute for respecting descriptor positions.

`D("param", I(i))` refers to slot `i` of the enclosing `type` declaration;
`0 <= i < parameterCount`. Parameter names supply readable source names, while
the ordered slots determine binding. Nested applications do not introduce a
parameter scope. A module-level bare parameter is invalid.

`apply` supplies every argument position exactly once, in declaration order.
A reference to a nongeneric declaration is the same operation with `L()`.
There is no inference, partial application, implicit `any`, defaults or constraint
inference or alternate shorthand in the canonical value grammar.

### Type and Data bounds

Every canonical parameter declares its bound. `type` admits any well-formed
type expression; `data` requires a statically established data-only type. Type
and Data are parameter bounds, not value descriptors or wrappers. Readable
`Page<T>` means a Type-bound slot; the canonical input always spells it out.

Tuple, compound, list-of, closed record and union admit either domain. They are
data when all their value-bearing operands are data. Wire is never data, even
when its message type is data. Scalar, atom, exact and refinement forms are data.
Other existing forms require data operands: seq, set-of, map-of, open records,
intersection, complement and guarded recursion. These restrictions preserve
existing laws without inventing capability key equality, mixed complements or
recursive capability semantics. `any` still means any data value.

Resolve all declarations, including open generics, under their declared bounds.
`Forward<D: Data> = DataPage<D>` succeeds; `Forward<T> = DataPage<T>` fails before
specialization. A Type slot cannot satisfy Data merely because a future caller
might supply data. A Data-only constructor likewise rejects a Type slot.

Data classification follows resolved declaration bodies and actual parameter
use. `Ignore<T> = scalar(utf8-text)` remains data even at `Ignore<Wire<Text>>`;
`Wire<D: Data> = H("wire", D)` never returns data. All arguments still undergo
binding and bound checks, including unused arguments. The check is a bounded
syntax-directed proof, not arbitrary subtype, emptiness or equivalence solving.

## 4. Modules and immutable imports

One document defines one module. A local target resolves in the module where
that expression was authored. An imported target resolves through that module's
import map, then selects an exported declaration by name. Source order is
irrelevant: declarations may refer forward to other declarations in the module.

An import pins the exact canonical bytes of the target module. A resolver is
supplied module values by the caller and must verify both the SHA-256 pin and
the expected module name. A name, file path, registry version or URL alone is
not sufficient. Retrieval locations are application configuration, not part of
the declaration. The engine and transformations do no implicit network IO.

Imports must resolve transitively, including imports unused by a selected
output. The dependency graph must be acyclic in this revision. Identically
named modules with different pins remain different modules and can be imported
through different aliases. Resolution must never rebind a dependency merely
because another module has the same name.

The digest is an address for canonical document bytes, not a new ontos identity
rule, semantic-equivalence test, authority proof or behavioral compatibility
fingerprint. This proposal defines plain SHA-256 of codec-v1 bytes: no implicit
domain prefix, source-text hashing or unresolved placeholder pin.

## 5. Complete pagination example

First define the reusable pagination module. `Pagination` below names this exact
module value for the subsequent equation; it is not an extra canonical field.

```text
Pagination = D("module", I(2), T("example.pagination"), M{}, M{
  "Page": D("type", L(D("parameter", T("T"), A("type"))),
    H("record", A("closed"),
      H("field", T("items"), A("req"),
        H("list-of", D("param", I(0)))),
      H("field", T("next"), A("opt"),
        H("scalar", A("utf8-text"))))),
  "DataPage": D("type", L(D("parameter", T("D"), A("data"))),
    D("apply", D("local", T("Page")), L(D("param", I(0)))))
})
```

`Page` has one Type-bound parameter. Its required `items` field is a list of
that type. `DataPage` is an ordinary constrained alias of that same declaration;
it adds no value wrapper or copied record. Its optional `next` field contains text when present; explicit null
is not admitted. No pagination behavior, fetch operation or cursor protocol is
implied by this type declaration.

The consumer imports that exact module and applies `Page` to its own `Person`:

```text
People = D("module", I(2), T("example.people"), M{
  "pagination": D("import", T("example.pagination"), Pin(Pagination))
}, M{
  "Person": D("type", L(),
    H("record", A("closed"),
      H("field", T("name"), A("req"),
        H("scalar", A("utf8-text"))))),
  "PeoplePage": D("type", L(),
    D("apply", D("imported", T("pagination"), T("Page")), L(
      D("apply", D("local", T("Person")), L()))))
})
```

`Pin(Pagination)` is fully determined by the first equation. The stored import
contains the resulting 32-byte atom, not the expression or a placeholder string.
No manually selected package version or handwritten expanded page descriptor
is necessary. Both documents can be constructed without executing user code.

In familiar notation, these exact values declare:

```text
example.pagination: Page<T> = { items: List<T>, next?: Text }
                    DataPage<D: Data> = Page<D>
example.people:     Person = { name: Text }
                    PeoplePage = pagination.Page<Person>
```

This last display is explanatory shorthand, not a second canonical format.
`Page<Person>` and `DataPage<Person>` specialize to the same ground descriptor.
`DataPage<Page<Person>>` also succeeds. If an ordinary imported `Cell<Person>`
contains a Wire, `Page<Cell<Person>>` is valid but `DataPage<Cell<Person>>` and
`DataPage<list-of(Cell<Person>)>` fail. No checker recognizes the name Cell.

## 6. Application and specialization

Resolution preserves the original module, declaration and parameter bindings.
For an application, first resolve the target in the author's module and resolve
each argument in the caller's scope. Check each argument against the callee's declared bound. Only then substitute those bound argument
expressions for the callee's indexed parameters. Moving raw names into the
callee's module and resolving them there is forbidden.

For example, `Person` in `PeoplePage` remains `example.people.Person`, even if
the pagination module later also declares a `Person`. Two declarations may both
call a parameter `T` without sharing its binding. A nested `Page<Page<T>>`
retains the outer declaration's parameter inside both applications.

An open declaration remains open in the checked model and in generated native
generics. Specialization is a separate operation for obtaining a concrete
descriptor; native generation does not require monomorphizing every declaration.
Specializing the example's `PeoplePage` yields exactly:

```text
H("record", A("closed"),
  H("field", T("items"), A("req"),
    H("list-of",
      H("record", A("closed"),
        H("field", T("name"), A("req"),
          H("scalar", A("utf8-text")))))),
  H("field", T("next"), A("opt"),
    H("scalar", A("utf8-text"))))
```

This is an existing ground Horos descriptor. `Program.DataDescriptor`
specializes, proves the result data-only and checks its ground well-formedness
before returning it for the existing `WF`/`Conforms` or diagnostic APIs.
`Program.Specialize` also accepts and returns closed mixed type expressions;
those are not inputs to the ground checker. Those APIs do not accept an open template as if it were a
descriptor. The proposal adds no separate value validator or runtime codec.

The checked declaration graph keeps `PeoplePage`, its application of the
imported `Page`, and the argument's `Person` origin even when a consumer also
requests this expanded view. Aliases introduce no value wrapper, but their
declaration identity is not discarded.

## 7. Recursion and limits

Existing guarded `rec`/`ref` descriptors retain their meaning and guard rules.
They are not reused to encode generic slots or external declaration references.
Guarded well-formedness does not assert that a descriptor has an inhabitant;
this layer adds no competing productivity or nonempty-type rule.

The declaration-application graph must be acyclic in this revision;
direct alias cycles and recursive generic applications are rejected. Recursive
data can still be expressed using existing `rec`/`ref` within a declaration.
Regular recursion across named declarations is deferred rather than given
implicit expansion or termination semantics.

Each declaration body and each application argument must be closed with respect
to horos recursion binders. An argument may contain its own guarded `rec`/`ref`
or an enclosing declaration's generic parameter, but cannot capture a `ref`
bound outside that argument. Concrete arguments substituted for parameters must
be well-formed, recursively closed descriptors. This keeps substitution from
capturing a callee's recursion name. A future extension that permits recursive
references across application boundaries must specify hygienic elaboration.

Resource limits must report failure/exhaustion explicitly. They cannot turn an
unknown declaration into `any`, hide an import cycle, silently drop a member or
make an open template appear concrete. Existing bounded conformance caveats
remain documented in the descriptor contract.

## 8. Identity and normalization

Canonical here means one precisely specified value representation, not a unique
normal form for all types that classify the same values.

- Module identity is its ontos value; `Pin` addresses its canonical bytes.
- A resolved declaration is identified by its module pin and declaration name.
- A generic binder is identified by that declaration and its zero-based slot.
- An application retains its resolved target and bound arguments. A named alias
  retains its own declaration identity as well as the referenced declaration.

Map embeddings normalize map entry order. Lists, parameter order, descriptor
operand order, literal values and names remain exact. There is no alias
expansion, union sorting/deduplication, binder renaming, nullable simplification
or structural-equivalence rewriting before encoding or pinning a module.
For example, changing a parameter's display name changes the module document
and its pin, even though corresponding substitution may classify the same values.

Source spans, comments, human documentation, target names and compiler profiles
are separate tooling metadata. They are not extra positional arguments or
unknown fields in this closed grammar. Ontos structural identity does not
establish authority, library ownership or lawful behavioral substitution.

## 9. Checking and acceptance examples

The declaration checker checks the parameterized descriptor grammar and bindings before
running the final descriptor well-formedness check.

| Input | Required outcome |
| --- | --- |
| Both pagination modules above | Resolve; keep an open `Page` and applied `PeoplePage` |
| `Page<Page<Person>>` | Preserve both applications; specialize nested item types |
| Same parameter spelling in two declarations | Bind to distinct declaration slots |
| `param(1)` in a one-parameter declaration | Reject out-of-range slot |
| Bare parameter in a zero-parameter declaration | Reject unbound slot |
| `Page` with zero or two arguments | Reject arity mismatch |
| A generic target supplied as an argument without application | Reject; no higher-kinded arguments |
| Unknown declaration or import alias in admitted source | Reject the source expression |
| Required pin absent from the supplied pool | Incomplete context; no validity conclusion |
| Available pinned module contradicts the import's expected name | Reject the supplied context |
| Duplicate map key or parameter name; wrong tag/arity/revision | Reject structure |
| Parameter used as a record mode or field name | Reject; that operand is literal, not a type position |
| Parameter-shaped tuple inside `exact` | Preserve literal data, without interpreting a slot |
| Cyclic imports or cyclic declaration applications | Reject in revision 2 |
| A recursion `ref` captured from outside an application argument | Reject the open recursion boundary |
| `next` absent / present text / present null | Valid / valid / invalid for the example |

These authored cases are exercised by the Go declaration tests, including
pagination conformance through the existing checker, nested binding, literal
boundaries, invalid inputs and explicit exhaustion. Native generation remains
separate future work; existing cross-language ground-descriptor vectors remain
unchanged.

The Go tooling's structured diagnostics distinguish source rejection, incomplete
dependency context, invalid caller configuration and resource exhaustion. Every
failed resolution returns no program. Validity is not inferred from an error's
message or from missing output; consumers inspect its code and subject as
documented in the [Go API](../../core/go/declarations/README.md).

The dependency pool supplies unlabelled values. An unrelated supplied value does
not assert that it satisfies a missing pin. A wrong expected module name is a
contradiction about the supplied context, not proof that the root source is
invalid under every other context. These diagnostic distinctions do not change
the canonical format or any declaration/descriptor rule.

## 10. Ownership and delivery boundary

Horos owns this type semantic layer, its encoding, resolution/substitution rules
and declaration checking/specialization APIs. Bittype owns readable syntax,
AST/CST projections, located frontend diagnostics, AST transformations and
native source generation. Its frontend must implement/delegate the horos rules,
not define another data type algebra. Horos must not depend on bittype.

Function, Stream, Cell and future behavioral contracts remain ordinary pinned
library declarations. Their static shapes compose with primitive Wire and
structural constructors; their behavior stays with libraries. See the
[Wire and library examples](wire-types.md). There is no separate Horos/Wire
language, data wrapper, closed-embedding API or template-copying transformation.
Ontos's value-level ground embedding and recovery remain separate and unchanged.

Forbidden in general generators, engine, Wire, runtime and hydration:

- Contract-specific kinds, name switches, built-in imports or registrations.
- Recognizing behavioral ownership from structural fingerprints or equal shapes.
- Replacing canonical input with extraction from a native language.
- Hiding adapter, request/reply, stream or Cell implementations in type generation.
- Widening ground `any`, accepting live endpoints as ground data, silently
  converting old declaration revisions or inventing a fallback profile.

The Go [declarations API](../../core/go/declarations/README.md) supplies checked
modules, immutable resolved expressions, binding identities, symbolic bound
checking and concrete specialization. Bittype projects it as one AST-only
frontend. No native generator, text parser, formatter or runtime binding is added.
Reverse native-to-Horos migration remains later work.

## 11. Clean migration

The [4 November decision](unified-declarations.md) supersedes both previous
declaration profiles. Rewrite revision-1 data parameters as explicit Data slots
and old mixed parameters as Type slots. Convert their structural type forms to
H forms, remove schema-level Data wrappers and use the revision-2 D envelope.
Recompute pins from dependency leaves outward. Do not rewrite opaque literals.
Old envelopes, bounds, parameter lists and retired constructor tags are errors;
there is no dual reader, compatibility alias or exported `core/go/wire` package.
Historical release records stay immutable. The ground descriptor vocabulary and
Ontos/Bitwire/Bitruntime/Bitnode value and transport contracts are unchanged.
