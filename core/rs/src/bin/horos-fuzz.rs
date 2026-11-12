//! `horos-fuzz` — the differential-fuzzing GENERATOR for conformance (horos-internal#72).
//!
//! A seeded, deterministic emitter of randomized `(descriptor, value)` pairs — the input
//! half of seeded conformance. Each line of stdout is one pair:
//!
//! ```text
//! <descriptor_hex> <value_hex>
//! ```
//!
//! where both fields are `ontos-codec-v1` lowercase hex — the exact byte form the four
//! `horos-conform` command-line tools decode (`conformance/run.py --seed` runs every pair
//! through Go, Rust, TypeScript and Python for `wf`, `conforms` and `diagnose` and
//! requires identical answers). There is **no oracle**: the generator emits descriptors
//! and values WITHOUT computing any expected judgment. Cross-core agreement IS the
//! property — a pair on which the cores disagree is the bug, whatever the "right" answer
//! is. So the generator is free to emit well-formed, malformed, conforming,
//! non-conforming, and boundary inputs indiscriminately; every one is a valid probe.
//!
//! Half the pairs come from the **scoping family** (ADR 0011): `rec`s that reuse a pool
//! of three names, capture skeletons, and values derived from the descriptor so its
//! unfoldings are taken, lexically or as the checkers before ADR 0011 read them. After
//! the pairs, one report line on stderr gives the seed's share of descriptors that
//! shadow a name and of those that are capture-prone, the only shape in which readings of
//! scope can differ.
//!
//! **Determinism is mandatory.** The PRNG is a pure splitmix64 seeded ONLY from the CLI
//! `--seed` argument (no system entropy), so a seed always reproduces the same stream —
//! a divergence found in CI replays bit-for-bit from its logged seed, on the same
//! generator revision. Values are kept well under the codec's `max_depth` (1024) so every
//! emitted pair decodes cleanly in all four cores (a decode error is a harness failure,
//! not a cross-core divergence); the deep-nesting boundary deliberately straddles horos's
//! `MAX_RECURSION_DEPTH` (512), where the recursion-depth guardrail trips identically
//! across the cores.
//!
//! Usage:
//!
//! ```text
//! horos-fuzz --seed <u64> --count <n>     # emit <n> pairs from <seed>
//! horos-fuzz <seed> <count>               # positional form (same meaning)
//! ```

use horos::NAMESPACE;
use ontos_codec::encode;
use ontos_core::{Tuple, Value};
use ontos_data::{encode_bool, encode_int, encode_list, encode_map, encode_set, encode_text};
use std::rc::Rc;

// ===========================================================================
// deterministic PRNG — splitmix64 (seeded ONLY from --seed; no system entropy)
// ===========================================================================

/// A pure splitmix64 generator. Tiny, fast, and fully reproducible: the same seed yields
/// the same stream on every platform, so a fuzz divergence replays bit-for-bit from its
/// logged seed. Deliberately NOT seeded from the clock/OS — reproducibility is the point.
struct Rng {
    state: u64,
}

impl Rng {
    fn new(seed: u64) -> Self {
        Rng { state: seed }
    }

    /// One splitmix64 step.
    fn next_u64(&mut self) -> u64 {
        self.state = self.state.wrapping_add(0x9E37_79B9_7F4A_7C15);
        let mut z = self.state;
        z = (z ^ (z >> 30)).wrapping_mul(0xBF58_476D_1CE4_E5B9);
        z = (z ^ (z >> 27)).wrapping_mul(0x94D0_49BB_1331_11EB);
        z ^ (z >> 31)
    }

    /// Uniform-ish in `[0, n)` (n > 0). Modulo bias is irrelevant for a fuzzer.
    fn below(&mut self, n: u64) -> u64 {
        if n == 0 {
            0
        } else {
            self.next_u64() % n
        }
    }

    /// `true` with probability `1/n`.
    fn one_in(&mut self, n: u64) -> bool {
        self.below(n) == 0
    }

    /// A small count in `[lo, hi]` (inclusive).
    fn range(&mut self, lo: usize, hi: usize) -> usize {
        if hi <= lo {
            return lo;
        }
        lo + self.below((hi - lo + 1) as u64) as usize
    }
}

// ===========================================================================
// value-level building blocks (the raw ontos values a descriptor classifies)
// ===========================================================================

fn atom(bytes: &[u8]) -> Value {
    Value::atom(bytes.to_vec())
}

fn tup(items: Vec<Value>) -> Value {
    Value::Tuple(Tuple::new(items))
}

/// A descriptor node `Tuple(Atom("horos/<ctor>"), args…)`.
fn desc(ctor: &str, mut args: Vec<Value>) -> Value {
    let mut items = vec![atom(format!("{NAMESPACE}{ctor}").as_bytes())];
    items.append(&mut args);
    tup(items)
}

/// A random short atom byte string. Sometimes empty, sometimes multi-byte UTF-8, sometimes
/// raw bytes (incl. non-UTF-8), so the recognizers' atom/text lanes are both exercised.
fn rand_atom_bytes(rng: &mut Rng) -> Vec<u8> {
    match rng.below(6) {
        0 => Vec::new(),                       // empty atom — the len-0 boundary
        1 => "héllo→世界".as_bytes().to_vec(), // multi-byte UTF-8
        2 => vec![0xff, 0xfe, 0x00, 0x80],     // raw, non-UTF-8 bytes
        3 => b"nil".to_vec(),
        4 => {
            // a random-length run of ASCII letters
            let len = rng.range(1, 8);
            (0..len).map(|_| b'a' + rng.below(26) as u8).collect()
        }
        _ => {
            // a single random byte
            vec![rng.below(256) as u8]
        }
    }
}

/// A random small int magnitude, occasionally near interesting boundaries.
fn rand_int(rng: &mut Rng) -> i128 {
    match rng.below(8) {
        0 => 0,
        1 => -1,
        2 => 1,
        3 => i128::from(i64::MAX),
        4 => i128::from(i64::MIN),
        5 => rng.below(512) as i128, // a small non-negative (len-bound-ish)
        6 => -(rng.below(512) as i128), // a small negative
        _ => rng.next_u64() as i128 - (1i128 << 63), // a wide spread
    }
}

