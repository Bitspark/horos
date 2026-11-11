//! The horos descriptor checker over ontos values. See docs/spec/horos.md.

#![forbid(unsafe_code)]

#[cfg(test)]
mod diagnostic_regressions;

use std::collections::HashMap;

use ontos_core::Value;
use std::cmp::Ordering;

use ontos_data::{
    read_bool, read_int, read_int_bytes, read_list, read_map, read_set, read_text, recognize_int,
};

/// A persistent (cons-list) stack of scoped entries threaded through the recursion by
/// reference. Each `rec` frame pushes one node that **borrows** the parent chain
/// instead of cloning it (the old `env.to_vec()` / `pending.to_vec()` per frame), so a
/// descent costs O(1) and allocates nothing on the native stack the recursion already
/// uses. Lookups walk head→tail, i.e. **innermost-first**: the innermost binding of a
/// name shadows outer ones. A node of the conformance environment is also its binding's
/// closure (ADR 0011): its tail is the environment its `rec` was written in.
struct Scope<'a, T> {
    head: T,
    tail: Option<&'a Scope<'a, T>>,
}

impl<'a, T> Scope<'a, T> {
    /// Push `head` onto `tail`, returning a new node that borrows the tail. The node
    /// lives on the caller's stack frame; no heap allocation, no copy of the tail.
    fn push(head: T, tail: Option<&'a Scope<'a, T>>) -> Scope<'a, T> {
        Scope { head, tail }
    }
}

/// Innermost-first iterator over a [`Scope`] chain (and its `Option` handle).
struct ScopeIter<'s, 'a, T> {
    node: Option<&'s Scope<'a, T>>,
}

impl<'s, 'a, T> Iterator for ScopeIter<'s, 'a, T> {
    type Item = &'s T;
    fn next(&mut self) -> Option<&'s T> {
        let cur = self.node?;
        self.node = cur.tail;
        Some(&cur.head)
    }
}

/// Iterate an `Option<&Scope>` handle innermost-first (the empty chain yields nothing).
fn scope_iter<'s, 'a, T>(s: Option<&'s Scope<'a, T>>) -> ScopeIter<'s, 'a, T> {
    ScopeIter { node: s }
}

/// Whether the name chain `s` contains `name` (by content). The byte-slice analogue of
/// the reference core's `pending.contains(&nb)` over the cons-list `pending`.
fn scope_contains(s: Option<&Scope<'_, &[u8]>>, name: &[u8]) -> bool {
    scope_iter(s).any(|n| *n == name)
}

/// The innermost node of `s` whose head satisfies `hit`: the node, not only its head, so
/// a `ref` can continue with a `rec` binding's own chain as its environment (ADR 0011).
fn scope_find<'s, 'a, T>(
    mut s: Option<&'s Scope<'a, T>>,
    hit: impl Fn(&T) -> bool,
) -> Option<&'s Scope<'a, T>> {
    while let Some(node) = s {
        if hit(&node.head) {
            return Some(node);
        }
        s = node.tail;
    }
    None
}

/// The reserved descriptor-label namespace. Every descriptor combinator is a
/// `Tuple(Atom("horos/<ctor>"), args...)`; a tuple whose first child is not an atom
/// under this prefix is not a horos descriptor (per ontos-data §1.1 reservation).
pub const NAMESPACE: &str = "horos/";

/// The uniform cap on `atom-len` bounds (horos-internal#18). A well-formed `atom-len` descriptor
/// requires each of its bounds to fit in the **signed 128-bit range**
/// `[i128::MIN, i128::MAX] == [-2^127, 2^127 - 1]`; a bound whose magnitude lands
/// **outside** this range makes the descriptor **non-well-formed** (and non-conforming)
/// identically across the Rust, Go, and TS cores.
///
/// This Rust core enforces the cap *implicitly*: [`read_int`] materializes a bound into
/// an `i128` and returns `Err(IntOutOfRange)` for any value past `i128::MAX` (positive)
/// or below `i128::MIN` (negative), and every `atom-len` arm maps that `Err` to `false`.
/// The Go (`*big.Int`) and TS (`bigint`) cores read into unbounded integers, so they
/// apply the **same** cap explicitly (see `ATOM_LEN_BOUND_*` there). Byte-length bounds
/// are tiny in practice, so this cap never rejects a meaningful descriptor — its sole
/// purpose is to make atom-len byte-identity a theorem rather than a "never happens"
/// argument. The constants below document the cap; the `read_int` call is the
/// enforcement (a bound = 2^127, just past `i128::MAX`, is non-wf in all three cores).
pub const ATOM_LEN_BOUND_MAX: i128 = i128::MAX; // 2^127 - 1
/// See [`ATOM_LEN_BOUND_MAX`]: the inclusive lower end of the in-range bound window.
pub const ATOM_LEN_BOUND_MIN: i128 = i128::MIN; // -2^127

