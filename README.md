# horos

**Types and schemas over [ontos](https://github.com/Bitspark/ontos) values.**

A descriptor is an ontos value that describes a set of ontos values. horos checks
whether a descriptor is well formed and whether a value conforms to it. Go, Rust
and TypeScript implementations share one conformance corpus. A source-only Python
implementation supplies an additional validation lane.

The model covers atoms, scalar embeddings, tuples, labelled compounds, lists,
sets, maps, open or closed records, unions, intersections, complements and guarded
recursive descriptors. Required fields and optional fields have distinct meanings.
Value identity and encoding remain ontos's.

```text
record closed
  field "name" req (scalar utf8-text)
  field "age"  opt (scalar int)
```

This shorthand describes a text-keyed map with a required name and optional age.
The actual descriptor is an ontos tree, documented in the
[form contract](docs/spec/horos.md). See [examples](docs/guide/examples.md),
[diagnostics and limits](docs/architecture/decisions/0008-diagnostic-outcomes-kleene.md)
and [distribution](docs/distribution.md).

## Install

```sh
npm install @bitspark/horos@0.6.0
cargo add bitspark-horos@0.6.0
go get github.com/Bitspark/horos/core/go@v0.6.0
```

The Rust package's library name is `horos`. All three packages use public ontos
0.15.0, retaining a single ontos value type in each dependency graph. The
TypeScript package exports its checker at `@bitspark/horos` and diagnostics at
`@bitspark/horos/diagnose`.

## Scope

horos owns the data-only descriptor model and value checking. Language syntax, AST
transformations and source generation belong to bittype. Function, Stream, Cell
and other behavioral contracts belong to ordinary libraries; they are not horos
constructors. The [canonical declaration model](docs/spec/declarations.md)
specifies modules, immutable imports, indexed type parameters and applications,
using pagination. Its [Go tooling package](core/go/declarations/README.md)
implements resolution and specialization in current source. These APIs are not
in the published 0.6.0 packages; Rust and TypeScript remain ground-descriptor
implementations. Bittype owns AST projections and future native transformations.

The [data/hydration boundary](docs/spec/hydrated-types.md) keeps the existing
Ontos and Horos cores data-only. Optional `ontos/wire` and
[horos/wire](docs/spec/wire-types.md) extensions are documentation proposals for
mixed values and their type/declaration semantics. They replace the earlier
Bittype-owned contract model proposal; Bittype supplies tooling for the canonical
definitions. Existing ground APIs and declaration revision 1 are unchanged.
Hydration remains in Wire/runtime, and Function, Cell and Stream remain ordinary
library declarations with library-owned behavior. No extension is implemented or
published by this documentation.

Use diagnostics when a caller must distinguish a completed mismatch from an
exhausted check. The boolean API preserves its existing bounded behavior,
including the limitations documented in the contract.

## Develop

Run `bash scripts/check.sh <target>` for `public`, `go`, `rust`, `typescript`,
`python` or `conformance`. `all` requires every toolchain and an `ONTOS_PY_ROOT`
checkout at v0.15.0. Missing dependencies fail the check; no language is silently
skipped. [CONTRIBUTING.md](CONTRIBUTING.md) describes the workflow.

## History and license

This repository begins with fresh public history at 0.6.0. Earlier development,
releases and proof-integration APIs remain in a private repository. Historical
references written `horos-internal#N`, and issue notes retained in the original
conformance corpus, identify that earlier history rather than public issues.

Apache-2.0; see [LICENSE](LICENSE) and [NOTICE](NOTICE). The pre-public source's
MIT notice is retained in [LICENSE-MIT](LICENSE-MIT).
