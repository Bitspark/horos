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
//!
//! **Locations** ([ADR 0009](../../../docs/architecture/decisions/0009-diagnostic-locations.md)):
//! [`diagnose_located`] and [`diagnose_wf_located`] return the unchanged [`Diagnosis`] with
//! the one [`Location`] the form contract selects: the value path, the descriptor position
//! and the reason. `Diagnosis` stays the `Copy` enum callers match on, so the location
//! travels beside it rather than inside it.

use ontos_core::Value;

use std::collections::{HashMap, HashSet};

use crate::{
    atom_bytes, head, is_leaf_ctor, leaf_classifies, leaf_well_formed, read_field, read_int,
    read_list, read_map, read_set, read_text, MAX_RECURSION_DEPTH,
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
// locations (ADR 0009, docs/spec/horos.md "Diagnostic locations")
// ---------------------------------------------------------------------------

/// One step of a value path, from the checked value towards the value at fault. Indices
/// are 0-based and count canonical order.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum Step {
    /// Item `i` of a bare tuple (tuple, seq, compound; a compound's first child is item 1).
    Item(usize),
    /// Element `i` of a canonical list or set.
    Elem(usize),
    /// The key of entry `i` of a canonical map.
    Key(usize),
    /// The value of entry `i` of a canonical map.
    Value(usize),
    /// The value of the record field with this name.
    Field(String),
}

/// Why a location was reported, spelled as in the form contract by [`Reason::as_str`].
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Reason {
    /// The descriptor at `at` does not accept the value at `path`.
    Refused,
    /// A required record field is absent; [`Location::field`] names it.
    MissingField,
    /// A closed record has an undeclared field; [`Location::field`] names it.
    ExtraField,
    /// No alternative of the union at `at` went deeper into the value than the union.
    NoAlternative,
    /// The inner descriptor of the complement at `at` accepted the value.
    Complement,
    /// The declared depth bound was reached entering the descriptor at `at`.
    Limit,
    /// The value at `at` is not a horos descriptor.
    NotDescriptor,
    /// The descriptor at `at` has an unknown `horos/` constructor.
    UnknownConstructor,
    /// The descriptor at `at` has the wrong number of arguments.
    Arity,
    /// Argument [`Location::arg`] of the descriptor at `at` has the wrong form.
    Argument,
    /// Record field [`Location::arg`] repeats an earlier field name.
    DuplicateField,
    /// The `ref` at `at` names no enclosing `rec`.
    UnboundRef,
    /// The `ref` at `at` is reachable from its `rec` without a structural descent.
    UnguardedRef,
}

impl Reason {
    /// The form contract's spelling of this reason.
    pub fn as_str(self) -> &'static str {
        match self {
            Reason::Refused => "refused",
            Reason::MissingField => "missing-field",
            Reason::ExtraField => "extra-field",
            Reason::NoAlternative => "no-alternative",
            Reason::Complement => "complement",
            Reason::Limit => "limit",
            Reason::NotDescriptor => "not-descriptor",
            Reason::UnknownConstructor => "unknown-constructor",
            Reason::Arity => "arity",
            Reason::Argument => "argument",
            Reason::DuplicateField => "duplicate-field",
            Reason::UnboundRef => "unbound-ref",
            Reason::UnguardedRef => "unguarded-ref",
        }
    }
}

/// Where a check failed: the value path, the descriptor position (tuple-item indices from
/// the root descriptor; a `ref` continues at its `rec` body), and the reason.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Location {
    pub path: Vec<Step>,
    pub at: Vec<usize>,
    pub reason: Reason,
    /// The record field, for [`Reason::MissingField`] and [`Reason::ExtraField`].
    pub field: Option<String>,
    /// The item index of the faulty argument within the descriptor at `at`.
    pub arg: Option<usize>,
}

/// A [`Diagnosis`] with its location: `None` iff the diagnosis is `Conforms`.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Located {
    pub diagnosis: Diagnosis,
    pub location: Option<Location>,
}

impl Located {
    fn from_out(o: Out) -> Located {
        Located {
            diagnosis: o.diagnosis,
            location: o.location.map(|l| *l),
        }
    }
}

/// The recursion's outcome. The boxed location keeps every recursive frame small, so the
/// bounded descent fits the same stacks as the boolean recognizer.
struct Out {
    diagnosis: Diagnosis,
    location: Option<Box<Location>>,
}

impl Out {
    fn conforms() -> Out {
        Out {
            diagnosis: Diagnosis::Conforms,
            location: None,
        }
    }
    fn with(diagnosis: Diagnosis, location: Location) -> Out {
        Out {
            diagnosis,
            location: Some(Box::new(location)),
        }
    }
    fn mismatch(location: Location) -> Out {
        Out::with(Diagnosis::Mismatch, location)
    }
    fn not_well_formed(location: Location) -> Out {
        Out::with(Diagnosis::NotWellFormed, location)
    }
    fn exhausted(location: Location) -> Out {
        Out::with(Diagnosis::Exhausted(Limit::RecursionDepth), location)
    }
    fn path_len(&self) -> usize {
        self.location.as_ref().map_or(0, |l| l.path.len())
    }
}

