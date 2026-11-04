# General structural kinds

**Revision-4 contract, implemented in Go source, 4 November 2026.** Kinds
classify checked type descriptions and type constructors. Type and Data are
ordinary definitions in the `horos.kinds` module. The language has no named
Type/Data cases, dedicated Data summaries or callback predicates.

The [canonical declaration specification](declarations.md) fixes the encoding,
names and binding rules. This page defines the kind judgments and their limits.
The accepted direction comes from the
[general-kind consultation](https://github.com/Bitspark/bittype/issues/59), with
two adaptations: the module envelope has no new signature digest, and proper-kind
inclusion uses finite cuts of positive recursive hedge automata rather than a
general parity-automaton engine.

## 1. Types, kinds and sorts

A type describes values. A kind describes types or type constructors. The type
`text` describes text values; the ordinary kind Scalar selects particular
structural type descriptions. Kind definitions may constrain their children:

```text
kind Flat = Scalar | List<Scalar>
kind Rows = Record<Flat>
kind Tree = nu X. Scalar | List<X> | Record<X>
```

These equations are explanatory notation, not Bittype's source grammar. List
and Record here are kind definitions, while `list-of(text)` is a type. Kind
application and type application operate at different levels. Names have no
capitalization rule and no semantic significance outside ordinary binding.

Each kind has a finite simple sort:

```text
Sort = proper | (Sort, ...) -> Sort
```

An arrow sort has a nonempty ordered input list and one result sort. Its result
can itself be a constructor sort. Sorts distinguish proper types from constructor
values without imposing any data-only policy. Union and intersection operands
must have the same sort. Top and bottom exist at every sort.

The prelude defines Type as top at the proper sort. A different local kind named
Type has the same status as any other local definition. Ground Horos `any` is
one proper type, not kind top. The empty type union is a type describing no
values; kind bottom contains no types at all.

Kinds classify structure, not all possible runtime inhabitants. A type union
with scalar and record arms has a union root; it does not automatically belong
to the kind union `Scalar | Record`. A record with a Wire field remains outside
Data even if another required field makes that record impossible to inhabit.

## 2. Observation graphs and node patterns

A closed proper type is observed by resolving aliases and known applications,
retaining primitive form tags, and following only designated type children.
The result is a finite graph whose vertical unfolding may be infinite:

- Ground `rec`/`ref` binders become lexical backedges.
- An exact literal has no observed type children; its payload stays opaque.
- A record contributes every field type in order, including optional fields.
- An open record also contributes one implicit `any` child for additional fields.
- Labels, presence markers and other literal metadata keep their ordinary
  descriptor meaning and are not searched for embedded type syntax.

A node pattern matches one form, its selected metadata and its entire finite
ordered child sequence. Sequence expressions provide epsilon, one item,
concatenation, alternatives and star. Star permits any finite number of matched
items, never an infinitely wide tuple or record.

Only two metadata selectors are available in this revision:

| Observation selector | Canonical key | Admitted text values |
| --- | --- | --- |
| scalar.selector | `selector`, on `scalar` | `int`, `utf8-text`, `bool` |
| record.mode | `mode`, on `record` | `open`, `closed` |

Selector map keys and values are canonical Ontos text embeddings. An omitted
selector is a wildcard. Other metadata remains present and subject to primitive
admission, but kind patterns cannot predicate on it. There are no arbitrary
label tests, numeric-bound predicates, callbacks or subtree-equality operators.

For example, the kind expression for List takes one proper-kind parameter and
uses `node("list-of", {}, item(K))`. Record uses
`node("record", {}, star(item(K)))`. An open record whose named fields are scalar
does not establish `Record<Scalar>` because its implicit any child is not Scalar.

## 3. Ordinary prelude definitions

`horos.kinds` is a normal canonical module. Authors explicitly import its module
identity before using its names. The resolver also uses the bundled module
internally to bind primitive operand contracts. This internal use does not make
its names implicitly visible in user modules.

The following equations expand only into ordinary node, sequence, union,
reference and recursive-kind forms. `leaf(form)` abbreviates a node with an empty
selector map and epsilon children:

```text
Type       = top(proper)
Any        = leaf("any")
AtomFamily = leaf("atom") | leaf("atom-exact")
           | leaf("atom-len") | leaf("atom-in")
Text       = node("scalar", {selector: "utf8-text"}, epsilon)
Scalar     = node("scalar", {selector: "int"}, epsilon)
           | Text | node("scalar", {selector: "bool"}, epsilon)
           | leaf("text-len") | leaf("key-grammar")
           | leaf("text-non-blank") | leaf("lowercase-uuid")
Exact      = leaf("exact")

Tuple<K>        = node("tuple", {}, star(item(K)))
Compound<K>     = node("compound", {}, star(item(K)))
Seq<K>          = node("seq", {}, item(K))
List<K>         = node("list-of", {}, item(K))
Set<K>          = node("set-of", {}, item(K))
Map<K,V>        = node("map-of", {}, concat(item(K), item(V)))
Record<K>       = node("record", {}, star(item(K)))
ClosedRecord<K> = node("record", {mode: "closed"}, star(item(K)))
OpenRecord<K>   = node("record", {mode: "open"}, star(item(K)))
UnionOf<K>      = node("union", {}, star(item(K)))
AndOf<K>        = node("and", {}, star(item(K)))
Not<K>          = node("not", {}, item(K))
Wire<K>         = node("wire", {}, item(K))
WireFamily      = Wire<Type>

Data = nu D.
    Any | AtomFamily | Scalar | Exact
  | Tuple<D> | Compound<D> | Seq<D>
  | List<D> | Set<D> | Map<D,D>
  | ClosedRecord<D> | OpenRecord<D>
  | UnionOf<D> | AndOf<D> | Not<D>
```

There is no hidden data-leaf flag. An exact text singleton belongs to Exact,
without automatically becoming a scalar-shaped type. Ground rec/ref require no
extra alternatives after observation lowering. The open record's implicit any
tail belongs to the same ordinary Data definition.

Renaming a definition, importing an equivalent one or defining an unrelated
recursive category uses the same machinery. A user's unrelated Data declaration
does not alter primitive contracts: those refer to the bundled module's actual
declaration identity, not its display spelling.

The language revision fixes primitive forms, supported metadata selectors,
type-child positions and formation rules. Their implementation is separate from
the general kind-model engine and the ground value checker. There is no separate
schema registry, signature digest or independently managed formation artifact.

## 4. Recursion and formation

Named kind-definition references form an acyclic graph. Recursion is explicit
inside a nonempty simultaneous `nu` group of proper-kind equations. Recursive
variables select a group by lexical depth and an equation by slot. Kind-definition
parameters remain fixed inside the group; growing recursive instantiations are
not admitted.

Every recursive cycle must cross a node's type-child descent. Boolean
connectives, aliases and sequence composition alone do not guard a cycle.
`nu X. X` is invalid before simplification. Recursive groups denote greatest
fixed points, allowing regular infinite vertical observations such as the graph
of a guarded recursive list. Their child sequences are always finite.

Ground recursion uses a separate rule: refs must follow strict runtime-value
descent. Kind guarding neither supplies a ground recursion guard nor changes
the meaning of a ground complement. Not is a positive structural node pattern,
even though the type `not(T)` denotes a complement within ground values.

Primitive operand contracts preserve existing admission:

| Primitive forms | Operand contract |
| --- | --- |
| Ground leaves and exact | No type children |
| Tuple, compound, list-of, closed record, union | Proper type children |
| Wire | Proper message type |
| Seq, set-of, map-of, open record, and, not | Children belong to the bundled ordinary Data kind |
| Rec | Existing lexical/guard rules and body membership in that ordinary Data kind |
| Ref | A valid lexical ground-recursion backedge |

The Data requirement in this table is an ordinary bound kind reference. It is
not a name-sensitive branch in membership or inclusion.

## 5. Constructors, result contracts and symbolic values

An arrow kind `(A1, ..., An) => B` promises admission of those input kinds and
a result belonging to B. It promises nothing about other inputs. Result sorts
may be constructor sorts. Declaration parameter-list arity is distinct from the
sort of its value: a zero-parameter alias can denote a constructor.

```text
Alias : Type => Type = Page
Pick<F: Type => Type> : Type => Type = F
Pick<Page><text>
```

These examples assume an explicit prelude import. Constructor-valued aliases and
results require a suitable result annotation; a proper-top default would not fit.
Each application supplies one complete nonempty argument list. Partial
application and anonymous type-level lambdas remain outside revision 4.

For a known constructor, establish a requested arrow by first proving its domain
is admitted by the original parameter contracts, then evaluating the retained
body under fresh rigid inputs and establishing the requested result. Every open
body must already satisfy its original contracts. A narrower query cannot repair
an invalid declaration, and an ignored argument is still checked.

The same Page body can establish `Data => Data` and `Scalar => Rows`. More
generally, for each particular suitable K it can establish
`K => Record<List<K> | Text>`. This is a query for an instantiated kind, not a
universally quantified arrow. `Flat => Rows` does not generally hold: a list
element produces a nested list in the items field. Ignore can establish a scalar
result for every admitted input; Send can establish `Wire<K>` for a K message.

Abstract constructors expose their declared contracts. If `F: Flat => Rows` and
`A: Flat`, then `F<A>` establishes Rows. The wrapper `{value: F<A>}` instead has
`Record<Rows>` structure. If F promises only `Flat => Flat`, its result might be
text and does not establish Rows.

Keep joint contracts distinct from alternatives:

```text
(A => B) & (A => C)  applied to A establishes B & C
(A => B) | (A => C)  applied to A establishes B | C
```

The finite proof procedure considers alternatives of callee contracts and cases
of certified argument bounds. Every possible alternative must admit every
relevant argument case. Intersect simultaneously applicable guarantees within a
case, then union the results across alternatives. A repeated rigid parameter
retains one identity when assumptions split; occurrences are not independent.
Do not distribute `Record<A | B>` into `Record<A> | Record<B>`.

Arrow compatibility uses contravariant domains and covariant results,
recursively through finite sorts. These are sound sufficient rules, not complete
semantic constructor subtyping. An unrestricted-input Wrapped contract cannot
accept a constructor promised only `Data => Data`; it needs the original input
guarantee as well, possibly through an intersection of arrows.

Retained symbolic values preserve bodies, assumptions and binding environments.
They do not promise a principal inferred regular kind. For example,
`Duplicate<T> = tuple(T,T)` preserves equality between two unknown subtrees,
which ordinary regular shape patterns cannot generally express.

## 6. Membership and finite-cut inclusion

Closed proper membership is exact within the admitted kind fragment. Build
positive Boolean obligations for pairs of observation nodes and instantiated
kind states. Node predicates inspect the form/selected metadata; sequence
transitions consume the entire finite child list. Union creates disjunction and
intersection conjunction. Solve recursive obligations at their greatest fixed
point by propagating failures until stable. A previously visited pair alone is
not evidence of membership or valid recursion.

Proper-kind inclusion operates over a free structural observation universe that
contains all admitted Horos observations. It admits potentially infinite regular
vertical unfolding and only finite child lists. The fixed metadata partitions
provide a finite alphabet abstraction.

Compile positive recursive shapes to hedge automata, with horizontal finite-word
automata for sequences. Compose simultaneous obligations before computing live
states: an intersection must constrain the same children. Determine which states
have a finite or infinite regular continuation. Only then allow an internal cut
leaf at live states and decide inclusion of the resulting finite-prefix languages.
Separately live states do not justify a cut for an incompatible intersection.

This is the finite-cut route for positive greatest-fixed-point kinds, extended
to finite child sequences and the fixed selector partitions. It does not use a
general parity-automaton engine or add public kind negation. No complexity claim
for a simpler tree language automatically covers all these constructions.

Free-shape inclusion is sufficient for inclusion among admitted Horos types.
Failed free-shape inclusion does not necessarily exhibit an admissible Horos
counterexample. For example, `nu X. Wire<X>` has an infinite free observation,
but current ground-recursion admission cannot realize it. Constructor-contract
reasoning is deliberately incomplete even when its finite proof search finishes.

## 7. Outcomes and resource limits

| Outcome | Meaning |
| --- | --- |
| Proven | The requested guarantee is established |
| Refuted | A checked closed value fails membership, or an admitted counterexample establishes failure |
| Unproved | The available assumptions or supported proof rules do not establish the claim |
| Invalid | Syntax, binding, sort, ownership or request is malformed |
| Exhausted | A resource budget prevented completion |

The Go contract uses Proven/Refuted/Unproved verdicts and errors for Invalid or
Exhausted. An unrealizable free-shape counterexample is not a Refuted Horos
membership claim. A required obligation that remains Unproved prevents publication
of a checked Program. Missing membership facts assert nothing, and completion
does not mean enumeration of every true membership.

Raw admission, aggregate semantic work and expanded output have separate budgets.
Kind compilation, automata, comparisons, environments, alternatives and cache
growth consume the current operation's allowance. Materialization charges every
repeated output occurrence even when internal storage is shared. Caches remain
operation-local; checked Programs and exposed facts remain immutable. No failed
operation returns a usable partial model or output.

## 8. Ground and library boundaries

Ground-descriptor extraction requires a closed proper value, membership in the
bundled ordinary ground kind, materialization, and the unchanged ground WF
judgment. Those steps can separately exhaust. A subset proof does not rewrite
the descriptor's syntax. Ground Ontos values and Go/Rust/TypeScript ground Horos
semantics remain unchanged; declaration tooling remains a separate Go layer.

Bittype projects sorts, declared contracts, symbolic bodies and individual
memberships with their assumptions. Its generic carrier does not understand
Scalar, List, Record, Data or any behavioral library. Types-only native generation
uses an explicit representation contract; native enforcement requires a supported
encoding or returns Unsupported. Kind declarations themselves are compile-time
definitions, not native type exports.

The existing types-only profile covers named closed records, list fields,
optional fields, text/bool scalars and its admitted ordinary proper generics.
Residual constructor parameters, constructor results or aliases, unused proper
parameters, integer scalars and other unsupported descriptor forms remain
Unsupported across the resolved closure. Kind declarations do not cause hidden
specialization, pruning or a new runtime constraint layer.

Function, Stream and Cell remain ordinary library declarations. A kind membership
never selects their behavior, native representation or runtime authority. Adding
an ordinary kind definition requires no general engine change. Adding a primitive
type form changes Horos's versioned form/metadata/child contracts and is a distinct
language change.

Revision 4 removes the closed Claim/Interface and DataSummary model, its dedicated
Data evaluator and the revision-3 reader. Quantified constructor schemes,
growing recursive kind instantiations, partial application, source lambdas and
arbitrary relational subtree predicates remain separate extensions.
