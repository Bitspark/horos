# Conformance corpus

`conformance.json` is the original hand-authored v1 oracle, retained byte-for-byte
at the public extraction. `conformance.sha256` pins those bytes. Every descriptor
and value uses ontos-codec-v1 hexadecimal; the booleans are expected judgments
authored independently of evaluating the checker.

Historical issue numbers in its notes refer to pre-public horos development.
They are provenance, not public issue numbers. The source definitions remain in
`core/rs/src/vectors.rs`. Do not derive expected answers from recognizer output.

The Go and Rust package-local testdata copies are mechanical copies checked
against this single authored corpus. They allow downloaded packages to run their
own tests. The Python, Go, Rust and TypeScript lanes replay the same judgments.
The CLI harness additionally checks the three built command-line implementations.
Seeded differential testing checks agreement, not independent correctness.

`diagnose.json` is the hand-authored corpus of diagnostic locations
([ADR 0009](../docs/architecture/decisions/0009-diagnostic-locations.md)). Its
expected `diagnoseWf` and `diagnose` results are written from the
[form contract](../docs/spec/horos.md#diagnostic-locations) in `diagnose.py`,
which only encodes the inputs. Run `python3 vectors/diagnose.py` with
`ONTOS_PY_ROOT` set to rewrite the corpus and its Go and Rust package copies;
the Python lane checks they match the source. Go, Rust, TypeScript and Python
replay it, and the CLI harness checks the three built command-line tools against it.

`integer-bounds.json` is the hand-authored oracle for `int-min` and `int-max`
([ADR 0010](../docs/architecture/decisions/0010-integer-bounds.md)), in the
original oracle's format, since that oracle is pinned byte for byte. Its source is
`integer-bounds.py`, which encodes the inputs only; run it as `diagnose.py` above.
All four lanes and the CLI harness replay it.

`recursion-scope.json` is the hand-authored oracle for `ref` scoping
([ADR 0011](../docs/architecture/decisions/0011-lexical-ref-scope.md)): a `ref`
denotes the `rec` that lexically encloses it. Each case carries the boolean `wf`
and `conforms` of the original oracle's format, and beside them the expected
`diagnose_wf` and `diagnosis` in the JSON form of the location corpus. Its source
is `recursion-scope.py`, which encodes the inputs only; run it as `diagnose.py`
above. All four lanes and the CLI harness replay its booleans and its
diagnoses.

`mixed-inhabitation.json` is the hand-authored corpus for
[mixed inhabitation](../docs/spec/mixed-inhabitation.md)
([ADR 0012](../docs/architecture/decisions/0012-mixed-inhabitation.md)): whether an
ontos/wire mixed value inhabits a materialized Descriptor. Horos has no checker
for it; a binding backend replays it. Descriptors are ontos-codec-v1 hex, and
values use ontos's value-tree notation with a `{"wire": name}` leaf. Its source is
`mixed-inhabitation.py`, which encodes the inputs and checks their form; run it as
`diagnose.py` above. The Go declarations tests check that every descriptor is a
materialized Descriptor and that the verdicts for descriptors without `wire` are
the ground judgment.

`checking-work.json` is the hand-authored oracle for checking work
([ADR 0013](../docs/architecture/decisions/0013-bounded-checking-work.md)):
descriptors whose alternatives overlap, against values 64 levels deep, which a
checker bounded by its input finishes at once. It has the format of
`recursion-scope.json`, and its source is `checking-work.py`. All four lanes and
the CLI harness replay its booleans and its diagnoses.
