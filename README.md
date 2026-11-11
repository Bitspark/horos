# horos

**Types and schemas over [ontos](https://github.com/Bitspark/ontos) values.**

A descriptor is an ontos value that describes a set of ontos values. horos checks
whether a descriptor is well formed and whether a value conforms to it. Go, Rust
and TypeScript implementations share one conformance corpus. A source-only Python
implementation supplies an additional validation lane.

The model covers atoms, scalar embeddings, integer bounds, tuples, labelled
compounds, lists, sets, maps, open or closed records, unions, intersections,
complements and guarded recursive descriptors. Required fields and optional fields have distinct meanings.
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
npm install @bitspark/horos@0.9.0
cargo add bitspark-horos@0.9.0
go get github.com/Bitspark/horos/core/go@v0.10.2
```

The Rust package's library name is `horos`. The releases above use public ontos
0.16.0. The source on main requires public ontos 0.17.0 in all three packages and
the Python lane; the next release moves every package together. Each dependency
graph retains a single ontos value type. Consumers must upgrade their Ontos
dependencies together; no old-version adapter or copied value implementation is
provided. The [Go 0.10.2 release](docs/releases/v0.10.2.md)
moves standard kind definitions into Horos source; it does not publish new npm
or Cargo packages.
The TypeScript package exports its checker at `@bitspark/horos` and diagnostics at
`@bitspark/horos/diagnose`.

## Native type generation

[Bittype's Horos mapping packages](https://github.com/Bitspark/bittype/tree/main/generation/syntax/mapping/horos)
supply native representations and explicit default compositions for TypeScript,
Go, Rust and C++. Bittype owns generation; Horos owns the canonical type model.
Horos contains no generator module depending on Bittype and no runtime bindings.

## Scope

Horos owns ground data descriptors and conformance, plus one
[canonical declaration language](docs/spec/declarations.md) for modules, immutable
imports, type declarations and applications. The revision-4 contract is
implemented in the [Go declarations layer](core/go/declarations/README.md).
It replaces the closed Type/Data interface model with
[general structural kinds](docs/spec/kinds.md), finite constructor contracts,
constructor aliases and constructor-valued results.

Type and Data are ordinary definitions in the explicitly imported `horos.kinds`
module, maintained in [Horos source](core/go/declarations/standard/horos.kinds.horos).
Bittype's ordinary source encoder prepares its canonical distribution asset;
Horos decodes and loads that module through the same machinery as consumer
modules. See the [standard module guide](core/go/declarations/standard/README.md).
User-defined categories use the same node patterns, finite child
sequences, unions, intersections and positive recursion. `Page<T>` is authored
once: its body can establish `Data => Data` or guarantees involving unrelated
kinds such as Scalar and Rows. The kind engine is separate from Horos primitive
formation and ground value checking. Ground `any` remains data-only. Revision 4
has no compatibility reader or new module signature field.

Rust and TypeScript retain their ground descriptor implementations. Bittype owns
AST/CST projections, semantic facts and native transformations with explicit
supported profiles. Kind validity does not imply native support for constructor
values. Function, Stream and Cell remain ordinary libraries with library-owned
behavior, as shown by the [Wire type examples](docs/spec/wire-types.md). The
declarations layer adds no native generator, runtime binding or live capability
validator. The
[data/hydration boundary](docs/spec/hydrated-types.md) preserves Ontos's value
models and the existing Wire/runtime responsibilities.

Use diagnostics when a caller must distinguish a completed mismatch from an
exhausted check. Every non-conforming diagnosis also names where the check
failed: the path into the value, the descriptor position, and the missing or
extra record field or the malformed argument, chosen in one fixed order
([diagnostic locations](docs/spec/horos.md#diagnostic-locations)). The boolean
API preserves its existing bounded behavior, including the limitations
documented in the contract.

## Develop

Run `bash scripts/check.sh <target>` for `public`, `go`, `rust`, `typescript`,
`python` or `conformance`. `all` requires every toolchain and an `ONTOS_PY_ROOT`
checkout at v0.17.0. Missing dependencies fail the check; no language is silently
skipped. [CONTRIBUTING.md](CONTRIBUTING.md) describes the workflow.

## History and license

This repository begins with fresh public history at 0.6.0. Earlier development,
releases and proof-integration APIs remain in a private repository. Historical
references written `horos-internal#N`, and issue notes retained in the original
conformance corpus, identify that earlier history rather than public issues.

Apache-2.0; see [LICENSE](LICENSE) and [NOTICE](NOTICE). The pre-public source's
MIT notice is retained in [LICENSE-MIT](LICENSE-MIT).
