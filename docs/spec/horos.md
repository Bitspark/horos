# The horos form contract

The v1 descriptor vocabulary classifies finite, ground ontos values. The 0.6.0
public extraction retains the existing recognizer and diagnostic semantics.
Descriptors are themselves values, represented as labelled compounds:

```text
Tuple(Atom("horos/<constructor>"), argument0, argument1, ...)
```

This document uses `C(arguments...)` as shorthand for that representation. `int`,
`text`, `list`, `map`, `set`, `bool` and `null` used as values below are the
canonical [ontos/data embeddings](https://github.com/Bitspark/ontos/blob/v0.17.0/docs/spec/ontos-data.md).
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
| `int-min(int(lo))` | Canonical `int` greater than or equal to `lo` |
| `int-max(int(hi))` | Canonical `int` less than or equal to `hi` |
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

## Integer refinements

`int-min` and `int-max` each first recognize a canonical `int`, exactly as
`scalar(int)` does, and refuse any other value. The bound is inclusive. A closed
range is `and(int-min(int(lo)), int-max(int(hi)))`; the empty intersection is
well formed and matches nothing. Values have no width bound: the comparison is
exact for every canonical integer. A bound must be a canonical integer in the
signed 128-bit range, like `atom-len` bounds, but it may be negative. See the
[integer bounds decision](../architecture/decisions/0010-integer-bounds.md).

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

ontos 0.17.0 supplies stack-safe Rust value equality and destruction. The Rust CLI
uses a larger thread stack for the checker's bounded descriptor/value traversal.
Go and TypeScript compare `exact` literals iteratively; data inside a literal is
not descriptor descent and does not consume the checker's depth budget.

The Python source is a corpus-validation implementation, not a published runtime
with the same deep-input qualification. Its host interpreter recursion limit can
be reached before the logical depth cap; callers must not rely on it for arbitrary
deep or adversarial inputs.

## Diagnostic locations

Every diagnosis other than `conforms` carries exactly one location naming where
the check failed. Locations are additive: they never change a verdict. The
[location decision](../architecture/decisions/0009-diagnostic-locations.md)
records the reasons. Its JSON form, used by the command-line tools and the
`vectors/diagnose.json` corpus, is:

```text
{"path": [step, ...], "at": [int, ...], "reason": text}
  + "field": text   for missing-field and extra-field
  + "arg": int      for argument, duplicate-field, unbound-ref and unguarded-ref
```

`path` leads from the checked value to the value at fault. Its steps are:

| Step | Selects |
| --- | --- |
| `{"kind":"item","index":i}` | Item `i` of a bare tuple; a compound's first child is item 1 |
| `{"kind":"elem","index":i}` | Element `i` of a canonical list or set |
| `{"kind":"key","index":i}` | The key of entry `i` of a canonical map |
| `{"kind":"value","index":i}` | The value of entry `i` of a canonical map |
| `{"kind":"field","name":s}` | The value of the record field named `s` |

Indices are 0-based and count canonical order. `at` is the position of the
descriptor at fault: the tuple-item indices leading from the root descriptor to
it, so `union(A, B)` has `B` at `[2]` and a record's first field type is at
`[2, 3]`. A `ref` continues at the position of its `rec` body, so `at` is always
a node of the written descriptor. `arg` is an item index within the descriptor at
`at`; `at` extended by `arg` is the faulty argument's position.

### Mismatch

The checker reports the first failure in this order. A descriptor's own checks
come before its children.

- A leaf descriptor that does not accept the value reports itself: `refused`.
  `exact` is a leaf.
- `tuple`, `seq` and `compound` report `refused` for a value that is not a bare
  tuple, has the wrong arity or carries a different label; then their children in
  item order.
- `list-of`, `set-of` and `map-of` report `refused` for a value that is not the
  canonical collection; then elements in order, and map entries in order with the
  key before the value.
- `record` reports, in order: `refused` for a value that is not a canonical map;
  `refused` at the first key that is not text, with a final `key(i)` step;
  `missing-field` for the first absent required field in descriptor order;
  `extra-field` for the first undeclared key of a closed record in canonical
  order. Then the present declared fields, in descriptor order.
- `and` reports its first mismatching conjunct.
- `not` reports itself with `complement`: its inner descriptor accepted the value.
- `rec` and `ref` are transparent.
- A `union` takes the location of the first alternative with the longest value
  path. If that path is no longer than the union's own, the union reports itself
  with `no-alternative`. The empty union reports `no-alternative`.

### Exhaustion

The location follows the first exhausted sub-outcome, in the order above, among
those that decided the verdict, down to the descriptor being entered when the
depth bound was reached. Its reason is `limit`. A union is exhausted only when
none of its alternatives conforms, and an intersection only when none mismatches.

### Malformed descriptors

`diagnoseWf` checks a descriptor's own form before its child descriptors. The own
form is checked in this order: `not-descriptor` (not a tuple headed by a UTF-8
atom beginning `horos/`), `unknown-constructor`, `arity`, then each argument in
order against the arguments before it. A failed argument reports `argument` with
its `arg`. In `atom-len(lo, hi)` and `text-len(lo, hi)` with `lo > hi`, the fault
is `hi`. A record's mode and each field form are its arguments, and a repeated
field name is `duplicate-field` at the later field. A `ref` reports
`unbound-ref` or `unguarded-ref` at its name. Child descriptors follow in item
order. Among them exhaustion takes precedence over a malformed sibling, as the
verdict rules require. When `diagnose` stops at its well-formedness gate, it
reports that check's location with an empty path.

## Boundaries

The current model supplies neither structural subtyping nor schema equivalence.
Two descriptors may classify the same values while remaining distinct ontos
values. Naming, generic templates, specialization, schema migration, native type
generation, cross-field relations and behavioral contracts are separate work.
Library names such as Function, Stream and Cell are not built-in descriptor kinds.

The [declaration model](declarations.md) is a separate layer for named modules,
interfaces, higher-kinded generic slots and applications. Go's declarations
package defines revision 3 with explicit Data proofs. It preserves the existing
descriptor constructors and adds static Wire expressions; the ground checker accepts neither modules, open
templates nor capability types. `Program.DataDescriptor` establishes the ground
boundary before handing a specialized result to these APIs.

The [data/hydration boundary](hydrated-types.md) separates one type language
from distinct data and capability value domains. Bittype supplies projections
and separately qualified generators; Wire/runtime owns hydration. Function, Stream and Cell
remain ordinary library declarations, never ground descriptor constructors.