/// A canonical int beyond 128 bits (17 to 24 magnitude bytes), of either sign (ADR 0010).
fn wide_int(rng: &mut Rng) -> Value {
    let mut payload = vec![u8::from(rng.one_in(2))];
    payload.push(rng.range(1, 255) as u8);
    for _ in 0..rng.range(16, 23) {
        payload.push(rng.below(256) as u8);
    }
    tup(vec![atom(b"int"), atom(&payload)])
}

/// An int-min/int-max bound: mostly in range, sometimes 2^127 (non-wf, past i128).
fn rand_int_bound(rng: &mut Rng) -> Value {
    if rng.one_in(8) {
        int_two_pow_127()
    } else {
        encode_int(rand_int(rng))
    }
}

/// A random GROUND value (an ordinary ontos value, not a descriptor), depth-bounded so the
/// encoded form stays well under the codec's `max_depth`. Mixes atoms, raw tuples, and the
/// ontos-data embeddings (int/text/bool/list/map/set), incl. the empty-collection edges.
fn rand_value(rng: &mut Rng, depth: usize) -> Value {
    if depth == 0 || rng.one_in(2) {
        // a leaf
        return match rng.below(8) {
            0 => atom(&rand_atom_bytes(rng)),
            1 => encode_int(rand_int(rng)),
            2 => encode_text(&rand_text(rng)),
            3 => encode_bool(rng.one_in(2)),
            4 => atom(b"nil"),
            5 => tup(vec![]), // the empty tuple
            6 => wide_int(rng),
            _ => encode_list(&[]), // the empty list
        };
    }
    match rng.below(8) {
        0 => {
            // a raw tuple of children
            let n = rng.range(0, 4);
            tup((0..n).map(|_| rand_value(rng, depth - 1)).collect())
        }
        1 => {
            // a compound-shaped tuple: Atom(label) then children
            let n = rng.range(0, 3);
            let mut items = vec![atom(&rand_atom_bytes(rng))];
            items.extend((0..n).map(|_| rand_value(rng, depth - 1)));
            tup(items)
        }
        2 => {
            let n = rng.range(0, 4);
            encode_list(
                &(0..n)
                    .map(|_| rand_value(rng, depth - 1))
                    .collect::<Vec<_>>(),
            )
        }
        3 => {
            // a set — encode_set may fail on duplicate/unsorted; fall back to an atom then.
            let n = rng.range(0, 3);
            let elems: Vec<Value> = (0..n).map(|_| rand_value(rng, depth - 1)).collect();
            encode_set(&elems).unwrap_or_else(|_| atom(b"set-fallback"))
        }
        4 => {
            // a map keyed by text (the record/map-of shape).
            let n = rng.range(0, 3);
            let entries: Vec<(Value, Value)> = (0..n)
                .map(|i| (encode_text(&format!("k{i}")), rand_value(rng, depth - 1)))
                .collect();
            encode_map(&entries).unwrap_or_else(|_| atom(b"map-fallback"))
        }
        5 => {
            // a cons cell `(int, child)` — the recursive-descriptor shape.
            tup(vec![encode_int(rand_int(rng)), rand_value(rng, depth - 1)])
        }
        6 => {
            // a node `(Atom("node"), child, child)` — the int-tree shape.
            tup(vec![
                atom(b"node"),
                rand_value(rng, depth - 1),
                rand_value(rng, depth - 1),
            ])
        }
        _ => atom(&rand_atom_bytes(rng)),
    }
}

/// A random text string, occasionally multi-byte, occasionally empty.
fn rand_text(rng: &mut Rng) -> String {
    match rng.below(5) {
        0 => String::new(),
        1 => "héllo".to_string(),
        2 => "→世界🌍".to_string(),
        3 => "ada".to_string(),
        _ => {
            let len = rng.range(1, 6);
            (0..len)
                .map(|_| (b'a' + rng.below(26) as u8) as char)
                .collect()
        }
    }
}

// ===========================================================================
// descriptor generation — the full v1 DSL, well-formed AND deliberately broken
// ===========================================================================

/// A random scalar-kind atom. Mostly the three valid kinds, occasionally a bogus one
/// (which makes `scalar` non-wf) so the wf-rejection lane is exercised.
fn rand_scalar_kind(rng: &mut Rng) -> Value {
    match rng.below(8) {
        0 => atom(b"float"), // non-wf kind
        1 | 2 => atom(b"utf8-text"),
        3 | 4 => atom(b"int"),
        5 | 6 => atom(b"bool"),
        _ => atom(&rand_atom_bytes(rng)), // possibly bogus
    }
}

/// A field clause for `record`: `field(text(name), Atom(req|opt), T)`. Occasionally emits
/// a malformed presence flag so the record-wf lane sees rejections too.
fn rand_field(rng: &mut Rng, depth: usize) -> Value {
    let presence = match rng.below(6) {
        0 => atom(b"maybe"), // bogus presence → non-wf field
        1 | 2 => atom(b"req"),
        _ => atom(b"opt"),
    };
    desc(
        "field",
        vec![
            encode_text(&rand_text(rng)),
            presence,
            rand_descriptor(rng, depth),
        ],
    )
}

