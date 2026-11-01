# horos for Rust

`cargo add bitspark-horos@0.6.0`

The library is named `horos`. Use `horos::wf`, `horos::conforms` and
`horos::diagnose::diagnose` over `ontos_core::Value`. The package uses the public
bitspark-ontos-core/data/codec 0.15.0 crates.

See the [form contract](https://github.com/Bitspark/horos/blob/v0.6.0/docs/spec/horos.md),
including checking-depth and recursive-value-destruction limits. The package
includes a checked copy of the conformance oracle for its tests.