/// A persistent stack node borrowed from the parent frame: positions, value paths, `rec`
/// bindings and pending names cost no allocation until a failure materializes them.
struct Link<'a, T> {
    head: T,
    tail: Option<&'a Link<'a, T>>,
    len: usize,
}

type Chain<'a, T> = Option<&'a Link<'a, T>>;

fn link<'a, T>(head: T, tail: Chain<'a, T>) -> Link<'a, T> {
    Link {
        head,
        tail,
        len: tail.map_or(0, |l| l.len) + 1,
    }
}

fn chain_iter<'a, T>(mut c: Chain<'a, T>) -> impl Iterator<Item = &'a T> {
    std::iter::from_fn(move || {
        let node = c?;
        c = node.tail;
        Some(&node.head)
    })
}

/// A value path step that borrows a record field name from the checking frame.
#[derive(Clone, Copy)]
enum StepRef<'a> {
    Item(usize),
    Elem(usize),
    Key(usize),
    Value(usize),
    Field(&'a str),
}

impl StepRef<'_> {
    fn owned(self) -> Step {
        match self {
            StepRef::Item(i) => Step::Item(i),
            StepRef::Elem(i) => Step::Elem(i),
            StepRef::Key(i) => Step::Key(i),
            StepRef::Value(i) => Step::Value(i),
            StepRef::Field(n) => Step::Field(n.to_owned()),
        }
    }
}

type Pos<'a> = Chain<'a, usize>;
type Path<'a> = Chain<'a, StepRef<'a>>;
type Names<'a> = Chain<'a, &'a [u8]>;

/// An in-scope `rec` binding: its name, its body and the body's descriptor position.
#[derive(Clone, Copy)]
struct Binding<'a> {
    name: &'a [u8],
    body: &'a Value,
    pos: Pos<'a>,
}

type Env<'a> = Chain<'a, Binding<'a>>;

fn locate(path: Path<'_>, pos: Pos<'_>, reason: Reason) -> Location {
    let mut steps: Vec<Step> = chain_iter(path).map(|s| s.owned()).collect();
    steps.reverse();
    let mut at: Vec<usize> = chain_iter(pos).copied().collect();
    at.reverse();
    Location {
        path: steps,
        at,
        reason,
        field: None,
        arg: None,
    }
}

fn names_contain(names: Names<'_>, name: &[u8]) -> bool {
    chain_iter(names).any(|n| *n == name)
}

/// Kleene conjunction with locations (ADR 0008 D3): the first decisive `Mismatch` in order
/// wins; otherwise the first exhausted sub-outcome; otherwise the first malformed one.
struct AllFold {
    exhausted: Option<Out>,
    not_wf: Option<Out>,
}

impl AllFold {
    fn new() -> AllFold {
        AllFold {
            exhausted: None,
            not_wf: None,
        }
    }
    /// Adds the next sub-outcome in order; returns it when it decides the conjunction.
    fn add(&mut self, o: Out) -> Option<Out> {
        match o.diagnosis {
            Diagnosis::Mismatch => return Some(o),
            Diagnosis::Conforms => {}
            Diagnosis::NotWellFormed => {
                self.not_wf.get_or_insert(o);
            }
            Diagnosis::Exhausted(_) => {
                self.exhausted.get_or_insert(o);
            }
        }
        None
    }
    fn finish(self) -> Out {
        self.exhausted.or(self.not_wf).unwrap_or_else(Out::conforms)
    }
}

// ---------------------------------------------------------------------------
// the public entry points
// ---------------------------------------------------------------------------

/// Diagnose `wf(T)`: `Conforms` when `T` is a well-formed descriptor, `NotWellFormed` when it
/// is conclusively not, and `Exhausted` when the well-formedness traversal itself could not
/// finish — the distinction horos-internal#211 asks for by name.
pub fn diagnose_wf(t: &Value) -> Diagnosis {
    diagnose_wf_located(t).diagnosis
}

/// Diagnose `v : T`. Mirrors [`crate::conforms`]: the `wf(T)` soundness gate runs first (spec
/// §3 MUST), so a descriptor that is not well-formed yields `NotWellFormed` and a descriptor
/// whose well-formedness could not be decided yields `Exhausted` — neither is reported as a
/// mismatch of the value.
pub fn diagnose(v: &Value, t: &Value) -> Diagnosis {
    diagnose_located(v, t).diagnosis
}

