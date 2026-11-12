# Checking work is bounded by the size of the input

Status: accepted, 12 November 2026. Found by seeded conformance (task 917d64ec);
decided by the horos coordinator on 12 November 2026.

## Context

The recognizers and the diagnostic APIs bound how deep a check may go (512 frames,
[ADR 0008](0008-diagnostic-outcomes-kleene.md)), but not how much work it may do. A
union whose alternatives overlap checks the same value against the same descriptor
node along more than one route, and nested unions multiply the routes. The class is
**checking work not bounded by input size**. Two shapes produce it:

- **Overlapping alternatives.** `rec(L, union(list-of(ref L), list-of(ref L)))`
  against `n` nested lists that fail at the bottom checks the innermost list along
  `2^n` routes. The Go checker took 0.05 s at depth 16, 1.1 s at depth 20 and 4.6 s
  at depth 22, about four times as long for every two levels, before and after
  [ADR 0011](0011-lexical-ref-scope.md) alike; the other implementations recurse the
  same way. A three-node descriptor and a value of a few dozen bytes could keep any
  implementation busy for hours: a denial of service for anyone who checks untrusted
  values.
- **Shared subvalues.** A value held in memory with one subvalue shared between
  several places is a tree of exponentially many occurrences; checking every
  occurrence repeats the same work.

Seeded conformance found the first shape through capture-shaped descriptors. Six
pairs of the scoping family (seeds 1009, 1020, 1036, 1037, 1049 and 1094 at
generator c48dca6) each took minutes in one implementation or another. Seed 1037's
pair took 3 min 23 s in Rust. The checker before ADR 0011 answered that pair at once,
because its name-keyed guard pruned the second route, wrongly.

## Decision

1. **Every composite sub-check is computed once per check.** The result of checking
   one value against one descriptor node at one depth is kept for the rest of the
   check, and every later route to the same key reuses it. Leaves (`any`, `atom`,
   `exact` and the other constructors that never recurse) are not kept: they do no
   descent.
2. **The key is (value, descriptor position, depth), and it is sound.** Under
   lexical scope (ADR 0011) a descriptor node's position in the written descriptor
   fixes its environment: the `rec`s around that position, whose bodies and closures
   are fixed by their positions in turn. A sub-check's verdict is therefore a function
   of the value, the position and the depth alone. The depth is part of the key, so
   exhaustion is exact: the same pair reached deeper is a different key and may
   exhaust where the shallower one completed. Before ADR 0011 this would not have been
   sound, because the environment depended on the route that unfolded a `ref`.
3. **Diagnoses keep the whole outcome, location included.** A location is a function
   of the value occurrence, whose path from the checked value is fixed, and of the
   descriptor position. The diagnostic APIs therefore key on the occurrence (its
   path), never on a value shared between several places.
4. **How each implementation names a value and a position.** All four bound the same
   key; they differ only in how they name its parts.
   - Rust values own their children and cannot share them, so the address of a node
     borrowed from the caller's value or descriptor names both the occurrence and the
     position. Collections and records are walked borrowed, never copied, so no
     temporary can reuse an address.
   - Go's ontos values expose no identity. Go numbers each occurrence by its parent's
     number and its item index, and each position the same way. Its diagnosis keeps
     one canonical path node and position node per step.
   - TypeScript and Python name a value by the object itself, so a subvalue shared
     between places is checked once by the boolean; they number positions from the
     root. Their diagnoses key on the path, as above. Python keeps the memo in the one
     interpreter frame per level that it used before, so the depth its host can reach
     is unchanged.
5. **The cost bound.** A check computes at most one result per key, so it does work
   proportional to (value occurrences) × (descriptor positions) × 513 × (the largest
   arity), plus the comparison of `exact` literals. That is polynomial in the size of
   the input; the depth bound is a constant factor. The memo holds at most one entry
   per key. A decoded value is a tree in every implementation, so for untrusted input
   every core is bounded by the same measure; TypeScript and Python additionally
   collapse shared subvalues in memory.
6. **No work budget.** A default budget on frames or time would turn large valid
   values into `false` or `exhausted`, changing the boolean's documented bounded
   behaviour and the meaning of `exhausted`. Polynomial cost does not need one.
   Callers that accept untrusted values bound their size before checking, for example
   with ontos's `measure`. A budget is reconsidered only if polynomial cost proves
   exploitable.
7. **Everything else stays.** Every verdict and every location, the depth bound and
   its accounting, the profile and the APIs are unchanged.

## Evidence

The authored corpora replay unchanged in every implementation. On seeds 1 to 3 of
the scoping generator (c676d42), 2,000 pairs each, the memoized Go checker gives the
same verdict and the same diagnosis, location included, as the checker before this
change on every pair. The overlapping family at depth 64, with a failing and a
conforming innermost value, and the six seed pairs finish within milliseconds in
every implementation, with the verdicts the checker before this change gave where it
finished.

## Consequences

A check allocates memo entries in proportion to the work it does. The fix ships in
0.11.1 of every package, whose release notes name it.