/// The shared recursion-depth guard (horos-internal#37). `conforms`/`wf` recurse on the native call
/// stack; termination is bounded by `|v|` (`wf`'s guardedness) but there is no *stack*
/// bound, so an adversarial-but-finite value (e.g. a tuple nested hundreds of thousands
/// deep) could overflow the native stack — a panic / `fatal` / `RangeError` — at a
/// core-dependent depth, breaking the "total, never-panics, decides identically" contract.
///
/// This cap restores totality: a descent past `MAX_RECURSION_DEPTH` `conforms_in`/`wf_in`
/// frames yields a **deterministic** result (`false` for `conforms`, non-wf for `wf`),
/// never a crash, at the **same logical depth** in all three cores (rs/go/ts increment a
/// depth counter at identical recursion points). Termination is thus bounded by
/// `min(|v|, MAX_RECURSION_DEPTH)`; this is a robustness guardrail, not a change to the
/// recognized language for in-bound inputs.
///
/// `512` is chosen far above any descriptor/value the spec, tests, or vectors exercise
/// (their depth is < ~30, so no existing judgment changes), and comfortably below the
/// smallest native stack budget of all three cores — validated empirically: each core's
/// deep-value test recurses to exactly this cap on its most constrained stack (Rust's
/// 2 MiB test-thread stack, Node/V8's default, Go's growable goroutine stack) and returns
/// a clean `false`/non-wf rather than overflowing. A heavier per-frame core (a debug Rust
/// build's `conforms_in` frames are ~2 KiB) sets the ceiling; 512 leaves head-room there
/// while every core trips at the same logical depth, so the decision stays byte-identical.
pub const MAX_RECURSION_DEPTH: usize = 512;

/// If `t` is `Tuple(Atom("horos/<ctor>"), args...)` with a valid-UTF-8 label under the
/// reserved namespace, return `(<ctor>, args)`. Otherwise `None` (not a horos
/// descriptor). Total — never panics.
///
/// through the SAME head reading the recognizer uses (no second descriptor parser).
pub(crate) fn head(t: &Value) -> Option<(&str, &[Value])> {
    let items = t.as_tuple()?.items();
    let (label, args) = items.split_first()?;
    let label = std::str::from_utf8(label.as_atom()?.bytes()).ok()?;
    let ctor = label.strip_prefix(NAMESPACE)?;
    Some((ctor, args))
}

/// Read the bytes of an atom value, or `None` if it is a tuple. `pub(crate)` for the horos-internal#85
/// leaf builtins (see [`head`]).
pub(crate) fn atom_bytes(v: &Value) -> Option<&[u8]> {
    v.as_atom().map(|a| a.bytes())
}

// ---------------------------------------------------------------------------
// data-encoded refinements (horos-internal#68) — the four named grammar-class predicates the
// visual-atlas consumer needs (atlas#533 Q4 / visual-atlas#534), each a closed,
// total, deterministic, single-value predicate (spec §5 admissibility). They
// measure Unicode CHARACTERS (code points), not bytes, so the multi-byte cases
// where char-count ≠ byte-count agree across the rs/go/ts cores. Implemented as
// explicit, dependency-free character scans (no regex engine) so the three cores
// decide byte-for-byte. The §5 grammar literals are ASCII, so a non-ASCII char
// never satisfies a grammar class — it is simply not one of the literal bytes.

/// `horos/key-grammar`: the decoded text matches `^[a-z0-9][a-z0-9._-]{0,63}$`.
/// Length is measured in characters; every grammar char is ASCII, so this is a
/// per-char class test over the decoded string (1–64 chars total).
fn matches_key_grammar(s: &str) -> bool {
    let mut chars = s.chars();
    let Some(first) = chars.next() else {
        return false; // empty — the leading class requires one char
    };
    if !(first.is_ascii_lowercase() || first.is_ascii_digit()) {
        return false;
    }
    for (tail, c) in chars.enumerate() {
        if tail >= 63 {
            return false; // more than 1 + 63 = 64 chars
        }
        if !(c.is_ascii_lowercase() || c.is_ascii_digit() || c == '.' || c == '_' || c == '-') {
            return false;
        }
    }
    true
}

