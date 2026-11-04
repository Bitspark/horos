# Go declaration tooling

The package `github.com/Bitspark/horos/core/go/declarations` supplies the
[revision-3 declaration model](../../../docs/spec/declarations.md). It consumes
Ontos 0.16.0 module values and provides no source parser, native generator or
runtime binding. Hosted qualification and publication of the current source are
tracked separately by release receipts.

## Resolution and identity

Use `Resolve(root, dependencies)` to check a root and the supplied transitive
dependency values. Resolution verifies canonical maps/lists, module pins and
names, interfaces, every reference and parameter, argument kinds and arity,
descriptor positions, recursion closure, acyclic declaration dependencies and
Data obligations. It checks unused imports, declarations and arguments too.
Unreferenced pool values are bounded and fingerprinted but are not included in
the resulting closure. There is no implicit IO or registry.

`Pin(value)` computes SHA-256 over codec-v1 bytes after bounded input admission;
it does not validate module meaning. `ResolveWithLimits` and `PinWithLimits`
accept explicit `Limits`. Every failed resolution returns a nil program.

A checked `Program` exposes immutable modules, declarations and expressions.
Declaration IDs retain the module digest and declaration name; parameter IDs add
the ordered slot index. Named references preserve definition scope, and
application arguments preserve caller scope. Slice accessors return independent
storage. Ontos values are immutable.

## Kinds, interfaces and expression facts

`Claim` has `TypeClaim` and `DataClaim`. Data is a refined kind of proper types
established data-only; erasing that refinement gives the proper shape kind Type.
It is not the ground value descriptor `any` or a runtime wrapper.

Construct immutable interfaces with `ProperInterface(claim)` and
`ConstructorInterface(inputs, resultClaim)`, each returning `(Interface, error)`.
Constructor inputs are nonempty; all results are proper. `Interface` exposes
`IsConstructor()`, `Inputs()`, `Result()`, `Kind()` and `Equal(other)`. Its erased
`Kind` exposes `IsConstructor()`, `Inputs()` and `Equal(other)`. Shape kinds have
no independently supplied Data annotation. Constructor inputs can themselves be
constructor interfaces, but partial application and constructor-valued results
are not admitted.

`Declaration.Parameters()` returns `TypeParameter{Name, Interface}` slots.
`Result()` returns the explicit result claim, `Interface()` its declared
admission/result interface, and `Kind()` its erased kind. The body is always a
proper type. A Type claim does not discard stronger inferred Data evidence.

The expression-form tag is `ExpressionKind`, distinct from a shape kind:

| Expression tag | Accessors and meaning |
| --- | --- |
| `Descriptor` | `Constructor()` and schema-designated `Operands()` |
| `Parameter` | `Parameter()` yields the stable binder identity |
| `Reference` | `Target()` yields the resolved declaration identity |
| `Application` | `Callee()` and ordered `Arguments()` retain checked expressions |

`Target()` applies to Reference only. Application does not hide a declaration
ID in place of its callee. `Value()` retains canonical source and `Path()` gives
the exact tuple-index path within the body. `Interface()` and `KindShape()`
expose the checked interface and erased shape. Descriptor operands also expose
exact paths; tooling must not search inside `exact` literals for apparent syntax.

`DataSummary()` exposes an immutable symbolic summary for a proper expression
or declaration body. Its `Kind()` is `SummaryTrue`, `SummaryFalse`, `SummaryNeed`,
`SummaryAnd` or `SummaryProbe`. `Parameter()` identifies a Need binder;
`Children()` returns conjunction children; `Probe()` returns the checked
Reference or Application origin, preserving its callee and argument structure.
A probe's arguments are interpreted in their retained binding environment.
These summaries are checked semantic facts, not new canonical source operands.

`DataStatus()` reports `DataProven`, `DataFailed` or `DataConditional` under the
available interfaces. Failed means the compositional Data criterion failed, not
that a non-data inhabitant was found. Conditional retains unresolved Need/Probe
evidence that may become provable with actual arguments. Resource exhaustion is
an error, not a Data status. Constructor expressions have no proper-type Data
summary: their `DataSummary()` is nil and `DataStatus()` is empty.

`program.Satisfies(declarationID, expectedInterface)` returns `(bool, error)`.
It checks original admission before evaluating the known declaration's result
under fresh rigid expected inputs. This can establish Page's `Data -> Data`
interface or Ignore's `Type -> Data` interface without another declaration.
Abstract constructor parameters use only their declared guarantees. A completed
false result is distinct from exhaustion or an invalid request.

## Typed specialization

Build invocation arguments through the checked program:

