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

1. **Results are kept only where routes can repeat.** A check reaches one sub-check
   along two routes only below a `union` or `and` with two or more alternatives over
   the same value. Every other node either passes each child a different part of the
   value (tuple, collection and record children are distinct occurrences) or has one
   child (`not`, `rec`, `ref`). A check therefore enters **shared mode** in the
   alternatives of such a node, and everything below them is shared. In shared mode the
   result of checking one value against one descriptor node at one depth is kept for the
   rest of the check, and every later route to the same key reuses it. Outside shared
   mode nothing is kept and no value or position is named, so a descriptor without such
   a node, or whose multi-branch nodes have only leaf alternatives, costs what it did
   before. Leaves (`any`, `atom`, `exact` and the other constructors that never
   recurse) are never kept: they do no descent.
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
   key; they differ only in how they name its parts, and they name them only in shared
   mode. The node where shared mode begins is reached by one route, so it takes a fresh
   name; a `rec` entered before shared mode names its body when a shared `ref` first
   needs it. A position is named only for a composite sub-check, never for a leaf.
   - Rust values own their children and cannot share them, so the address of a node
     names both the occurrence and the position. Outside shared mode the boolean walks
     the copies ontos/data returns, as before. Each shared region (a multi-branch node
     reached outside shared mode, and everything below it) therefore takes a memo of its
     own, and inside it every value is walked borrowed from the region's value, which
     outlives the memo. So no freed copy can lend its address to another value while
     the memo exists. Regions lose no reuse: outside shared mode a value occurrence is
     checked along one chain of one-child nodes, so it roots at most one region, and two
     regions share no key. The diagnostic APIs walk borrowed throughout and keep one memo
     per check.
   - Go's ontos values expose no identity. Go numbers each occurrence by its parent's
     number and its item index, and each position the same way. Its diagnosis keeps
     one canonical path node and position node per step.
   - TypeScript and Python name a value by the object itself, so a subvalue shared
     between places is checked once by the boolean; they number positions from the
     root. Their diagnoses key on the path, as above. Python keeps the memo in the one
     interpreter frame per level that it used before, so the depth its host can reach
     is unchanged.
5. **The cost bound.** Outside shared mode every sub-check is reached along one route,
   so each key is computed at most once there too; in shared mode the memo ensures it.
   A check therefore does work proportional to (value occurrences) × (descriptor
   positions) × 513 × (the largest arity), plus the comparison of `exact` literals.
   That is polynomial in the size of the input; the depth bound is a constant factor.
   The memo holds at most one entry per shared key. A decoded value is a tree in every
   implementation, so for untrusted input every core is bounded by the same measure;
   TypeScript and Python additionally collapse shared subvalues in memory. A check
   that never enters shared mode allocates and hashes nothing for the bound, so
   ordinary descriptors pay nothing for it (see the evidence).
6. **No work budget.** A default budget on frames or time would turn large valid
   values into `false` or `exhausted`, changing the boolean's documented bounded
   behaviour and the meaning of `exhausted`. Polynomial cost does not need one.
   Callers that accept untrusted values bound their size before checking, for example
   with ontos's `measure`. A budget is reconsidered only if polynomial cost proves
   exploitable.
7. **Everything else stays.** Every verdict and every location, the depth bound and
   its accounting, the profile and the APIs are unchanged.

## Evidence

- **Corpora:** the authored corpora replay unchanged in every implementation. The four
  command-line tools agree on all of them and on seeded conformance.
  `vectors/checking-work.json` pins the overlapping family at depth 64, with a failing and
  a conforming innermost value; every implementation finishes it at once.
- **The six seeds:** on all 12,000 pairs of seeds 1009, 1020, 1036, 1037, 1049 and 1094,
  the four implementations agree on every `conforms` and `diagnose`, and no call takes
  more than 0.3 s. The checker before this change gave the same verdict on the six slow
  pairs where it finished.
- **Seeded agreement with the old checker:** on the 6,000 pairs of seeds 1–3 of the
  seeded generator, the Go command-line tool before and after this change gives the same
  boolean and the same diagnosis, location included, on every pair.
- **Ordinary input:** `scripts/bench.py --against <main>` alternates the two checkouts run
  by run on a list of 200,000 closed records, with a field typed as an int or as a
  three-way union of leaves. On an idle machine Go's, TypeScript's and Python's
  `conforms` and `diagnose` stayed within +3% of the checker before this change, or
  got faster. Rust's boolean was at +15% (plain) and +17% (union) while its walk outside
  shared mode freed ontos/data's copies before walking the originals. Since that walk
  went back to the copies, and the mode became a compile-time parameter, it has measured
  between -1% and +4%; its diagnosis was 8% and 14% faster. Allocation is the same for the boolean in Go and Rust; a
  diagnosis allocates 5% less in Go, 1% more in Rust and up to 8% more heap in
  TypeScript; Python's peak is unchanged. Pull request #54 has the tables.

## Consequences

A check allocates memo entries in proportion to its shared work, and nothing outside
shared mode. The fix ships in
0.11.1 of every package, whose release notes name it.