/// A random horos descriptor, depth-bounded. Spans every v1 combinator and intentionally
/// produces both well-formed and non-well-formed shapes (bad scalar kinds, lo>hi atom-len,
/// duplicate record fields, unbound/unguarded refs, garbage labels). No judgment is
/// computed — agreement of rs/go/ts on whatever it is IS the test.
fn rand_descriptor(rng: &mut Rng, depth: usize) -> Value {
    if depth == 0 {
        // a leaf descriptor (no further descent)
        return match rng.below(8) {
            0 => desc("any", vec![]),
            1 => desc("atom", vec![]),
            2 => desc("atom-exact", vec![atom(&rand_atom_bytes(rng))]),
            3 => desc("scalar", vec![rand_scalar_kind(rng)]),
            4 => desc("exact", vec![rand_value(rng, 2)]),
            5 => desc("int-min", vec![rand_int_bound(rng)]),
            6 => desc("int-max", vec![rand_int_bound(rng)]),
            _ => desc("ref", vec![atom(b"x")]), // unbound at a leaf → non-wf
        };
    }
    match rng.below(22) {
        0 => desc("any", vec![]),
        1 => desc("atom", vec![]),
        2 => desc("atom-exact", vec![atom(&rand_atom_bytes(rng))]),
        3 => {
            // atom-len — bounds drawn to straddle wf (0<=lo<=hi) and non-wf (lo>hi, neg lo).
            let lo = rand_int(rng);
            let hi = rand_int(rng);
            desc("atom-len", vec![encode_int(lo), encode_int(hi)])
        }
        4 => {
            // atom-in — a set of (mostly) atoms; occasionally a non-atom element → non-wf.
            let n = rng.range(0, 4);
            let mut elems: Vec<Value> = (0..n).map(|_| atom(&rand_atom_bytes(rng))).collect();
            if rng.one_in(5) {
                elems.push(tup(vec![atom(b"t")])); // non-atom element → non-wf
            }
            let set_v = encode_set(&elems).unwrap_or_else(|_| encode_set(&[]).unwrap());
            desc("atom-in", vec![set_v])
        }
        5 => desc("scalar", vec![rand_scalar_kind(rng)]),
        6 => {
            let n = rng.range(0, 4);
            desc(
                "tuple",
                (0..n).map(|_| rand_descriptor(rng, depth - 1)).collect(),
            )
        }
        7 => desc("seq", vec![rand_descriptor(rng, depth - 1)]),
        8 => {
            // compound: Atom(label) then child descriptors.
            let n = rng.range(0, 3);
            let mut args = vec![atom(&rand_atom_bytes(rng))];
            args.extend((0..n).map(|_| rand_descriptor(rng, depth - 1)));
            desc("compound", args)
        }
        9 => desc("list-of", vec![rand_descriptor(rng, depth - 1)]),
        10 => desc("set-of", vec![rand_descriptor(rng, depth - 1)]),
        11 => desc(
            "map-of",
            vec![
                rand_descriptor(rng, depth - 1),
                rand_descriptor(rng, depth - 1),
            ],
        ),
        12 => {
            // record: a mode flag (mostly open|closed, sometimes bogus) then fields. May
            // emit a duplicate field name → non-wf.
            let mode = match rng.below(6) {
                0 => atom(b"maybe"), // bogus mode → non-wf
                1..=3 => atom(b"open"),
                _ => atom(b"closed"),
            };
            let n = rng.range(0, 3);
            let mut args = vec![mode];
            args.extend((0..n).map(|_| rand_field(rng, depth - 1)));
            if rng.one_in(6) && n > 0 {
                // force a duplicate field name → non-wf
                args.push(desc(
                    "field",
                    vec![encode_text("dup"), atom(b"req"), desc("any", vec![])],
                ));
                args.push(desc(
                    "field",
                    vec![encode_text("dup"), atom(b"opt"), desc("any", vec![])],
                ));
            }
            desc("record", args)
        }
        13 => {
            // union — incl. the empty-arity (n=0) edge (bottom).
            let n = rng.range(0, 3);
            desc(
                "union",
                (0..n).map(|_| rand_descriptor(rng, depth - 1)).collect(),
            )
        }
        14 => {
            // and — incl. the empty-arity (n=0) edge (top).
            let n = rng.range(0, 3);
            desc(
                "and",
                (0..n).map(|_| rand_descriptor(rng, depth - 1)).collect(),
            )
        }
        15 => desc("not", vec![rand_descriptor(rng, depth - 1)]),
        16 => desc("exact", vec![rand_value(rng, 2)]),
        17 => {
            // a GUARDED rec (cons-list shape): rec x. union(atom-exact "nil", tuple(child, ref x)).
            let body = desc(
                "union",
                vec![
                    desc("atom-exact", vec![atom(b"nil")]),
                    desc(
                        "tuple",
                        vec![
                            rand_descriptor(rng, depth - 1),
                            desc("ref", vec![atom(b"x")]),
                        ],
                    ),
                ],
            );
            desc("rec", vec![atom(b"x"), body])
        }
        18 => {
            // an UNGUARDED rec: rec x. ref x → non-wf.
            desc("rec", vec![atom(b"x"), desc("ref", vec![atom(b"x")])])
        }
        19 => {
            // an unbound ref → non-wf.
            desc("ref", vec![atom(b"y")])
        }
        20 => {
            // a garbage tuple under no reserved label — not a descriptor at all → non-wf.
            tup(vec![atom(&rand_atom_bytes(rng)), rand_value(rng, 1)])
        }
        _ => {
            // a non-UTF-8 label under the reserved prefix → not a descriptor (non-wf).
            tup(vec![atom(b"horos/\xff"), atom(b"x")])
        }
    }
}

// ===========================================================================
// boundary generators — the explicit edges the issue calls out
// ===========================================================================

/// A right-nested int cons value `(0, (0, … nil))`, `levels` deep, built ITERATIVELY (no
/// build-time recursion). Paired with the cons-list descriptor it straddles horos's
/// `MAX_RECURSION_DEPTH` (512) — the recursion-depth guardrail trips at the same logical
/// frame in all three cores.
fn deep_cons_value(levels: usize) -> Value {
    let mut v = atom(b"nil");
    for _ in 0..levels {
        v = tup(vec![encode_int(0), v]);
    }
    v
}

/// The guarded cons-list descriptor `rec l. (atom-exact "nil") | (int, ref l)`.
fn cons_list_descriptor() -> Value {
    desc(
        "rec",
        vec![
            atom(b"l"),
            desc(
                "union",
                vec![
                    desc("atom-exact", vec![atom(b"nil")]),
                    desc(
                        "tuple",
                        vec![
                            desc("scalar", vec![atom(b"int")]),
                            desc("ref", vec![atom(b"l")]),
                        ],
                    ),
                ],
            ),
        ],
    )
}