/// `horos/text-non-blank`: the decoded text has at least one non-whitespace
/// character. Whitespace is the fixed ASCII set {space, tab, LF, CR, VT, FF}; a
/// non-ASCII char is never whitespace (so it counts as non-blank), keeping the
/// three cores identical without dragging in Unicode whitespace tables.
fn is_non_blank(s: &str) -> bool {
    s.chars().any(|c| !is_ascii_ws(c))
}

/// The fixed ASCII whitespace set used by [`is_non_blank`] — mirrored byte-for-byte
/// in the Go and TS cores. Deliberately NOT Unicode-`White_Space` (which varies by
/// language runtime and would break tri-core parity).
fn is_ascii_ws(c: char) -> bool {
    matches!(c, ' ' | '\t' | '\n' | '\r' | '\u{0B}' | '\u{0C}')
}

/// `horos/lowercase-uuid`: the decoded text matches the lowercase canonical UUID
/// grammar `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`. All 36
/// chars are ASCII, so the byte length equals the char length for a match — but we
/// scan chars so a multi-byte char in a malformed input is rejected cleanly.
fn matches_lowercase_uuid(s: &str) -> bool {
    // Group lengths between the four hyphens: 8-4-4-4-12.
    const GROUPS: [usize; 5] = [8, 4, 4, 4, 12];
    let mut chars = s.chars();
    for (gi, &len) in GROUPS.iter().enumerate() {
        for _ in 0..len {
            match chars.next() {
                Some(c) if c.is_ascii_digit() || matches!(c, 'a'..='f') => {}
                _ => return false,
            }
        }
        if gi < GROUPS.len() - 1 {
            // A hyphen separates each group from the next.
            if chars.next() != Some('-') {
                return false;
            }
        }
    }
    chars.next().is_none() // no trailing characters
}

// ===========================================================================
// conformance — `v : T`
// ===========================================================================

/// Decide `v : T` — "does the value `v` exhibit the form described by `T`?"
///
/// **Total and search-free.** A value that does not conform — and a `T` that is not a
/// well-formed descriptor — both yield `false`; this never errors and never panics. A
/// value may conform to many descriptors at once (overlapping classification, not a
/// new identity).
pub fn conforms(v: &Value, t: &Value) -> bool {
    // Soundness gate (spec §3 MUST): `v : T` presupposes `wf(T)`. A descriptor that is
    // not well-formed admits no values, even where an individual arm would otherwise
    // match — so check `wf` once at the entry and never arm-by-arm below.
    wf(t) && conforms_in(v, t, None, 0)
}

/// An in-scope `rec` binding: the name (atom bytes) and its body descriptor.
type EnvEntry<'a> = (&'a [u8], &'a Value);

/// One `rec` binding as an environment node. Its tail is the environment the `rec` was
/// written in, so the node is the binding's closure.
type EnvNode<'s, 'a> = Scope<'s, EnvEntry<'a>>;

