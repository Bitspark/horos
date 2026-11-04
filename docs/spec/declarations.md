# Canonical declarations, revision 3

**Owner direction, 4 November 2026.** This is the current source contract for
Horos's Go declaration tooling. Revision 3 replaces revision 2 with explicit
constructor interfaces and higher-kinded parameters. Hosted qualification and
publication are tracked separately by release receipts.

There is one declaration language for data and capability types. Horos owns its
canonical representation, kinds, interfaces, binding and specialization.
Bittype owns readable syntax, AST/CST projections, semantic facts and native
transformations. Ground descriptor APIs retain their data-only meanings, and
Ontos retains value identity and encoding. See the
[unified decision and migration](unified-declarations.md).

## 1. Representation and notation

A module is a finite ground Ontos value. Type variables are encoded syntax, not
variables added to Ontos. Storage uses the frozen
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

`M` uses canonical map ordering and rejects duplicate keys. `L` preserves order.
Every string is exact, with no implicit Unicode normalization. `A` is a raw atom
and `T` is a text embedding. A pin contains 32 raw bytes, not hexadecimal text.
These equations determine values; notation and whitespace are not hashed.

## 2. Closed declaration vocabulary

```text
Claim       = A("type") | A("data")
Interface   = D("proper", Claim)
            | D("constructor", L(Interface, ...), Claim)
Parameter   = D("parameter", T(parameterName), Interface)
Declaration = D("type", L(Parameter, ...), Claim, body)
Module      = D("module", I(3), T(moduleName), imports, declarations)
Import      = D("import", T(expectedModuleName), Atom(32-byte digest))

E = a Horos descriptor with E in its designated child-type positions
  | H("wire", E)
  | D("param", I(zeroBasedParameterIndex))
  | D("local", T(declarationName))
  | D("imported", T(importAlias), T(declarationName))
  | D("apply", E, L(E, ...))
```

`imports` maps local aliases to Import values; `declarations` maps names to
Declaration values. All declarations are exported. The constructor interface's
input list and an application's argument list must be nonempty. A declaration
may have no parameters; refer to it directly instead of applying it to `L()`.
Canonical parameter interfaces and result claims are always explicit.

Names match `[A-Za-z_][A-Za-z0-9_]*`; module names are one or more such segments
separated by a single dot. Names are case-sensitive. Parameters in a declaration
have distinct names. Import aliases and declaration names occupy separate
namespaces. Record field names retain the descriptor's text semantics.

Unknown revisions, tags, claims, extra arguments and malformed interface trees
are errors. The module revision fixes the grammar; installing a newer package
does not silently extend an old document. Readable source abbreviations belong
to Bittype. An omitted ordinary source parameter annotation or result annotation
elaborates to explicit Type; no missing constructor annotation is inferred.

## 3. Kinds and interfaces

A proper type describes values. A type constructor accepts type-level arguments
and produces a proper type. Shape kinds record that distinction:

```text
K = Type | (K1, ..., Kn) => Type       n >= 1
```

Inputs may themselves be constructor kinds. Constructors are n-ary and fully
applied: `(Type, Type) => Type` is a two-input constructor, not a curried arrow.
Every declaration body and every application result must be a proper type.

Data is the refined kind, or subkind, of proper types established to describe
data only. Interfaces retain this requirement and constructor result guarantees:

```text
S = Type | Data | (S1, ..., Sn) => R   n >= 1
R = Type | Data
```

An interface's erased shape kind replaces Data with Type recursively. This
separates argument shape, accepted inputs and result evidence without creating
another type universe. `Type` is not the value descriptor `any`. Descriptions of
Wire capabilities are themselves Ontos data; that fact does not make the
described capability type Data.

A parameter has its declared interface. An unapplied reference to a declaration
with parameters denotes a constructor. A reference to a declaration without
parameters denotes its proper type. An application checks its callee kind and
every argument against the corresponding input interface, in order, then yields
a proper type. Every descriptor child-type position also requires a proper type.
Thus `Wrapped<Page, Person>` may be valid while `wire<Page>` and
`Wrapped<Person, Person>` are not.

An explicit Data result claim requires a proof for the entire open body under
its declared parameter interfaces. A Type result claim does not prevent the
checker from deriving stronger result evidence from that body.

### Interface compatibility

Write `S <= T` when every argument satisfying S can be used where T is expected.
Data satisfies Type. Constructor inputs are contravariant and results covariant:

```text
(A1, ..., An) => RA <= (B1, ..., Bn) => RB
    when every Bi <= Ai and RA <= RB
```

