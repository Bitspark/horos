# Diagnostics name where a check failed

Status: accepted, 10 November 2026. Issue #37.

## Context

`diagnose(v, T)` answers `mismatch` and nothing more. A consumer that refuses an
act, or validates a model at save, must say why: which part of the value, which
part of the descriptor, and for a record which field. Checking each conjunct of an
`and` separately explains nothing for unions or recursive descriptors.
`diagnoseWf` has the same gap for malformed descriptors.

## Decision

Every diagnosis other than `conforms` carries one **location**. The location is
additive: the verdicts, `limit`, the profile and every rule of
[ADR 0008](0008-diagnostic-outcomes-kleene.md) stay as they are, and a location
never changes which verdict is reported. The boolean recognizer is unchanged.

A location has a value path, a descriptor position and a reason, plus a field
name or an argument where the reason calls for one. It is selected by a fixed
order, so Go, Rust and TypeScript report the identical location for the same
input. The exact encoding and order are in the
[form contract](../../spec/horos.md#diagnostic-locations); this record keeps the
choices and their reasons.

1. **Value path steps** are `item(i)` for a bare tuple (tuple, seq, compound),
   `elem(i)` for a list or set element, `key(i)` and `value(i)` for the two sides
   of a map entry, and `field(name)` for a record field. Indices are 0-based and
   count canonical order, which ontos makes unique. A step never copies a key
   value, so a path is bounded by the descent depth, not by the size of keys.
   Field names are text and are given as names, as the asker needs.
2. **Descriptor positions** are ontos tuple-item paths into the descriptor as
   written: `union(A, B)` has `A` at `[1]`; a record's first field type is at
   `[2, 3]`, through the `field` form. A `ref` continues at the position of its
   `rec` body instead of appending, so a position is always a node of the written
   tree and is finite through recursion. The value path says which unfolding.
   Item indices, not argument indices, make a position usable on the raw ontos
   tuple with no horos-specific offset.
3. **The fixed order** is the descriptor's own checks before its children, and
   children in ascending value order: tuple items, collection elements, map
   entries key before value. Record children follow the descriptor's field order,
   which is how the record is written. A record's own checks run in this order:
   canonical map, text keys, missing required fields, then extra fields of a
   closed record. The own checks are decisive without descent and cheap, so
   reporting them first never hides a cheaper explanation behind a deeper one.
4. **Composition.** `and` reports its first mismatching conjunct. `not(T)` fails
   only when `T` accepted the value, so it reports itself with `complement`.
   `rec` and `ref` are transparent. A failed `union` reports the first
   alternative whose location has the longest value path, if that path goes
   deeper than the union's own value; otherwise it reports itself with
   `no-alternative`. Reporting only the union would explain nothing for a
   recursive descriptor such as a cons list, whose every level is a union. The
   alternative that descended furthest into the value is the one the value most
   nearly matched, and the longest-then-first rule keeps the choice fixed.
5. **Exhaustion** keeps ADR 0008's precedence. The location is that of the first
   exhausted sub-outcome in the same fixed order that decided the verdict, ending
   where the depth bound was reached, with reason `limit`. A path is therefore at
   most one step per counted descent and stays bounded by the profile.
6. **Well-formedness** checks a descriptor's own form first: the constructor,
   its arity, then its arguments in order, each against those before it. Its
   child descriptors follow in item order. The location names the descriptor's
   position and, when one argument is at fault, its item index. For `lo > hi`
   the fault is `hi`; for a repeated record field it is the later one. As in
   ADR 0008, an own-form fault is decisive, while among children exhaustion takes
   precedence over a malformed sibling.
7. **One location, not a list.** A complete list of failures could grow with the
   value, and through nested unions it could grow exponentially. One location in
   a fixed order is bounded, deterministic and enough to name the cause.
8. **Native shapes.** TypeScript adds `location` to the non-conforming variants
   and Go adds `Location` to `Diagnosis`, since both can carry an additional
   field without breaking a caller. Rust's `Diagnosis` is a `Copy` enum with
   fieldless variants that callers match on; giving them data would break every
   match. Rust therefore adds `diagnose_located` and `diagnose_wf_located`,
   returning the unchanged `Diagnosis` with its location, and `diagnose` keeps
   its signature. The command-line tools print the JSON form from the contract,
   which the shared corpus pins.
9. **Evidence.** `vectors/diagnose.json` is a separate, hand-authored corpus of
   expected locations, written from this record. The original oracle stays
   byte-for-byte unchanged. The Python source has no diagnostic API, so it does
   not replay the location corpus.

## Consequences

A caller can name the failing field, element or alternative without a second
check. Changing the order or encoding later changes observable output, so it
requires a new decision record and corpus revision. Equal locations across
implementations are checked on the authored corpus and on seeded inputs.
