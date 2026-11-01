# The horos form contract

The v1 descriptor vocabulary classifies finite, ground ontos values. The 0.6.0
public extraction retains the existing recognizer and diagnostic semantics.
Descriptors are themselves values, represented as labelled compounds:

```text
Tuple(Atom("horos/<constructor>"), argument0, argument1, ...)
```

This document uses `C(arguments...)` as shorthand for that representation. `int`,
`text`, `list`, `map`, `set`, `bool` and `null` used as values below are the
canonical [ontos/data embeddings](https://github.com/Bitspark/ontos/blob/v0.15.0/docs/spec/ontos-data.md).
An ordinary atom is an arbitrary finite byte string. Descriptors add no equality:
value and descriptor identity remain exact ontos structural identity.

## Judgments

`wf(T)` checks descriptor well-formedness: constructor, arity, argument forms,
field uniqueness and bound/guarded recursion references. `conforms(v,T)` first
checks `wf(T)` and then checks the value. A malformed descriptor admits no values.
These functions perform bounded structural checking; they do not infer types,
coerce values, insert defaults, resolve names, or execute user callbacks.

## Descriptors

In the following table, each constructor name has the `horos/` prefix, names and
flags marked `Atom` are raw atoms, and `T`, `K`, `V` are child descriptors.

| Descriptor arguments | Conformance |
| --- | --- |
| `any()` | Every value |
| `atom()` | Any atom |
| `atom-exact(Atom(bytes))` | Exactly those atom bytes |
| `atom-len(int(lo),int(hi))` | Atom byte length between inclusive bounds |
| `atom-in(set(Atom(a),...))` | Atom equals a member of the canonical set |
| `scalar(Atom(kind))` | Canonical `int`, `utf8-text` or `bool`, delegated to ontos/data |
| `tuple(T0,...,Tn)` | Bare tuple with exactly this arity and matching children |
| `seq(T)` | Bare tuple of any arity, all children matching T |
| `compound(Atom(label),T0,...)` | Tuple beginning with that exact atom label, followed by exactly the matching children |
| `list-of(T)` | Canonical list whose elements match T |
| `set-of(T)` | Canonical set whose elements match T |
| `map-of(K,V)` | Canonical map whose keys match K and values match V |
| `record(Atom(mode),fields...)` | A text-keyed canonical map obeying the field rules below |
| `union(T0,...)` | At least one alternative matches; empty union matches nothing |
| `and(T0,...)` | Every alternative matches; empty intersection matches everything |
| `not(T)` | Boolean complement of the inner recognizer; read the bounded-check qualification below |
| `exact(value)` | The singleton containing exactly this ontos value |
| `rec(Atom(name),T)` | Bind a recursion name in T |
| `ref(Atom(name))` | Unfold the nearest matching enclosing recursion binding, after structural descent |

Unknown constructors, incorrect arities, incorrect child forms, duplicate field
names, invalid flags and invalid recursion references are malformed descriptors.
`field` is a record member form, not an independent descriptor constructor.
An unrecognized descriptor is still an ordinary valid ontos value.

`atom-len` bounds must be nonnegative canonical integers, `lo <= hi`, each within
the signed 128-bit range. Scalar integer recognition itself has no host integer
width bound. `decimal` and `null` are not recognized scalar tags in v1. The
singleton null type is expressible as `exact(null())`; decimal recognition is
not supplied by a scalar alias.

## Records

Each field has the representation:

```text
Tuple(Atom("horos/field"), text(name), Atom("req" or "opt"), T)
```

Mode is mandatory: `open` permits additional text-keyed fields with unconstrained
values; `closed` rejects additional fields. Every required field must be present
and conform. An optional field may be absent; if present, it must conform. An
absent map entry and an entry containing null are different values. Duplicate
field names are rejected. The record always requires canonical map ordering and
text keys, including the keys of extra fields in open mode.

## Text refinements

Each first recognizes a canonical `utf8-text` value. These definitions are exact,
not aliases for platform-dependent string library behavior.

- `text-len(int(lo),int(hi))`: inclusive Unicode-code-point count bounds, with the
  same bound well-formedness rules as `atom-len`.
- `key-grammar()`: ASCII pattern `^[a-z0-9][a-z0-9._-]{0,63}$`.
- `text-non-blank()`: at least one character outside the fixed ASCII whitespace
  set space, tab, LF, CR, VT and FF. Non-ASCII whitespace is accepted.
- `lowercase-uuid()`: the lowercase hexadecimal 8-4-4-4-12 UUID spelling. This
  checks spelling, not UUID version or variant semantics.

## Guarded recursion

Every path from a `rec` to its matching `ref` must descend into a strict child of
the value being checked. Tuple, compound, collection and record value positions
provide descent; `union`, `and` and `not` do not. An unbound or unguarded reference
is malformed. Inner bindings shadow matching outer names. The values themselves
remain finite acyclic trees; recursion is in the descriptor's interpretation.

`rec`/`ref` do not provide generic type parameters, module imports, external type
references or arbitrary fixed-point subtyping.

## Bounded checking and diagnostics

The implementations cap recursive descent at 512. The boolean functions classify
an over-limit descent as false. With complement, that inner false can produce
true without establishing that the inner check completed. Therefore a boolean
result alone does not certify completion on arbitrarily deep input.

The diagnostic APIs distinguish `conforms`, `mismatch`, `not_well_formed` and
`exhausted`, and report the checking profile. Complement preserves exhaustion.
For unions and intersections, a decisive alternative takes precedence over an
exhausted sibling regardless of order. On completed inputs diagnostics agree
with the boolean recognizer; exhaustion makes no completed classification claim.
See the [diagnostic rules](../architecture/decisions/0008-diagnostic-outcomes-kleene.md).

ontos 0.15.0 supplies stack-safe Rust value equality and destruction. The Rust CLI
uses a larger thread stack for the checker's bounded descriptor/value traversal.
Go and TypeScript compare `exact` literals iteratively; data inside a literal is
not descriptor descent and does not consume the checker's depth budget.

The Python source is a corpus-validation implementation, not a published runtime
with the same deep-input qualification. Its host interpreter recursion limit can
be reached before the logical depth cap; callers must not rely on it for arbitrary
deep or adversarial inputs.

## Boundaries

The current model supplies neither structural subtyping nor schema equivalence.
Two descriptors may classify the same values while remaining distinct ontos
values. Naming, generic templates, specialization, schema migration, native type
generation, cross-field relations and behavioral contracts are separate work.
Library names such as Function, Stream and Cell are not built-in descriptor kinds.
