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

The release workflow has `rehearse`, `publish` and `verify` modes. Rehearsal runs
without publishing. Publication uses the accepted source revision and package
artifacts, compares existing registry versions before reuse and verifies fresh
consumers. A release receipt records source, tags, artifact hashes and checks.
Registry credentials are confined to the publishing job; public consumer checks
run without them. Previously released versions are never overwritten.
