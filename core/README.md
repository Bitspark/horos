# Implementations

Go, Rust and TypeScript implement the same descriptor checker and diagnostic
rules. Each replays the original [conformance corpus](../vectors/conformance.json).
The source-only Python implementation supplies an additional oracle replay.

The public package surfaces are listed in [distribution](../docs/distribution.md).
The CLI accepts `wf <descriptor-hex>` or `conforms <descriptor-hex> <value-hex>`;
arguments are ontos-codec-v1 bytes in hexadecimal. It prints exactly `true` or
`false`, or fails with a nonzero exit status for malformed CLI input.
