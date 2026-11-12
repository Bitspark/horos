# Built-artifact conformance

`bash scripts/check.sh conformance` builds the Go, Rust and TypeScript CLIs,
replays the original oracle, the integer-bounds oracle, the ref-scoping oracle
(booleans and diagnoses) and the diagnostic-location corpus through each, then
compares judgments over 2,000
seeded descriptor/value pairs. For each pair the three diagnoses must be
identical, location included, and a completed diagnosis must agree with the
boolean. The generator chooses input; it does not author
expected results. A disagreement includes the reproducing input hex.

`python3 conformance/run.py --seed 123 --count 2000` repeats a sample after the
binaries are built. The nightly workflow adds a changing, recorded seed.
No credentials, private dependencies or silently skipped lanes are used.

The same target runs `conformance/ground.py` (issue #36). The Go declarations
test `TestGroundDescriptorEvidence` materialises ground descriptors through
`Program.GroundDescriptor` (records with optional fields, unions of exact
literals, recursion, `atom()` bytes and `scalar(int)`) and asserts the Go
checker against judgments authored from the specification, including a value
that exhausts the depth limit and is therefore `exhausted`, not `mismatch`. The
TypeScript `conforms` and `diagnose` must reach the same judgments twice: from
the package packed from `core/ts`, and from the published npm release
(`@bitspark/horos@0.11.0`) that consumers use. Each is installed in a fresh
consumer project.
