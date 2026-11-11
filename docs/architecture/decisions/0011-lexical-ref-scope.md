# A ref denotes the rec that lexically encloses it

Status: accepted, 11 November 2026. Found while giving the Python lane diagnostics;
decided by the horos coordinator on 11 November 2026.

## Context

The form contract says a `ref` unfolds "the nearest matching enclosing recursion
binding" and that "inner bindings shadow matching outer names". Well-formedness reads
that lexically: its guardedness rule matches each `ref` with the `rec` that encloses it
in the written descriptor.

The recognizers and the diagnostic APIs did not. Every implementation (Go, Rust,
TypeScript and Python) kept an environment of `rec` names and bodies, but a body did
not carry the environment it was written in. Unfolding a `ref` checked its `rec`'s body
in the environment of the place where the unfolding happened. The class of defect is
**environment-based unfolding without closures**: a free `ref` inside the unfolded body
is resolved again at the unfolding site, and an inner `rec` that rebinds its name there
captures it. That is dynamic scoping.

```text
T = rec(M, union(exact(a), rec(L, tuple(ref(M), rec(M, union(exact(b), tuple(ref(L))))))))
v = (a, ((a, b),))
```

In `L`'s body, `ref(M)` is written under the outer `M`, so `L = (outer M, inner M)`, and
`v` conforms: `(a, b)` is an `L` because `a` is an outer `M`. Every checker answered
`wf = true, conforms = false`. It unfolded `ref(L)` inside the inner `M`, so `L`'s
`ref(M)` became the inner `M`, which does not accept `a`. Well-formedness and
conformance therefore disagreed about which `rec` a `ref` denotes, and the guardedness
that well-formedness proved did not cover the `rec`/`ref` pairs conformance used.

The guard against unguarded unfolding had the same flaw. It refused a `ref` whose
*name* had been bound or unfolded at the current value without a descent since. That
identifies bindings by name, which is only sound while one name means one binding.

## Decision

1. **Lexical scope is the contract.** A `ref` denotes the nearest `rec` of its name
   among those that enclose it in the descriptor as written. Unfolding it checks the
   value against that `rec`'s body with the bindings in scope at that `rec`: the ones
   enclosing it, and the `rec` itself. Every `ref` inside the unfolded body keeps the
   binding it was written under, whatever `rec`s enclose the place where the unfolding
   happens. Renaming a binding together with its references, so that every `ref`
   still denotes the same `rec`, changes no judgment. This
   is the standard, capture-avoiding reading of recursive types, and it is the reading
   well-formedness already used.
2. **Each binding is a closure.** A `rec` creates one binding that records its name,
   its body and the environment it was written in. A `ref` looks up its binding,
   innermost first, and continues in that binding's environment instead of the current
   one. The environment then always holds exactly the `rec`s that enclose the
   descriptor node being checked. So a binding's environment is the binding's own node
   of a cons list (Go, Rust and TypeScript recognizers, Python, Rust diagnostics), or
   the prefix of the environment that ends with it (Go and TypeScript diagnostics).
   Restoring it needs no search beyond the lookup the `ref` already makes.
3. **The unguarded-unfolding guard is removed.** The recognizers refused a `ref` whose
   name had been bound or unfolded at the current value without a descent since, and
   each diagnostic API mirrored that. Keyed by name, the guard is wrong under lexical
   lookup: it refuses an outer binding whenever an inner `rec` of the same name has been
   entered at the same value, as in the corpus case `dynamic-scope-would-loop`. Keyed by
   binding, it never refuses on a well-formed descriptor. Between two descents, each
   unfolding continues in the body of a `rec` that strictly encloses every descriptor
   node checked since the last descent, because well-formedness requires a descent
   between each `rec` and every `ref` that denotes it. So no binding is unfolded twice,
   and no binding entered since the last descent is unfolded, before the value descends.
   Every caller checks well-formedness first, so the guard and the set of pending
   bindings it consulted are removed from every recognizer and diagnostic API.
4. **The depth bound is the termination backstop.** Every unfolding is one checking
   frame and counts against the depth bound of 512, in every implementation, as every
   other descriptor node does. Should the argument in decision 3 ever have a gap, a
   recurring unfolding runs into that bound: the worst case is an `exhausted` diagnosis,
   and the boolean's documented bounded `false`, never a hang. The corpus case
   `dynamic-scope-would-loop` pins this. A dynamic reading cycles on it without
   descending, but the lexical one completes 57 levels and reports `exhausted` at the
   bound on 58.
5. **Everything else stays.** The depth bound of 512 and its accounting (one count per
   checked descriptor node, each unfolding included), the verdict rules of
   [ADR 0008](0008-diagnostic-outcomes-kleene.md), the location rules of
   [ADR 0009](0009-diagnostic-locations.md), well-formedness, the boolean API and the
   profile are unchanged. A `ref` still continues at the position of its `rec`'s body
   (ADR 0009, decision 2). That `rec` is now the one that lexically encloses it, which is
   the reading ADR 0009 assumed, one `rec` per `ref`.
6. **No compatibility path.** The old reading contradicts the contract, so no flag, mode
   or alternative entry point retains it.
7. **Evidence.** The original oracle is pinned byte for byte, so the new judgments go
   in a third hand-authored oracle, `vectors/recursion-scope.json`, in the same format,
   with each judgment's diagnosis beside its boolean. It is written from this record in
   `vectors/recursion-scope.py`, which only encodes the inputs. It covers the reported
   reproducer, a false-positive twin, a descriptor a dynamic reading loops on, shadowing
   three deep, shadowing without capture, exhaustion through shadowed recursion and a
   lexically unguarded `ref`. Go, Rust, TypeScript and Python replay its booleans, and
   Go, Rust, TypeScript and the CLI harness replay its diagnoses. Go's checker before
   this change disagrees with 12 of its 33 value judgments, booleans or diagnoses, and a
   Go build with lexical lookup but a name-keyed guard disagrees with 4.

## The verdict change

This restores the contract, but it changes answers, so it is a behaviour change.
Release notes for the next version of every package must name it.

- **Which descriptors.** Only those in which the body of a `rec` contains a `ref` to an
  outer binding of some name, and that body is unfolded inside an inner `rec` of the
  same name: a capture. A descriptor in which no `rec` rebinds a name bound by an
  enclosing `rec` answers exactly as before, because each name then has one binding in
  scope. So does shadowing in which no captured body is unfolded under the shadow, such
  as the original oracle's `rec-nested-shadow`.
- **Both directions.** A value that the old reading refused can now conform (the
  reproducer), and a value that it accepted can now be refused: `(a, ((b, b),))`
  conformed because `b` is an inner `M`, but `L` requires an outer `M`, and `b` is not
  one.
- **Diagnoses follow.** `diagnose` changes verdict wherever the boolean does. A
  mismatch's location can move to the body of the `rec` the `ref` now denotes. An input
  that mismatched early under capture can now run on to the depth bound and report
  `exhausted`.
- **No pinned case changes.** In the original oracle, `rec-nested-shadow` is the only
  descriptor that binds a name twice, and its inner binding's body refers to no outer
  binding. The diagnostic corpus and the integer-bounds oracle bind every name once.
  Every implementation replays all three unchanged.
- **The declarations layer already agreed.** The Go declarations layer binds each `ref`
  to its lexical `rec` when it builds observations, and each application argument is
  closed under its own bindings, so substitution never introduces a capture. A
  declaration body that itself contains a capture was observed lexically by the kind
  engine and checked dynamically by the ground checker. The two now agree.
