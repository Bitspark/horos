//! Diagnostic checking outcomes (horos-internal#211, [ADR 0008](../../../docs/architecture/decisions/0008-diagnostic-outcomes-kleene.md)).
//!
//! An ADDITIVE companion to the frozen boolean surface. [`crate::wf`] and [`crate::conforms`]
//! are unchanged, the `vectors/conformance.json` oracle is unchanged, and nothing here
//! reinterprets what today's `false` means.
//!
//! What it adds is the distinction the boolean cannot carry: a completed structural
//! **mismatch**, a **malformed descriptor**, and a computation that reached the declared
//! **`MAX_RECURSION_DEPTH` bound** are three different pieces of evidence, and the boolean
//! renders all three as `false`.
//!
//! **The composition rules are the content** (ADR 0008 D2/D3), not the extra enum:
//!
//! - **Complement PRESERVES exhaustion.** `not(Exhausted) = Exhausted`, never `Conforms`. The
//!   frozen recognizer computes `!conforms_in(…)`, so an inner descent that trips the cap
//!   yields `false` and `not` reports an unqualified `true` — a completed positive out of a
//!   computation that never finished. That is the defect this module exists to name.
//! - **`union`/`and` are KLEENE**: a DECISIVE alternative beats an exhausted sibling. A union
//!   with one conforming branch conforms whatever an unfinished sibling would have said; an
//!   `and` with one mismatching branch mismatches. So the evaluator short-circuits on a
//!   decisive result but NEVER on an `Exhausted` — it keeps scanning the remaining finite
//!   alternatives for a decisive one. The answer is a function of the SET of sub-outcomes,
//!   never of traversal order.
//!
//! Every recursive arm mirrors [`crate::conforms`]'s own arm structure — the same depth
//! accounting, the same `pending` guard threading (same-`v` combinators thread it, a descent
//! into a child value clears it) — so a completed outcome here is the boolean's answer. That
//! agreement is swept over the whole oracle by `diagnosis_agrees_with_the_boolean_on_completed`
//! rather than assumed.

use ontos_core::Value;

use crate::{
    atom_bytes, head, is_leaf_ctor, leaf_classifies, leaf_well_formed, read_field, read_list,
    read_map, read_set, read_text, MAX_RECURSION_DEPTH,
};

/// The declared bound a [`Diagnosis::Exhausted`] reached. Named so a consumer can say WHICH
/// limit stopped the attempt rather than "something ran out".
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Limit {
    /// The `MAX_RECURSION_DEPTH` descent cap (spec §3 realization note, horos-internal#37/horos-internal#71).
    RecursionDepth,
}

impl Limit {
    /// The effective value of this limit in this build.
    pub fn value(self) -> usize {
        match self {
            Limit::RecursionDepth => MAX_RECURSION_DEPTH,
        }
    }
}

/// A checking outcome that distinguishes a completed decision from a reached bound.
///
/// `Exhausted` is **incomplete**: it is a statement about THIS bounded attempt and is never a
/// claim that the value does or does not have the form. It must not be read as a refutation.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Diagnosis {
    /// Completed: `v` exhibits the form `T` under [`Profile`].
    Conforms,
    /// Completed: `v` does not exhibit the form `T` under [`Profile`].
    Mismatch,
    /// Completed: `T` is not a well-formed horos descriptor, so it admits no values.
    NotWellFormed,
    /// INCOMPLETE: a declared bound was reached; no claim either way.
    Exhausted(Limit),
}

impl Diagnosis {
    /// Whether this outcome is a completed decision (everything but [`Diagnosis::Exhausted`]).
    pub fn is_complete(self) -> bool {
        !matches!(self, Diagnosis::Exhausted(_))
    }

