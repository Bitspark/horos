# Contributing

The owner's 4 November delivery rule applies: no backups, checksum/validation
or other check workflows, or provenance workflows as implicit task prerequisites.
Deletion means deletion within the authorized scope. Do not claim unperformed
checks passed. The library's semantic checking remains product behavior.

Use a task branch and a pull request. Keep descriptor meaning, implementation
behavior and conformance evidence aligned. Author expected judgments from the
specification; never regenerate expectations from the checker under test.

Existing targets in `scripts/check.sh` include the three native
packages, Python replay, package-consumer checks, the authored oracle and seeded
cross-language conformance. Toolchains are Go 1.25, Rust 1.96.0, Node 24 and Python
3.13. CI runs these on GitHub-hosted runners with public dependencies.

Source generation belongs to bittype. Behavioral contracts remain library-owned.
Do not add their names, adapters or protocols to the checker. A descriptor change
requires a concrete type-model case and corresponding cross-language evidence.

The [canonical declaration format](docs/spec/declarations.md) is implemented by
the Go declarations tooling package. Keep module, binder, general-kind,
application, membership and specialization rules in horos, with syntax/AST
projections and native generators in bittype. Ground-descriptor APIs do not accept open
templates. Keep source capabilities separate from published versions and other
language implementations.

The [unified declaration decision](docs/spec/unified-declarations.md), selected
by the owner on 4 November 2026, replaces the separate Horos/Wire profile.
Use one revision-4 module grammar with ordinary kind definitions and explicit
kind contracts. Constructor inputs and results may themselves be constructors;
each application remains fully saturated. Data is an ordinary recursive kind,
defined with the same structural operators as user-defined categories. Preserve
original input admission while deriving requested result guarantees from known
declaration bodies. Ground
WF/Conforms and `any` remain data-only; capabilities are static expressions until
a separate library/runtime integration gives them behavior. See the
[Go API](core/go/declarations/README.md) and [data/hydration boundary](docs/spec/hydrated-types.md).
Remove superseded profiles, exports and readers without aliases. Preserve opaque
literals, canonical pins, lexical binding and diagnostic subjects. Bittype owns
projections, not a parallel type algebra. Hydration and library protocols stay
with their existing owners.

Keep exact closed membership separate from inclusion over the free observation
universe and deliberately incomplete higher-order reasoning. Unknown implications
are Unproved, not false membership claims. Preserve ignored-argument admission,
literal opacity, lexical recursion, same-Program sealed values and aggregate
work/output limits. Never add a Data-specific summary or a kind-name switch.

Release versions and tags are immutable. Use the configured publishing accounts
and registries within the authorized release scope and current delivery rule.
Contributions use Apache-2.0; retain existing copyright and license notices.

The owner requires a clean dependency cut: consumers upgrade together. Do not
add duplicate Ontos models, old-version fallbacks, compatibility exports or
structural coercion to accept values from a different installed constructor family.
The release consumer checks install public Ontos 0.16.0 and Bitwire 0.7.0 alongside
Horos and check that ground values, codecs and diagnostics share one value model.