/// The ontos `int` value `2^127` (one past `i128::MAX`) — canonical bytes but
/// non-materializable as `i128`, the `atom-len` cap boundary. Built by hand.
fn int_two_pow_127() -> Value {
    let mut payload = vec![0x00u8, 0x80];
    payload.extend(std::iter::repeat_n(0x00u8, 15));
    tup(vec![atom(b"int"), atom(&payload)])
}

/// Emit a BOUNDARY pair: the explicit edges from horos-internal#72 — deep nesting around the 512 cap,
/// empty arities, near-cap atom-len bounds, multi-byte text. Returns `(descriptor, value)`.
fn boundary_pair(rng: &mut Rng) -> (Value, Value) {
    match rng.below(8) {
        0 => {
            // deep nesting straddling MAX_RECURSION_DEPTH (512): a value just below/above
            // the cap, over the small cons-list descriptor. Kept < codec max_depth (1024).
            let levels = rng.range(160, 600);
            (cons_list_descriptor(), deep_cons_value(levels))
        }
        1 => {
            // an atom-len bound AT the i128 cap (2^127) → non-wf in all three cores.
            let d = desc("atom-len", vec![encode_int(0), int_two_pow_127()]);
            (d, atom(&rand_atom_bytes(rng)))
        }
        2 => {
            // near-cap atom-len: lo/hi at i64/i128 extremes, value an empty/short atom.
            let lo = if rng.one_in(2) {
                0
            } else {
                i128::from(i64::MAX)
            };
            let hi = i128::MAX;
            let d = desc("atom-len", vec![encode_int(lo), encode_int(hi)]);
            (d, atom(&rand_atom_bytes(rng)))
        }
        3 => {
            // empty-arity union (bottom) over an arbitrary value.
            (desc("union", vec![]), rand_value(rng, 3))
        }
        4 => {
            // empty-arity and (top) over an arbitrary value.
            (desc("and", vec![]), rand_value(rng, 3))
        }
        5 => {
            // empty-arity tuple/seq edges over the empty tuple.
            let d = if rng.one_in(2) {
                desc("tuple", vec![])
            } else {
                desc("seq", vec![desc("any", vec![])])
            };
            (d, tup(vec![]))
        }
        6 => {
            // near-cap atom length: an atom whose byte length sits at a small exact bound,
            // checked against multi-byte UTF-8 content.
            let s = "→世界🌍";
            let len = s.len() as i128;
            let d = desc("atom-len", vec![encode_int(len), encode_int(len)]);
            (d, atom(s.as_bytes()))
        }
        _ => {
            // multi-byte text against scalar(utf8-text) and against atom-exact.
            let s = "héllo→世界🌍";
            let d = if rng.one_in(2) {
                desc("scalar", vec![atom(b"utf8-text")])
            } else {
                desc("atom-exact", vec![atom(s.as_bytes())])
            };
            let v = if rng.one_in(2) {
                encode_text(s)
            } else {
                atom(s.as_bytes())
            };
            (d, v)
        }
    }
}

// ===========================================================================
// scoping — recs that reuse names, refs under shadows, values that unfold them
// ===========================================================================
//
// ADR 0011's defect survived because the generator above binds one name per descriptor:
// a `ref` never met a shadowing `rec`, so no reading of scope could differ. This family
// draws `rec` names from a pool of three, builds capture skeletons on purpose (a body
// with a free `ref` unfolded inside an inner `rec` of the same name), and derives values
// from the descriptor so unfoldings are actually taken, often to a conforming end.

/// The names scoping descriptors bind: few, so nested `rec`s repeat them.
const NAMES: [&[u8]; 3] = [b"M", b"L", b"K"];

/// Lexical scope while generating: each enclosing `rec`'s name, and whether the value
/// has descended since it was entered (a `ref` to it is guarded only then).
#[derive(Clone, Default)]
struct GenScope {
    bindings: Vec<(&'static [u8], bool)>,
}

impl GenScope {
    fn enter(&self, name: &'static [u8]) -> GenScope {
        let mut s = self.clone();
        s.bindings.push((name, false));
        s
    }

    fn descend(&self) -> GenScope {
        let mut s = self.clone();
        for b in &mut s.bindings {
            b.1 = true;
        }
        s
    }

    /// Names whose innermost binding a `ref` here would reach guarded.
    fn guarded(&self) -> Vec<&'static [u8]> {
        NAMES
            .iter()
            .copied()
            .filter(|n| {
                self.bindings
                    .iter()
                    .rev()
                    .find(|(m, _)| m == n)
                    .is_some_and(|(_, g)| *g)
            })
            .collect()
    }

    fn bound(&self) -> Vec<&'static [u8]> {
        NAMES
            .iter()
            .copied()
            .filter(|n| self.bindings.iter().any(|(m, _)| m == n))
            .collect()
    }
}

fn pick<T: Copy>(rng: &mut Rng, xs: &[T]) -> T {
    xs[rng.below(xs.len() as u64) as usize]
}

fn rec_d(name: &[u8], body: Value) -> Value {
    desc("rec", vec![atom(name), body])
}

fn ref_d(name: &[u8]) -> Value {
    desc("ref", vec![atom(name)])
}

/// A leaf that ends an unfolding: a small exact atom, an int, any atom.
fn base_leaf(rng: &mut Rng) -> Value {
    match rng.below(6) {
        0 | 1 => desc(
            "exact",
            vec![atom(pick(rng, &[b"a".as_slice(), b"b", b"c", b"nil"]))],
        ),
        2 => desc(
            "atom-exact",
            vec![atom(pick(rng, &[b"a".as_slice(), b"b", b"nil"]))],
        ),
        3 => desc("scalar", vec![atom(b"int")]),
        4 => desc("atom", vec![]),
        _ => desc("exact", vec![encode_int(rng.below(3) as i128)]),
    }
}

