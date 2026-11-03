# Distribution

Version 0.6.0 starts the public history. Earlier versions belong to the preserved
private history and are not tags in this repository.

| Surface | Distribution | Dependency |
| --- | --- | --- |
| TypeScript | npmjs `@bitspark/horos@0.6.0` | ontos core, codec and data 0.15.0 |
| Rust | crates.io `bitspark-horos@0.6.0`, library `horos` | bitspark-ontos-core/codec/data 0.15.0 |
| Go | `github.com/Bitspark/horos/core/go@v0.6.0` | github.com/bitspark/ontos v0.15.0 |
| Python | Source-only validation lane | ontos v0.15.0 Python source |

The root `v0.6.0` and nested `core/go/v0.6.0` tags identify the same commit. There
are no private runtime dependencies, hidden registry routes or compatibility
exports for the pre-public proof/witness APIs. Code-generation tools do not form
part of the runtime dependency graph.

The release workflow has `rehearse`, `provenance`, `publish` and `verify` modes. Rehearsal runs
without publishing. Publication uses the accepted source revision and package
artifacts, compares existing registry versions before reuse and verifies fresh
consumers. A release receipt records source, tags, artifact hashes and checks.
Registry credentials are confined to the publishing job; public consumer checks
run without them. Previously released versions are never overwritten.

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

Once the package exists, configure its trusted GitHub publisher for
`Bitspark/horos`, `release.yml` and environment `release`. Resume `publish`/`verify`
with the original rehearsal so all registries and anonymous consumers are checked
before the GitHub release is finalized. Later publications use the trusted
publisher in the hosted workflow; the first-publication bootstrap is unnecessary.

Current Go source additionally contains `declarations` and optional `wire`
subpackages in the same `core/go` module. They are absent from the published
0.6.0 package; tooling consumers must pin an immutable source revision until a
later release. No corresponding Rust or TypeScript declaration APIs are claimed.
