# Seeded conformance runs

This records seeded runs beyond the 2,000-pair check: which generator and cores
ran, which seeds, how many judgments, and what was found. A seed reproduces its
pairs only on the generator revision it was run with. Each run compares the Go,
Rust and TypeScript CLIs and the Python lane's `core/py/horos-conform.py` with
`conformance/run.py --seed <seed> --count <count>`, and a command that runs
longer than `--timeout` seconds (120 by default) is a finding.

## The scoping family, 12 November 2026

Task 917d64ec. The generator before this change (3ba6b65) binds one name per
recursive descriptor, so none of its descriptors is capture-prone. For seeds 1
to 3, 2,000 pairs each, the Go CLI built from 3ba6b65 judges every one of its
pairs exactly as the Go CLI built from f01d244 does: on those inputs the
checker before
[ADR 0011](../docs/architecture/decisions/0011-lexical-ref-scope.md) and the
fixed one cannot be told apart. The scoping family's generator (c676d42) on the
same seeds:

| Seed | Shadowing | Capture-prone | `conforms` differs | `diagnose` differs |
| --- | --- | --- | --- | --- |
| 1 | 661 (33.0%) | 591 (29.6%) | 18 | 65 |
| 2 | 673 (33.6%) | 592 (29.6%) | 25 | 72 |
| 3 | 666 (33.3%) | 596 (29.8%) | 26 | 72 |

"Differs" counts pairs that the two Go CLIs judge differently. Every seed of
the new generator would have exposed the defect, which had survived every
seeded run before it. On the same seeds the four current cores agree on all
6,000 judgments each.

## Sustained runs

Cores at f01d244 (main after ADR 0011) in every run below; the harness and
generator are those of this branch at the commit named.

| Date | Generator | Seeds | Pairs per seed | Judgments | Findings |
| --- | --- | --- | --- | --- | --- |
| 12 Nov 2026 | c48dca6 | 100 seeds | 2,000 | 564,000 in 94 seeds | No disagreement. 6 seeds exceeded the per-call timeout; they are tracked privately and fixed next. |
| 12 Nov 2026 | c676d42 | 2000–2099 | 2,000 | 600,000 | None. Slowest unit 2.4 s; capture-prone share 27.6–32.8% per seed. |

Judgments count each of `wf`, `conforms` and `diagnose` once per pair; each was
answered identically by all four cores.