/// A structural wrapper around `children`, so a `ref` inside is guarded: a tuple with an
/// occasional extra sibling, a compound, or for one child a list, a record field or a map
/// value.
fn guard(rng: &mut Rng, mut children: Vec<Value>) -> Value {
    let single = children.len() == 1;
    match rng.below(if single { 6 } else { 3 }) {
        0 => {
            if rng.one_in(3) {
                let at = rng.below(children.len() as u64 + 1) as usize;
                children.insert(at, base_leaf(rng));
            }
            desc("tuple", children)
        }
        1 | 2 => {
            let mut args = vec![atom(pick(rng, &[b"m".as_slice(), b"l", b"k"]))];
            args.append(&mut children);
            desc("compound", args)
        }
        3 => desc("list-of", children),
        4 => desc(
            "record",
            vec![
                atom(if rng.one_in(2) {
                    b"open".as_slice()
                } else {
                    b"closed"
                }),
                desc(
                    "field",
                    vec![
                        encode_text("f"),
                        atom(if rng.one_in(3) {
                            b"opt".as_slice()
                        } else {
                            b"req"
                        }),
                        children.remove(0),
                    ],
                ),
            ],
        ),
        _ => desc(
            "map-of",
            vec![desc("scalar", vec![atom(b"utf8-text")]), children.remove(0)],
        ),
    }
}

/// `union` of `alt` with a base case, in either order.
fn with_base(rng: &mut Rng, alt: Value) -> Value {
    let base = base_leaf(rng);
    if rng.one_in(2) {
        desc("union", vec![base, alt])
    } else {
        desc("union", vec![alt, base])
    }
}

/// A descriptor that is capture-prone by construction (ADR 0011's class): `rec n` around
/// `rec r`, whose body holds a free `ref n` and an inner `rec n` around a `ref r`. Read
/// dynamically, unfolding `ref r` under the inner `n` rebinds the free `ref n`.
fn capture_skeleton(rng: &mut Rng) -> Value {
    let n = pick(rng, &NAMES);
    let r = loop {
        let r = pick(rng, &NAMES);
        if r != n {
            break r;
        }
    };
    let inner = match rng.below(3) {
        // The reproducer's shape: the free ref and the inner rec side by side.
        0 => {
            let g = guard(rng, vec![ref_d(r)]);
            let inner_n = rec_d(n, with_base(rng, g));
            guard(rng, vec![ref_d(n), inner_n])
        }
        // The loop shape: the free ref an alternative at the inner rec's own value.
        1 => {
            let inner_n = rec_d(n, with_base(rng, ref_d(r)));
            desc("union", vec![ref_d(n), guard(rng, vec![inner_n])])
        }
        // Three deep: a third name between them, unfolded under the innermost `n` too.
        _ => {
            let k = NAMES.iter().copied().find(|x| *x != n && *x != r).unwrap();
            let g = guard(rng, vec![ref_d(r), ref_d(k)]);
            let innermost = rec_d(n, with_base(rng, g));
            let middle = rec_d(k, guard(rng, vec![ref_d(n), innermost]));
            let inner_n = rec_d(n, with_base(rng, middle));
            guard(rng, vec![ref_d(n), inner_n])
        }
    };
    let body_r = if rng.one_in(3) {
        with_base(rng, inner)
    } else {
        inner
    };
    let g = guard(rng, vec![rec_d(r, body_r)]);
    rec_d(n, with_base(rng, g))
}

/// A descriptor over the name pool: `rec`s that often rebind a name already in scope,
/// `ref`s to any binding in reach (mostly guarded, sometimes not), structural guards and
/// unions with base cases.
fn scoped_descriptor(rng: &mut Rng, depth: usize, sc: &GenScope) -> Value {
    let guarded = sc.guarded();
    if depth == 0 {
        return if !guarded.is_empty() && rng.one_in(2) {
            ref_d(pick(rng, &guarded))
        } else {
            base_leaf(rng)
        };
    }
    match rng.below(12) {
        0..=2 => {
            let bound = sc.bound();
            let name = if !bound.is_empty() && rng.below(3) < 2 {
                pick(rng, &bound)
            } else {
                pick(rng, &NAMES)
            };
            let body = scoped_descriptor(rng, depth - 1, &sc.enter(name));
            rec_d(
                name,
                if rng.one_in(4) {
                    body
                } else {
                    with_base(rng, body)
                },
            )
        }
        3 | 4 => {
            if !guarded.is_empty() && !rng.one_in(10) {
                ref_d(pick(rng, &guarded))
            } else {
                // Unguarded or unbound: a malformed probe.
                ref_d(pick(rng, &NAMES))
            }
        }
        5 | 6 => {
            let n = rng.range(1, 3);
            desc(
                "union",
                (0..n)
                    .map(|_| scoped_descriptor(rng, depth - 1, sc))
                    .collect(),
            )
        }
        7 => desc(
            "and",
            vec![scoped_descriptor(rng, depth - 1, sc), desc("any", vec![])],
        ),
        8 => desc("not", vec![scoped_descriptor(rng, depth - 1, sc)]),
        9 | 10 => {
            let n = rng.range(1, 3);
            let inner = sc.descend();
            let children = (0..n)
                .map(|_| scoped_descriptor(rng, depth - 1, &inner))
                .collect();
            guard(rng, children)
        }
        _ => base_leaf(rng),
    }
}

/// The descriptor's constructor and arguments, if it is one.
fn head(t: &Value) -> Option<(&str, &[Value])> {
    let items = t.as_tuple()?.items();
    let label = std::str::from_utf8(items.first()?.as_atom()?.bytes()).ok()?;
    Some((label.strip_prefix(NAMESPACE)?, &items[1..]))
}

/// A binding while deriving values: name, body, and the environment it was written in.
struct Bind {
    name: Vec<u8>,
    body: Value,
    parent: Option<Rc<Bind>>,
}

fn lookup(env: &Option<Rc<Bind>>, name: &[u8]) -> Option<Rc<Bind>> {
    let mut node = env.clone();
    while let Some(b) = node {
        if b.name == name {
            return Some(b);
        }
        node = b.parent.clone();
    }
    None
}

/// How derived values resolve a `ref`: lexically (the contract), or in the unfolding
/// site's environment, as the checkers before ADR 0011 did.
#[derive(Clone, Copy, PartialEq)]
enum Reading {
    Lexical,
    Dynamic,
}

fn has_ref(t: &Value) -> bool {
    match head(t) {
        Some(("ref", _)) => true,
        Some(("exact" | "atom-exact" | "atom-in", _)) => false,
        Some((_, args)) => args.iter().any(has_ref),
        None => false,
    }
}

