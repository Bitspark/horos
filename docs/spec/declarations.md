# Canonical declarations, revision 4

**Canonical revision 4, implemented in Go source, 4 November 2026.**
Revision 4 replaces the closed Type/Data interface model with general structural
kinds, ordinary named kind definitions and constructor contracts. It keeps one
module language for data and capability types. The [kind specification](kinds.md)
defines membership, inclusion and constructor reasoning; this page fixes the
canonical representation and declaration rules.

Horos owns these semantics. Bittype owns readable syntax, AST/CST projections,
semantic facts and types-only native transformations. Ground Horos and Ontos
retain their existing meanings. The general kind-model engine is separate from
primitive Horos formation and the ground value checker.

## 1. Representation and notation

A module is a finite ground Ontos value. Type and kind variables are encoded
syntax, not variables added to Ontos. Encoding uses
[ontos-codec-v1](https://github.com/Bitspark/ontos/blob/v0.16.0/docs/spec/ontos-codec.md).

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

M uses canonical map ordering and rejects duplicate keys; L preserves order.
Strings are exact, without implicit Unicode normalization. A is a raw atom and
T a text embedding. An import pin contains 32 raw bytes, not hexadecimal text.
These equations determine values; notation and whitespace are not stored.

## 2. Canonical vocabulary

The existing module envelope retains revision, name, imports and declarations.
It has no signatureDigest or separately configured schema identity.

```text
Module = D("module", I(4), T(moduleName), imports, declarations)
Import = D("import", T(expectedModuleName), Atom(32-byte module digest))

S = D("sort-proper")
  | D("sort-arrow", L(S, ...), S)

KR = D("kind-local", T(name))
   | D("kind-imported", T(alias), T(name))

K = D("top", S)
  | D("bottom", S)
  | D("kind-param", I(slot))
  | KR
  | D("kind-apply", KR, L(K, ...))
  | D("or", S, L(K, ...))
  | D("and", S, L(K, ...))
  | D("node", T(form), selectorMap, Q)
  | D("nu", L(K, ...), I(entry))
  | D("recursive-var", I(depth), I(slot))
  | D("arrow", L(K, ...), K)

Q = D("epsilon")
  | D("item", K)
  | D("concat", L(Q, ...))
  | D("alternative", L(Q, ...))
  | D("star", Q)

KindParameter = D("kind-parameter", T(name), S)
TypeParameter = D("parameter", T(name), K)

Declaration = D("kind", L(KindParameter, ...), S, K)
            | D("type", L(TypeParameter, ...), K, E)

E = existing Horos descriptor form with E only in designated type-child positions
  | H("wire", E)
  | D("param", I(slot))
  | D("local", T(name))
  | D("imported", T(alias), T(name))
  | D("apply", E, L(E, ...))
```

Imports map aliases to Import values. One declarations map contains both kind
and type definitions, all exported in their own declaration category. A reference
must resolve to the category its syntax requests. Kind expressions cannot be
substituted for type expressions or vice versa.

The grammar has these cardinality and sort rules:

- Sort arrows, kind arrows and application argument lists are nonempty.
- Or, and, concat and alternative have at least two operands. Use top/bottom or
  epsilon for the corresponding explicit base cases.
- A nu group has at least one proper-kind equation; entry selects an equation by
  zero-based index. Recursive variables use zero-based slot and lexical group
  depth, with depth zero selecting the innermost group.
- Node, nu and recursive variables have proper sort. Item contains a proper kind.
- Union/intersection operands have the stored common sort. An arrow's inputs and
  result determine its constructor sort, recursively.
- Kind applications name a kind declaration and supply all its arguments.
  A bare kind reference names a zero-parameter definition. Kind definitions are
  first order: their parameters range over kinds of declared sorts, not over
  parameterized kind definitions.
- A type declaration's result is any kind, including a constructor kind. Its
  parameter-list arity does not determine the sort of its result.
- Canonical sorts, parameter contracts and result kinds are always explicit.

Names match `[A-Za-z_][A-Za-z0-9_]*`; module names are one or more such segments
separated by dots. Names are case-sensitive and need no capitalization convention.
Parameters within a declaration have distinct names. Import aliases and
canonical declaration names occupy separate namespaces; a single declarations
map cannot contain two entries with the same name. Record-field names retain the
ground descriptor's text semantics.

Unknown revisions/tags, malformed selectors, extra operands, invalid references
or ill-sorted expressions are errors. Bittype owns source abbreviations. An
omitted ordinary source annotation may elaborate to proper top, not an implicit
reference to a privileged Type name. A constructor-valued alias or result needs
an annotation of the appropriate constructor sort.

## 3. Node patterns and the ordinary prelude

A node's form is a text name such as `list-of`, `record` or `scalar`. Its sequence
pattern matches the entire ordered list of observed type children. Epsilon
matches none; item matches one child; concat joins finite sequences; alternative
chooses one sequence language; star repeats a sequence any finite number of times.

Selector maps use canonical text keys and text values. The only selector keys
are `selector` on scalar and `mode` on record. Their values are respectively
`int`/`utf8-text`/`bool` and `open`/`closed`. The dotted names scalar.selector and
record.mode identify these observation fields in prose; they are not canonical
map keys. An omitted selector is a wildcard. All other metadata retains its
primitive validation but is not available for kind predicates.

For example, a canonical ordinary kind definition for List is:

```text
"List": D("kind", L(
  D("kind-parameter", T("K"), D("sort-proper"))), D("sort-proper"),
  D("node", T("list-of"), M{}, D("item", D("kind-param", I(0)))))
```

A Text definition can use:

```text
D("kind", L(), D("sort-proper"),
  D("node", T("scalar"), M{"selector": T("utf8-text")}, D("epsilon")))
```

The bundled `horos.kinds` module provides Type, Data and the structural families
through this same language. Its authoritative definitions are maintained in
[Horos source](../../core/go/declarations/standard/horos.kinds.horos); Bittype's
ordinary frontend prepares the canonical asset that Horos decodes and loads.
Type is proper top. Data is an ordinary guarded
recursive union of the ground forms, with recursively constrained children and
no Wire alternative; [its complete family definition](../../core/go/declarations/standard/horos.kinds.horos)
contains no hidden Data flag.

Authors explicitly import `horos.kinds` to use its names. The resolver uses that
exact bundled module internally for primitive operand contracts; a user's
unrelated declaration called Data cannot alter them. There is no implicit name
import, callback registry, second language ID or separate formation artifact.

## 4. Binding, dependencies and recursion

Imports retain exact module identity and the expected module name. The caller
supplies dependency values; the resolver also makes its exact bundled prelude
available for primitive formation. Resolution performs no implicit network or
filesystem lookup. Equal module names with different pins remain distinct.

Each type definition has one ordered type-parameter scope. Its `param(i)` refers
to that definition's slot. Each kind definition has one ordered kind-parameter
scope for `kind-param(i)`. Explicit recursive groups add only recursive-kind
binders. Names and parameters do not rebind when an expression moves through an
application: constructors retain their definition environment, while arguments
retain the caller's bindings.

Every named semantic reference contributes a dependency, including references
inside ignored arguments. Named type and kind dependency cycles are rejected.
Kind recursion must use explicit guarded nu groups with fixed kind parameters.
Every recursive-kind cycle crosses a node's type-child descent; Boolean
operators, aliases and sequence operators alone do not guard it.

Ground rec/ref is separate. Resolve its lexical binders and retain the existing
strict runtime-value descent requirement. In structural observations, valid
rec/ref become backedges rather than kind-family alternatives. Type arguments
remain independently closed under their ground recursion binders: substitution
must not capture a ref from outside the argument. Data membership supplies no
recursion guard.

Only schema-designated type positions are traversed. Scalar selectors, modes,
field names/presence, recursion names, bounds and compound labels remain literal
operands. `H("exact", D("param", I(0)))` contains no free parameter. The same
opacity applies to literal tuples resembling references, applications or kind
syntax. Preserve their exact values and ordering.

## 5. Complete pagination example

The following equations spell the canonical module fields using the notation
above. Prelude denotes the bundled canonical `horos.kinds` value, not an extra
stored field or a newly invented identity. KType/KData are ordinary imported
kind references.

```text
KType = D("kind-imported", T("k"), T("Type"))
KData = D("kind-imported", T("k"), T("Data"))

Pagination = D("module", I(4), T("example.pagination"), M{
  "k": D("import", T("horos.kinds"), Pin(Prelude))
}, M{
  "Page": D("type", L(
    D("parameter", T("T"), KType)), KType,
    H("record", A("closed"),
      H("field", T("items"), A("req"), H("list-of", D("param", I(0)))),
      H("field", T("next"), A("opt"), H("scalar", A("utf8-text"))))),
  "DataPage": D("type", L(
    D("parameter", T("D"), KData)), KData,
    D("apply", D("local", T("Page")), L(D("param", I(0)))))
})

People = D("module", I(4), T("example.people"), M{
  "k": D("import", T("horos.kinds"), Pin(Prelude)),
  "pagination": D("import", T("example.pagination"), Pin(Pagination))
}, M{
  "Person": D("type", L(), KData,
    H("record", A("closed"),
      H("field", T("name"), A("req"), H("scalar", A("utf8-text"))))),
  "PeoplePage": D("type", L(), KData,
    D("apply", D("imported", T("pagination"), T("Page")), L(
      D("local", T("Person")))))
})
```

Page has one declaration identity and one binder. DataPage adds an ordinary
input/result contract without copying Page's record. Next may be absent or
present with text; present null is not admitted. No cursor protocol or fetching
behavior follows from this type.

Page<Person> and DataPage<Person> yield the same ground structure. If an ordinary
Cell<Person> declaration contains capabilities, Page<Cell<Person>> remains a valid
proper type but does not belong to Data. DataPage<Cell<Person>> cannot discharge
its argument contract. No checker recognizes the name Cell.

The same checked Page body can answer a non-Data query such as Scalar => Rows,
where Rows is `Record<Scalar | List<Scalar>>`. General result reasoning retains
that body and its assumptions; there is no dedicated Data summary language.

## 6. Constructor values and applications

A reference to a declaration with parameters denotes its constructor value.
A reference to a zero-parameter declaration denotes that declaration's body
value, which may itself be a constructor. Every type application checks the
callee's contract and supplies one complete nonempty argument list. Its result
may be applied again when its sort is a constructor sort.

For example, with the explicit prelude import from the preceding section:

```text
AnyConstructor = D("arrow", L(KType), KType)

"Alias": D("type", L(), AnyConstructor, D("local", T("Page")))

"Pick": D("type", L(
  D("parameter", T("F"), AnyConstructor)), AnyConstructor,
  D("param", I(0)))

"PickedText": D("type", L(), KType,
  D("apply",
    D("apply", D("local", T("Pick")), L(D("local", T("Page")))),
    L(H("scalar", A("utf8-text")))))
```

Alias is a distinct declaration with no parameters whose value denotes Page's
constructor. Pick's one argument and its constructor result are separate levels.
This does not admit partial application or source lambdas.

Known constructors retain checked bodies and lexical environments. To establish
a requested arrow, first establish admission by the original input contracts,
then reason about the body under fresh rigid requested inputs and establish the
result contract. An open definition must already be valid under its original
contracts. All arguments, including unused ones, are checked before reduction.

Abstract constructors expose their declared finite contracts. Intersections mean
simultaneous guarantees; unions mean unknown alternatives. Every possible callee
alternative must admit a call. Compatible-arrow reasoning is contravariant in
inputs and covariant in results. Its sound finite proof rules do not decide all
semantic constructor inclusions. See [constructor reasoning](kinds.md#5-constructors-result-contracts-and-symbolic-values).

## 7. Checked values and the ground boundary

The [Go declaration contract](../../core/go/declarations/README.md) exposes
Program-owned kinds and symbolic type-level values. Public handles are sealed;
matching source bytes in another Program do not establish shared handle ownership.
Application preserves lexical bindings and can return either a proper value or
a constructor. The original open declarations remain immutable.

Descriptor materialization requires a closed proper value and preserves the
actual descriptor forms, including Wire where admitted by the declaration
language. Ground-descriptor extraction additionally establishes membership in
the bundled ordinary ground kind and applies the unchanged ground WF judgment.
A type inclusion does not rewrite a descriptor into a different form. These
operations create no endpoint, runtime value validator, codec or hydration.

The exact closed membership procedure, free-shape inclusion and incomplete
constructor proof procedure have different guarantees. A failed free-shape
inclusion can expose an observation that no admissible Horos descriptor realizes;
it is not automatically a concrete nonmembership result.

## 8. Outcomes and resource limits

Proven means an established guarantee. Refuted requires closed nonmembership or
an admissible counterexample. Unproved means the available assumptions or proof
rules are insufficient. Invalid denotes malformed syntax, binding, sort,
ownership or request; Exhausted denotes a resource limit. In the Go contract,
Proven/Refuted/Unproved are verdicts and Invalid/Exhausted are errors. Required
obligations must be Proven before a checked Program is published.

Default limits remain depth 256, 100,000 nodes/work units, 16 MiB of canonical
bytes and 256 supplied modules. Configured depth is at most 512; limits must be
positive. Raw admission, aggregate semantic work and expanded output have separate
budgets. Automata, sequence states, Boolean alternatives, environments, keys,
comparisons and cache growth consume the semantic operation's allowance.

Caches are local to an operation. Expanded output charges repeated occurrences
even when immutable internal storage is shared. A compact symbolic result can
establish a contract while its complete materialization exceeds the output limit.
Every failed operation returns no usable partial Program, model or output.
Exhaustion establishes neither invalid source nor nonmembership.

## 9. Identity and clean replacement

Canonical means a precise value representation, not a unique representative of
all semantically equivalent types or kinds. Module pins identify canonical bytes;
declaration identity adds the name; parameter identity adds its ordered slot.
Alias expansion, union sorting, binder renaming and semantic simplification do
not occur before encoding. Source spans and native naming choices are separate
Bittype metadata.

Equivalent kind definitions may establish mutual inclusion without becoming the
same declaration. Equal structural observations establish no library ownership,
runtime authority or permission to substitute a behavioral contract.

Revision 4 replaces revision 3 rather than accepting both. Rewrite closed claims
as ordinary kind expressions, introduce ordinary kind definitions/imports where
needed, and retain explicit result kinds. Canonical module values and their
normal import references change with the rewritten declarations. Remove the old
reader, Claim/Interface, DataSummary, dedicated Data evaluator and RequiresData
route without aliases. Existing historical releases remain historical material.

## 10. Ownership and feature boundary

The kind-model engine implements general sorts, kind expressions and algorithms.
The declaration layer owns Horos primitive observations, metadata and operand
contracts. Ground WF/Conforms retains its existing data-only implementation.
No general checker recognizes the name Data, Type, Scalar, List or Record.

Bittype projects declared sorts/contracts, symbolic bodies and individual
memberships under their actual assumptions. Its generic graph and engine
interpret no Horos kind or behavioral-library names. Native generation remains
types-only with explicit representation limits; arbitrary native constraint
enforcement requires a supported encoding or returns Unsupported.

Function, Stream and Cell remain ordinary libraries. Ontos, Wire, transport and
hydration retain their respective value and operational responsibilities.
Revision 4 does not add kind complement, recursive arrow schemes, quantified
constructor schemes, growing kind instantiations, partial application, source
lambdas or arbitrary subtree predicates. An actual new primitive form changes
Horos's versioned form/metadata/child contracts; it is not an ordinary kind alias.
