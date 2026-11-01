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