/// How deep a derived value nests, which keeps each seed's run time predictable. Deep
/// values still come from the boundary pairs.
const MAX_DERIVED_DEPTH: usize = 12;

/// A value shaped by `t`, aiming to conform under `reading`: unions pick an alternative
/// (a `ref`-free one once `budget` runs low), `ref`s unfold, collections hold derived
/// elements. A rare random subtree turns it into a near miss. Not a judgment: whatever
/// the checkers answer is compared, never this intent.
fn derive(
    rng: &mut Rng,
    t: &Value,
    env: &Option<Rc<Bind>>,
    budget: &mut i32,
    reading: Reading,
    depth: usize,
) -> Value {
    *budget -= 1;
    if depth > MAX_DERIVED_DEPTH {
        return atom(b"deep");
    }
    if rng.one_in(80) {
        return rand_value(rng, 1);
    }
    let Some((ctor, args)) = head(t) else {
        return rand_value(rng, 1);
    };
    let kids = |rng: &mut Rng, budget: &mut i32, ts: &[Value]| -> Vec<Value> {
        ts.iter()
            .map(|c| derive(rng, c, env, budget, reading, depth + 1))
            .collect()
    };
    match (ctor, args) {
        ("exact", [v]) | ("atom-exact", [v]) => v.clone(),
        ("atom-in", [set]) => match ontos_data::read_set(set) {
            Ok(elems) if !elems.is_empty() => pick(rng, &elems.iter().collect::<Vec<_>>()).clone(),
            _ => atom(&rand_atom_bytes(rng)),
        },
        ("atom-len", [lo, _]) => match ontos_data::read_int(lo) {
            Ok(n) if (0..=16).contains(&n) => atom(&vec![b'a'; n as usize + rng.below(2) as usize]),
            _ => atom(&rand_atom_bytes(rng)),
        },
        ("scalar", [k]) => match k.as_atom().map(|a| a.bytes()) {
            Some(b"int") => encode_int(rand_int(rng)),
            Some(b"utf8-text") => encode_text(&rand_text(rng)),
            Some(b"bool") => encode_bool(rng.one_in(2)),
            _ => rand_value(rng, 1),
        },
        ("int-min", [b]) | ("int-max", [b]) => match ontos_data::read_int(b) {
            Ok(n) => {
                let step = rng.below(3) as i128;
                encode_int(if ctor == "int-min" {
                    n.saturating_add(step)
                } else {
                    n.saturating_sub(step)
                })
            }
            Err(_) => encode_int(rand_int(rng)),
        },
        ("tuple", ts) => tup(kids(rng, budget, ts)),
        ("seq", [c]) => {
            let n = if *budget > 0 { rng.range(0, 3) } else { 0 };
            tup((0..n)
                .map(|_| derive(rng, c, env, budget, reading, depth + 1))
                .collect())
        }
        ("compound", [label, ts @ ..]) => {
            let mut items = vec![label.clone()];
            items.extend(kids(rng, budget, ts));
            tup(items)
        }
        ("list-of", [c]) => {
            let n = if *budget > 0 { rng.range(0, 3) } else { 0 };
            encode_list(
                &(0..n)
                    .map(|_| derive(rng, c, env, budget, reading, depth + 1))
                    .collect::<Vec<_>>(),
            )
        }
        ("set-of", [c]) => {
            let n = if *budget > 0 { rng.range(0, 3) } else { 0 };
            let mut elems: Vec<Value> = Vec::new();
            for _ in 0..n {
                let e = derive(rng, c, env, budget, reading, depth + 1);
                if !elems.contains(&e) {
                    elems.push(e);
                }
            }
            encode_set(&elems).unwrap_or_else(|_| atom(b"set-fallback"))
        }
        ("map-of", [k, v]) => {
            let n = if *budget > 0 { rng.range(0, 3) } else { 0 };
            let mut entries: Vec<(Value, Value)> = Vec::new();
            for _ in 0..n {
                let key = derive(rng, k, env, budget, reading, depth + 1);
                if entries.iter().all(|(seen, _)| *seen != key) {
                    let value = derive(rng, v, env, budget, reading, depth + 1);
                    entries.push((key, value));
                }
            }
            encode_map(&entries).unwrap_or_else(|_| atom(b"map-fallback"))
        }
        ("record", [mode, fields @ ..]) => {
            let mut entries: Vec<(Value, Value)> = Vec::new();
            for f in fields {
                let Some(("field", [name, flag, ty])) = head(f) else {
                    continue;
                };
                let required = flag.as_atom().is_some_and(|a| a.bytes() == b"req");
                if (required || rng.one_in(2)) && entries.iter().all(|(seen, _)| seen != name) {
                    let value = derive(rng, ty, env, budget, reading, depth + 1);
                    entries.push((name.clone(), value));
                }
            }
            if mode.as_atom().is_some_and(|a| a.bytes() == b"open") && rng.one_in(4) {
                let extra = encode_text("extra");
                if entries.iter().all(|(seen, _)| *seen != extra) {
                    entries.push((extra, rand_value(rng, 1)));
                }
            }
            encode_map(&entries).unwrap_or_else(|_| atom(b"map-fallback"))
        }
        ("union", alts) if !alts.is_empty() => {
            // While budget lasts, mostly take an alternative that unfolds further; then
            // one that ends.
            let (unfolding, ending): (Vec<&Value>, Vec<&Value>) =
                alts.iter().partition(|a| has_ref(a));
            let alt = if *budget > 0 && !unfolding.is_empty() && rng.below(4) < 3 {
                pick(rng, &unfolding)
            } else if *budget <= 0 && !ending.is_empty() {
                pick(rng, &ending)
            } else {
                pick(rng, &alts.iter().collect::<Vec<_>>())
            };
            derive(rng, alt, env, budget, reading, depth)
        }
        ("and", [first, ..]) => derive(rng, first, env, budget, reading, depth),
        ("rec", [name, body]) => {
            let Some(n) = name.as_atom() else {
                return rand_value(rng, 1);
            };
            let bind = Rc::new(Bind {
                name: n.bytes().to_vec(),
                body: body.clone(),
                parent: env.clone(),
            });
            derive(rng, body, &Some(bind), budget, reading, depth)
        }
        ("ref", [name]) => {
            let found = name.as_atom().and_then(|n| lookup(env, n.bytes()));
            match found {
                Some(b) if *budget > -40 => {
                    let unfold_env = match reading {
                        Reading::Lexical => Some(b.clone()),
                        Reading::Dynamic => env.clone(),
                    };
                    derive(rng, &b.body, &unfold_env, budget, reading, depth)
                }
                _ => rand_value(rng, 1),
            }
        }
        _ => rand_value(rng, 1),
    }
}

