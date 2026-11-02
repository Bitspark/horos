# Contributing

Use a task branch and a pull request. Keep descriptor meaning, implementation
behavior and conformance evidence aligned. Author expected judgments from the
specification; never regenerate expectations from the checker under test.

Run the targets in `scripts/check.sh`. The full gate includes the three native
packages, Python replay, package-consumer checks, the authored oracle and seeded
cross-language conformance. Toolchains are Go 1.25, Rust 1.96.0, Node 24 and Python
3.13. CI runs these on GitHub-hosted runners with public dependencies.

Source generation belongs to bittype. Behavioral contracts remain library-owned.
Do not add their names, adapters or protocols to the checker. A descriptor change
requires a concrete type-model case and corresponding cross-language evidence.

The [canonical declaration format](docs/spec/declarations.md) is a documentation
proposal. Keep its modules, generic binders, application and specialization rules
in horos, with syntax/AST projections and native generators in bittype. Do not
present proposed declaration values as supported by the released checker or add
an implementation while the task is limited to documentation.

Release versions and tags are immutable. Maintainers rehearse the exact candidate,
review its receipt, publish it and verify installations from the public registries.
Contributions use Apache-2.0; retain existing copyright and license notices.