/// `conforms` with the recursion environment threaded through:
///
/// `env` binds each `rec` name that lexically encloses `t` to its body descriptor. A `ref`
/// denotes its nearest enclosing binding in the written descriptor and continues in that
/// binding's own environment, never the unfolding site's (ADR 0011).
///
/// Termination: callers check `wf(t)` first, and `wf` requires a descent between every
/// `rec` and each `ref` that denotes it. So between two descents each unfolding reaches a
/// `rec` strictly above every descriptor node checked since the last descent, and each
/// descent strictly shrinks the finite `v`. Every unfolding is also one counted frame, so
/// `MAX_RECURSION_DEPTH` is the backstop.
fn conforms_in<'a>(v: &Value, t: &'a Value, env: Option<&EnvNode<'_, 'a>>, depth: usize) -> bool {
    // Recursion-depth guard (horos-internal#37): past MAX_RECURSION_DEPTH frames, return a
    // deterministic `false` rather than risk a native-stack overflow. Counted at the
    // same point in all three cores so the trip depth is byte-identical.
    if depth > MAX_RECURSION_DEPTH {
        return false;
    }
    let Some((ctor, args)) = head(t) else {
        return false; // not a horos descriptor
    };
    match (ctor, args) {
        // Top.
        ("any", []) => true,

        // Atoms.
        ("atom", []) => v.as_atom().is_some(),
        ("atom-exact", [b]) => match (atom_bytes(v), atom_bytes(b)) {
            (Some(vb), Some(bb)) => vb == bb,
            _ => false,
        },
        // Bounds materialize via `read_int` into `i128`; a bound outside
        // `[ATOM_LEN_BOUND_MIN, ATOM_LEN_BOUND_MAX]` (i.e. past i128) makes `read_int`
        // return `Err`, which maps to `false` here — the horos-internal#18 cap, mirrored explicitly by
        // the unbounded Go/TS cores so the three agree at 2^127.
        ("atom-len", [lo, hi]) => match (atom_bytes(v), read_int(lo), read_int(hi)) {
            (Some(vb), Ok(lo), Ok(hi)) => (lo..=hi).contains(&(vb.len() as i128)),
            _ => false,
        },
        ("atom-in", [set_v]) => match (atom_bytes(v), read_set(set_v)) {
            (Some(vb), Ok(elems)) => elems.iter().any(|e| atom_bytes(e) == Some(vb)),
            _ => false,
        },

        // Text refinements (horos-internal#68) — each first recognizes `v` as a canonical
        // utf8-text (delegating to the frozen ontos/data recognizer, re-minting
        // nothing) and then applies a closed, total, single-value character
        // predicate. Char-count (not byte-count) is measured, so multi-byte text
        // is bounded/matched identically across the three cores.
        ("text-len", [lo, hi]) => match (read_text(v), read_int(lo), read_int(hi)) {
            (Ok(s), Ok(lo), Ok(hi)) => (lo..=hi).contains(&(s.chars().count() as i128)),
            _ => false,
        },
        ("key-grammar", []) => match read_text(v) {
            Ok(s) => matches_key_grammar(&s),
            Err(_) => false,
        },
        ("text-non-blank", []) => match read_text(v) {
            Ok(s) => is_non_blank(&s),
            Err(_) => false,
        },
        ("lowercase-uuid", []) => match read_text(v) {
            Ok(s) => matches_lowercase_uuid(&s),
            Err(_) => false,
        },

        // Integer bounds (ADR 0010): a canonical int of any width, compared exactly with
        // an inclusive bound that `read_int` materializes into the i128 range.
        ("int-min", [b]) => read_int(b)
            .ok()
            .and_then(|b| compare_int(v, b))
            .is_some_and(|o| o != Ordering::Less),
        ("int-max", [b]) => read_int(b)
            .ok()
            .and_then(|b| compare_int(v, b))
            .is_some_and(|o| o != Ordering::Greater),

        // Scalars — delegate to the frozen ontos/data recognizers.
        ("scalar", [k]) => match atom_bytes(k) {
            // Structural recognition — a canonical value beyond a binding's host integer
            // width is still recognized (the materialization limit is not a recognition
            // miss, ontos-data §5.1), so all cores agree. horos re-mints nothing.
            Some(b"int") => recognize_int(v).is_ok(),
            Some(b"utf8-text") => read_text(v).is_ok(),
            Some(b"bool") => read_bool(v).is_ok(),
            _ => false,
        },

        // Tuples & compounds. Each child descriptor types a child VALUE — a descent.
        ("tuple", ts) => match v.as_tuple() {
            Some(tup) => {
                let items = tup.items();
                items.len() == ts.len()
                    && items
                        .iter()
                        .zip(ts)
                        .all(|(c, ct)| conforms_in(c, ct, env, depth + 1))
            }
            None => false,
        },
        ("seq", [te]) => match v.as_tuple() {
            Some(tup) => tup
                .items()
                .iter()
                .all(|c| conforms_in(c, te, env, depth + 1)),
            None => false,
        },
        ("compound", [label, ts @ ..]) => match (v.as_tuple(), atom_bytes(label)) {
            (Some(tup), Some(lb)) => {
                let items = tup.items();
                items.len() == ts.len() + 1
                    && atom_bytes(&items[0]) == Some(lb)
                    && items[1..]
                        .iter()
                        .zip(ts)
                        .all(|(c, ct)| conforms_in(c, ct, env, depth + 1))
            }
            _ => false,
        },

        // Structural collections — well-formedness delegates to ontos/data; the element
        // descriptors add the per-element checks (descents).
        ("list-of", [te]) => match read_list(v) {
            Ok(elems) => elems.iter().all(|e| conforms_in(e, te, env, depth + 1)),
            Err(_) => false,
        },
        ("set-of", [te]) => match read_set(v) {
            Ok(elems) => elems.iter().all(|e| conforms_in(e, te, env, depth + 1)),
            Err(_) => false,
        },
        ("map-of", [tk, tv]) => match read_map(v) {
            Ok(entries) => entries.iter().all(|(k, val)| {
                conforms_in(k, tk, env, depth + 1) && conforms_in(val, tv, env, depth + 1)
            }),
            Err(_) => false,
        },

        // Records — a well-formed map keyed by utf8-text, with required/optional fields
        // and a mandatory open/closed flag.
        ("record", [mode, fields @ ..]) => conforms_record(v, mode, fields, env, depth),

        // Composition — recurse on the SAME `v`.
        ("union", ts) => ts.iter().any(|ct| conforms_in(v, ct, env, depth + 1)),
        ("and", ts) => ts.iter().all(|ct| conforms_in(v, ct, env, depth + 1)),
        ("not", [t1]) => !conforms_in(v, t1, env, depth + 1),

        // Singleton — membership by ontos structural identity (defines no new equality).
        ("exact", [v0]) => v == v0,

        // Recursion. `rec` binds the name; `ref` unfolds the binding that lexically
        // encloses it. Each unfolding is one counted frame (ADR 0011).
        ("rec", [name, body]) => match name.as_atom() {
            Some(nm) => {
                // Push one borrowed node (no per-frame copy); the child frame sees this
                // binding innermost-first. The node closes over `env`.
                let e = Scope::push((nm.bytes(), body), env);
                conforms_in(v, body, Some(&e), depth + 1)
            }
            None => false,
        },
        ("ref", [name]) => match name.as_atom() {
            Some(nm) => {
                let nb = nm.bytes();
                // Innermost-first: an inner `rec` shadows an outer binding of the same
                // name, and `env` holds only the bindings that enclose this `ref`.
                let Some(b) = scope_find(env, |(k, _)| *k == nb) else {
                    return false; // unbound reference
                };
                // Unfold in the binding's own environment, so the body's free refs keep
                // the bindings they were written under.
                conforms_in(v, b.head.1, Some(b), depth + 1)
            }
            None => false,
        },

        // Unknown ctor or wrong arity → not a recognized descriptor.
        _ => false,
    }
}

