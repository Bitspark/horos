# Built-artifact conformance

`bash scripts/check.sh conformance` builds the Go, Rust and TypeScript CLIs and
runs them beside the Python lane's `core/py/horos-conform.py`: four
implementations behind one command-line contract (`wf`, `conforms`,
`diagnose-wf`, `diagnose`). It replays the original oracle, the integer-bounds
oracle, the ref-scoping oracle (booleans and diagnoses) and the
diagnostic-location corpus through each, then compares judgments over 2,000
seeded descriptor/value pairs. For each pair the four diagnoses must be
identical, location included, and a completed diagnosis must agree with the
boolean. The generator chooses input; it does not author expected results. Every
disagreement of a run is printed with its reproducing input hex before the run
fails. The Python CLI needs `ONTOS_PY_ROOT`, as the Python lane does; it is never
skipped.

`python3 conformance/run.py --seed 123 --count 2000` repeats a sample after the
binaries are built; `--jobs` sets how many judgments are checked at once (the
number of processors by default). The nightly workflow adds a changing, recorded
seed. Larger counts and seed ranges belong to the nightly or manual runs; the
check stays at 2,000 pairs so it fits in CI time. Sustained runs are recorded in
[seeded-runs.md](seeded-runs.md). No credentials, private dependencies or
silently skipped lanes are used.

## What the generator emits

`horos-fuzz` (in `core/rs/src/bin`) draws half of its pairs from a scoping
family, a quarter from targeted boundaries, and a quarter from random
descriptors with random values or values derived from them. The scoping family
exists because the defect behind
[ADR 0011](../docs/architecture/decisions/0011-lexical-ref-scope.md) survived
seeded runs: every recursive descriptor bound one name, so no `ref` ever met a
shadowing `rec`. Its descriptors bind a pool of three names, so `rec`s rebind
names in scope. Its capture skeletons put a free `ref` inside a body that is
unfolded under an inner `rec` of the same name, in the reproducer, loop and
three-deep shapes. Its values are derived from the descriptor, so the unfoldings
are actually taken, often to a conforming end. They are derived either lexically
or with the old dynamic reading, which yields the values on which the two
readings disagree.

After the pairs the generator prints one line on stderr, which `run.py` shows:
the seed's share of descriptors that shadow a name, and of descriptors that are
capture-prone. A descriptor is capture-prone when a `ref` unfolds a body that has
a free `ref` to a name rebound by a `rec` between the two. That is the only shape
in which the lexical and dynamic readings can differ. About a third of each
seed's descriptors shadow, and about 30 percent are capture-prone. Neither
figure is a judgment.

The same target runs `conformance/ground.py` (issue #36). The Go declarations
test `TestGroundDescriptorEvidence` materialises ground descriptors through
`Program.GroundDescriptor` (records with optional fields, unions of exact
literals, recursion, `atom()` bytes and `scalar(int)`) and asserts the Go
checker against judgments authored from the specification, including a value
that exhausts the depth limit and is therefore `exhausted`, not `mismatch`. The
TypeScript `conforms` and `diagnose` must reach the same judgments twice: from
the package packed from `core/ts`, and from the published npm release
(`@bitspark/horos@0.9.0`) that consumers use. Each is installed in a fresh
consumer project.
