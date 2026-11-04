# Implementations

Go, Rust and TypeScript implement the same descriptor checker and diagnostic
rules. Each replays the original [conformance corpus](../vectors/conformance.json).
The source-only Python implementation supplies an additional oracle replay.

The public package surfaces are listed in [distribution](../docs/distribution.md).
Go additionally implements the unified [declaration model](go/declarations/README.md)
with explicit Type/Data bounds, static Wire expressions and specialization. These declaration APIs are not currently
implemented in Rust, TypeScript or Python; the cross-language descriptor corpus
continues to qualify the unchanged ground checker.
The CLI accepts `wf <descriptor-hex>` or `conforms <descriptor-hex> <value-hex>`;
arguments are ontos-codec-v1 bytes in hexadecimal. It prints exactly `true` or
`false`, or fails with a nonzero exit status for malformed CLI input.