    /// The legacy boolean this outcome corresponds to, WHERE ONE IS DETERMINED (ADR 0008 D4):
    /// `Some(true)` for `Conforms`, `Some(false)` for `Mismatch`/`NotWellFormed`, and **`None`
    /// for `Exhausted`** — the boolean is unconstrained there, and collapsing the outcome into
    /// it is exactly what horos-internal#211 asks not to do.
    pub fn as_completed_bool(self) -> Option<bool> {
        match self {
            Diagnosis::Conforms => Some(true),
            Diagnosis::Mismatch | Diagnosis::NotWellFormed => Some(false),
            Diagnosis::Exhausted(_) => None,
        }
    }
}

/// The checking profile a [`Diagnosis`] is stated under. A completed negative is completed
/// **under this profile**, never absolutely — so the profile travels with the verdict.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct Profile {
    /// The `MAX_RECURSION_DEPTH` descent cap.
    pub max_recursion_depth: usize,
    /// The inclusive `atom-len` bound range (the signed 128-bit cap, horos-internal#18/horos-internal#57).
    pub atom_len_bound_min: i128,
    pub atom_len_bound_max: i128,
}

/// The profile this build decides under.
pub fn profile() -> Profile {
    Profile {
        max_recursion_depth: MAX_RECURSION_DEPTH,
        atom_len_bound_min: crate::ATOM_LEN_BOUND_MIN,
        atom_len_bound_max: crate::ATOM_LEN_BOUND_MAX,
    }
}

// ---------------------------------------------------------------------------
// the Kleene folds (ADR 0008 D2/D3)
// ---------------------------------------------------------------------------

/// `not`: swap the completed verdicts, PRESERVE exhaustion. Never completes an incomplete
/// subdecision — the rule this module exists for.
fn kleene_not(inner: Diagnosis) -> Diagnosis {
    match inner {
        Diagnosis::Conforms => Diagnosis::Mismatch,
        Diagnosis::Mismatch => Diagnosis::Conforms,
        // A non-wf inner cannot arise below the entry `wf` gate, but map it conservatively
        // rather than inventing a verdict for it.
        Diagnosis::NotWellFormed => Diagnosis::NotWellFormed,
        Diagnosis::Exhausted(l) => Diagnosis::Exhausted(l),
    }
}

/// Kleene disjunction over a finite alternative list: a decisive `Conforms` wins over an
/// exhausted sibling; otherwise an exhaustion makes the whole thing unknown.
fn kleene_any(outcomes: impl IntoIterator<Item = Diagnosis>) -> Diagnosis {
    let mut exhausted = None;
    let mut not_wf = false;
    for o in outcomes {
        match o {
            Diagnosis::Conforms => return Diagnosis::Conforms, // decisive — stop here
            Diagnosis::Mismatch => {}
            Diagnosis::NotWellFormed => not_wf = true,
            Diagnosis::Exhausted(l) => exhausted = exhausted.or(Some(l)),
        }
    }
    match (exhausted, not_wf) {
        (Some(l), _) => Diagnosis::Exhausted(l),
        (None, true) => Diagnosis::NotWellFormed,
        (None, false) => Diagnosis::Mismatch,
    }
}

/// Kleene conjunction: a decisive `Mismatch` wins over an exhausted sibling.
fn kleene_all(outcomes: impl IntoIterator<Item = Diagnosis>) -> Diagnosis {
    let mut exhausted = None;
    let mut not_wf = false;
    for o in outcomes {
        match o {
            Diagnosis::Mismatch => return Diagnosis::Mismatch, // decisive — stop here
            Diagnosis::Conforms => {}
            Diagnosis::NotWellFormed => not_wf = true,
            Diagnosis::Exhausted(l) => exhausted = exhausted.or(Some(l)),
        }
    }
    match (exhausted, not_wf) {
        (Some(l), _) => Diagnosis::Exhausted(l),
        (None, true) => Diagnosis::NotWellFormed,
        (None, false) => Diagnosis::Conforms,
    }
}

// ---------------------------------------------------------------------------
// the public entry points
// ---------------------------------------------------------------------------

