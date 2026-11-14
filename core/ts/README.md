# horos for TypeScript

`npm install @bitspark/horos@0.11.0`

Import `wf` and `conforms` from `@bitspark/horos`, and `diagnose` and
`checkingProfile` from `@bitspark/horos/diagnose`. Values come from the public
@bitspark/ontos-core 0.17.0 package.

See the [form contract](https://github.com/Bitspark/horos/blob/v0.11.0/docs/spec/horos.md).
The package exports declarations for both entrypoints. There are no private
proof/witness entrypoints or compatibility aliases.

The source binding additionally exports `recordForm`, `RecordField` and
`RecordMode` from the root entrypoint. These additions are not in published
0.11.0. They construct named required/optional fields with an explicit open or
closed mode; child descriptors and native domain values remain the caller's
responsibility. See the [exact construction contract](../../docs/spec/horos.md#typescript-construction).
