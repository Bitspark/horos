//! `horos-fuzz` — the differential-fuzzing GENERATOR for the tri-core (horos-internal#72).
//!
//! A seeded, deterministic emitter of randomized `(descriptor, value)` pairs — the input
//! half of the differential-fuzzing leg. Each line of stdout is one pair:
//!
//! ```text
//! <descriptor_hex> <value_hex>
//! ```
//!
//! where both fields are `ontos-codec-v1` lowercase hex — the exact byte form the three
//! `horos-conform` CLIs decode (`conformance/fuzz.sh` runs the pairs through rs/go/ts for
//! both `wf` and `conforms` and asserts every verdict agrees). There is **no oracle**:
//! the generator emits descriptors and values WITHOUT computing any expected judgment.
//! Cross-core agreement IS the property — a pair on which rs/go/ts disagree is the bug,
//! whatever the "right" answer is. So the generator is free to emit well-formed,
//! malformed, conforming, non-conforming, and boundary inputs indiscriminately; every
//! one is a valid probe.
//!
//! **Determinism is mandatory.** The PRNG is a pure splitmix64 seeded ONLY from the CLI
//! `--seed` argument (no system entropy), so a seed always reproduces the same stream —
//! a divergence found in CI replays bit-for-bit from its logged seed. Values are kept
//! well under the codec's `max_depth` (1024) so every emitted pair decodes cleanly in all
//! three cores (a decode error is a harness failure, not a cross-core divergence); the
//! deep-nesting boundary deliberately straddles horos's `MAX_RECURSION_DEPTH` (512), where
//! the recursion-depth guardrail trips identically across the cores.
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

/// A random GROUND value (an ordinary ontos value, not a descriptor), depth-bounded so the
/// encoded form stays well under the codec's `max_depth`. Mixes atoms, raw tuples, and the
/// ontos-data embeddings (int/text/bool/list/map/set), incl. the empty-collection edges.
fn rand_value(rng: &mut Rng, depth: usize) -> Value {
    if depth == 0 || rng.one_in(2) {
        // a leaf
        return match rng.below(7) {
            0 => atom(&rand_atom_bytes(rng)),
            1 => encode_int(rand_int(rng)),
            2 => encode_text(&rand_text(rng)),
            3 => encode_bool(rng.one_in(2)),
            4 => atom(b"nil"),
            5 => tup(vec![]),      // the empty tuple
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
        return match rng.below(6) {
            0 => desc("any", vec![]),
            1 => desc("atom", vec![]),
            2 => desc("atom-exact", vec![atom(&rand_atom_bytes(rng))]),
            3 => desc("scalar", vec![rand_scalar_kind(rng)]),
            4 => desc("exact", vec![rand_value(rng, 2)]),
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
// driver
// ===========================================================================

fn to_hex(bytes: &[u8]) -> String {
    let mut s = String::with_capacity(bytes.len() * 2);
    for b in bytes {
        s.push_str(&format!("{b:02x}"));
    }
    s
}

/// Generate one `(descriptor, value)` pair. With probability ~1/3 it is a targeted
/// boundary pair; otherwise an independently-random descriptor and value (so conformance
/// is usually `false`, occasionally `true` — both exercise the cross-core comparison).
fn gen_pair(rng: &mut Rng) -> (Value, Value) {
    if rng.below(3) == 0 {
        boundary_pair(rng)
    } else {
        let desc_depth = rng.range(1, 4);
        let val_depth = rng.range(1, 5);
        (rand_descriptor(rng, desc_depth), rand_value(rng, val_depth))
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
    for _ in 0..count {
        let (d, v) = gen_pair(&mut rng);
        let line = format!("{} {}\n", to_hex(&encode(&d)), to_hex(&encode(&v)));
        // A broken pipe (downstream closed) is not an error worth a nonzero exit.
        if out.write_all(line.as_bytes()).is_err() {
            return;
        }
    }
    let _ = out.flush();
}
