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
npm install @bitspark/horos@0.10.0
cargo add bitspark-horos@0.10.0
go get github.com/Bitspark/horos/core/go@v0.10.0
```

The Rust package's library name is `horos`. All three packages use public ontos
0.16.0, retaining a single ontos value type in each dependency graph. Consumers
must upgrade their Ontos dependencies together; no old-version adapter or copied
value implementation is provided. The commands name the 0.10.0 release target;
publication and installed-consumer qualification require the corresponding
[release receipt](docs/releases/v0.10.0.md).
The TypeScript package exports its checker at `@bitspark/horos` and diagnostics at
`@bitspark/horos/diagnose`.

## Scope

Horos owns ground data descriptors and conformance, plus one
[canonical declaration language](docs/spec/declarations.md) for modules, immutable
imports, kinds, interfaces and applications. Its [Go tooling](core/go/declarations/README.md)
checks higher-kinded constructor parameters and Data guarantees, and specializes
data or capability types. `Page<T>` is authored once; the same Page can be passed
as a constructor and can establish `Data -> Data`. `DataPage<D: Data> = Page<D>`
adds an explicit input restriction. Ground `any` and value checking remain
data-only. Revision 3 cleanly replaces the bound-only revision-2 grammar without
compatibility readers.

Rust and TypeScript retain their ground descriptor implementations. Bittype owns
AST/CST projections, semantic facts and separately qualified native transformations. Function,
Stream and Cell remain ordinary libraries with library-owned behavior, as shown
by the [Wire type examples](docs/spec/wire-types.md). There are no generators,
runtime bindings or live capability validators in this change. The
[data/hydration boundary](docs/spec/hydrated-types.md) preserves Ontos's value
models and the existing Wire/runtime responsibilities.

Use diagnostics when a caller must distinguish a completed mismatch from an
exhausted check. The boolean API preserves its existing bounded behavior,
including the limitations documented in the contract.

## Develop

Run `bash scripts/check.sh <target>` for `public`, `go`, `rust`, `typescript`,
`python` or `conformance`. `all` requires every toolchain and an `ONTOS_PY_ROOT`
checkout at v0.16.0. Missing dependencies fail the check; no language is silently
skipped. [CONTRIBUTING.md](CONTRIBUTING.md) describes the workflow.

## History and license

This repository begins with fresh public history at 0.6.0. Earlier development,
releases and proof-integration APIs remain in a private repository. Historical
references written `horos-internal#N`, and issue notes retained in the original
conformance corpus, identify that earlier history rather than public issues.

Apache-2.0; see [LICENSE](LICENSE) and [NOTICE](NOTICE). The pre-public source's
MIT notice is retained in [LICENSE-MIT](LICENSE-MIT).
