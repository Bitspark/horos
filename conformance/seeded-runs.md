# Seeded conformance runs

This records seeded runs beyond the 2,000-pair check: which generator and cores
ran, which seeds, how many judgments, and what was found. A seed reproduces its
pairs only on the generator revision it was run with. Each run compares the Go,
Rust and TypeScript CLIs and the Python lane's `core/py/horos-conform.py` with
`conformance/run.py --seed <seed> --count <count>`.

## The scoping family, 12 November 2026

Task 917d64ec. The generator before this change (3ba6b65) binds one name per
recursive descriptor, so none of its descriptors is capture-prone. For seeds 1
to 3, 2,000 pairs each, the Go CLI built from 3ba6b65 judges every one of its
pairs exactly as the Go CLI built from f01d244 does: on those inputs the
checker before
[ADR 0011](../docs/architecture/decisions/0011-lexical-ref-scope.md) and the
fixed one cannot be told apart. The scoping family's generator (c48dca6) on the
same seeds:

| Seed | Shadowing | Capture-prone | `conforms` differs | `diagnose` differs |
| --- | --- | --- | --- | --- |
| 1 | 693 (34.6%) | 620 (31.0%) | 23 | 75 |
| 2 | 730 (36.5%) | 663 (33.1%) | 27 | 80 |
| 3 | 647 (32.4%) | 576 (28.8%) | 24 | 72 |

"Differs" counts pairs that the two Go CLIs judge differently. Every seed of
the new generator would have exposed the defect, which had survived every
seeded run before it. On the same seeds the four current cores agree on all
6,000 judgments each.

## Sustained runs

| Date | Generator | Cores | Seeds | Pairs per seed | Judgments | Findings |
| --- | --- | --- | --- | --- | --- | --- |
| 12 Nov 2026 | c48dca6 | f01d244 | 1000–1099 | 2,000 | in progress | |