/// A field descriptor: `Tuple(Atom("horos/field"), utf8-text(name), Atom(req|opt), T)`.
/// Returns `(name, required, field_type)` if `f` is a well-formed field, else `None`.
pub(crate) fn read_field(f: &Value) -> Option<(String, bool, &Value)> {
    let (ctor, args) = head(f)?;
    if ctor != "field" {
        return None;
    }
    let [name, presence, ty] = args else {
        return None;
    };
    let name = read_text(name).ok()?;
    let required = match atom_bytes(presence)? {
        b"req" => true,
        b"opt" => false,
        _ => return None,
    };
    Some((name, required, ty))
}

fn conforms_record<'a>(
    v: &Value,
    mode: &Value,
    fields: &'a [Value],
    env: Option<&Scope<'_, EnvEntry<'a>>>,
    depth: usize,
) -> bool {
    // mode is a bare atom flag (open | closed).
    let open = match atom_bytes(mode) {
        Some(b"open") => true,
        Some(b"closed") => false,
        _ => return false,
    };
    // v must be a well-formed ontos/data map.
    let Ok(entries) = read_map(v) else {
        return false;
    };
    // Index the map's utf8-text keys to their values in a single pass — a map-backed
    // lookup replacing the old per-field linear `present.iter().find`. `read_map`
    // already guarantees keys are sorted and dup-free by codec bytes; distinct keys
    // therefore decode to distinct text strings, so no `insert` ever collides and the
    // map preserves the reference core's "first match in map order" exactly. A non-text
    // key makes `v` not a record-shaped value (same early `false` as the linear scan).
    let mut present: HashMap<String, &Value> = HashMap::with_capacity(entries.len());
    for (k, val) in &entries {
        let Ok(key) = read_text(k) else {
            return false;
        };
        present.insert(key, val);
    }
    // Single pass over the declared fields drives BOTH checks (no second `read_field`
    // sweep): per field, look the name up in `present` (O(1)) for the required/optional
    // verdict, and — in closed mode — count how many present keys a declared field
    // covers. Field names are pairwise-distinct (`wf(record)`, checked at entry), so a
    // present declared key is counted at most once.
    let mut covered = 0usize;
    for f in fields {
        let Some((name, required, ty)) = read_field(f) else {
            return false; // malformed field descriptor
        };
        match present.get(name.as_str()) {
            Some(val) => {
                covered += 1;
                if !conforms_in(val, ty, env, depth + 1) {
                    return false;
                }
            }
            None => {
                if required {
                    return false;
                }
            }
        }
    }
    // Closed: no present key outside the declared fields. With distinct field names,
    // `covered` is exactly the number of present keys that match some declared field;
    // a present key outside the declared set exists iff `covered < present.len()`. This
    // replaces the reference core's second `read_field` pass + O(fields×keys) sweep.
    if !open && covered != present.len() {
        return false;
    }
    true
}

