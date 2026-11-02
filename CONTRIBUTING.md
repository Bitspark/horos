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

The [canonical declaration format](docs/spec/declarations.md) is implemented by
the Go declarations tooling package. Keep module, binder, application and
specialization rules for data declarations in horos, with syntax/AST projections
and native generators in bittype. Ground-descriptor APIs do not accept open
templates. Keep source capabilities separate from published versions and other
language implementations.

Ontos and Horos remain data-only. Follow the [data/hydration boundary](docs/spec/hydrated-types.md):
Bittype owns the proposed Wire type and mixed declarations above Horos; existing
Wire/runtime hydration stays above it. Do not widen Horos generic arguments or
ground judgments to capabilities, or add a Horos Wire constructor.

Release versions and tags are immutable. Maintainers rehearse the exact candidate,
review its receipt, publish it and verify installations from the public registries.
Contributions use Apache-2.0; retain existing copyright and license notices.