/// [`diagnose_wf`] with the location of a malformed or exhausted check (ADR 0009). The
/// location names the descriptor position, and the faulty argument where there is one.
pub fn diagnose_wf_located(t: &Value) -> Located {
    let w = Wx {
        bound: None,
        pending: None,
        pos: None,
        depth: 0,
    };
    Located::from_out(diagnose_wf_in(t, w))
}

/// [`diagnose`] with the location of every outcome other than `Conforms` (ADR 0009): the
/// value path, the refusing sub-descriptor and, for records, the missing or extra field.
/// When the well-formedness gate decides, its location is reported with an empty path.
pub fn diagnose_located(v: &Value, t: &Value) -> Located {
    let w = diagnose_wf_located(t);
    match w.diagnosis {
        Diagnosis::Conforms => {
            let cx = Cx {
                env: None,
                pending: None,
                pos: None,
                path: None,
                depth: 0,
            };
            Located::from_out(diagnose_in(v, t, cx))
        }
        Diagnosis::Mismatch | Diagnosis::NotWellFormed => Located {
            diagnosis: Diagnosis::NotWellFormed,
            location: w.location,
        },
        Diagnosis::Exhausted(_) => w,
    }
}

// ---------------------------------------------------------------------------
// the value recursion — an arm-for-arm mirror of `conforms_in`
// ---------------------------------------------------------------------------

/// The checking context of one frame: `rec` bindings, the unguarded names at this value
/// node, the descriptor position, the value path and the counted depth.
#[derive(Clone, Copy)]
struct Cx<'a> {
    env: Env<'a>,
    pending: Names<'a>,
    pos: Pos<'a>,
    path: Path<'a>,
    depth: usize,
}

impl Cx<'_> {
    fn here(self, reason: Reason) -> Location {
        locate(self.path, self.pos, reason)
    }
    fn refused(self) -> Out {
        Out::mismatch(self.here(Reason::Refused))
    }
}

/// Check the same value against the child descriptor at item `item`; `pending` threads.
fn same<'a>(v: &Value, ct: &'a Value, cx: Cx<'a>, item: usize) -> Out {
    let p = link(item, cx.pos);
    diagnose_in(
        v,
        ct,
        Cx {
            pos: Some(&p),
            depth: cx.depth + 1,
            ..cx
        },
    )
}

/// Check a child value against the child descriptor at `items` below the position, one
/// path step further. A descent into a child value clears `pending`.
fn descend<'a>(cv: &Value, ct: &'a Value, cx: Cx<'a>, items: &[usize], step: StepRef<'a>) -> Out {
    let s = link(step, cx.path);
    let p1 = link(items[0], cx.pos);
    let inner = |pos| Cx {
        env: cx.env,
        pending: None,
        pos,
        path: Some(&s),
        depth: cx.depth + 1,
    };
    match items.get(1) {
        Some(&second) => {
            let p2 = link(second, Some(&p1));
            diagnose_in(cv, ct, inner(Some(&p2)))
        }
        None => diagnose_in(cv, ct, inner(Some(&p1))),
    }
}

fn diagnose_in<'a>(v: &Value, t: &'a Value, cx: Cx<'a>) -> Out {
    if cx.depth > MAX_RECURSION_DEPTH {
        return Out::exhausted(cx.here(Reason::Limit));
    }
    let Some((ctor, args)) = head(t) else {
        return cx.refused(); // not a horos descriptor
    };
    // Leaves never recurse into a value position, so they are always decisive here.
    if is_leaf_ctor(ctor) {
        return if leaf_classifies(v, t) {
            Out::conforms()
        } else {
            cx.refused()
        };
    }
    match ctor {
        "union" => union_in(v, args, cx),
        "and" => and_in(v, args, cx),
        "not" => not_in(v, args, cx),
        "rec" => rec_in(v, args, cx),
        "ref" => ref_in(v, args, cx),
        "tuple" | "seq" | "compound" => tuple_in(v, ctor, args, cx),
        "list-of" | "set-of" => collection_in(v, ctor, args, cx),
        "map-of" => map_in(v, args, cx),
        "record" => record_in(v, args, cx),
        _ => cx.refused(),
    }
}

/// Kleene disjunction (ADR 0008 D3): a conforming alternative decides; otherwise an
/// exhausted one. A mismatch reports the first alternative with the longest value path if it
/// went deeper than the union's own value, else the union itself (ADR 0009).
#[inline(never)]
fn union_in<'a>(v: &Value, ts: &'a [Value], cx: Cx<'a>) -> Out {
    let mut exhausted: Option<Out> = None;
    let mut not_wf: Option<Out> = None;
    let mut deepest: Option<Out> = None;
    for (i, ct) in ts.iter().enumerate() {
        let o = same(v, ct, cx, i + 1);
        match o.diagnosis {
            Diagnosis::Conforms => return o, // decisive — stop here
            Diagnosis::Mismatch => {
                if deepest.as_ref().is_none_or(|d| o.path_len() > d.path_len()) {
                    deepest = Some(o);
                }
            }
            // NotWellFormed cannot arise below the entry gate.
            Diagnosis::NotWellFormed => {
                not_wf.get_or_insert(o);
            }
            Diagnosis::Exhausted(_) => {
                exhausted.get_or_insert(o);
            }
        }
    }
    if let Some(o) = exhausted.or(not_wf) {
        return o;
    }
    let own = cx.path.map_or(0, |l| l.len);
    match deepest {
        Some(d) if d.path_len() > own => d,
        _ => Out::mismatch(cx.here(Reason::NoAlternative)),
    }
}

