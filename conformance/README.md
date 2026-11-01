# Built-artifact conformance

`bash scripts/check.sh conformance` builds the Go, Rust and TypeScript CLIs,
replays the original oracle through each, then compares judgments over 2,000
seeded descriptor/value pairs. The generator chooses input; it does not author
expected results. A disagreement includes the reproducing input hex.

`python3 conformance/run.py --seed 123 --count 2000` repeats a sample after the
binaries are built. The nightly workflow adds a changing, recorded seed.
No credentials, private dependencies or silently skipped lanes are used.
