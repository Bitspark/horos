# Distribution

## Current release: 0.11.0

Version 0.11.0 publishes npm, crates.io and the Go module at one version again.
Root `v0.11.0` and `core/go/v0.11.0` identify the same accepted source commit.
npm and crates.io go from 0.9.0 to 0.11.0. They skip 0.10.0 because the Go module
already used v0.10.0 (retracted), v0.10.1 and v0.10.2 for the Go-only declaration
deliveries ([0.10.1](releases/v0.10.1.md), [0.10.2](releases/v0.10.2.md)).
Existing tags and registry versions are never replaced. See the
[release notes](releases/v0.11.0.md).

Publication uses the hosted release workflow described below, as 0.9.0 did:
`rehearse` on the accepted main revision, the two tags on that revision, then
`publish`, which runs `verify` and finalizes the GitHub release. The npm package
already exists, so the first-publication bootstrap is unnecessary.

Version 0.6.0 starts the public history. Earlier versions belong to the preserved
private history and are not tags in this repository.

| Surface | Distribution | Dependency |
| --- | --- | --- |
| TypeScript | npmjs `@bitspark/horos@0.11.0` | ontos core, codec and data 0.17.0 |
| Rust | crates.io `bitspark-horos@0.11.0`, library `horos` | bitspark-ontos-core/codec/data 0.17.0 |
| Go | `github.com/Bitspark/horos/core/go@v0.11.0` | github.com/bitspark/ontos v0.17.0 |
| Python | Source-only validation lane | ontos v0.17.0 Python source |

Earlier releases (npm and Cargo 0.9.0, Go v0.10.2 and before) use Ontos 0.16.0.

There are no private runtime dependencies, hidden registry routes or compatibility
exports for the pre-public proof/witness APIs. Code-generation tools do not form
part of the runtime dependency graph.

The release workflow has `rehearse`, `provenance`, `diagnose`, `publish` and `verify` modes. Rehearsal runs
without publishing. Publication uses the accepted source revision and package
artifacts, compares existing registry versions before reuse and verifies fresh
consumers. A release receipt records source, tags, artifact hashes and checks.
Registry authorization is confined to publication and its nonpublishing
authentication diagnostic; public consumer checks run without it. Previously
released versions are never overwritten.

`diagnose` uses the accepted rehearsal and the same trusted workflow/environment
identity. It compares pinned native npm clients with uploads and scripts disabled,
reporting only fixed OIDC stages, numeric statuses and presence booleans. It does
not change publisher settings, candidate archives or tags. A diagnostic pass is
not a publication or installed-consumer result; see the
[0.7.0 diagnostic contract](releases/v0.7.0.md#diagnosing-trusted-publication).

For the first npm publication, `provenance` signs the accepted archive on the
public repository's GitHub-hosted runner without uploading to npm. It requires
the same tags, passing source checks and rehearsal as publication, and the
dispatch revision must equal the accepted source. npm 12.2.1 supplies its own
standard statement and Sigstore implementation. The resulting bundle is checked
against the archive, source, workflow, repository identity and transparency log.
Its signer must be this repository's `release.yml` on `main`.

For a first publication, npm's
[libnpmpublish API](https://github.com/npm/cli/tree/latest/workspaces/libnpmpublish)
accepts the original archive buffer, its manifest and the signed bundle through
`provenanceFile`. Use the normal native npm configuration and write-MFA flow,
public access and explicit npmjs registry routing for `@bitspark`. Supply only
`provenanceFile`; automatic provenance generation must remain disabled.

The npm 12.2.1 CLI cannot combine this package's `publishConfig.provenance: true`
with `--provenance-file`. Adding `--provenance=false` also fails its argument
validation. The native publishing API avoids that option conflict while retaining
bundle verification and registry authentication. Keep the original CI artifact
and its hashes; do not repack, edit the archive, synthesize CI identity or copy
login credentials into CI.

Once the package exists and a qualified release is ready for a real upload,
configure its trusted GitHub publisher for `Bitspark/horos`, `release.yml` and
environment `release`. npm requires its first successful publication within
[two days of creation](https://docs.npmjs.com/trusted-publishers/#trusted-publisher-configuration-expiry).
Configuration readback, an authenticated dry-run and reuse of an already published
archive do not validate the publisher. The first real OIDC upload binds the
configuration to the repository's immutable identity. Preserve GitHub's immutable
subject settings; do not add a name-only subject or reusable-token workaround.

If that initial window expires, renew only the expired configuration with the
same repository, workflow, environment and permissions, then publish the accepted
candidate promptly. Do not create a dummy version to validate access. Keep
publisher configuration, successful authentication, actual publication and fresh
installed-consumer verification distinct in the release evidence.

Resume `publish`/`verify` with the original rehearsal so all registries and
anonymous consumers are checked before the GitHub release is finalized. When
repairing access after the workflow has advanced, rerun the original failed
publication at its original revision to keep automatic npm provenance tied to
the tagged source. Never replace tags or archives or rewrite GitHub identity
variables. Later publications use the trusted publisher in the hosted workflow;
the first-publication bootstrap is unnecessary.

The Go module includes the unified revision-4 `declarations` package, since 0.10.1.
Ordinary structural kinds, constructor contracts and checked symbolic bodies
express generic contracts in one language. Earlier revisions are no longer
accepted; imports must be repinned after canonical module migration. The removed
`wire` package and template-lifting facility remain absent.
No corresponding Rust or TypeScript declaration APIs are claimed. All consumers
must align to Ontos 0.17.0; no compatibility shim, local replacement or duplicate
value implementation bridges older versions. In TypeScript, a second installed
Ontos core means different Atom/Tuple constructors even if its bytes are unchanged.

The installed-consumer gate explicitly installs Ontos core/codec/data/wire 0.17.0
with Horos, checks a single constructor family, replays ground conformance through
the Ontos codec, and checks the static ground/mixed boundary against Ontos's native
mixed model (`NativeValue` from `@bitspark/ontos-wire/native`).

Its Bitwire qualification is suspended. Bitwire 0.7.0, the latest release on
11 November 2026, pins Ontos 0.16.0 exactly, so installing it would add a second
Ontos constructor family, which the gate rejects. The gate does not override
Bitwire's dependencies: that would qualify a combination Bitwire never released.
Every run prints the suspension and the verification receipt records it. When
Bitwire publishes a release on Ontos 0.17.0, restore its install, its message
codec round trip and its `HydratedValue` types. Bitwire is only a qualification
dependency, not a Horos runtime dependency.
Rust and Go consumer checks use public Ontos 0.17.0. These checks establish no
live mixed-value conformance, capability authority or service deployment.
