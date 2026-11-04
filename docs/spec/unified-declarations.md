# Unified declarations and general kinds, revision 4

**Implemented in Go source, 4 November 2026.** Horos has
one declaration language for data and capability types. The
[general-kind consultation](https://github.com/Bitspark/bittype/issues/59)
replaces the closed Type/Data interface model with arbitrary structural kinds
and finite constructor contracts. The normative
[declaration specification](declarations.md) fixes the canonical representation;
the [kind specification](kinds.md) defines the judgments.

Ground descriptors and conformance keep their existing meanings. Revision 4
changes the declaration language cleanly, with no second Wire profile, retired
reader, template-lifting path or compatibility API.

## Ordinary definitions, one language

A kind selects type descriptions or type constructors. It is distinct from a
type, which describes values. Sorts distinguish proper values from constructors
with finite ordered inputs and a result; results can themselves be constructors.
Union and intersection are available at every sort. Kind names are arbitrary.

The ordinary `horos.kinds` module defines Type as proper top and Data as a
positive recursive union of data-shaped primitives whose children satisfy the
same ordinary category. User modules import this module explicitly before using
its names. The resolver's internal use of the bundled prelude for primitive
formation does not create implicit source bindings.

The following are explanatory kind equations:

```text
Flat = Scalar | List<Scalar>
Rows = Record<Flat>
Tree = nu X. Scalar | List<X> | Record<X>
```

List and Record are ordinary kind definitions here, not new type constructors.
Node patterns inspect a primitive form, its selected metadata and its entire
finite sequence of type children. Positive recursion uses guarded simultaneous
greatest fixed points. Only scalar selector and record mode are available as
metadata predicates. Exact payloads stay opaque; ground rec/ref are observed as
lexical backedges; open records include an implicit any child.

Data is one instance of this model. Neither the generic kind engine nor a
membership query branches on the spelling Type, Data, Scalar or Rows. Primitive
operand admission refers to particular ordinary prelude definitions by identity.
Ground `any` remains one data type, not the kind Type.

## One Page, several structural guarantees

The same Page declaration retains its original body:

```text
Page<T> = { items: list<T>; next?: text; }
DataPage<D: Data> = Page<D>
```

Page admits arbitrary proper types. A query can establish `Data => Data`, or
`Scalar => Rows`, from that body under the requested input assumptions. For each
particular suitable K it can also establish
`K => Record<List<K> | Text>`. This does not introduce a universally quantified
constructor scheme. `Flat => Rows` does not follow: an input list produces a
nested list in the items field.

Known constructors retain their bodies and binding environments. Abstract
constructors expose their declared promises. Under `F: Flat => Rows` and
`A: Flat`, `F<A>` establishes Rows; a wrapper record containing it has
`Record<Rows>` structure. A promise of `Flat => Flat` does not establish Rows.
Intersections retain joint promises, while unions retain alternatives. Every
possible callee alternative must admit each relevant argument case. Repeated
rigid inputs remain correlated across those cases.

Constructor-valued aliases and results are admitted with explicit result
contracts:

```text
Alias : Type => Type = Page
Pick<F: Type => Type> : Type => Type = F
Pick<Page><text>
```

A zero-parameter alias can denote a constructor. A declaration's parameter-list
arity is not its result sort. Each application supplies one complete nonempty
argument list; partial application and source lambdas remain outside revision 4.
The omitted result annotation defaults to proper top, so constructor aliases
must state their constructor result contract.

## What the judgments establish

Closed proper membership is exact within the admitted positive recursive
fragment. It observes a checked type graph and solves positive Boolean
obligations at a greatest fixed point. Seeing a pair before is not sufficient
evidence of membership or valid guarding.

Proper-kind inclusion uses positive recursive hedge automata, horizontal
finite-word automata and finite cuts at live states. It is not a general parity
engine. Inclusion over the free observation universe is sufficient for Horos
inclusion, but failure can involve an observation no admitted Horos descriptor
realizes. Such a result does not establish a concrete Horos counterexample.

Constructor reasoning is sound and incomplete. It preserves bodies, assumptions,
identities and binding environments rather than promising a principal regular
kind for every symbolic value. A narrower query cannot repair an invalid open
body, and unused arguments still have to satisfy their contracts.

Proven establishes a guarantee. Refuted requires closed nonmembership or an
admitted counterexample. Unproved means the available assumptions or supported
rules do not establish the claim. Invalid requests and exhausted budgets are
errors. A required Unproved obligation prevents publication of a checked
Program; missing facts assert nothing.

## Ground, tooling and library boundaries

The Go declarations layer owns resolution, kinds, contracts, checked type values
and materialization. Its general kind-model engine is separate from the fixed
Horos primitive formation rules and ground checker. The module envelope remains
revision, name, imports and declarations with revision 4; it gains no signature
digest or separate schema artifact.

`Descriptor` materializes a closed proper type, which may still contain Wire.
`GroundDescriptor` additionally establishes membership in the bundled ordinary
ground kind and applies unchanged ground WF rules. Kind inclusion does not
rewrite a descriptor, add a recursion guard or grant a runtime capability.

Bittype owns readable syntax, AST/CST projections, provider facts and native
mappings. Its generic graph does not interpret kind names. Kind declarations
are compile-time definitions rather than native type exports. Canonical validity
does not establish native support: the types-only profile retains its explicit
representation limits and returns Unsupported for residual constructor inputs,
results or aliases. No hidden specialization or closure pruning is implied.

Function, Stream and Cell remain ordinary library declarations. Runtime
behavior, native adapters, authority and protocol laws stay with the owning
libraries. Horos adds no live-value checker or codec. Ontos, Bitwire and
Bitruntime retain their value-domain boundaries.

## Clean replacement

Current canonical documents use revision 4. Parameters and declaration results
carry ordinary kind expressions; declarations can define kinds as well as types.
Source references preserve their definition and caller scopes. Literal payloads
remain opaque, and named definition cycles remain forbidden outside explicit
kind recursion.

The revision-3 Claim/Interface/DataSummary model, dedicated Data evaluator,
Specialize/DataDescriptor API and decoder are removed. Immutable historical
releases and research remain historical evidence, not alternate executable
formats. Type lambdas, partial application, universally quantified constructor
schemes, growing recursive kind instantiations and arbitrary relational subtree
predicates remain separate extensions requiring concrete use cases.
