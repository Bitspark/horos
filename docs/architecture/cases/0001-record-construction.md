# HOROS-ACX-001: record construction in consumers

## Responsibility and defect

Resource contributions and deployment bindings were constructing generic
`horos/record` and `horos/field` compounds themselves. Their helpers selected
required/optional markers and encoded names, while each contribution correctly
owned its own field meanings. The generic construction responsibility was
repeated outside Horos because the public TypeScript binding exposed checking
but no record constructor.

This is the ownership failure described by
[AC-02](https://github.com/Bitspark/platform/blob/main/docs/concepts/architectural-correctness.md#ac-02)
and [AC-11](https://github.com/Bitspark/platform/blob/main/docs/concepts/architectural-correctness.md#ac-11).
Having only one current consumer or a short helper does not transfer descriptor
syntax ownership. Domain field selection remains with each contribution.

## Owner correction

The source TypeScript API now supplies `recordForm(mode, fields)`, with explicit
`RecordField.name`, `presence` and `form`. It constructs the existing syntax,
refuses duplicate names and invalid local inputs, and leaves whole-descriptor
validity to `wf`/`diagnoseWf`. It preserves child Values and descriptor order.
The [form contract](../../spec/horos.md#typescript-construction) defines the
exact API and failure behavior.

The interface follows the required record meanings independently of the former
helpers, as [AC-09](https://github.com/Bitspark/platform/blob/main/docs/concepts/architectural-correctness.md#ac-09)
requires. It does not select domain names, encode native field values, infer
schemas, add a descriptor kind or close free recursion references. Any consumer
with materially different fields can use the same constructor unchanged.

## Qualification and limits

The TypeScript tests use independently written descriptor syntax and existing
checker judgments: required/optional/null distinctions, open/closed records,
canonical map and text-key requirements, nested records with opaque exact
payloads, lexical recursion, Unicode names, duplicate and malformed inputs,
diagnostic field order and input ownership after construction.

Local qualification passed TypeScript typechecking, lint and all 47 tests,
including installation and typechecking of the packed public package in an
isolated consumer. The required cross-language CI remains pending for the
landing revision.

The owner capability is source-only and uses the existing Ontos 0.17.0 family.
The published Horos 0.11.0 release is unchanged. This change supplies only the
TypeScript construction binding; all native checker meanings remain unchanged.
Consumer removal of local constructors is delivered by the respective resource
and deployment owners and is not established by these owner tests.
