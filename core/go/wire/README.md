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

## Explicit lifting

`Lift(groundProgram, roots, options)` retains selected declarations, their ordered
parameters, and application dependencies under new optional identities. The
input's entire import closure is checked before pruning. The default derived
module name is the source name plus `.wire`; callers can explicitly select
module and declaration names through `LiftOptions`. Invalid names, collisions
and mappings to absent source identities fail.

Lifting visits parameters and applications before structural constructors.
Supported structure is lifted even when already closed. A remaining closed
subtree is specialized independently and embedded as `Data(G)`. An unsupported
open constructor returns `UnsupportedLift`; the caller gets no derivation.
In particular, a closed application to an unliftable generic declaration fails
instead of silently embedding that application. `Embed` is a separate explicit
operation for that different intent.

For pagination, `Page<T>` retains its slot and produces a distinct `Page` in
the derived module. The derived slot can bind `Wire(Data(Person))`; the source
data slot still cannot. Library declarations such as Function, Stream and Cell
compose through ordinary imports, applications and these structural constructors.
The package neither recognizes their names nor implements their behavior.

`Derivation` exposes copied source pins, derived module values and pins, selected
roots, declaration and parameter mappings, and source/target expression paths.
Each mapping names the optional profile/revision. `Program(entryPin, limits)`
checks a selected derived entry against the returned module bundle. Distinct
roots may belong to independent modules. Source values and identities are never
rewritten. Repeating a lift with the same inputs/options gives the same bytes.

## Bounds and evidence

Limits cover depth, unfolded occurrences, bytes and module count. Input closure,
transformation work, closed expansion, final output and validation are separately
bounded. Provenance paths and labels consume the byte budget. Limits do not change
type membership: exhaustion is a request refusal, distinct from unsupported lift
or invalid source/context. Failed operations return no usable partial program or
derivation. Accessors copy mutable collections; immutable Ontos values can share
storage safely.

Tests cover independently authored pagination output, exact derived pins, nested
and imported binding, multiple roots, optional absence versus null, literal
opacity, no fallback, pruning with retained source provenance, closed embedding,
cycles, complete import admission, naming collisions, resource exhaustion and
concurrent reads. A test-only interpreter checks ground-instance preservation
and substitution coherence on the published lifted layouts. It is not a public
mixed-value checker or evidence of live capability authority.

Structural type well-formedness establishes neither handle authenticity nor
liveness, authorization, message behavior or protocol compliance. Those remain
the runtime composition's responsibility. No send, export, hydration, connection
or resource acquisition occurs during these static operations.
