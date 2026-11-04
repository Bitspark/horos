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
the Go declarations tooling package. Keep module, binder, kind/interface,
application, Data-proof and specialization rules in horos, with syntax/AST
projections and native generators in bittype. Ground-descriptor APIs do not accept open
templates. Keep source capabilities separate from published versions and other
language implementations.

The [unified declaration decision](docs/spec/unified-declarations.md), selected
by the owner on 4 November 2026, replaces the separate Horos/Wire profile.
Use one revision-3 module grammar with explicit interfaces and result claims.
Constructor inputs may themselves be constructors; applications remain fully
saturated and produce proper types. Data is a refined kind established through
the prescribed compositional proof rules. Preserve original input admission
while deriving stronger result guarantees for known declarations. Ground
WF/Conforms and `any` remain data-only; capabilities are static expressions until
a separate library/runtime integration gives them behavior. See the
[Go API](core/go/declarations/README.md) and [data/hydration boundary](docs/spec/hydrated-types.md).
Remove superseded profiles, exports and readers without aliases. Preserve opaque
literals, canonical pins, lexical binding and diagnostic subjects. Bittype owns
projections, not a parallel type algebra. Hydration and library protocols stay
with their existing owners.

Qualify substitution and Data preservation with independently authored judgments
and a bounded eager-expansion oracle. Cover abstract versus known constructors,
nested interface polarity, invalid unused arguments, literal opacity, every
semantic reference in cycle checking, recursion closure, same-Program sealed
arguments and aggregate work/output limits. Formatting, compilation and tests
run through the maintained hosted gates; a source commit alone is not validation.

Release versions and tags are immutable. Maintainers rehearse the exact candidate,
review its receipt, publish it and verify installations from the public registries.
Contributions use Apache-2.0; retain existing copyright and license notices.

The owner requires a clean dependency cut: consumers upgrade together. Do not
add duplicate Ontos models, old-version fallbacks, compatibility exports or
structural coercion to accept values from a different installed constructor family.
The release consumer checks install public Ontos 0.16.0 and Bitwire 0.7.0 alongside
Horos and check that ground values, codecs and diagnostics share one value model.
