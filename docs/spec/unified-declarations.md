# Unified declarations and kinds, revision 3

**Owner direction, 4 November 2026.** The owner first selected one declaration
language for data and capability types, then requested a proper kind system.
The accepted [kind-system consultation](https://github.com/Bitspark/bittype/issues/55)
and [Horos implementation work](https://github.com/Bitspark/horos/issues/29)
extend that one language. Revision 3 replaces revision 2 cleanly. Hosted
qualification and publication remain separate release evidence.

The normative [declaration specification](declarations.md) defines the exact
format and judgments. This page records the selected direction and migration.
There is no separate Horos/Wire profile, compatibility reader or template-lifting
path. Ground data descriptors and their conformance APIs retain their meanings.

## One language and explicit constructor interfaces

Documents remain finite ground Ontos values encoded with ontos-codec-v1.
Imports retain expected names and exact SHA-256 pins. Resolve the complete
reachable import closure, including unused imports. Unrelated pool values are
bounded and hashed but not interpreted as modules.

Using the A/T/I/L/M/H/D notation from the declaration specification:

```text
Claim = A("type") | A("data")
Interface = D("proper", Claim)
          | D("constructor", L(Interface, ...), Claim)

D("module", I(3), T(name), imports, declarations)
D("import", T(expectedName), Atom(32-byte pin))
D("type", L(D("parameter", T(name), Interface), ...), Claim, body)
D("param", I(zeroBasedSlot))
D("local", T(name))
D("imported", T(alias), T(name))
D("apply", calleeExpression, L(argument, ...))
```

Constructor input lists and application argument lists are nonempty. All
applications and declaration bodies produce proper types. Unapplied named
constructors may be arguments; zero-parameter declarations are direct references.
Inputs may themselves have constructor interfaces, with finite explicit kinds.
No partial application, constructor-valued body, bare constructor alias, source
type lambda, kind variable or kind-polymorphic scheme is admitted.

Data is the refined kind of proper types established data-only. Erasing Data to
Type gives the shape kind; retaining it in an interface states argument admission
or a result guarantee. Shape, admission and established result evidence serve
different purposes. Type is not ground `any`. A type description is Ontos data
even when the type describes capabilities.

## One Page, with preserved result precision

```text
Page<T> = { items: list<T>; next?: text; }
DataPage<D: Data> = Page<D>
Person = { name: text; }
Wrapped<F: Type -> Type, T> = { value: F<T>; }
DataWrapped<F: Data -> Data, D: Data> = { value: F<D>; }
Compose<F: Type -> Type, G: Type -> Type, T> = F<G<T>>
Ignore<T> = text
Send<T> = wire<T>
```

There is one Page identity and binder. DataPage is an ordinary restricted alias.
Page accepts Type inputs and its body establishes Data output for Data inputs.
Ignore establishes Data output for every admitted Type input. Send never has a
Data result. These are checked facts about ordinary declarations, not a catalogue
of recognized library names.

| Application | Required result |
| --- | --- |
| `Wrapped<Page,Person>` | Data |
| `Wrapped<Ignore,wire<Person>>` | Data |
| `Wrapped<Send,Person>` | Valid Type, non-Data |
| `Compose<Page,Page,Person>` | Data |
| `DataWrapped<Page,Person>` | Data |
| `DataWrapped<Send,Person>` | Reject the constructor argument |
| `DataWrapped<Ignore,wire<Person>>` | Reject the proper argument |
| `Wrapped<DataPage,Person>` | Reject DataPage's narrower admission |

Compare constructor inputs contravariantly and results covariantly. A known
declaration can establish a stronger result interface by checking its original
admission and evaluating its summary under fresh rigid expected inputs. An
abstract constructor parameter uses only its declared guarantee. Therefore
`DataPage<F<D>>` is invalid under `F: Type -> Type`, even when D is Data; it is
valid under `F: Data -> Data`. A proposed caller cannot repair an invalid body.

Data proofs follow actual parameter use. Retain unresolved constructor probes
with their checked expressions, bindings and identities. A conditional proof is
neither permanent failure nor exhaustion. Validate every argument before summary
simplification, including ignored arguments and messages in Wire types.

## Ground and library boundaries

Tuple, compound, list-of, closed record and union admit proper data or capability
types and are Data when their value-bearing children are Data. Wire is never
Data. Other ground constructors retain their Data-only operands. Exact literals,
labels and other non-type operands remain opaque to declaration traversal.

Data is a compositional proof criterion, not an emptiness or arbitrary semantic
subtyping solver. Ground `any`, complement and guarded recursion retain their
existing meanings. A Data-preserving constructor supplies no recursion guard;
arguments remain independently recursion-closed and final output is rechecked.

Function, Stream, Cell and unfamiliar contracts remain ordinary pinned library
declarations. Their behavior, runtime adapters and authority are library/runtime
responsibilities. Horos adds no live-value validator or codec. Ontos, Bitwire,
Bitruntime and Bitnode remain unchanged.

## Tooling and checking obligations

Horos owns decoding, kinds/interfaces, reference resolution, Data summaries and
typed specialization. Bittype projects authoritative checked facts and owns
readable syntax and separately qualified native mappings. Its agnostic engine
and semantic carrier interpret none of the Type/Data/Wire vocabulary.

Every semantic named reference contributes a dependency edge, including an
unapplied constructor in an ignored argument. Cycles reject before body checking.
Constructor handles used for specialization are sealed to the same checked
Program; equal canonical bytes in another Program do not confer ownership.

Keep aggregate semantic work bounded, including interfaces, comparisons,
environments, summaries and caches. Keep raw admission and expanded-output
budgets separate. Count repeated output occurrences. Failed operations expose no
usable partial model. Exhaustion is incomplete checking, not invalid source or
proof of Data.

Qualification must independently establish substitution preservation, Data-proof
preservation and agreement between bounded symbolic analysis and eager expansion.
Retain independent negative cases, concurrency/mutation protection, diagnostic
subjects, the complete ground conformance gate and Bittype's native pagination
qualification. None is implied by a source commit alone.

## Clean migration

1. Replace revision-2 Type/Data atom bounds with `proper` interfaces. Add the
   explicit Type result claim unless a stronger claim is authored and checked.
2. Change the module revision to 3. Replace zero-argument applications with
   direct local/imported references. Keep generic applications fully saturated.
3. Recompute canonical dependency pins from leaves outward. Preserve literal
   payloads, names, ordering, source identities and native representation origins.
4. Remove superseded bound-only APIs and revision-2 readers without aliases.
   Re-pin consumers to the verified public replacement dependency.
5. Preserve immutable historical releases and research evidence. Complete hosted
   checks and installed-consumer verification before claiming delivery.

Intersections/conditional contracts for abstract constructors remain deferred
until an accepted case needs arbitrary-Type admission and conditional Data
preservation together. Partial application, constructor-valued results, lambdas
and kind polymorphism require their own concrete cases and qualification.