- `program.Proper(value)` returns a checked closed proper-type `Argument`.
- `program.Constructor(declarationID)` returns a checked named-constructor
  `Argument` from that program.

Both return `(Argument, error)`. Argument internals are sealed. Zero-value
Argument handles and handles from another Program are rejected, including a different Program
resolving the same canonical bytes. A matching declaration ID alone does not
establish checked ownership. There is no implicit cross-Program import.

`program.Specialize(declarationID, []Argument)` validates all original input
contracts and produces a closed proper type expression. Use nil for the argument
list of a zero-parameter declaration. `program.DataDescriptor` has the same
argument API and additionally establishes Data, returning the existing ground
descriptor for WF/Conforms. A mixed result fails with `BoundViolation`; there is
no live-value validator. Both operations return nil on every failure, including
exhaustion, and preserve the original open program.

All arguments are checked even when unused. Actual constructors retain their
identity and defining environment when passed through parameters. Proving Data
for a known application never repairs an invalid open declaration. Data
preservation also does not establish recursion guardedness: application
arguments remain independently recursion-closed, and closed output is checked
again after expansion.

## Diagnostics

Use `errors.As` to inspect `*declarations.Error`. `Code` and `Subject` supply
machine-readable outcomes; do not parse `Message`.

| Code | Subject | Meaning |
| --- | --- | --- |
| `missing-dependency` | `context` | A required pin is absent from the supplied pool; checking is incomplete |
| `import-mismatch` | `context` | A pinned module's name contradicts its import |
| `unknown-import` | `source` or `context` | An expression names an absent import alias |
| `unknown-declaration` | `source`, `context` or `request` | A reference or requested operation names a declaration outside the verified closure |
| `invalid` | `source`, `context` or `request` | Malformed source/context or an invalid invocation |
| `kind-mismatch` | `source`, `context` or `request` | A constructor/proper shape does not fit its expression position or argument slot |
| `bound-violation` | `source`, `context` or `request` | An interface or Data obligation is not established |
| `cycle` | `source` or `context` | A forbidden import or declaration-reference cycle |
| `invalid-options` | `configuration` | Caller-supplied checking limits are invalid |
| `exhausted` | `request` | Valid resource limits prevented completion; source/context is not rejected |

An expression failure retains its defining `Module`, `Declaration` and `Path`.
An application callee has path component `1`; argument i has suffix `2, i+1`.
`CalleeParameter` identifies a parameter callee where applicable, and `Expected`
retains the required interface. A failure in dependency source has subject
`context` and identifies its defining module in `Dependency`. Unknown named
references expose `Target`. No location is invented if admission failed before
one could be established.

When `Declaration` is empty and `ImportAlias` is present, an import failure
identifies the importing `Module`, required `Dependency` and `ExpectedName`.
Name mismatches also record `ActualName`. Nested failures retain the innermost
failing import. An unrelated supplied value while a pin is missing is still
`missing-dependency`; it is not evidence of a contradicted pin. Rejected context
does not establish that root source is invalid under every other context.

## Limits and qualification

Default limits are depth 256, 100,000 node visits/work steps, 16 MiB of canonical
bytes and 256 supplied modules including the root. Depth lies in 1..512; other
limits must be positive. Admission counts the full input pool, including
literals, before encoding. One semantic-work budget covers expression reading,
dependency checking, interface comparison, typed environments, symbolic
evaluation and cache growth. It is not reset per phase, declaration or query.
Generated interface and environment keys consume that operation's byte allowance
before allocation. Evaluation caches are operation-local and keyed by exact
bindings and interfaces.

Specialization bounds argument admission and expansion, then independently
admits the complete expanded output before descriptor checking. Repeated output
occurrences count even when immutable subtrees share storage. Existing Horos
checker limits remain in force. A small shared source graph may prove Data
cheaply while its expanded descriptor exceeds the output budget.

The package reuses the ground checker for literal forms and guarded recursion.
Only checked proper expressions in designated descriptor positions can be
replaced by `any` in an internal grammar skeleton. Shape, binding, arity and
independent argument closure are established for this syntax/guard pass;
interface obligations are checked separately before publishing a Program.
This is not a rule that substitutes arbitrary tuples or constructor kinds with
`any`.

The maintained hosted Go gate (`scripts/check.sh go`) must qualify higher-order
interfaces, independent eager-expansion/Data agreement, cache-order permutations,
invalid open bodies and unused arguments, imported identity, literal opacity,
cycles, recursion boundaries, sealed handles, diagnostics, limits and concurrent
immutable access. Retain the complete ground cross-language conformance gate.
The presence of tests or this contract is not a passing hosted receipt.