/// Static scoping facts about a descriptor, for the report: whether a `rec` rebinds a
/// name an enclosing `rec` binds (shadowing), and whether a `ref` unfolds a body with a
/// free `ref` to some name inside an inner `rec` of that name (capture-prone: the only
/// shape in which dynamic and lexical readings can differ).
#[derive(Default)]
struct ScopeFacts {
    shadowing: bool,
    capture: bool,
}

/// Child descriptors of a descriptor node (never the inside of a literal).
fn children(t: &Value) -> Vec<&Value> {
    match head(t) {
        Some((
            "exact" | "atom-exact" | "atom-in" | "atom-len" | "scalar" | "int-min" | "int-max"
            | "ref",
            _,
        ))
        | None => vec![],
        Some(("compound", [_, ts @ ..])) => ts.iter().collect(),
        Some(("record", [_, fields @ ..])) => fields
            .iter()
            .filter_map(|f| match head(f) {
                Some(("field", [_, _, ty])) => Some(ty),
                _ => None,
            })
            .collect(),
        Some(("rec", [_, body])) => vec![body],
        Some((_, args)) => args.iter().collect(),
    }
}

fn scope_facts(t: &Value) -> ScopeFacts {
    // Pass 1: number the recs in preorder and collect each one's free ref names.
    fn free_names(
        t: &Value,
        stack: &mut Vec<(Vec<u8>, usize)>,
        free: &mut Vec<Vec<Vec<u8>>>,
        facts: &mut ScopeFacts,
    ) {
        match head(t) {
            Some(("rec", [name, body])) => {
                let Some(n) = name.as_atom() else { return };
                if stack.iter().any(|(m, _)| m == n.bytes()) {
                    facts.shadowing = true;
                }
                free.push(Vec::new());
                stack.push((n.bytes().to_vec(), free.len() - 1));
                free_names(body, stack, free, facts);
                stack.pop();
            }
            Some(("ref", [name])) => {
                let Some(n) = name.as_atom() else { return };
                if let Some(bi) = stack.iter().rposition(|(m, _)| m == n.bytes()) {
                    for (_, id) in &stack[bi + 1..] {
                        free[*id].push(n.bytes().to_vec());
                    }
                }
            }
            _ => {
                for c in children(t) {
                    free_names(c, stack, free, facts);
                }
            }
        }
    }
    // Pass 2: a ref whose binder's body has a free name that a rec between them rebinds.
    fn captured(
        t: &Value,
        stack: &mut Vec<(Vec<u8>, usize)>,
        next: &mut usize,
        free: &[Vec<Vec<u8>>],
    ) -> bool {
        match head(t) {
            Some(("rec", [name, body])) => {
                let Some(n) = name.as_atom() else {
                    return false;
                };
                stack.push((n.bytes().to_vec(), *next));
                *next += 1;
                let hit = captured(body, stack, next, free);
                stack.pop();
                hit
            }
            Some(("ref", [name])) => {
                let Some(n) = name.as_atom() else {
                    return false;
                };
                let Some(bi) = stack.iter().rposition(|(m, _)| m == n.bytes()) else {
                    return false;
                };
                let binder = stack[bi].1;
                stack[bi + 1..]
                    .iter()
                    .any(|(m, _)| free[binder].contains(m))
            }
            _ => children(t)
                .into_iter()
                .any(|c| captured(c, stack, next, free)),
        }
    }
    let mut facts = ScopeFacts::default();
    let mut free = Vec::new();
    free_names(t, &mut Vec::new(), &mut free, &mut facts);
    facts.capture = captured(t, &mut Vec::new(), &mut 0, &free);
    facts
}

/// One pair of the scoping family: a capture skeleton or a pool descriptor, and a value
/// derived from it (lexically mostly, dynamically often, sometimes at random).
fn scoping_pair(rng: &mut Rng) -> (Value, Value) {
    let d = if rng.below(5) < 3 {
        capture_skeleton(rng)
    } else {
        let depth = rng.range(2, 5);
        scoped_descriptor(rng, depth, &GenScope::default())
    };
    let mut budget = pick(rng, &[8, 24, 60, 120]);
    let v = match rng.below(10) {
        0..=5 => derive(rng, &d, &None, &mut budget, Reading::Lexical, 0),
        6..=8 => derive(rng, &d, &None, &mut budget, Reading::Dynamic, 0),
        _ => rand_value(rng, 3),
    };
    (d, v)
}

// ===========================================================================
// driver
// ===========================================================================

fn to_hex(bytes: &[u8]) -> String {
    let mut s = String::with_capacity(bytes.len() * 2);
    for b in bytes {
        s.push_str(&format!("{b:02x}"));
    }
    s
}

/// Generate one `(descriptor, value)` pair: half from the scoping family, a quarter
/// targeted boundary pairs, a quarter a random descriptor with a value that is random or
/// derived from it (so conformance is often `true` as well as `false`).
fn gen_pair(rng: &mut Rng) -> (Value, Value) {
    match rng.below(4) {
        0 | 1 => scoping_pair(rng),
        2 => boundary_pair(rng),
        _ => {
            let depth = rng.range(1, 4);
            let d = rand_descriptor(rng, depth);
            let v = if rng.one_in(2) {
                let mut budget = 24;
                derive(rng, &d, &None, &mut budget, Reading::Lexical, 0)
            } else {
                let depth = rng.range(1, 5);
                rand_value(rng, depth)
            };
            (d, v)
        }
    }
}

