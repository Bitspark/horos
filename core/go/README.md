# horos for Go

`go get github.com/Bitspark/horos/core/go@v0.11.1`

Use `WF(descriptor)`, `Conforms(value, descriptor)` and `Diagnose(value, descriptor)`
over values from `github.com/bitspark/ontos/core/go`. `CheckingProfile()` identifies
the effective checking limits. The module uses ontos v0.17.0.

See the [form contract](https://github.com/Bitspark/horos/blob/v0.11.1/docs/spec/horos.md).
The checked testdata copy of the shared oracle makes the module's own tests
available in a downloaded Go module, without a surrounding monorepo checkout.

The separate [declarations package](declarations/README.md) implements the
[revision-4 contract](../../docs/spec/declarations.md). It adds arbitrary
structural kinds, ordinary prelude definitions
and constructor contracts to the canonical declaration language. Type and Data
are names in an explicitly imported `horos.kinds` module, not special cases in
the general kind engine. Constructor aliases and constructor-valued results use
checked type values sealed to a Program.

The complete [standard kinds module](declarations/standard/horos.kinds.horos) is
authored in Horos. Bittype's existing frontend prepares its canonical Ontos asset
before packaging. The declarations package embeds and decodes that asset, then
uses ordinary module loading and kind admission. Downstream builds need no
Bittype executable. See [authoring and regeneration](declarations/standard/README.md).

This layer separates exact closed proper membership, free-shape inclusion and
sound but incomplete constructor reasoning. Its verdicts are Proven, Refuted and
Unproved; invalid requests and exhausted budgets are errors. Ground extraction
requires ordinary ground-kind membership and the existing WF judgment. None of
these declaration operations changes the ground APIs above or adds a live-value
checker.