#[inline(never)]
fn and_in<'a>(v: &Value, ts: &'a [Value], cx: Cx<'a>) -> Out {
    let mut fold = AllFold::new();
    for (i, ct) in ts.iter().enumerate() {
        if let Some(o) = fold.add(same(v, ct, cx, i + 1)) {
            return o;
        }
    }
    fold.finish()
}

/// Complement swaps the completed verdicts and PRESERVES exhaustion (ADR 0008 D2); a
/// complement that fails reports itself.
#[inline(never)]
fn not_in<'a>(v: &Value, args: &'a [Value], cx: Cx<'a>) -> Out {
    let [t1] = args else {
        return cx.refused();
    };
    let inner = same(v, t1, cx, 1);
    match inner.diagnosis {
        Diagnosis::Conforms => Out::mismatch(cx.here(Reason::Complement)),
        Diagnosis::Mismatch => Out::conforms(),
        // NotWellFormed cannot arise below the entry gate; exhaustion is preserved.
        Diagnosis::NotWellFormed | Diagnosis::Exhausted(_) => inner,
    }
}

/// `rec` binds its name to its body and the body's position; the location stays in the
/// written descriptor.
#[inline(never)]
fn rec_in<'a>(v: &Value, args: &'a [Value], cx: Cx<'a>) -> Out {
    let [name, body] = args else {
        return cx.refused();
    };
    let Some(nb) = atom_bytes(name) else {
        return cx.refused();
    };
    let bp = link(2, cx.pos);
    let e = link(
        Binding {
            name: nb,
            body,
            pos: Some(&bp),
        },
        cx.env,
    );
    let p = link(nb, cx.pending);
    let inner = Cx {
        env: Some(&e),
        pending: Some(&p),
        pos: Some(&bp),
        path: cx.path,
        depth: cx.depth + 1,
    };
    diagnose_in(v, body, inner)
}

/// `ref` continues at its `rec` body's position — mirrors the recognizer's guard exactly.
#[inline(never)]
fn ref_in<'a>(v: &Value, args: &'a [Value], cx: Cx<'a>) -> Out {
    let [name] = args else {
        return cx.refused();
    };
    let Some(nb) = atom_bytes(name) else {
        return cx.refused();
    };
    if names_contain(cx.pending, nb) {
        return cx.refused(); // unguarded — the recognizer's `false`
    }
    let Some(b) = chain_iter(cx.env).find(|b| b.name == nb) else {
        return cx.refused(); // unbound
    };
    let p = link(nb, cx.pending);
    let inner = Cx {
        pending: Some(&p),
        pos: b.pos,
        depth: cx.depth + 1,
        ..cx
    };
    diagnose_in(v, b.body, inner)
}

/// `tuple`, `seq` and `compound`: the bare tuple, its arity and label, then the items.
#[inline(never)]
fn tuple_in<'a>(v: &Value, ctor: &str, args: &'a [Value], cx: Cx<'a>) -> Out {
    let Some(tup) = v.as_tuple() else {
        return cx.refused();
    };
    let items = tup.items();
    let mut fold = AllFold::new();
    match (ctor, args) {
        ("tuple", ts) => {
            if items.len() != ts.len() {
                return cx.refused();
            }
            for (i, (c, ct)) in items.iter().zip(ts).enumerate() {
                if let Some(o) = fold.add(descend(c, ct, cx, &[i + 1], StepRef::Item(i))) {
                    return o;
                }
            }
        }
        ("seq", [te]) => {
            for (i, c) in items.iter().enumerate() {
                if let Some(o) = fold.add(descend(c, te, cx, &[1], StepRef::Item(i))) {
                    return o;
                }
            }
        }
        ("compound", [label, ts @ ..]) => {
            let Some(lb) = atom_bytes(label) else {
                return cx.refused();
            };
            if items.len() != ts.len() + 1 || atom_bytes(&items[0]) != Some(lb) {
                return cx.refused();
            }
            for (i, ct) in ts.iter().enumerate() {
                let j = i + 1;
                if let Some(o) = fold.add(descend(&items[j], ct, cx, &[j + 1], StepRef::Item(j))) {
                    return o;
                }
            }
        }
        _ => return cx.refused(),
    }
    fold.finish()
}

