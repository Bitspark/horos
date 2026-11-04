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

The existing Ontos and Horos cores remain data-only. Follow the
[data/hydration boundary](docs/spec/hydrated-types.md) and optional
[horos/wire profile](docs/spec/wire-types.md), selected by the owner on
3 November 2026 to replace the Bittype-owned contract model proposal. Horos owns
that profile's canonical type/declaration and lifting semantics; Bittype
owns its tooling projections and transformations. Do not widen revision 1's
generic arguments or ground judgments, infer a profile from Wire occurrences,
or revive a superseded syntax as an alias. The optional static Go profile and lifting are implemented in `core/go/wire`;
see its [API scope](core/go/wire/README.md). Horos 0.7.0 aligns every supported
consumer with Ontos 0.16.0 and includes both Go declaration profiles. Ground
semantics and historical release artifacts are unchanged. Live value checking and runtime integration remain
outside that package.
Hydration and library protocols stay with their existing owners.

Release versions and tags are immutable. Maintainers rehearse the exact candidate,
review its receipt, publish it and verify installations from the public registries.
Contributions use Apache-2.0; retain existing copyright and license notices.

The owner requires a clean dependency cut: consumers upgrade together. Do not
add duplicate Ontos models, old-version fallbacks, compatibility exports or
structural coercion to accept values from a different installed constructor family.
The release consumer checks install public Ontos 0.16.0 and Bitwire 0.7.0 alongside
Horos and check that ground values, codecs and diagnostics share one value model.
