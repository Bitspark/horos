# Horos mappings for Bittype

This optional Go module supplies Horos-owned native type mappings for the public
[Bittype generation SPI](https://github.com/Bitspark/bittype/blob/main/docs/generation-spi.md).
It uses the [automatic composer](https://github.com/Bitspark/bittype/blob/main/docs/generation-composition.md);
Horos core and Bittype do not depend on this integration module.

```text
generation/syntax/go/              structural declaration convention and associations
generation/syntax/typescript/go/   TypeScript mappings and exact bindings
generation/syntax/golang/go/       Go mappings and native optional helper
generation/syntax/rust/go/         Rust mappings and exact bindings
generation/syntax/cpp/go/          C++ mappings and exact bindings
transforms/horos-to-typescript/go/  optional default composition
transforms/horos-to-go/go/          optional default composition
transforms/horos-to-rust/go/        optional default composition
transforms/horos-to-cpp/go/         optional default composition
integration/go/                   cross-package and native compiler qualification
```

The default compositions require explicit TreeID and Profile, plus Go Package
or C++ Namespace where applicable. They consume the exact supplied Horos
resolution/analysis evidence. They do not resolve dependencies, run hidden
analysis, produce proofs or implement runtime adapters.

The `pagination-representation/1` profile covers text, bool, list, named closed
records, optional fields, proper generic parameters and direct named applications.
It retains an open Page<T> and reuses it for Page<Person> and nested Page uses.
Proper kind constraints are checked at the source but may be erased in native
parameters. `pagination-enforcement/1` is explicitly unsupported. Neither
profile promises arbitrary native instantiations satisfy Horos contracts.

Use Config.Configure to add exact library bindings or guarded overrides to a
fresh Setup. A source selector uses a resolved module identity and declaration
name, or a Horos builtin constructor/discriminator. It is never based on an
expanded shape or a kind name. An override names the existing mapping's ID in
ExpectedID; duplicate, missing or stale overrides fail. All mappings receive
child representations under the same effective configuration. Failure is final,
with no implicit fallback.

The ordinary structural mapping publishes every requested root, introduces the
native formals once and defines each demanded body. Imported definitions are
expanded only when requested by selected rules. A use-only library mapping can
return an external native type without lowering its source body; requesting that
same declaration as a public root still requires a publication implementation.
The native header form and body representation must agree: changing the record
representation can require replacing its structural publication policy too.

[Composition tests](integration/go/composition_test.go) replace List with a
ReadonlyArray mapping through Page and add an ordinary Cell mapping whose Wire
body has no default representation. They compile the chosen native declaration,
without changes to Bittype, runtime or Wire. No Cell builtin, registration or
behavior is added. Other libraries can implement the public operations directly;
the Direct and Structural helpers are optional Horos representation policies.
Their captured association tokens establish the selected invocation relation,
not a native constraint proof or runtime conformance check.

[Pagination tests](integration/go/pagination_test.go) preserve the four-language
outputs, imported identities, source admission and explicit unsupported cases.
[Verification tests](integration/go/verification_test.go) capture the actual
generated Go AST, invoke its semantic providers explicitly and check the Page
claim and nested reuse with Bittype's public Logos integration. These test-only
claims are separate from generation. The installed-consumer fixture uses public
modules without local replacements.

CI runs this module with Go 1.27.3, TypeScript 5.9.3, Rust 1.90 and GCC 14.2. It
runs Go tests and vet, native compiler tests with BITTYPE_NATIVE=1 and an isolated
installed consumer. Core Horos's existing language/conformance gates remain
independent. No npm/Cargo package or Horos core dependency changes are required.