/// `list-of` and `set-of`: the canonical collection, then its elements in order.
#[inline(never)]
fn collection_in<'a>(v: &Value, ctor: &str, args: &'a [Value], cx: Cx<'a>) -> Out {
    let [te] = args else {
        return cx.refused();
    };
    let elems = if ctor == "list-of" {
        read_list(v)
    } else {
        read_set(v)
    };
    let Ok(elems) = elems else {
        return cx.refused();
    };
    let mut fold = AllFold::new();
    for (i, e) in elems.iter().enumerate() {
        if let Some(o) = fold.add(descend(e, te, cx, &[1], StepRef::Elem(i))) {
            return o;
        }
    }
    fold.finish()
}

/// `map-of`: the canonical map, then its entries in order, each key before its value.
#[inline(never)]
fn map_in<'a>(v: &Value, args: &'a [Value], cx: Cx<'a>) -> Out {
    let [tk, tv] = args else {
        return cx.refused();
    };
    let Ok(entries) = read_map(v) else {
        return cx.refused();
    };
    let mut fold = AllFold::new();
    for (i, (k, val)) in entries.iter().enumerate() {
        if let Some(o) = fold.add(descend(k, tk, cx, &[1], StepRef::Key(i))) {
            return o;
        }
        if let Some(o) = fold.add(descend(val, tv, cx, &[2], StepRef::Value(i))) {
            return o;
        }
    }
    fold.finish()
}

/// The record arm: canonical map, text keys, missing required fields in descriptor order,
/// extra fields of a closed record in canonical order, then the present declared fields in
/// descriptor order.
#[inline(never)]
fn record_in<'a>(v: &Value, args: &'a [Value], cx: Cx<'a>) -> Out {
    let [mode, fields @ ..] = args else {
        return cx.refused();
    };
    let closed = match atom_bytes(mode) {
        Some(b"closed") => true,
        Some(b"open") => false,
        _ => return cx.refused(),
    };
    let Ok(entries) = read_map(v) else {
        return cx.refused();
    };
    let mut keys: Vec<String> = Vec::with_capacity(entries.len());
    for (i, (k, _)) in entries.iter().enumerate() {
        match read_text(k) {
            Ok(key) => keys.push(key),
            Err(_) => {
                let s = link(StepRef::Key(i), cx.path);
                return Out::mismatch(locate(Some(&s), cx.pos, Reason::Refused));
            }
        }
    }
    let index: HashMap<&str, usize> = keys
        .iter()
        .enumerate()
        .map(|(i, k)| (k.as_str(), i))
        .collect();
    let mut declared = Vec::with_capacity(fields.len());
    for f in fields {
        let Some(field) = read_field(f) else {
            return cx.refused();
        };
        declared.push(field);
    }
    let field_fault = |reason, name: &str| {
        let mut loc = cx.here(reason);
        loc.field = Some(name.to_owned());
        Out::mismatch(loc)
    };
    for (name, required, _) in &declared {
        if *required && !index.contains_key(name.as_str()) {
            return field_fault(Reason::MissingField, name);
        }
    }
    if closed {
        let names: HashSet<&str> = declared.iter().map(|(n, _, _)| n.as_str()).collect();
        if let Some(extra) = keys.iter().find(|k| !names.contains(k.as_str())) {
            return field_fault(Reason::ExtraField, extra);
        }
    }
    let mut fold = AllFold::new();
    for (k, (name, _, ty)) in declared.iter().enumerate() {
        let Some(&i) = index.get(name.as_str()) else {
            continue;
        };
        let o = descend(&entries[i].1, ty, cx, &[k + 2, 3], StepRef::Field(name));
        if let Some(o) = fold.add(o) {
            return o;
        }
    }
    fold.finish()
}

// ---------------------------------------------------------------------------
// the well-formedness recursion — an arm-for-arm mirror of `wf_in`
// ---------------------------------------------------------------------------

/// The well-formedness context of one frame: bound and unguarded names, the position and
/// the counted depth.
#[derive(Clone, Copy)]
struct Wx<'a> {
    bound: Names<'a>,
    pending: Names<'a>,
    pos: Pos<'a>,
    depth: usize,
}

impl Wx<'_> {
    fn fault(self, reason: Reason, arg: Option<usize>) -> Out {
        let mut loc = locate(None, self.pos, reason);
        loc.arg = arg;
        Out::not_well_formed(loc)
    }
}

