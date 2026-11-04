# Unified declarations, revision 2

**Owner decision, 4 November 2026. Implemented for the 0.9.0 candidate; hosted qualification and publication are tracked by release receipts.** This supersedes
the separate ground declaration revision 1 and optional Horos/Wire declaration
profile. One Horos language owns modules, expressions, parameters and applications.
There are no old-format readers, profile aliases or template-lifting paths.
Ground data descriptors and their conformance APIs retain their existing meaning.

## Canonical format

Documents remain finite ground Ontos values encoded with ontos-codec-v1.
Use A(s) for a UTF-8 atom, T(s) for embedded text, I(n) for an embedded integer,
L(...) for the canonical list and M{...} for a canonical map with text keys.
H(c,...) is Tuple(A("horos/" + c),...). D(c,...) is
Tuple(A("horos/decl/" + c),...). Imports retain expected names and exact SHA-256
pins of canonical module bytes, with the complete reachable import closure checked, including unused declared
imports. Unrelated pool values are bounded and hashed but not interpreted as modules.

```text
D("module", I(2), T(name), imports, declarations)
D("import", T(expectedName), Atom(32-byte pin))
D("type", L(D("parameter", T(name), A(bound)), ...), body)
bound = "type" | "data"
D("param", I(zeroBasedSlot))
D("apply", target, L(argument, ...))
target = D("local", T(name)) | D("imported", T(alias), T(name))
```

Bounds are explicit in canonical documents. Type accepts any admitted type;
Data requires a statically established data-only type. Readable Page<T> notation
abbreviates a Type-bound parameter; readable syntax is not an implemented parser.
Names, map uniqueness, argument order, lexical scope and literal opacity retain
the declaration rules. Unknown bounds/revisions and old wire tags are errors.
There are no defaults, inference, value parameters or higher-kinded slots.

## One expression language

Existing horos descriptors admit parameters/applications at their type positions.
H("wire", E) describes a sending capability accepting E. Tuple, compound, list-of,
closed record and union can contain any admitted types. A structural aggregate
is data when its value-bearing children are data. Wire is never data, regardless
of the message type. Scalar, atom, exact and refinement forms remain data.

Other existing constructors retain data-only operands in this revision: seq,
set-of, map-of, open records, intersections, complement and guarded recursion.
This preserves their existing laws without inventing mixed key equality, mixed
complements or recursive capability semantics. Any remains the data universe;
it is not the universe of admissible type arguments. Exact literals, labels and
other non-type operands are opaque to substitution and domain classification.

Data(G) is removed from declaration syntax: G occurs directly. This does not
remove Ontos's value-level embedding/recovery operations. Type descriptions are
data even when they describe capabilities. No live endpoint enters a document.

## Pagination and bounds

```text
Page<T> = record closed { items: list-of(T), next?: scalar(utf8-text) }
DataPage<D: Data> = Page<D>
Person = record closed { name: scalar(utf8-text) }
```

There is one Page identity and binder. DataPage is an ordinary constrained alias,
not a copied record or a value wrapper. Page<Person>, DataPage<Person> and
DataPage<Page<Person>> are data. Page<Cell<Person>> is valid when the imported
Cell definition contains capabilities; DataPage<Cell<Person>> and
DataPage<list-of(Cell<Person>)> fail. No checker recognizes the name Cell.

Check generic bodies under their declared bounds. Forward<D: Data> = DataPage<D>
is valid; Forward<T> = DataPage<T> is invalid before specialization. Resolve
application arguments in the caller's scope, check the callee's bounds, then
substitute. Follow declarations and their actual parameter use: an ignored
parameter does not make a data result contain capabilities. Classification is a
bounded, syntax-directed proof, not arbitrary subtype or inhabitation solving.

Data arguments alone do not imply a data result: Wire<Person> is a capability.
Page<D> is data because of Page's body. Generic source remains intact when a
caller separately requests specialization. A closed data specialization yields
the existing ground descriptor and can use WF/Conforms. Mixed specializations
are static type expressions; this revision adds no mixed value validator.

## Ownership and tooling

Horos owns revision 2 checking, resolution, symbolic bounds and specialization.
Bittype has one Horos AST/check/resolution/analysis/provider family. Parameter
AST nodes retain explicit bounds; provider-owned semantic facts expose them.
The agnostic engine and semantic carrier do not interpret Type, Data or Wire.
Function, Stream, Cell and unfamiliar contracts remain ordinary pinned library
declarations. No generators, runtime bindings, protocol code or builtin contract
catalogue are part of this work. Ontos, Bitwire, Bitruntime and Bitnode are unchanged.

## Migration and acceptance

1. Ground revision-1 parameters become explicit Data parameters; former mixed
   parameters become Type parameters. Rewrite maintained documents once and
   recompute dependency pins. Never silently widen existing restricted slots.
2. Convert former mixed structural expressions to unified horos expressions,
   remove Data wrappers and use the one declaration envelope. Preserve literal
   payloads, even when their bytes resemble retired syntax.
3. Consolidate pagination on Page<Type> and use DataPage where a restriction is
   intended. Delete the separate Horos/Wire public API and Bittype frontend,
   profiles, architecture allowances and superseded active documentation.
4. Qualify nested/imported pagination, generic forwarding, phantom parameters,
   symbolic Data-only operands, source ownership, bounds diagnostics, old-input
   rejection, bounded work and exact ground specialization. Preserve all ground
   cross-language conformance and native Bittype gates.
5. Publish the next lockstep breaking Horos release after the independently
   owned 0.8.0 delivery. Upgrade Bittype to the verified public dependency, without
   local replacements, fallback decoders or compatibility exports. Historical
   release/research evidence remains immutable.

Hosted gates must identify the exact committed source. Independent review and
public installed-consumer verification are required before delivery is claimed.