/// Parse `--seed N --count M` (or positional `N M`). Defaults: seed 1, count 1000.
fn parse_args(args: &[String]) -> Result<(u64, usize), String> {
    let mut seed: u64 = 1;
    let mut count: usize = 1000;
    let mut positional: Vec<&String> = Vec::new();
    let mut i = 0;
    while i < args.len() {
        match args[i].as_str() {
            "--seed" => {
                i += 1;
                seed = args
                    .get(i)
                    .ok_or("--seed needs a value")?
                    .parse()
                    .map_err(|e| format!("bad --seed: {e}"))?;
            }
            "--count" | "-n" => {
                i += 1;
                count = args
                    .get(i)
                    .ok_or("--count needs a value")?
                    .parse()
                    .map_err(|e| format!("bad --count: {e}"))?;
            }
            _ => positional.push(&args[i]),
        }
        i += 1;
    }
    if let Some(s) = positional.first() {
        seed = s.parse().map_err(|e| format!("bad seed: {e}"))?;
    }
    if let Some(c) = positional.get(1) {
        count = c.parse().map_err(|e| format!("bad count: {e}"))?;
    }
    Ok((seed, count))
}

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let (seed, count) = match parse_args(&args) {
        Ok(v) => v,
        Err(msg) => {
            eprintln!("horos-fuzz: {msg}");
            eprintln!(
                "usage: horos-fuzz --seed <u64> --count <n>   (or: horos-fuzz <seed> <count>)"
            );
            std::process::exit(2);
        }
    };

    let mut rng = Rng::new(seed);
    let stdout = std::io::stdout();
    let mut out = std::io::BufWriter::new(stdout.lock());
    use std::io::Write;
    let (mut shadowing, mut capture) = (0usize, 0usize);
    for _ in 0..count {
        let (d, v) = gen_pair(&mut rng);
        let facts = scope_facts(&d);
        shadowing += usize::from(facts.shadowing);
        capture += usize::from(facts.capture);
        let line = format!("{} {}\n", to_hex(&encode(&d)), to_hex(&encode(&v)));
        // A broken pipe (downstream closed) is not an error worth a nonzero exit.
        if out.write_all(line.as_bytes()).is_err() {
            return;
        }
    }
    let _ = out.flush();
    // The scoping report goes to stderr, so stdout stays one pair per line.
    let share = |n: usize| {
        if count == 0 {
            0.0
        } else {
            100.0 * n as f64 / count as f64
        }
    };
    eprintln!(
        "horos-fuzz: seed={seed} count={count} shadowing={shadowing} ({:.1}%) capture-prone={capture} ({:.1}%)",
        share(shadowing),
        share(capture)
    );
}

#[cfg(test)]
mod tests {
    use super::*;

    fn exact_d(s: &[u8]) -> Value {
        desc("exact", vec![atom(s)])
    }

    /// ADR 0011's reproducer: L's free ref(M) is unfolded under the inner M.
    fn reproducer() -> Value {
        rec_d(
            b"M",
            desc(
                "union",
                vec![
                    exact_d(b"a"),
                    rec_d(
                        b"L",
                        desc(
                            "tuple",
                            vec![
                                ref_d(b"M"),
                                rec_d(
                                    b"M",
                                    desc(
                                        "union",
                                        vec![exact_d(b"b"), desc("tuple", vec![ref_d(b"L")])],
                                    ),
                                ),
                            ],
                        ),
                    ),
                ],
            ),
        )
    }

    #[test]
    fn the_reproducer_is_capture_prone() {
        let facts = scope_facts(&reproducer());
        assert!(facts.shadowing && facts.capture);
    }

    #[test]
    fn renaming_the_inner_binding_removes_the_capture() {
        // The corpus's reproducer-renamed: nothing shadows, so readings cannot differ.
        let t = rec_d(
            b"M",
            desc(
                "union",
                vec![
                    exact_d(b"a"),
                    rec_d(
                        b"L",
                        desc(
                            "tuple",
                            vec![
                                ref_d(b"M"),
                                rec_d(
                                    b"K",
                                    desc(
                                        "union",
                                        vec![exact_d(b"b"), desc("tuple", vec![ref_d(b"L")])],
                                    ),
                                ),
                            ],
                        ),
                    ),
                ],
            ),
        );
        let facts = scope_facts(&t);
        assert!(!facts.shadowing && !facts.capture);
    }

    #[test]
    fn shadowing_without_a_free_ref_is_not_capture() {
        // The pinned oracle's rec-nested-shadow: the inner body refers only to itself.
        let inner = rec_d(
            b"x",
            desc(
                "union",
                vec![
                    exact_d(b"inil"),
                    desc(
                        "tuple",
                        vec![desc("scalar", vec![atom(b"int")]), ref_d(b"x")],
                    ),
                ],
            ),
        );
        let t = rec_d(
            b"x",
            desc(
                "union",
                vec![exact_d(b"onil"), desc("tuple", vec![inner, ref_d(b"x")])],
            ),
        );
        let facts = scope_facts(&t);
        assert!(facts.shadowing && !facts.capture);
    }

    #[test]
    fn a_cons_list_neither_shadows_nor_captures() {
        let facts = scope_facts(&cons_list_descriptor());
        assert!(!facts.shadowing && !facts.capture);
    }

    #[test]
    fn literals_are_not_descriptors() {
        // A rec/ref spelled inside an exact literal is data, not scope.
        let t = desc("exact", vec![reproducer()]);
        let facts = scope_facts(&t);
        assert!(!facts.shadowing && !facts.capture);
    }

    #[test]
    fn every_capture_skeleton_is_capture_prone() {
        let mut rng = Rng::new(7);
        for _ in 0..500 {
            assert!(scope_facts(&capture_skeleton(&mut rng)).capture);
        }
    }

    #[test]
    fn a_seed_reproduces_its_pairs() {
        let run = |seed| {
            let mut rng = Rng::new(seed);
            (0..200).map(|_| gen_pair(&mut rng)).collect::<Vec<_>>()
        };
        assert!(run(11) == run(11));
        assert!(run(11) != run(12));
    }
}