/// Diagnose `wf(T)`: `Conforms` when `T` is a well-formed descriptor, `NotWellFormed` when it
/// is conclusively not, and `Exhausted` when the well-formedness traversal itself could not
/// finish — the distinction horos-internal#211 asks for by name.
pub fn diagnose_wf(t: &Value) -> Diagnosis {
    diagnose_wf_in(t, &[], &[], 0)
}

/// Diagnose `v : T`. Mirrors [`crate::conforms`]: the `wf(T)` soundness gate runs first (spec
/// §3 MUST), so a descriptor that is not well-formed yields `NotWellFormed` and a descriptor
/// whose well-formedness could not be decided yields `Exhausted` — neither is reported as a
/// mismatch of the value.
pub fn diagnose(v: &Value, t: &Value) -> Diagnosis {
    match diagnose_wf(t) {
        Diagnosis::Conforms => diagnose_in(v, t, &[], &[], 0),
        Diagnosis::Mismatch | Diagnosis::NotWellFormed => Diagnosis::NotWellFormed,
        Diagnosis::Exhausted(l) => Diagnosis::Exhausted(l),
    }
}

// ---------------------------------------------------------------------------
// the recursions — arm-for-arm mirrors of `conforms_in` / `wf_in`
// ---------------------------------------------------------------------------

type Env = [(Vec<u8>, Value)];
type Pending = [Vec<u8>];

fn pushed_env(env: &Env, name: &[u8], body: &Value) -> Vec<(Vec<u8>, Value)> {
    let mut out = env.to_vec();
    out.push((name.to_vec(), body.clone()));
    out
}

fn pushed_pending(pending: &Pending, name: &[u8]) -> Vec<Vec<u8>> {
    let mut out = pending.to_vec();
    out.push(name.to_vec());
    out
}

fn lookup<'a>(env: &'a Env, name: &[u8]) -> Option<&'a Value> {
    env.iter().rev().find_map(|(k, b)| (k == name).then_some(b))
}

fn diagnose_in(v: &Value, t: &Value, env: &Env, pending: &Pending, depth: usize) -> Diagnosis {
    if depth > MAX_RECURSION_DEPTH {
        return Diagnosis::Exhausted(Limit::RecursionDepth);
    }
    let Some((ctor, args)) = head(t) else {
        return Diagnosis::Mismatch; // not a horos descriptor
    };

    // Leaves never recurse into a value position, so they are always decisive here.
    if is_leaf_ctor(ctor) {
        return if leaf_classifies(v, t) {
            Diagnosis::Conforms
        } else {
            Diagnosis::Mismatch
        };
    }

    match (ctor, args) {
        // Composition on the SAME v — `pending` threads through unchanged.
        ("union", ts) => kleene_any(
            ts.iter()
                .map(|ct| diagnose_in(v, ct, env, pending, depth + 1)),
        ),
        ("and", ts) => kleene_all(
            ts.iter()
                .map(|ct| diagnose_in(v, ct, env, pending, depth + 1)),
        ),
        ("not", [t1]) => kleene_not(diagnose_in(v, t1, env, pending, depth + 1)),

        // Recursion — mirrors the recognizer's guard exactly.
        ("rec", [name, body]) => match name.as_atom() {
            Some(nm) => {
                let nb = nm.bytes();
                diagnose_in(
                    v,
                    body,
                    &pushed_env(env, nb, body),
                    &pushed_pending(pending, nb),
                    depth + 1,
                )
            }
            None => Diagnosis::Mismatch,
        },
        ("ref", [name]) => match name.as_atom() {
            Some(nm) => {
                let nb = nm.bytes();
                if pending.iter().any(|p| p.as_slice() == nb) {
                    return Diagnosis::Mismatch; // unguarded — the recognizer's `false`
                }
                match lookup(env, nb) {
                    Some(body) => diagnose_in(
                        v,
                        &body.clone(),
                        env,
                        &pushed_pending(pending, nb),
                        depth + 1,
                    ),
                    None => Diagnosis::Mismatch, // unbound
                }
            }
            None => Diagnosis::Mismatch,
        },

        // Structural arms: the gate is decisive, and a descent into a CHILD VALUE clears
        // `pending` (a constructor of `v` was consumed).
        _ => match structural_children(v, ctor, args) {
            None => Diagnosis::Mismatch, // the structural gate failed — decisive
            Some(children) => kleene_all(
                children
                    .iter()
                    .map(|(cv, ct)| diagnose_in(cv, ct, env, &[], depth + 1)),
            ),
        },
    }
}