Shapes and arities must match. This finite structural comparison concerns
constructor admission, not native container variance or descriptor subtyping.

| Supplied interface | Expected interface | Outcome |
| --- | --- | --- |
| `Type -> Data` | `Data -> Type` | Accept |
| `Data -> Data` | `Type -> Type` | Reject: narrower input admission |
| `Type -> Type` | `Data -> Data` | Reject: no Data result guarantee |

For a known declaration, the checker can establish an additional interface from
its checked body. To check original inputs `B1...Bn` against expected
`(A1...An) => R`, first require each `Ai <= Bi`. Then evaluate its symbolic result
under fresh rigid arguments with the expected interfaces; a Data result requires
a proof. Rigid proof arguments are internal identities, not fabricated source
declarations. An abstract constructor parameter has no known body, so only its
declared interface and structural compatibility are available.

For example, `Page<T> = { items: list<T>; }` accepts Type and also establishes
`Data -> Data`. `Ignore<T> = text` establishes `Type -> Data`.
`Send<T> = wire<T>` establishes neither Data result. But
`OnlyData<D: Data> = text` still cannot satisfy `Type -> Type`: a constant result
does not widen its declared input admission.

## 4. Descriptor positions and Data proofs

The child-type positions are exactly those in the [ground form
contract](horos.md), plus Wire's message position. Scalar selectors, record
modes, field names, recursion names, bounds and compound labels remain literal
operands with their existing checks. A parameter cannot occupy those positions.

`exact(value)` contains literal data. This singleton has no free parameter:

```text
H("exact", D("param", I(0)))
```

Its only member is that literal tuple. Apply the same opacity rule to literals
resembling applications, references, interfaces or retired syntax. A search for
tag-shaped tuples anywhere in an Ontos tree is not declaration traversal.

| Expression category | Data rule |
| --- | --- |
| Scalar, atom, exact and refinement forms; ground `any` | Data after their ordinary checks |
| Wire | Never Data; its message must still be a valid proper type |
| Tuple, compound, list-of, closed record, union | Data when all designated value-bearing children are Data |
| Seq, set-of, map-of, open record, intersection, complement, guarded recursion | Require Data operands before admitting the expression |
| Proper parameter declared Data | Data |
| Proper parameter declared Type | Retain a symbolic requirement |
| Abstract constructor application | Retain the call and use its declared result guarantee |
| Known constructor application | Evaluate its checked summary with the actual arguments |

Data is the prescribed compositional certificate. The checker does not solve
arbitrary emptiness, inhabitation or semantic equivalence. `union()` is Data;
`wire<union()>` is not. Failure to obtain a certificate is not proof that a
non-data inhabitant exists.

Summaries preserve true, failed, and conditional results. A conditional
requirement or constructor probe can become provable when arguments become
known. It is not permanent failure, and it is not resource exhaustion. Probes
retain checked expression arguments, constructor identities and their binding
environments; Data booleans alone would lose information. Source summaries stay
immutable when one application is evaluated.

Every argument is checked, even when the result ignores it. An invalid argument
to Ignore, or an invalid message within Wire, remains an error. A Data-only use
of an unresolved conditional result fails its bound; an ordinary Type use may
remain valid. Checking a concrete application cannot repair an invalid open body.

```text
Bad<F: Type -> Type, D: Data> = DataPage<F<D>>       // reject
Good<F: Data -> Data, D: Data> = DataPage<F<D>>      // accept
BadInput<F: Data -> Data> = F<wire<Person>>         // reject
```

The current interface grammar cannot simultaneously state that an abstract F
accepts every Type and preserves Data inputs. `F: Type -> Type` guarantees no
Data result; `F: Data -> Data` permits Data inputs only. Known Page can establish
both interfaces from its body. Intersections and conditional interface contracts
remain separate future work.

## 5. Complete pagination example

These equations spell every canonical field; `Pagination` and `People` name the
module values for the subsequent pins, not extra stored fields.

```text
Pagination = D("module", I(3), T("example.pagination"), M{}, M{
  "Page": D("type", L(
    D("parameter", T("T"), D("proper", A("type")))), A("type"),
    H("record", A("closed"),
      H("field", T("items"), A("req"), H("list-of", D("param", I(0)))),
      H("field", T("next"), A("opt"), H("scalar", A("utf8-text"))))),
  "DataPage": D("type", L(
    D("parameter", T("D"), D("proper", A("data")))), A("type"),
    D("apply", D("local", T("Page")), L(D("param", I(0)))))
})

People = D("module", I(3), T("example.people"), M{
  "pagination": D("import", T("example.pagination"), Pin(Pagination))
}, M{
  "Person": D("type", L(), A("type"),
    H("record", A("closed"),
      H("field", T("name"), A("req"), H("scalar", A("utf8-text"))))),
  "PeoplePage": D("type", L(), A("type"),
    D("apply", D("imported", T("pagination"), T("Page")), L(
      D("local", T("Person")))))
})
```

