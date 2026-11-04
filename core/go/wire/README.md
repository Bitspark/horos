# Static horos/wire declarations in Go

Import `github.com/Bitspark/horos/core/go/wire` from the existing `core/go`
module. The package implements [revision 1](../../../docs/spec/wire-types.md)
of the optional profile. It consumes immutable ground Ontos descriptions; it
does not consume live handles or depend on bitwire, bitruntime or Deixis.

`Resolve` / `ResolveWithLimits` check the complete pinned import closure and
return an immutable `Program`. `Program.Specialize` substitutes concrete optional
expressions into generic slots. `WF` checks closed optional expressions.
`Reference` pairs the exact defining module/declaration with profile and revision;
the module's hash alone is not a semantic compatibility promise.

The optional grammar admits `Data(G)`, `Wire(E)`, tuples, compounds, lists,
closed records and unions. `Data(G)` contains an independently closed ground
descriptor: upper slots and applications never bind through it. Ground
`declarations.Resolve`, specialization and checker domains are unchanged.
Ground and optional programs have different Go handle types, while immutable
declaration facts and binder mechanics share a private implementation.

## Direct declarations and closed embedding

Author `Page<T>` directly in horos/wire when T must accept both data and
capabilities. That same Page can be applied to `Data(Person)` or `Wire(Data(Person))`.
It keeps one module/declaration identity, one ordered parameter list and the
original applications. Specialization produces a closed optional expression;
it does not replace the generic declaration or generate native source.
Library-owned Function, Stream and Cell compose through ordinary imports and
applications. This package neither recognizes their names nor supplies behavior.

`Embed(groundProgram, declarationID, arguments, limits)` specializes an existing
ground data declaration and wraps the closed descriptor in `Data`. It rechecks
the complete ground import context under the supplied limits. It creates no
optional declaration, does not widen a data parameter and rejects open or
capability arguments.

Template lifting, its derivation/naming metadata and the corresponding Bittype
transformer were removed on 4 November 2026. No alias or fallback remains.

## Bounds and evidence

Limits cover depth, unfolded occurrences, bytes and module count. Resolution,
specialization and embedding bound their inputs, expansion and results.
Exhaustion is a request refusal, distinct from invalid syntax or context; it
does not change type membership. Failed operations return no usable partial
program or descriptor. Accessors copy mutable collections; immutable Ontos
values can share storage safely.

Tests cover directly authored pagination over data and Wire arguments,
imported and caller-side binding, profile separation, literal opacity, library
composition, closed ground embedding, cycles, full import admission, resource
exhaustion and concurrent reads. Ground conformance tests qualify embedded
specializations. These are not a public mixed-value checker or evidence of
live capability authority.

Structural type well-formedness establishes neither handle authenticity nor
liveness, authorization, message behavior or protocol compliance. Those remain
the runtime composition's responsibility. No send, export, hydration, connection
or resource acquisition occurs during these static operations.
