# Go declaration tooling

The package `github.com/Bitspark/horos/core/go/declarations` implements the
[revision-2 declaration model](../../../docs/spec/declarations.md) in Horos
0.9.0. It consumes Ontos 0.16.0
module values and supplies no source parser, native generator or runtime binding.

Use `Resolve(root, dependencies)` to check a root and the supplied transitive
dependency values. Resolution verifies canonical maps/lists, module pins and
names, declaration targets, parameter slots, argument arity, descriptor positions,
recursion closure, acyclic application graphs and symbolic Type/Data bounds. It resolves unused imports and
declarations too. Unreferenced pool values are bounded and fingerprinted but are
not included in the resulting closure. There is no implicit IO or registry.

`Pin(value)` computes SHA-256 over codec-v1 bytes after bounded input admission;
it does not validate module meaning. `ResolveWithLimits` and `PinWithLimits`
accept explicit `Limits`. Every failed resolution returns a nil program.

The checked `Program` exposes immutable modules, declarations and expressions.
IDs retain the module digest and declaration name; parameter IDs add the ordered
slot index. Descriptor operands expose exact tuple paths, so tooling can project
type positions without searching or interpreting data inside `exact` literals.
Application arguments retain their caller's bound targets and parameters.
Slice accessors return independent storage; Ontos values are immutable.

`Declaration.Parameters()` returns immutable `TypeParameter{Name, Bound}` slots;
`TypeBound` admits any well-formed type and `DataBound` requires data. Bodies are
checked symbolically under those bounds; applications preserve caller binding
and check each target slot before substitution. Bounds are checked on all open
declarations, not deferred until a concrete invocation.

`program.Specialize(declarationID, arguments)` takes closed type expressions,
checks bounds and produces a closed type expression. It preserves the original
open program. `program.DataDescriptor` additionally proves that the result is
data-only and returns the existing ground descriptor for WF/Conforms. A mixed
result fails with `BoundViolation`; no live value validator is added. Both
operations return nil on every failure, including exhaustion.

Use `errors.As` to inspect `*declarations.Error`. `Code` and `Subject` supply
machine-readable results; do not parse `Message`. The former ambiguous
`resolution` code is removed without an alias.

| Code | Subject | Meaning |
| --- | --- | --- |
| `missing-dependency` | `context` | A required pin is absent from the supplied pool; checking is incomplete |
| `import-mismatch` | `context` | A pinned module is available but its name contradicts the import's expected name |
| `unknown-import` | `source` or `context` | An expression names an alias absent from its defining module |
| `unknown-declaration` | `source` or `context` | An expression names a declaration absent from the verified target module |
| `invalid` | `source` or `context` | Malformed root/declaration input, or malformed dependency input, respectively |
| `bound-violation` | `source`, `context` or `request` | A generic argument, constructor operand or requested data result cannot be established as Data |
| `cycle` | `source` or `context` | A forbidden import or declaration cycle |
| `invalid-options` | `configuration` | Caller-supplied checking limits are invalid |
| `exhausted` | `request` | Valid resource limits prevented completion; source/context is not rejected |

`Specialize` also reports `invalid` or `unknown-declaration` about `request` for
a missing checked program, wrong argument count, invalid/non-ground argument, or
requested declaration outside that program. Those are invocation errors, not
conclusions about checked source.
Every failed operation retains its existing empty-output guarantee.

A located expression failure retains its defining `Module`, `Declaration` and
the tuple-index `Path` within that declaration body. A failure in imported source
has subject `context` and also identifies that defining module in `Dependency`;
the same declaration checked as the root has subject `source`. Unknown declaration
errors expose the resolved `Target` identity.

When `Declaration` is empty and `ImportAlias` is present, a located import failure
identifies the importing `Module`, required `Dependency` digest and `ExpectedName`;
a name mismatch additionally records `ActualName`. Nested import failures retain
the innermost failing import. No location is invented if input admission failed
before one was known.

The dependency pool is unlabelled. Supplying an unrelated value while omitting a
required pin is still `missing-dependency`; it does not prove a contradicted pin
assertion. A rejected supplied context does not establish that the root source is
invalid under every valid context. A missing dependency or resource exhaustion
must not become a completed negative semantic verdict.

Default limits are depth 256, 100,000 node visits/work steps, 16 MiB of canonical
encoded bytes and 256 supplied modules including the root. Depth is configurable
within 1..512; other limits must be positive. Input admission counts the entire
supplied pool, including literal values, before encoding. Expression/graph checking and symbolic bound checking each have their own
node-work budget. Symbolic declaration summaries are cached and substituted in
caller scope; unused parameters do not taint a data result. Specialization bounds argument admission
plus expansion work, then independently admits the complete expanded result
before descriptor checking. Existing Horos diagnostic limits remain in force.
Output occurrences count even when immutable subtrees share storage.

The package reuses Horos's descriptor checker for literal forms and guarded
recursion. Only validated declaration expressions in descriptor positions are
replaced with `any` for that grammar check; binding, arity and independent
argument closure have already been checked. Merely replacing arbitrary tuples
with `any` is not the algorithm. Descriptor operands are rebuilt together so a
wide record or tuple does not cause one complete tuple copy per field.

Run the maintained remote Go gate (`scripts/check.sh go` on a GitHub runner).
It includes data/mixed pagination, bound forwarding, phantom parameters, imported
Function/Cell examples, nested binding, every parameterized descriptor
position, specialization conformance, literal boundaries, invalid structures,
cycles, limits, defensive accessors and concurrent specialization under the
race detector. The existing cross-language ground conformance gate is retained.