Page has one identity and one binder. DataPage is an ordinary restricted alias;
it adds no copied record or value wrapper. `next` can be absent or present with
text; present null is not admitted. No cursor protocol or fetch behavior follows
from the descriptor.

`Page<Person>` and `DataPage<Person>` expand to the same ground descriptor.
`DataPage<Page<Person>>` also succeeds. If a library's `Cell<Person>` contains
Wire capabilities, `Page<Cell<Person>>` is valid but `DataPage<Cell<Person>>`
fails. No general checker recognizes the name Cell.

### Constructor arguments

The following canonical declaration accepts a constructor and a proper type:

```text
"Wrapped": D("type", L(
  D("parameter", T("F"),
    D("constructor", L(D("proper", A("type"))), A("type"))),
  D("parameter", T("T"), D("proper", A("type")))), A("type"),
  H("record", A("closed"),
    H("field", T("value"), A("req"),
      D("apply", D("param", I(0)), L(D("param", I(1)))))))
```

In explanatory readable notation:

```text
Wrapped<F: Type -> Type, T> = { value: F<T>; }
Compose<F: Type -> Type, G: Type -> Type, T> = F<G<T>>
DataWrapped<F: Data -> Data, D: Data> = { value: F<D>; }
Ignore<T> = text
Send<T> = wire<T>
```

| Application | Outcome |
| --- | --- |
| `Wrapped<Page, Person>` | Data |
| `Wrapped<Ignore, wire<Person>>` | Data: the admitted argument is unused |
| `Wrapped<Send, Person>` | Valid Type, not Data |
| `Compose<Page, Page, Person>` | Data |
| `DataWrapped<Page, Person>` | Data: Page establishes the preservation guarantee |
| `DataWrapped<Send, Person>` | Reject the constructor argument |
| `DataWrapped<Ignore, wire<Person>>` | Reject the proper argument's Data obligation |
| `Wrapped<DataPage, Person>` | Reject DataPage's narrower constructor admission |

## 6. Modules, binding and specialization

Imports pin exact canonical bytes and an expected module name. The caller
supplies the full transitive module closure, including unused imports; resolution
performs no implicit retrieval or IO. Unrelated pool values are bounded and
fingerprinted but do not become part of the resolved closure. Equal module names
with different pins remain different modules.

Resolve references in the module where the expression was authored. Application
arguments retain their caller's binding. A named constructor retains its own
definition environment when supplied to another declaration. Never move raw names
to another module and resolve them again there. `param(i)` refers to the enclosing
declaration's ordered slot; applications and interface trees create no new source
parameter scopes. Equal spellings in different declarations do not share binders.

The [Go API](../../core/go/declarations/README.md) checks canonical modules and
publishes immutable declarations, expressions, interfaces and Data summaries.
Specialization is a separate operation. It accepts checked proper-type arguments
and sealed constructor handles belonging to that same checked Program. It rejects
zero or foreign handles, including handles from a different Program resolving
identical bytes. A declaration ID alone is not evidence of checked ownership.

Specialization checks every original input contract, substitutes by stable
binder identity, expands in definition scope and revalidates the closed result.
It leaves the open source and its provenance intact. `DataDescriptor` additionally
establishes Data and returns the existing ground descriptor for WF/Conforms.
`Specialize` may return a closed mixed type expression, which is not a ground
checker input. Neither operation adds a live-value checker or runtime codec.

## 7. Dependency cycles, recursion and bounds

The declaration dependency graph includes every semantic named reference,
including unapplied constructors and ignored arguments. It must be acyclic:

```text
IgnoreF<F: Type -> Type, T> = text
Loop<T> = IgnoreF<Loop,T>                 // reject the reference cycle
```

Ground guarded `rec`/`ref` remains separate. Each body and each application
argument is independently closed under its recursion binders. Arguments may use
their own guarded recursion and their enclosing declaration's parameters, but
cannot capture a `ref` bound outside the argument. Data preservation does not
establish a recursion guard: `Id<T> = T` preserves Data but cannot make an
unguarded `rec X. Id<X>` lawful. Final expanded descriptors are checked again.
Guarded well-formedness does not assert that a descriptor has an inhabitant.

