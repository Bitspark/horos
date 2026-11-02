# Go declaration tooling

The package `github.com/Bitspark/horos/core/go/declarations` implements the
[revision-1 declaration model](../../../docs/spec/declarations.md) in current
source. It is not included in the published 0.6.0 packages. It consumes Ontos
module values and supplies no source parser, native generator or runtime binding.

Use `Resolve(root, dependencies)` to check a root and the supplied transitive
dependency values. Resolution verifies canonical maps/lists, module pins and
names, declaration targets, parameter slots, argument arity, descriptor positions,
recursion closure and acyclic application graphs. It resolves unused imports and
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

`program.Specialize(declarationID, arguments)` takes concrete ground descriptor
arguments and produces a ground Horos descriptor. It preserves the original open
program and checks the result through `DiagnoseWF`. Use the existing Horos
checker for value conformance. Every failure returns a nil descriptor, including
resource exhaustion; no partial or widened result is usable.

Errors have codes `invalid`, `resolution`, `cycle` and `exhausted`. Use
`errors.As` to inspect `*declarations.Error`; a located expression error retains
its module, declaration and tuple path. Exhaustion is not a completed semantic
rejection. Invalid limits are an input error.

Default limits are depth 256, 100,000 node visits/work steps, 16 MiB of canonical
encoded bytes and 256 supplied modules including the root. Depth is configurable
within 1..512; other limits must be positive. Input admission counts the entire
supplied pool, including literal values, before encoding. Expression/graph
checking has its own node-work budget. Specialization bounds argument admission
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
It includes pagination, nested/imported binding, every lifted descriptor
position, specialization conformance, literal boundaries, invalid structures,
cycles, limits, defensive accessors and concurrent specialization under the
race detector. The existing cross-language ground conformance gate is retained.
