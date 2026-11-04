# Canonical declarations, revision 1

**2 November 2026; distribution updated 4 November. Status: implemented in the
Go declarations tooling package, included in Horos 0.8.0.** The owner authorized the declaration
model and bittype AST after reviewing this format. The existing ground descriptor
APIs remain separate. Rust and TypeScript declaration tooling, readable source
syntax, native generators and runtime bindings are outside this increment.

The owner selected horos for the canonical data type model and bittype for
syntax, AST/CST infrastructure and native type generation. This declaration
layer adds names, modules, generic parameters and applications to the existing
[descriptor vocabulary](horos.md). It does not redefine records, collections,
scalars, value equality or conformance. The first generation milestone is data
types only, for TypeScript, Go, Rust and C++.

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
| `module` | `I(1), T(moduleName), imports, declarations` |
| `import` | `T(expectedModuleName), Atom(32-byte SHA-256 digest)` |
| `type` | `L(T(parameterName), ...), body` |
| `param` | `I(zeroBasedParameterIndex)` |
| `apply` | `target, L(argument, ...)` |
| `local` | `T(declarationName)` |
| `imported` | `T(importAlias), T(declarationName)` |

`imports` is an `M` from local aliases to `import` values. `declarations` is an
`M` from declaration names to `type` values. All declarations are exported in
this first revision. Empty maps and parameter/argument lists are explicit;
omitted arguments and defaults do not exist. Unknown tags, extra arguments and
unknown module revisions are errors. `local` and `imported` are targets, not
standalone type expressions.

Declaration names, parameter names and import aliases match
`[A-Za-z_][A-Za-z0-9_]*`. Module names are one or more such segments separated
by a single dot. Names are case-sensitive. Parameters within one declaration
must have distinct names. Import aliases and declaration names occupy separate
namespaces, selected explicitly by the target form. Record field names retain
horos's existing text semantics; they are not restricted to these identifiers.

The module revision fixes this declaration grammar and the v1 descriptor
vocabulary retained by horos 0.8.0, using ontos 0.16.0 embeddings and codec v1.
A package version does not silently extend a document's admitted grammar.

## 3. Type expressions reuse descriptor positions

A body or argument is a type expression `E`:

```text
E = a horos v1 descriptor, with each child-descriptor position admitting E
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
There is no inference, partial application, implicit `any`, constraint/default
language or alternate shorthand in the canonical value grammar.

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
Pagination = D("module", I(1), T("example.pagination"), M{}, M{
  "Page": D("type", L(T("T")),
    H("record", A("closed"),
      H("field", T("items"), A("req"),
        H("list-of", D("param", I(0)))),
      H("field", T("next"), A("opt"),
        H("scalar", A("utf8-text")))))
})
```

`Page` has one type parameter. Its required `items` field is an ontos list of
that type. Its optional `next` field contains text when present; explicit null
is not admitted. No pagination behavior, fetch operation or cursor protocol is
implied by this data declaration.

The consumer imports that exact module and applies `Page` to its own `Person`:

```text
People = D("module", I(1), T("example.people"), M{
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
example.people:     Person = { name: Text }
                    PeoplePage = pagination.Page<Person>
```

This last display is explanatory shorthand, not a second canonical format.

## 6. Application and specialization

Resolution preserves the original module, declaration and parameter bindings.
For an application, first resolve the target in the author's module and resolve
each argument in the caller's scope. Only then substitute those bound argument
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

This is an existing ground horos descriptor. Future declaration tooling must
check its well-formedness before passing it to the existing `WF`/`Conforms` or
diagnostic APIs. Those APIs do not accept an open template as if it were a
descriptor. The proposal adds no separate value validator or runtime codec.

The checked declaration graph keeps `PeoplePage`, its application of the
imported `Page`, and the argument's `Person` origin even when a consumer also
requests this expanded view. Aliases introduce no value wrapper, but their
declaration identity is not discarded.

## 7. Recursion and the first revision's limits

Existing guarded `rec`/`ref` descriptors retain their meaning and guard rules.
They are not reused to encode generic slots or external declaration references.
Guarded well-formedness does not assert that a descriptor has an inhabitant;
this layer adds no competing productivity or nonempty-type rule.

The declaration-application graph must be acyclic in this first revision;
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
| Cyclic imports or cyclic declaration applications | Reject in revision 1 |
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

Horos owns this data semantic layer, its encoding, resolution/substitution rules
and declaration checking/specialization APIs. Bittype owns readable syntax,
AST/CST projections, located frontend diagnostics, AST transformations and
native source generation. Its frontend must implement/delegate the horos rules,
not define another data type algebra. Horos must not depend on bittype.

Function, Stream, Cell and future behavioral contracts remain library-owned.
These data-only declarations do not model a live function or stream as an ontos
data value or supply their bindings. Their eventual declarations compose through
explicit library identity in the optional [horos/wire profile](wire-types.md),
with separate library-owned runtime realizations. Generic data parameters remain
data-only. Author a generic that must accept capabilities directly in horos/wire;
its same parameter can accept data embeddings and capability types. There is no
template-copying or cross-profile declaration transformation. Bittype projects
each authored profile without defining another canonical type model.

Forbidden in general generators, engine, Wire, runtime and hydration:

- Contract-specific kinds, name switches, built-in imports or registrations.
- Recognizing behavioral ownership from structural fingerprints or equal shapes.
- Replacing the canonical input with extraction from a native language.
- Hiding adapter, request/reply, stream or Cell implementations in type generation.

The Go [declarations API](../../core/go/declarations/README.md) supplies checked
modules, immutable resolved expressions, binding identities and concrete
specialization. Bittype consumes it as an AST-only frontend. This increment adds
no native generator, text parser, formatter, behavioral binding or package release.
Reverse native-to-Horos migration remains later work.

The [data/hydration boundary](hydrated-types.md) records the 3 November owner
direction: existing Ontos and Horos cores stay data-only, while optional
`ontos/wire` and `horos/wire` extensions own proposed mixed values and types.
This supersedes Bittype's ownership of the proposed contract model without
changing this ground revision. The earlier idea of a Wire constructor in ground
Horos revision 2 remains superseded; the optional profile has a separate envelope
and interpretation. Neither old proposal is accepted as a compatibility format.
Hydration remains above the value/type models in the Wire/runtime layers.