// ===========================================================================
// well-formedness — `wf(T)`
// ===========================================================================

/// Decide `wf(T)` — "is `T` a well-formed horos descriptor?" Total and search-free;
/// recurses into sub-descriptors (which are finite). A descriptor that fails `wf` is a
/// valid ontos value, simply not a recognized descriptor; `conforms(_, T)` over such a
/// `T` is `false`.
///
/// For `rec`/`ref`: a `ref` must be **bound** by an enclosing `rec` and **guarded** —
/// reachable only after a descent into the value. An unbound or unguarded `ref` (e.g.
/// `rec(x, ref(x))` or `rec(x, union(ref(x), …))`) is not well-formed.
pub fn wf(t: &Value) -> bool {
    wf_in(t, None, None, 0)
}

fn wf_in<'a>(
    t: &'a Value,
    bound: Option<&Scope<'_, &'a [u8]>>,
    pending: Option<&Scope<'_, &'a [u8]>>,
    depth: usize,
) -> bool {
    // Recursion-depth guard (horos-internal#37): mirrors `conforms_in` — past MAX_RECURSION_DEPTH
    // frames a descriptor is treated as non-wf, deterministically and at the same depth
    // in all three cores, so `wf` can never overflow the native stack on a deep-but-finite
    // descriptor.
    if depth > MAX_RECURSION_DEPTH {
        return false;
    }
    let Some((ctor, args)) = head(t) else {
        return false;
    };
    match (ctor, args) {
        ("any", []) | ("atom", []) => true,
        ("atom-exact", [b]) => b.as_atom().is_some(),
        // wf requires 0 <= lo <= hi AND both bounds in i128 range (the horos-internal#18 cap): a bound
        // past `ATOM_LEN_BOUND_MAX` makes `read_int` return `Err` → non-wf, identically
        // in Go/TS (which cap explicitly). A bound = 2^127 is non-wf in all three cores.
        ("atom-len", [lo, hi]) => match (read_int(lo), read_int(hi)) {
            (Ok(lo), Ok(hi)) => (0..=hi).contains(&lo),
            _ => false,
        },
        ("atom-in", [set_v]) => match read_set(set_v) {
            // Every element must be an atom (an atom-membership set).
            Ok(elems) => elems.iter().all(|e| e.as_atom().is_some()),
            Err(_) => false,
        },
        // Text refinements (horos-internal#68). `text-len` carries char-count bounds with the same
        // `0 <= lo <= hi` + i128-cap discipline as `atom-len` (horos-internal#18); the three
        // grammar-class refinements are nullary named predicates.
        ("text-len", [lo, hi]) => match (read_int(lo), read_int(hi)) {
            (Ok(lo), Ok(hi)) => (0..=hi).contains(&lo),
            _ => false,
        },
        ("key-grammar", []) | ("text-non-blank", []) | ("lowercase-uuid", []) => true,
        ("int-min", [b]) | ("int-max", [b]) => read_int(b).is_ok(),
        ("scalar", [k]) => matches!(atom_bytes(k), Some(b"int" | b"utf8-text" | b"bool")),
        // Descent combinators clear `pending` (a value constructor is consumed).
        ("tuple", ts) => ts.iter().all(|ct| wf_in(ct, bound, None, depth + 1)),
        ("seq", [te]) => wf_in(te, bound, None, depth + 1),
        ("compound", [label, ts @ ..]) => {
            label.as_atom().is_some() && ts.iter().all(|ct| wf_in(ct, bound, None, depth + 1))
        }
        ("list-of", [te]) | ("set-of", [te]) => wf_in(te, bound, None, depth + 1),
        ("map-of", [tk, tv]) => {
            wf_in(tk, bound, None, depth + 1) && wf_in(tv, bound, None, depth + 1)
        }
        ("record", [mode, fields @ ..]) => wf_record(mode, fields, bound, depth),
        // Same-`v` combinators thread the guard through.
        ("union", ts) | ("and", ts) => ts.iter().all(|ct| wf_in(ct, bound, pending, depth + 1)),
        ("not", [t1]) => wf_in(t1, bound, pending, depth + 1),
        ("exact", [_v0]) => true, // any single ontos value is a valid singleton
        ("rec", [name, body]) => match name.as_atom() {
            Some(nm) => {
                let nb = nm.bytes();
                // Borrowed push onto each chain (no per-frame copy).
                let b = Scope::push(nb, bound);
                let p = Scope::push(nb, pending);
                wf_in(body, Some(&b), Some(&p), depth + 1)
            }
            None => false,
        },
        ("ref", [name]) => match name.as_atom() {
            Some(nm) => {
                let nb = nm.bytes();
                scope_contains(bound, nb) && !scope_contains(pending, nb)
            }
            None => false,
        },
        _ => false,
    }
}