/// A leaf descriptor's own-form fault: its arity, then each argument in order against the
/// arguments before it. `None` when the leaf is well formed.
fn leaf_wf_fault(ctor: &str, args: &[Value]) -> Option<(Reason, Option<usize>)> {
    let arity = |n: usize| (args.len() != n).then_some((Reason::Arity, None));
    let argument = |k: usize| Some((Reason::Argument, Some(k)));
    match ctor {
        "any" | "atom" | "key-grammar" | "text-non-blank" | "lowercase-uuid" => arity(0),
        "exact" => arity(1),
        "atom-exact" => arity(1).or_else(|| {
            if args[0].as_atom().is_some() {
                None
            } else {
                argument(1)
            }
        }),
        "scalar" => arity(1).or_else(|| {
            if matches!(atom_bytes(&args[0]), Some(b"int" | b"utf8-text" | b"bool")) {
                None
            } else {
                argument(1)
            }
        }),
        "atom-in" => arity(1).or_else(|| match read_set(&args[0]) {
            Ok(elems) if elems.iter().all(|e| e.as_atom().is_some()) => None,
            _ => argument(1),
        }),
        "int-min" | "int-max" => arity(1).or_else(|| {
            if read_int(&args[0]).is_ok() {
                None
            } else {
                argument(1)
            }
        }),
        "atom-len" | "text-len" => arity(2).or_else(|| {
            let lo = match read_int(&args[0]) {
                Ok(lo) if lo >= 0 => lo,
                _ => return argument(1),
            };
            match read_int(&args[1]) {
                Ok(hi) if hi >= lo => None,
                _ => argument(2),
            }
        }),
        _ => Some((Reason::UnknownConstructor, None)),
    }
}

fn diagnose_wf_in<'a>(t: &'a Value, w: Wx<'a>) -> Out {
    if w.depth > MAX_RECURSION_DEPTH {
        return Out::exhausted(locate(None, w.pos, Reason::Limit));
    }
    let Some((ctor, args)) = head(t) else {
        return w.fault(Reason::NotDescriptor, None);
    };
    if is_leaf_ctor(ctor) {
        // The verdict stays the recognizer's; the fault only names where it failed.
        if leaf_well_formed(t) {
            return Out::conforms();
        }
        let found = leaf_wf_fault(ctor, args);
        debug_assert!(
            found.is_some(),
            "leaf fault disagrees with leaf_well_formed"
        );
        let (reason, arg) = found.unwrap_or((Reason::Argument, None));
        return w.fault(reason, arg);
    }
    // Descent combinators clear `pending`; same-value combinators thread it.
    let descent = Wx { pending: None, ..w };
    match (ctor, args) {
        ("tuple", ts) => wf_children(ts, 1, descent),
        ("seq", [_]) | ("list-of", [_]) | ("set-of", [_]) | ("map-of", [_, _]) => {
            wf_children(args, 1, descent)
        }
        ("compound", [label, ts @ ..]) => {
            if label.as_atom().is_none() {
                return w.fault(Reason::Argument, Some(1));
            }
            wf_children(ts, 2, descent)
        }
        ("union", ts) | ("and", ts) => wf_children(ts, 1, w),
        ("not", [_]) => wf_children(args, 1, w),
        ("rec", [name, body]) => {
            let Some(nb) = atom_bytes(name) else {
                return w.fault(Reason::Argument, Some(1));
            };
            let b = link(nb, w.bound);
            let p = link(nb, w.pending);
            let bp = link(2, w.pos);
            let inner = Wx {
                bound: Some(&b),
                pending: Some(&p),
                pos: Some(&bp),
                depth: w.depth + 1,
            };
            diagnose_wf_in(body, inner)
        }
        ("ref", [name]) => match atom_bytes(name) {
            None => w.fault(Reason::Argument, Some(1)),
            Some(nb) if !names_contain(w.bound, nb) => w.fault(Reason::UnboundRef, Some(1)),
            Some(nb) if names_contain(w.pending, nb) => w.fault(Reason::UnguardedRef, Some(1)),
            Some(_) => Out::conforms(),
        },
        ("record", [mode, fields @ ..]) => wf_record_in(mode, fields, w),
        (
            "seq" | "list-of" | "set-of" | "map-of" | "compound" | "not" | "rec" | "ref" | "record",
            _,
        ) => w.fault(Reason::Arity, None),
        _ => w.fault(Reason::UnknownConstructor, None),
    }
}

/// Child descriptors at items `first..`, in item order, under the Kleene conjunction:
/// exhaustion takes precedence over a malformed sibling.
#[inline(never)]
fn wf_children<'a>(ts: &'a [Value], first: usize, w: Wx<'a>) -> Out {
    let mut fold = AllFold::new();
    for (i, ct) in ts.iter().enumerate() {
        let p = link(first + i, w.pos);
        let inner = Wx {
            pos: Some(&p),
            depth: w.depth + 1,
            ..w
        };
        if let Some(o) = fold.add(diagnose_wf_in(ct, inner)) {
            return o;
        }
    }
    fold.finish()
}