Default limits are depth 256, 100,000 nodes/work steps, 16 MiB of canonical bytes
and 256 supplied modules including the root. Configured depth lies in 1..512;
other limits must be positive. Raw admission counts the complete supplied pool,
including literals, before encoding. One semantic-work budget covers expression
reading, dependency checking, interface traversal, comparison, environments,
summary evaluation and cache growth. It is not reset per phase, declaration or
query. Generated interface and environment keys consume the semantic operation's
byte allowance before allocation.

Evaluation caches are local to an operation and retain exact expression,
constructor, interface and binding identities. A declaration's immutable symbolic
summary is not overwritten with one application's answer. Expanded output has
separate admission: repeated occurrences count even when storage is shared.
A small source DAG may have a cheap Data proof and still exhaust output limits.
The ground descriptor checker retains its own diagnostic limits.

Every failed resolution returns no Program, and failed specialization returns no
usable partial result. Exhaustion is an incomplete request, never proof of invalid
source or of Data. It cannot trigger `any` substitution, omitted members, a weaker
translation or a hidden fallback. Missing dependency context and malformed source
also retain distinct diagnostic meanings.

## 8. Identity and normalization

Canonical means a precisely specified value representation, not one normal form
for extensionally equal types.

- Module identity is its Ontos value; Pin addresses its canonical bytes.
- Declaration identity is the defining module digest and declaration name.
- Parameter identity adds the declaration's zero-based slot index.
- References, callees and arguments retain their identities and origin; an alias
  retains its own declaration identity even if expansion gives an equal descriptor.

Map embeddings normalize entry order. Lists, parameter order, descriptor operand
order, literals and names remain exact. There is no alias expansion, union
sorting/deduplication, binder renaming or semantic simplification before encoding.
Renaming a binder changes the document and pin even when behavior is preserved.

Structural kind/interface comparison and named expansion are separate operations
from canonical identity. No extensional constructor equality, beta/eta equality
or descriptor-equivalence solver is provided. Equal shapes never establish
behavioral library ownership or lawful substitution. Source spans, comments,
native profiles and compiler names remain separate tooling metadata.

## 9. Rejection boundary and acceptance obligations

The current language rejects partial or over-application, constructor-valued
bodies, bare constructor aliases, source type lambdas, kind variables and
kind-polymorphic schemes. For example:

```text
Return<F: Type -> Type> = F       // constructor-valued body
P = Page                         // constructor-valued alias
Compose<Page>                    // incomplete application
Return<Page><Person>              // no constructor-valued application result
```

An authored `P<T> = Page<T>` remains an ordinary distinct declaration. No tool
manufactures it as a replacement Page or template-lifting path.

Qualification must establish these preservation obligations:

1. Substitution of arguments satisfying interfaces preserves kind and
   well-formedness.
2. An open Data proof remains valid under every admitted substitution.
3. Bounded closed symbolic evaluation agrees with independent eager expansion
   and primitive Data classification.

Expected judgments must be authored independently of the production checker.
Include Page/Ignore/Send precision, nested higher-order polarity, invalid open
bodies, unused invalid arguments, identity and caller scope, literal lookalikes,
ignored-reference cycles, recursion capture, malformed interfaces, explicit
result claims, cache-order independence, foreign handles, budget exhaustion and
mutation/concurrency protection. Retain the full ground cross-language gate.
Native compiler qualification belongs to Bittype and does not prove Data for
arbitrary native instantiations.

## 10. Ownership and clean migration

Horos owns this calculus. Bittype projects checked facts through its agnostic
carrier and provides readable syntax and separately qualified native mappings.
The carrier and general engine do not interpret Type, Data, Wire or library
names. Function, Stream, Cell and unfamiliar contracts remain ordinary pinned
libraries; their behavior does not follow from structural shape. See the
[Wire examples](wire-types.md).

Migrate revision-2 proper bounds to `proper` interfaces, add explicit Type result
claims where no Data claim was authored, use revision 3, and replace empty
applications with direct references. Recompute pins from dependency leaves
outward. Preserve opaque literal payloads exactly. The replacement reader rejects
revision 2 and older formats; there are no compatibility aliases or dual readers.
Historical releases and research evidence remain immutable.

This change does not widen ground any, admit endpoints as ground data, define
runtime recognition, modify hydration, add library-name dispatch or revive a
separate Horos/Wire language. Ontos, Bitwire, Bitruntime and Bitnode retain their
existing value and transport responsibilities.