fn wf_record<'a>(
    mode: &Value,
    fields: &'a [Value],
    bound: Option<&Scope<'_, &'a [u8]>>,
    depth: usize,
) -> bool {
    if !matches!(atom_bytes(mode), Some(b"open" | b"closed")) {
        return false;
    }
    let mut names: Vec<String> = Vec::with_capacity(fields.len());
    for f in fields {
        let Some((name, _required, ty)) = read_field(f) else {
            return false;
        };
        if !wf_in(ty, bound, None, depth + 1) {
            return false;
        }
        if names.contains(&name) {
            return false; // duplicate field name
        }
        names.push(name);
    }
    true
}

// ===========================================================================
// ===========================================================================

/// Compares a canonical `int` of any width with an i128 bound; `None` when `v` is not a
/// canonical `int`. A value beyond i128 lies beyond every bound, so its sign decides.
fn compare_int(v: &Value, bound: i128) -> Option<Ordering> {
    let (negative, _) = read_int_bytes(v).ok()?;
    Some(match read_int(v) {
        Ok(n) => n.cmp(&bound),
        Err(_) if negative => Ordering::Less,
        Err(_) => Ordering::Greater,
    })
}

/// Whether `t` is a **leaf** descriptor (a §4 leaf / §5 refinement / `exact`) — i.e. NOT a
pub(crate) fn is_leaf_ctor(ctor: &str) -> bool {
    matches!(
        ctor,
        "any"
            | "atom"
            | "atom-exact"
            | "atom-len"
            | "atom-in"
            | "int-min"
            | "int-max"
            | "text-len"
            | "key-grammar"
            | "text-non-blank"
            | "lowercase-uuid"
            | "scalar"
            | "exact"
    )
}

/// Decide a LEAF descriptor `t` against `v` (the `horos/leaf(T, v)` builtin). `false` for any
/// non-leaf ctor. Reuses the recognizer's leaf arms by delegating to [`conforms_in`] at depth
/// 0 with empty scopes — a leaf arm never recurses into a value position, so the scopes are
/// inert and the result is exactly the recognizer's leaf verdict.
pub(crate) fn leaf_classifies(v: &Value, t: &Value) -> bool {
    match head(t) {
        Some((ctor, _)) if is_leaf_ctor(ctor) => conforms_in(v, t, None, 0),
        _ => false,
    }
}

/// Decide whether a LEAF descriptor `t` is well-formed (the `horos/wf-leaf(T)` builtin).
/// `false` for any non-leaf ctor. Delegates to [`wf_in`] (a leaf arm never recurses).
pub(crate) fn leaf_well_formed(t: &Value) -> bool {
    match head(t) {
        Some((ctor, _)) if is_leaf_ctor(ctor) => wf_in(t, None, None, 0),
        _ => false,
    }
}

pub mod diagnose;

#[cfg(test)]
mod tests;

#[cfg(test)]
mod vectors;
