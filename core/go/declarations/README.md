# Go declaration tooling

**Revision-4 API, implemented in Go source, 4 November 2026.** The
package `github.com/Bitspark/horos/core/go/declarations` is the public Go boundary
for [canonical declarations](../../../docs/spec/declarations.md) and
[general structural kinds](../../../docs/spec/kinds.md). It consumes Ontos
module values and supplies no source parser, native generator or runtime binding.
The internal kind-model engine is separate from primitive Horos formation and
from the existing ground checker.

## Resolution and identity

`Resolve(root, dependencies)` admits a root and its supplied transitive dependency
values. It resolves canonical imports, kind and type names, parameter slots,
sorts, primitive operands and constructor contracts. Every required obligation
must be established before a checked `Program` is returned. Unused imports,
declarations and arguments remain subject to admission. An ignored argument
cannot make an invalid type or contract acceptable.

Module pins identify canonical bytes; import declarations also state the expected
module name. `Pin(value)` determines that identity without establishing the
module's meaning. `ResolveWithLimits` and `PinWithLimits` accept explicit limits.
There is no implicit IO or registry. Unreferenced pool values consume admission
resources but do not enter the resolved closure.

The ordinary kind prelude is exposed through:

```go
Prelude() core.Value
PreludePin() Digest
PreludeImport() core.Value
```

Its module name is `horos.kinds`. `PreludeImport()` supplies the canonical import
value; the importing module chooses its alias. Resolution uses the exact bundled
prelude internally for primitive formation contracts. A user must still import
it explicitly to refer to Type, Data, Scalar, List or any other prelude name.
A local declaration called Data does not replace those bound formation contracts.

A checked Program retains immutable source values, declarations, resolved
references and binding environments. Declaration identity contains its module
identity and name; parameter identity adds its ordered slot. Named references
retain definition scope, while arguments retain caller scope. Kind-definition
parameters and type-declaration parameters have separate binding domains.

## Checked kinds and type values

The public API uses `KindHandle` and `TypeValue`. Both are sealed to the checked
Program that creates them. Zero handles and handles from a different Program are
invalid, even when the other Program resolved equal canonical bytes. There is no
implicit cross-Program import.

| Program method | Result and purpose |
| --- | --- |
| `CompileKind(module Digest, expr core.Value)` | `(KindHandle, error)`; compile a canonical kind expression in the named module's scope |
| `Value(id)` | `(TypeValue, error)`; obtain the checked value of a type declaration |
| `Proper(expr core.Value)` | `(TypeValue, error)`; admit a closed proper type expression |
| `Apply(callee TypeValue, arguments []TypeValue)` | `(TypeValue, error)`; apply one complete nonempty argument list |
| `Satisfies(value TypeValue, kind KindHandle)` | `(Verdict, error)`; establish membership or a constructor guarantee |
| `Includes(actual KindHandle, expected KindHandle)` | `(Verdict, error)`; establish actual ⊆ expected |
| `Descriptor(value TypeValue)` | `(core.Value, error)`; materialize a closed proper type expression |
| `GroundDescriptor(value TypeValue)` | `(core.Value, error)`; establish ordinary ground-kind membership and return a ground descriptor |

A type value may be proper or a constructor. `Value` preserves that distinction:
a zero-parameter alias of Page is still a constructor. `Apply` can return another
constructor when the declaration's result sort requires it. Every application is
fully saturated at its current stage; a following application can consume the
constructor result. There is no partial application or source type lambda.

Kind and expression projections retain canonical source through `Value()` and
expose checked sort/contract metadata through `Sort()` and `Contract()`. A sort is
a finite simple shape, such as proper or `(proper) -> proper`. A contract is an
ordinary kind expression, including unions or intersections of arrows. It is not
a privileged Type/Data flag or a replacement for source syntax. An expression's
contract is its static source-context guarantee, separate from its published
membership facts.

