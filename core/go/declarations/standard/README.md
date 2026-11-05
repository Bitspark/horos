# Standard kinds module

[horos.kinds.horos](horos.kinds.horos) is the authoritative source of the
`horos.kinds` module. It defines Type, Any, AtomFamily, Int, Text, Bool, Scalar,
Exact, List, Seq, Set, Not, Wire, Tuple, Compound, UnionOf, AndOf, Map, Record,
ClosedRecord, OpenRecord and Data using the ordinary Horos kind language.

The sibling `horos.kinds.ontos` is its generated canonical Ontos module encoding.
Only the `.horos` file is edited. The declarations package embeds the asset and
decodes it with the ordinary Ontos codec. Its existing module loader then binds
and admits all definitions through the general kind calculus.

## Authoring and generation

Bittype owns the readable parser and existing syntax-to-canonical conversion.
Its [horos-module command](https://github.com/Bitspark/bittype/tree/main/tools/horos-module)
accepts one ordinary module and writes its canonical encoding. It does not know
the prelude name or perform semantic resolution.

Using an already available `horos-module` executable, run from this directory:

```sh
horos-module -o horos.kinds.ontos horos.kinds.horos
```

The executable must be available before preparing a Horos revision that needs
its output. In particular, do not build it through a workspace replacement that
points at an unfinished Horos checkout missing this asset. The tool can link an
earlier Horos release, but lowering the new module does not resolve its definitions
through that release's prelude.

The source and generated asset ship together in the Go module. Downstream Horos
builds consume the packaged asset and do not run Bittype or locate source files
at runtime. Bittype can update its Horos dependency after the new Horos release
has been published. There is no fallback Go definition table or private parser.

## Loading, imports and source locations

Three relationships remain distinct:

| Relationship | Purpose |
| --- | --- |
| Selected standard module | Supplies the Data declaration used by primitive formation. |
| Explicit module import | Makes names such as h.Data available in a user module. |
| Supplied source tree | Gives source-aware tools actual syntax sites and text locations. |

The standard Data definition is an ordinary declaration selected by module
identity and scope. A local Data or an explicitly imported different version
does not replace that formation binding. A change to the selected Data body can
affect primitive admission even in programs without an explicit prelude import.

Canonical-only callers need no source tree. Tools that need original source
locations supply this maintained file through their ordinary parsing and Files
pipeline. Projection from a canonical module can produce an AST, but cannot
recover original text offsets. Bittype's narrow handling of an absent standard
module source remains appropriate for canonical-only sessions.

`PreludePin()` identifies the canonical module value through the existing
identity model. Formatting or comments do not affect identity when lowering
produces the same value; equivalent kind meaning alone does not establish equal
canonical module identity.