/// A record's own form (mode, then each field form and name in order) before its field
/// types at `[k, 3]`.
#[inline(never)]
fn wf_record_in<'a>(mode: &'a Value, fields: &'a [Value], w: Wx<'a>) -> Out {
    if !matches!(atom_bytes(mode), Some(b"open" | b"closed")) {
        return w.fault(Reason::Argument, Some(1));
    }
    let mut names: Vec<String> = Vec::with_capacity(fields.len());
    let mut types = Vec::with_capacity(fields.len());
    for (k, field) in fields.iter().enumerate() {
        let Some((name, _, ty)) = read_field(field) else {
            return w.fault(Reason::Argument, Some(k + 2));
        };
        if names.contains(&name) {
            return w.fault(Reason::DuplicateField, Some(k + 2));
        }
        names.push(name);
        types.push(ty);
    }
    let mut fold = AllFold::new();
    for (k, ty) in types.into_iter().enumerate() {
        let p1 = link(k + 2, w.pos);
        let p2 = link(3, Some(&p1));
        let inner = Wx {
            pending: None,
            pos: Some(&p2),
            depth: w.depth + 1,
            ..w
        };
        if let Some(o) = fold.add(diagnose_wf_in(ty, inner)) {
            return o;
        }
    }
    fold.finish()
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

    fn text(s: &str) -> Value {
        ontos_data::encode_text(s)
    }
    fn field(name: &str, required: bool, ty: Value) -> Value {
        Value::tuple([
            a("horos/field"),
            text(name),
            a(if required { "req" } else { "opt" }),
            ty,
        ])
    }
    fn map(entries: Vec<(Value, Value)>) -> Value {
        ontos_data::encode_map(&entries).unwrap()
    }
    fn loc(path: Vec<Step>, at: Vec<usize>, reason: Reason) -> Location {
        Location {
            path,
            at,
            reason,
            field: None,
            arg: None,
        }
    }
    fn mismatch(l: Location) -> Located {
        Located {
            diagnosis: Diagnosis::Mismatch,
            location: Some(l),
        }
    }
    fn not_wf(at: Vec<usize>, reason: Reason, arg: Option<usize>) -> Located {
        Located {
            diagnosis: Diagnosis::NotWellFormed,
            location: Some(Location {
                arg,
                ..loc(vec![], at, reason)
            }),
        }
    }

    /// ADR 0009: a record names its missing or extra field before any child, and a child by
    /// its field name at the field type's position, through the `field` form.
    #[test]
    fn records_name_the_missing_extra_or_failing_field() {
        let t = d(
            "record",
            vec![
                a("closed"),
                field("name", true, d("scalar", vec![a("utf8-text")])),
                field("age", false, scalar_int()),
            ],
        );
        let missing = diagnose_located(&map(vec![(text("age"), text("x"))]), &t);
        assert_eq!(
            missing,
            mismatch(Location {
                field: Some("name".into()),
                ..loc(vec![], vec![], Reason::MissingField)
            })
        );
        let extra = diagnose_located(
            &map(vec![(text("name"), text("n")), (text("zzz"), text("x"))]),
            &t,
        );
        assert_eq!(
            extra,
            mismatch(Location {
                field: Some("zzz".into()),
                ..loc(vec![], vec![], Reason::ExtraField)
            })
        );
        let child = diagnose_located(
            &map(vec![(text("age"), text("x")), (text("name"), text("n"))]),
            &t,
        );
        assert_eq!(
            child,
            mismatch(loc(
                vec![Step::Field("age".into())],
                vec![3, 3],
                Reason::Refused
            ))
        );
        let int_key = diagnose_located(&map(vec![(encode_int(1), text("n"))]), &t);
        assert_eq!(
            int_key,
            mismatch(loc(vec![Step::Key(0)], vec![], Reason::Refused))
        );
        assert_eq!(
            diagnose(&map(vec![(text("name"), text("n"))]), &t),
            Diagnosis::Conforms
        );
        assert_eq!(
            diagnose_located(&map(vec![(text("name"), text("n"))]), &t).location,
            None
        );
    }

    /// Map keys and values are distinct steps, at the key and value descriptor positions.
    #[test]
    fn maps_separate_the_key_from_the_value() {
        let t = d(
            "map-of",
            vec![d("scalar", vec![a("utf8-text")]), scalar_int()],
        );
        let bad_value = map(vec![(text("a"), encode_int(1)), (text("b"), text("x"))]);
        assert_eq!(
            diagnose_located(&bad_value, &t),
            mismatch(loc(vec![Step::Value(1)], vec![2], Reason::Refused))
        );
        let bad_key = map(vec![(encode_int(1), encode_int(1))]);
        assert_eq!(
            diagnose_located(&bad_key, &t),
            mismatch(loc(vec![Step::Key(0)], vec![1], Reason::Refused))
        );
        let list = d("list-of", vec![d("not", vec![scalar_int()])]);
        assert_eq!(
            diagnose_located(&ontos_data::encode_list(&[a("x"), encode_int(2)]), &list),
            mismatch(loc(vec![Step::Elem(1)], vec![1], Reason::Complement))
        );
    }

    /// Through rec/ref a union reports its deepest alternative, at a position of the
    /// written descriptor; with no deeper alternative it reports itself.
    #[test]
    fn unions_through_recursion_report_the_deepest_alternative() {
        let t = cons_list();
        let inner_text = Value::tuple([encode_int(1), Value::tuple([text("x"), a("[]")])]);
        assert_eq!(
            diagnose_located(&inner_text, &t),
            mismatch(loc(
                vec![Step::Item(1), Step::Item(0)],
                vec![2, 2, 1],
                Reason::Refused
            ))
        );
        let bad_tail = Value::tuple([encode_int(1), a("nope")]);
        assert_eq!(
            diagnose_located(&bad_tail, &t),
            mismatch(loc(vec![Step::Item(1)], vec![2], Reason::NoAlternative))
        );
        assert_eq!(
            diagnose_located(&a("nope"), &t),
            mismatch(loc(vec![], vec![2], Reason::NoAlternative))
        );
        assert_eq!(
            diagnose_located(&encode_int(1), &d("union", vec![])),
            mismatch(loc(vec![], vec![], Reason::NoAlternative))
        );
    }

    /// An exhausted check names where the bound was reached: the first exhausted
    /// sub-outcome in the fixed order, one path step per value descent.
    #[test]
    fn exhaustion_names_where_the_bound_was_reached() {
        std::thread::Builder::new()
            .stack_size(64 * 1024 * 1024)
            .spawn(|| {
                let got = diagnose_located(&deep_cons(171), &cons_list());
                assert_eq!(got.diagnosis, Diagnosis::Exhausted(Limit::RecursionDepth));
                let l = got.location.expect("an exhausted location");
                let mut path = vec![Step::Item(1); 170];
                path.push(Step::Item(0));
                assert_eq!(l, loc(path, vec![2, 2, 1], Reason::Limit));
                // Under `not` the count shifts by one: the bound is reached entering the
                // `exact` alternative of the union at the 170th tail.
                let not = diagnose_located(&deep_cons(171), &d("not", vec![cons_list()]));
                assert_eq!(not.diagnosis, Diagnosis::Exhausted(Limit::RecursionDepth));
                assert_eq!(
                    not.location,
                    Some(loc(vec![Step::Item(1); 170], vec![1, 2, 1], Reason::Limit))
                );
            })
            .expect("spawn")
            .join()
            .expect("exhaustion");
    }

    /// diagnose_wf names the malformed constructor's position and the faulty argument.
    #[test]
    fn malformed_descriptors_name_the_constructor_and_argument() {
        let dup = d(
            "record",
            vec![
                a("closed"),
                field("a", true, scalar_int()),
                field("a", false, scalar_int()),
            ],
        );
        assert_eq!(
            diagnose_wf_located(&dup),
            not_wf(vec![], Reason::DuplicateField, Some(3))
        );
        let lo_gt_hi = d(
            "union",
            vec![
                d("atom", vec![]),
                d("atom-len", vec![encode_int(5), encode_int(1)]),
            ],
        );
        assert_eq!(
            diagnose_wf_located(&lo_gt_hi),
            not_wf(vec![2], Reason::Argument, Some(2))
        );
        assert_eq!(
            diagnose_wf_located(&d("atom-len", vec![encode_int(-1), a("x")])),
            not_wf(vec![], Reason::Argument, Some(1))
        );
        assert_eq!(
            diagnose_wf_located(&d("union", vec![d("atom", vec![]), a("x")])),
            not_wf(vec![2], Reason::NotDescriptor, None)
        );
        assert_eq!(
            diagnose_wf_located(&d("list-of", vec![d("float", vec![])])),
            not_wf(vec![1], Reason::UnknownConstructor, None)
        );
        assert_eq!(
            diagnose_wf_located(&d("seq", vec![])),
            not_wf(vec![], Reason::Arity, None)
        );
        let unbound = d(
            "rec",
            vec![a("L"), d("list-of", vec![d("ref", vec![a("M")])])],
        );
        assert_eq!(
            diagnose_wf_located(&unbound),
            not_wf(vec![2, 1], Reason::UnboundRef, Some(1))
        );
        let unguarded = d(
            "rec",
            vec![a("L"), d("union", vec![d("ref", vec![a("L")])])],
        );
        assert_eq!(
            diagnose_wf_located(&unguarded),
            not_wf(vec![2, 1], Reason::UnguardedRef, Some(1))
        );
        let record_child = d(
            "record",
            vec![a("open"), field("a", true, d("scalar", vec![a("float")]))],
        );
        assert_eq!(
            diagnose_wf_located(&record_child),
            not_wf(vec![2, 3], Reason::Argument, Some(1))
        );
        // diagnose reports the gate's location with an empty path
        assert_eq!(
            diagnose_located(&encode_int(1), &lo_gt_hi),
            not_wf(vec![2], Reason::Argument, Some(2))
        );
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