Declarations and expressions expose source-context `Membership` facts.
`Membership.Kind()` identifies the established kind and `Assumptions()` returns
its assumptions; each `Assumption` exposes `Parameter()` and `Kind()`. A
declaration's full checked contract has no assumptions. Body and operand facts
can depend on the original parameter contracts. Queries do not mutate this
published fact set, and it is not a principal-kind inference API.

## Guarantees and uncertainty

Type and Data are ordinary named kind definitions. Closed proper membership
inspects the observed type graph: aliases and known applications are resolved,
rec/ref become lexical backedges, exact literals remain opaque, and open records
include their implicit any tail. Membership is exact within the admitted positive
recursive fragment, subject to resource limits.

Proper-kind inclusion uses positive recursive hedge automata, finite child
sequences and finite cuts at live states. It reasons over a free observation
universe containing all admitted Horos observations. Free-shape inclusion is
sufficient for a Horos guarantee; a failed free-shape comparison need not supply
an admissible Horos counterexample. `Includes` reports that inconclusive case as
Unproved; it does not fabricate a realizing descriptor. This is not a general
parity engine.

Constructor reasoning is sound but incomplete. A known constructor retains its
body and environment: the checker admits a requested input domain against the
original contracts, introduces rigid inputs, and establishes the requested result.
An abstract constructor exposes its declared promises. Intersections retain joint
promises; unions retain alternatives. Repeated uses of one rigid input remain
correlated when cases split.

For example, the existing Page body can establish `Data => Data` and
`Scalar => Record<Scalar | List<Scalar>>`. These use the same general machinery as
an unrelated user kind. Under an abstract `F: Flat => Rows` and `A: Flat`, `F<A>`
establishes Rows; a record containing it has `Record<Rows>` structure. An
`F: Flat => Flat` promise alone cannot establish Rows. No principal inferred
regular kind is promised for every symbolic body.

| Verdict | Meaning |
| --- | --- |
| `Proven` | The requested guarantee is established |
| `Refuted` | Closed membership fails, or an admissible counterexample establishes failure |
| `Unproved` | The available assumptions or supported proof rules do not establish the claim |

Malformed syntax, binding, sort or handle ownership yields an `Invalid` error.
A resource limit yields `Exhausted`. Neither error is a membership verdict.
Unproved is not false, and a missing membership fact asserts nothing. A required
Unproved obligation prevents creation of a checked Program. Completed resolution
does not enumerate every true membership.

## Materialization and the ground boundary

`Descriptor` requires a closed proper value. Its result can contain Wire, so it
is not automatically a descriptor for ground `WF` or `Conforms`.
`GroundDescriptor` additionally establishes membership in the bundled ordinary
ground kind, materializes the result and applies the unchanged ground WF rules.
It returns the existing ground descriptor rather than a wrapper or another
value profile. Each stage can exhaust independently.

All original argument contracts apply even when the body ignores an argument.
A query cannot repair an invalid open declaration. Kind membership does not add
a ground recursion guard: arguments remain independently recursion-closed and
materialized output must satisfy the appropriate ground rules. Exact literals
and other non-type operands are never searched for apparent declarations.

Neither method creates a live value, recognizes an endpoint, grants authority or
establishes protocol compliance. Function, Stream and Cell remain ordinary
library declarations whose runtime meanings belong to their libraries.

## Resource bounds

Default limits are depth 256, 100,000 node visits/work steps, 16 MiB of canonical
bytes and 256 supplied modules including the root. Depth lies in 1..512; other
limits must be positive. Raw admission counts the full pool, including literals.
Aggregate semantic work covers compilation, automata, comparisons, case splits,
environments and cache growth. It is not reset for each phase or declaration.

Expanded-output admission is separate. Repeated occurrences consume the output
budget even when immutable subtrees share storage. Ground checker limits remain
in force. Caches are operation-local and retain exact identities and bindings;
a failed operation exposes no usable partial Program, handle or output.

Revision 4 replaces Claim, Interface, DataSummary, named-constructor Argument and
the Specialize/DataDescriptor API. It has no revision-3 reader or compatibility
aliases. Source and runtime integrations should use the canonical revision-4
contract and the checked value operations above.
