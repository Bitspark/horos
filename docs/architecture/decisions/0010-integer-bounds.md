# Integers get one-sided inclusive bounds

Status: accepted, 10 November 2026. Issue #36, row 30.

## Context

horos bounds atoms (`atom-len`) and text (`text-len`) but not integers. A model
whose count must be non-negative can state that law only as a predicate private to
its library, so the law is neither portable nor machine-readable from the
contract. The concrete case is Content's `check()` count. The coordinator decided
on 10 November 2026 that horos adds integer bounds.

## Decision

1. **Two one-sided constructors**, `int-min(int(lo))` and `int-max(int(hi))`.
   `int-min` accepts a canonical `int` `v` with `lo <= v`, and `int-max` one with
   `v <= hi`. A closed range is their intersection:
   `and(int-min(int(0)), int-max(int(9)))`. The motivating law is half-open,
   `int-min(int(0))`, and needs nothing else.
2. **Not a pair with a placeholder.** A pair `int-range(lo, hi)` would need a
   marker for a missing side. Using `null` there would make an absent bound and a
   null value mean the same thing, which horos keeps distinct everywhere else,
   for example for absent record fields. A pair without a marker cannot express
   the half-open case at all. The one-sided pair also needs no `lo <= hi` rule:
   an empty intersection is well formed and matches nothing, like the empty union.
3. **Inclusive, as `atom-len` and `text-len` are.** For integers an exclusive
   bound is the inclusive bound one step over, so a second form would add nothing.
4. **Each first recognizes a canonical `int`**, exactly as `scalar(int)` does,
   delegated to ontos/data. Any other value, including a non-canonical integer
   encoding, a decimal or text, is refused. The bounds never coerce.
5. **Values keep arbitrary width.** The comparison is exact for every canonical
   integer. A value outside the signed 128-bit range lies beyond every
   well-formed bound, so it is decided by its sign alone.
6. **Bound well-formedness mirrors `atom-len`.** The single argument must be a
   canonical `int` in the signed 128-bit range that the profile already reports
   for `atom-len` bounds. That keeps bounds host-sized in every core and keeps one
   bound range in the profile. Unlike `atom-len`, a negative bound is accepted,
   because these bounds are values, not lengths. Any other argument, or any other
   arity, is malformed.
7. **Diagnostics follow ADR 0009.** A refused value reports `refused` at the
   bound's position. A malformed bound reports `argument` with `arg` 1, and a
   wrong arity reports `arity`.
8. **Evidence.** The original oracle is pinned byte for byte, so the new
   judgments go in a second hand-authored oracle, `vectors/integer-bounds.json`,
   in the same format. It is written from this record in
   `vectors/integer-bounds.py`, which only encodes the inputs. Go, Rust,
   TypeScript and Python replay it, and so does the CLI harness. Its values include
   integers beyond 128 bits on both sides. The diagnostic corpus gains the
   locations.

## Consequences

Older checkers report `int-min` and `int-max` as malformed descriptors, which is
honest: they cannot check them. The declaration layer's standard `horos.kinds`
module and its descriptor schema do not yet list the new constructors. Adding
them changes the standard module's encoded asset, which Bittype's encoder
prepares, so it is a separate step. Reasoning about bounds in the kind engine and
Bittype's native mappings are also separate work.