/// The structural gate + the ordered child sub-judgments, for the arms that descend into a
/// child VALUE. Mirrors the recognizer's recursive arms (and matches `crate::decompose`'s
/// worklist for them); `None` means the gate failed, so `v` cannot conform regardless of any
/// child. `union`/`and`/`not`/`rec`/`ref` are handled by the caller (they do not descend).
fn structural_children(v: &Value, ctor: &str, args: &[Value]) -> Option<Vec<(Value, Value)>> {
    match (ctor, args) {
        ("tuple", ts) => {
            let items = v.as_tuple()?.items();
            if items.len() != ts.len() {
                return None;
            }
            Some(items.iter().cloned().zip(ts.iter().cloned()).collect())
        }
        ("seq", [te]) => Some(
            v.as_tuple()?
                .items()
                .iter()
                .map(|c| (c.clone(), te.clone()))
                .collect(),
        ),
        ("compound", [label, ts @ ..]) => {
            let items = v.as_tuple()?.items();
            let lb = atom_bytes(label)?;
            if items.len() != ts.len() + 1 || atom_bytes(items.first()?)? != lb {
                return None;
            }
            Some(items[1..].iter().cloned().zip(ts.iter().cloned()).collect())
        }
        ("list-of", [te]) => Some(
            read_list(v)
                .ok()?
                .into_iter()
                .map(|e| (e, te.clone()))
                .collect(),
        ),
        ("set-of", [te]) => Some(
            read_set(v)
                .ok()?
                .into_iter()
                .map(|e| (e, te.clone()))
                .collect(),
        ),
        ("map-of", [tk, tv]) => Some(
            read_map(v)
                .ok()?
                .into_iter()
                .flat_map(|(k, val)| [(k, tk.clone()), (val, tv.clone())])
                .collect(),
        ),
        ("record", [mode, fields @ ..]) => record_children(v, mode, fields),
        _ => None,
    }
}

/// The record structural gate + present-field worklist — mirrors the recognizer's
/// `conforms_record`: a `utf8-text`-keyed map, every `req` field present, every present
/// declared field contributing a sub-judgment, and `closed` rejecting an extra key.
fn record_children(v: &Value, mode: &Value, fields: &[Value]) -> Option<Vec<(Value, Value)>> {
    let mode_b = atom_bytes(mode)?;
    let closed = match mode_b {
        b"closed" => true,
        b"open" => false,
        _ => return None,
    };
    let entries = read_map(v).ok()?;
    let mut keyed: Vec<(String, Value)> = Vec::with_capacity(entries.len());
    for (k, val) in entries {
        keyed.push((read_text(&k).ok()?, val));
    }
    let mut children = Vec::new();
    let mut declared: Vec<String> = Vec::with_capacity(fields.len());
    for f in fields {
        let (name, required, ty) = read_field(f)?;
        declared.push(name.clone());
        match keyed.iter().find(|(k, _)| *k == name) {
            Some((_, val)) => children.push((val.clone(), ty.clone())),
            None if required => return None, // a missing required field is decisive
            None => {}
        }
    }
    if closed && keyed.iter().any(|(k, _)| !declared.iter().any(|d| d == k)) {
        return None; // an extra key in a closed record is decisive
    }
    Some(children)
}

