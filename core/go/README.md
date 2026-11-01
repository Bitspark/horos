# horos for Go

`go get github.com/Bitspark/horos/core/go@v0.6.0`

Use `WF(descriptor)`, `Conforms(value, descriptor)` and `Diagnose(value, descriptor)`
over values from `github.com/bitspark/ontos/core/go`. `CheckingProfile()` identifies
the effective checking limits. The module uses Ontos v0.15.0.

See the [form contract](https://github.com/Bitspark/horos/blob/v0.6.0/docs/spec/horos.md).
The checked testdata copy of the shared oracle makes the module's own tests
available in a downloaded Go module, without a surrounding monorepo checkout.