fn diagnose_wf_in(t: &Value, bound: &Pending, pending: &Pending, depth: usize) -> Diagnosis {
    if depth > MAX_RECURSION_DEPTH {
        return Diagnosis::Exhausted(Limit::RecursionDepth);
    }
    let Some((ctor, args)) = head(t) else {
        return Diagnosis::NotWellFormed;
    };
    if is_leaf_ctor(ctor) {
        return if leaf_well_formed(t) {
            Diagnosis::Conforms
        } else {
            Diagnosis::NotWellFormed
        };
    }
    let wf_all = |ts: &[Value], bound: &Pending, pending: &Pending| {
        kleene_all(
            ts.iter()
                .map(|ct| diagnose_wf_in(ct, bound, pending, depth + 1)),
        )
    };
    match (ctor, args) {
        ("tuple", ts) => wf_all(ts, bound, &[]),
        ("seq", [te]) => diagnose_wf_in(te, bound, &[], depth + 1),
        ("compound", [label, ts @ ..]) => {
            if label.as_atom().is_none() {
                return Diagnosis::NotWellFormed;
            }
            wf_all(ts, bound, &[])
        }
        ("list-of", [te]) | ("set-of", [te]) => diagnose_wf_in(te, bound, &[], depth + 1),
        ("map-of", [tk, tv]) => kleene_all([
            diagnose_wf_in(tk, bound, &[], depth + 1),
            diagnose_wf_in(tv, bound, &[], depth + 1),
        ]),
        ("union", ts) | ("and", ts) => wf_all(ts, bound, pending),
        ("not", [t1]) => diagnose_wf_in(t1, bound, pending, depth + 1),
        ("rec", [name, body]) => match atom_bytes(name) {
            Some(name) => diagnose_wf_in(
                body,
                &pushed_pending(bound, name),
                &pushed_pending(pending, name),
                depth + 1,
            ),
            None => Diagnosis::NotWellFormed,
        },
        ("ref", [name]) => match atom_bytes(name) {
            Some(name) if bound.iter().any(|n| n == name) && !pending.iter().any(|n| n == name) => {
                Diagnosis::Conforms
            }
            _ => Diagnosis::NotWellFormed,
        },
        ("record", [mode, fields @ ..]) => {
            if !matches!(atom_bytes(mode), Some(b"open" | b"closed")) {
                return Diagnosis::NotWellFormed;
            }
            let mut names = Vec::new();
            let mut outcomes = Vec::new();
            for field in fields {
                let Some((name, _, ty)) = read_field(field) else {
                    return Diagnosis::NotWellFormed;
                };
                if names.contains(&name) {
                    return Diagnosis::NotWellFormed;
                }
                names.push(name);
                outcomes.push(diagnose_wf_in(ty, bound, &[], depth + 1));
            }
            kleene_all(outcomes)
        }
        _ => Diagnosis::NotWellFormed,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use ontos_codec::decode;
    use ontos_data::encode_int;
    use std::path::PathBuf;

    fn a(s: &str) -> Value {
        Value::atom(s.as_bytes().to_vec())
    }
    fn d(ctor: &str, args: Vec<Value>) -> Value {
        let mut items = vec![a(&format!("horos/{ctor}"))];
        items.extend(args);
        Value::tuple(items)
    }
    fn scalar_int() -> Value {
        d("scalar", vec![a("int")])
    }
    /// The oracle's own cons-list descriptor: wf, guarded, and a value nested past the cap
    /// trips the descent exactly as `depth-171-over-cap-false` does.
    fn cons_list() -> Value {
        d(
            "rec",
            vec![
                a("L"),
                d(
                    "union",
                    vec![
                        d("exact", vec![a("[]")]),
                        d("tuple", vec![scalar_int(), d("ref", vec![a("L")])]),
                    ],
                ),
            ],
        )
    }
    fn deep_cons(n: usize) -> Value {
        let mut v = a("[]");
        for _ in 0..n {
            v = Value::tuple([encode_int(1), v]);
        }
        v
    }

    fn load_oracle() -> serde_json::Value {
        let path: PathBuf = [env!("CARGO_MANIFEST_DIR"), "testdata", "conformance.json"]
            .iter()
            .collect();
        serde_json::from_str(&std::fs::read_to_string(&path).expect("conformance.json")).unwrap()
    }
    fn from_hex(s: &str) -> Vec<u8> {
        (0..s.len())
            .step_by(2)
            .map(|i| u8::from_str_radix(&s[i..i + 2], 16).unwrap())
            .collect()
    }

    /// ADR 0008 D4, SWEPT rather than asserted: on every oracle judgment where `diagnose`
    /// completes, it agrees with the frozen boolean. Where it reports `Exhausted`, the boolean
    /// is unconstrained — and the oracle's own depth-171 case is exactly such a point. This is
    /// also the anti-drift gate between the two recursions.
    #[test]
    fn diagnosis_agrees_with_the_boolean_on_completed() {
        std::thread::Builder::new()
            .stack_size(64 * 1024 * 1024)
            .spawn(|| {
                let doc = load_oracle();
                let mut completed = 0usize;
                let mut exhausted = 0usize;
                for c in doc["cases"].as_array().unwrap() {
                    let name = c["name"].as_str().unwrap();
                    let t = decode(&from_hex(c["descriptor"].as_str().unwrap())).unwrap();

                    match diagnose_wf(&t).as_completed_bool() {
                        Some(b) => {
                            assert_eq!(b, crate::wf(&t), "wf disagrees for {name}");
                            completed += 1;
                        }
                        None => exhausted += 1,
                    }

                    for (i, val) in c["values"].as_array().unwrap().iter().enumerate() {
                        let v = decode(&from_hex(val["value"].as_str().unwrap())).unwrap();
                        match diagnose(&v, &t).as_completed_bool() {
                            Some(b) => {
                                assert_eq!(
                                    b,
                                    crate::conforms(&v, &t),
                                    "conforms disagrees for {name} / value[{i}]"
                                );
                                completed += 1;
                            }
                            None => exhausted += 1,
                        }
                    }
                }
                assert!(
                    completed >= 100,
                    "expected >=100 completed, got {completed}"
                );
                assert_eq!(
                    exhausted, 1,
                    "expected exactly the one in-corpus exhaustion (depth-171)"
                );
            })
            .expect("spawn")
            .join()
            .expect("sweep");
    }

    /// The oracle's depth boundary, now SEPARATED: 170 is a completed conformance, 171 is an
    /// exhaustion — where the frozen boolean says a flat `false` for both.
    #[test]
    fn the_depth_boundary_separates_exhaustion_from_mismatch() {
        let t = cons_list();
        assert_eq!(diagnose(&deep_cons(170), &t), Diagnosis::Conforms);
        assert_eq!(
            diagnose(&deep_cons(171), &t),
            Diagnosis::Exhausted(Limit::RecursionDepth)
        );
        // the frozen boolean is UNCHANGED and still says false for the over-cap value
        assert!(crate::conforms(&deep_cons(170), &t));
        assert!(!crate::conforms(&deep_cons(171), &t));
        // a plain mismatch at the same descriptor is COMPLETED, not exhausted
        assert_eq!(diagnose(&a("nope"), &t), Diagnosis::Mismatch);
    }

    /// ADR 0008 D2 — THE rule this module exists for. `not` of an exhausted subdecision stays
    /// exhausted, where the frozen boolean reports an unqualified `true`.
    #[test]
    fn complement_preserves_exhaustion_where_the_boolean_says_true() {
        let inner = cons_list();
        let t = d("not", vec![inner.clone()]);
        let v = deep_cons(171);

        // the boolean: !false = TRUE — a completed positive from a computation that never
        // finished. This precondition is the defect being named.
        assert!(
            crate::conforms(&v, &t),
            "precondition: the frozen boolean reports true here"
        );
        assert_eq!(
            diagnose(&v, &t),
            Diagnosis::Exhausted(Limit::RecursionDepth),
            "not(Exhausted) must stay Exhausted"
        );
        // a NESTED complement does not launder it either
        let t2 = d("not", vec![d("not", vec![inner])]);
        assert_eq!(
            diagnose(&v, &t2),
            Diagnosis::Exhausted(Limit::RecursionDepth)
        );
        // sanity: over a SHALLOW value the same complement completes normally
        assert_eq!(diagnose(&deep_cons(2), &t), Diagnosis::Mismatch);
        assert_eq!(diagnose(&a("nope"), &t), Diagnosis::Conforms);
    }

    /// ADR 0008 D3 — a decisive alternative beats an exhausted sibling, in BOTH orders, so the
    /// answer is a function of the SET of sub-outcomes and never of traversal order.
    #[test]
    fn union_and_and_are_kleene_in_both_orders() {
        let deep = deep_cons(171);
        let exhausts = cons_list();
        let accepts = d("any", vec![]);
        let rejects = scalar_int(); // a tuple is decisively not a scalar int

        for args in [
            vec![exhausts.clone(), accepts.clone()],
            vec![accepts.clone(), exhausts.clone()],
        ] {
            assert_eq!(
                diagnose(&deep, &d("union", args)),
                Diagnosis::Conforms,
                "a decisive union branch must beat an exhausted sibling"
            );
        }
        assert_eq!(
            diagnose(&deep, &d("union", vec![exhausts.clone(), rejects.clone()])),
            Diagnosis::Exhausted(Limit::RecursionDepth),
            "no decisive branch: unknown, never a completed mismatch"
        );

        for args in [
            vec![exhausts.clone(), rejects.clone()],
            vec![rejects.clone(), exhausts.clone()],
        ] {
            assert_eq!(
                diagnose(&deep, &d("and", args)),
                Diagnosis::Mismatch,
                "a decisive and-branch mismatch must beat an exhausted sibling"
            );
        }
        assert_eq!(
            diagnose(&deep, &d("and", vec![exhausts, accepts])),
            Diagnosis::Exhausted(Limit::RecursionDepth),
            "no decisive mismatch: unknown, never a completed conformance"
        );
    }

    /// A malformed descriptor is its OWN completed outcome, not a mismatch of the value.
    #[test]
    fn a_malformed_descriptor_is_not_a_mismatch() {
        let bad = d("scalar", vec![a("float")]);
        assert_eq!(diagnose_wf(&bad), Diagnosis::NotWellFormed);
        assert_eq!(diagnose(&encode_int(1), &bad), Diagnosis::NotWellFormed);
        assert_eq!(diagnose(&a("x"), &scalar_int()), Diagnosis::Mismatch);
        // both are `false` to the frozen boolean — the distinction is the whole point
        assert!(!crate::conforms(&encode_int(1), &bad));
        assert!(!crate::conforms(&a("x"), &scalar_int()));
    }

    /// The profile travels with the verdict, and an exhaustion names its limit.
    #[test]
    fn the_profile_and_the_limit_are_stated() {
        let p = profile();
        assert_eq!(p.max_recursion_depth, MAX_RECURSION_DEPTH);
        assert_eq!(p.atom_len_bound_max, crate::ATOM_LEN_BOUND_MAX);
        assert_eq!(Limit::RecursionDepth.value(), MAX_RECURSION_DEPTH);
        assert!(Diagnosis::Conforms.is_complete());
        assert!(Diagnosis::Mismatch.is_complete());
        assert!(Diagnosis::NotWellFormed.is_complete());
        assert!(!Diagnosis::Exhausted(Limit::RecursionDepth).is_complete());
        assert_eq!(
            Diagnosis::Exhausted(Limit::RecursionDepth).as_completed_bool(),
            None,
            "an exhaustion must NOT collapse into a boolean"
        );
    }
}
