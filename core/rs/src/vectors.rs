//! The shared conformance oracle: hand-authored `wf`/`conforms` judgments, replayed
//! byte-for-byte by every core.
//!
//! Each case carries a descriptor and a set of values, each with the **expected**
//! `wf(descriptor)` and `conforms(value, descriptor)` — authored from
//! `docs/spec/horos.md`, never scraped from the implementation. [`authored`] builds the
//! cases natively; [`vectors_authored_selfcheck`] asserts this core agrees with the
//! authored bools and (when `HOROS_EMIT_VECTORS=1`) emits
//! `vectors/conformance.json` (descriptors/values as `ontos-codec-v1` hex);
//! [`vectors_replay`] reads that committed file and re-checks it — the exact replay the
//! Go and TS cores mirror, so the three agree on the same oracle.

use super::{conforms, wf};
use ontos_codec::{decode, encode};
use ontos_core::{Tuple, Value};
use ontos_data::{
    encode_bool, encode_decimal, encode_int, encode_list, encode_map, encode_null, encode_set,
    encode_text,
};

// ----- builders -----

fn a(bytes: &[u8]) -> Value {
    Value::atom(bytes.to_vec())
}
fn tup(items: Vec<Value>) -> Value {
    Value::Tuple(Tuple::new(items))
}
fn d(ctor: &str, args: Vec<Value>) -> Value {
    let mut items = vec![a(format!("horos/{ctor}").as_bytes())];
    items.extend(args);
    tup(items)
}
fn scalar(kind: &[u8]) -> Value {
    d("scalar", vec![a(kind)])
}
fn field(name: &str, required: bool, ty: Value) -> Value {
    d(
        "field",
        vec![
            encode_text(name),
            a(if required { b"req" } else { b"opt" }),
            ty,
        ],
    )
}
fn map_v(pairs: &[(&str, Value)]) -> Value {
    let entries: Vec<(Value, Value)> = pairs
        .iter()
        .map(|(k, val)| (encode_text(k), val.clone()))
        .collect();
    encode_map(&entries).unwrap()
}
/// A map from arbitrary (already-encoded) key values — used for the non-text-keyed map
/// case (e.g. int keys), where the KEY descriptor itself is under test.
fn map_int_key(pairs: &[(Value, Value)]) -> Value {
    encode_map(pairs).unwrap()
}

/// The cons-list recursive descriptor: `rec l. (atom-exact "nil") | (int, ref l)`.
fn cons_list() -> Value {
    d(
        "rec",
        vec![
            a(b"l"),
            d(
                "union",
                vec![
                    d("atom-exact", vec![a(b"nil")]),
                    d("tuple", vec![scalar(b"int"), d("ref", vec![a(b"l")])]),
                ],
            ),
        ],
    )
}

/// A binary tree of ints: `rec t. int | node(ref t, ref t)`. A recursive `compound`
/// with a DOUBLE `ref` — multi-descent / non-tail recursion (each `node` recurses into
/// two children), the well-behaved counterpart the oracle was missing (#19).
fn int_tree() -> Value {
    d(
        "rec",
        vec![
            a(b"t"),
            d(
                "union",
                vec![
                    scalar(b"int"),
                    d(
                        "compound",
                        vec![a(b"node"), d("ref", vec![a(b"t")]), d("ref", vec![a(b"t")])],
                    ),
                ],
            ),
        ],
    )
}

/// A nested/shadowing `rec`: an inner `rec x` rebinding the name `x` already bound by an
/// outer `rec x` (#19, the `env.iter().rev()` / `lookupLast` path). The OUTER body is
///
/// ```text
/// rec x. (atom-exact "onil") | (INNER, ref x)
/// ```
///
/// where `INNER = rec x. (atom-exact "inil") | (int, ref x)` is an int cons-list. The
/// `ref x` inside INNER must resolve to INNER's body (the shadowing binding); the `ref x`
/// in the outer tuple resolves to the OUTER body. A value `(onil, onil)` therefore does
/// NOT conform — its first element `onil` is fed to INNER, which accepts only `inil` or
/// `(int, …)` — proving the inner `ref` did not leak to the outer binding.
fn nested_shadow_rec() -> Value {
    let inner = d(
        "rec",
        vec![
            a(b"x"),
            d(
                "union",
                vec![
                    d("atom-exact", vec![a(b"inil")]),
                    d("tuple", vec![scalar(b"int"), d("ref", vec![a(b"x")])]),
                ],
            ),
        ],
    );
    d(
        "rec",
        vec![
            a(b"x"),
            d(
                "union",
                vec![
                    d("atom-exact", vec![a(b"onil")]),
                    d("tuple", vec![inner, d("ref", vec![a(b"x")])]),
                ],
            ),
        ],
    )
}

/// The ontos `int` value `2^127` — one past `i128::MAX`, the #18 cap boundary. Built
/// directly (it overflows the `i128` that `encode_int` takes): a canonical int is
/// `Tuple(Atom("int"), Atom([sign, magnitude_be_minimal…]))`; here sign `0x00`
/// (non-negative) and magnitude `0x80` followed by fifteen `0x00` bytes (the 16-byte
/// big-endian form of `2^127`, with no leading zero, so it is canonical/recognized but
/// non-materializable as an `i128`). `read_int` / `ReadInt` then reject it (out of i128
/// range) in all three cores, making an `atom-len` bound of `2^127` non-wf everywhere.
fn int_two_pow_127() -> Value {
    let mut payload = vec![0x00u8, 0x80]; // sign = non-negative, top magnitude byte
    payload.extend(std::iter::repeat_n(0x00u8, 15)); // 15 trailing zero bytes → 2^127
    tup(vec![a(b"int"), a(&payload)])
}

// ----- canonical-form negatives (#71): non-canonical ontos-data values that the rs/go/ts
// recognizers reject, so a conforms over a wf descriptor whose recognizer they feed is
// `false` in all three cores. Built by hand (the `encode_*` builders only ever produce the
// canonical form), so these pin the ontos-data lanes directly — the seam where the TS
// `@bitspark/ontos-data 0.4.0` pin (same source tree as rs/go `v0.2.0` today) could drift.

/// A `set` value whose elements are NOT strictly ascending by `ontos-codec-v1` byte order:
/// `Tuple(Atom("set"), Atom("b"), Atom("a"))`. `Atom("b")` encodes greater than `Atom("a")`,
/// so `read_set` returns `UnsortedElements` — not a canonical set, so it conforms to nothing.
fn set_out_of_order() -> Value {
    tup(vec![a(b"set"), a(b"b"), a(b"a")])
}

/// A `set` value with a duplicate element: `Tuple(Atom("set"), Atom("a"), Atom("a"))`. The
/// two equal element encodings make `read_set` return `DuplicateElement` — non-canonical.
fn set_duplicate_elem() -> Value {
    tup(vec![a(b"set"), a(b"a"), a(b"a")])
}

/// A `map` value whose entries are NOT strictly ascending by key bytes:
/// `Tuple(Atom("map"), (text "b" → int 2), (text "a" → int 1))`. The key `"b"` encodes
/// greater than `"a"`, so `read_map` returns `UnsortedEntries` — non-canonical.
fn map_out_of_order() -> Value {
    tup(vec![
        a(b"map"),
        tup(vec![encode_text("b"), encode_int(2)]),
        tup(vec![encode_text("a"), encode_int(1)]),
    ])
}

/// A `map` value with a duplicate key: `Tuple(Atom("map"), (text "a" → int 1),
/// (text "a" → int 2))`. The equal key encodings make `read_map` return `DuplicateKey`.
fn map_duplicate_key() -> Value {
    tup(vec![
        a(b"map"),
        tup(vec![encode_text("a"), encode_int(1)]),
        tup(vec![encode_text("a"), encode_int(2)]),
    ])
}

/// A non-minimal-magnitude `int`: `Tuple(Atom("int"), Atom([0x00, 0x00, 0x05]))` — sign byte
/// `0x00` (non-negative) followed by a magnitude `0x00 0x05` carrying a leading zero byte.
/// The canonical spelling of 5 is `[0x00, 0x05]`; the leading-zero form is a second spelling,
/// so the structural `recognize_int` gate (shared by `read_int`) rejects it — `scalar(int)`
/// does not conform. Distinct from `int_two_pow_127` (a *canonical but beyond-i128* magnitude
/// that `recognize_int` ACCEPTS and only `read_int` rejects): this one fails recognition.
fn int_non_minimal() -> Value {
    tup(vec![a(b"int"), a(&[0x00u8, 0x00, 0x05])])
}

// ----- depth cap (#71/#37): the same `false` at the same logical depth in all three cores.
// `conforms`/`wf` increment a depth counter at identical recursion points (rs/go/ts), so a
// >512-deep VALUE checked against the tiny recursive `cons_list` descriptor drives the
// recognizer past the same number of `conforms_in` frames in every core. Each cons cell
// costs a fixed bundle of frames (rec → union → tuple → ref → rec …), identical across the
// cores, so the trip boundary is exact and tri-core-stable: a 170-deep value still conforms,
// a 171-deep value reaches `depth > MAX_RECURSION_DEPTH` (= 512) at the same frame in all
// three and returns a deterministic `false` (a guardrail, NOT a crash). Pinning the cons
// value (a deep VALUE over a SMALL descriptor) keeps the boundary hex compact — far smaller
// than nesting the descriptor 513 deep — and exercises the `conforms`-side cap the #37
// guard exists for. Built ITERATIVELY, so constructing/dropping it never itself recurses.

/// A right-nested int cons-list value `(0, (0, … nil))` `levels` deep (the values
/// `cons_list()` describes), built iteratively (no recursion at build / no deep `Value`
/// recursion until the recognizer walks it under its depth guard).
fn deep_cons_value(levels: usize) -> Value {
    let mut v = a(b"nil");
    for _ in 0..levels {
        v = tup(vec![encode_int(0), v]);
    }
    v
}

// ----- the authored cases -----

struct Vc {
    value: Value,
    conforms: bool,
    note: &'static str,
}
struct Case {
    name: &'static str,
    descriptor: Value,
    wf: bool,
    values: Vec<Vc>,
}
fn vc(value: Value, conforms: bool, note: &'static str) -> Vc {
    Vc {
        value,
        conforms,
        note,
    }
}

/// Hand-authored from `docs/spec/horos.md`. Covers every combinator (accept + reject)
/// plus a representative `wf`-rejection set.
fn authored() -> Vec<Case> {
    vec![
        Case {
            name: "any",
            descriptor: d("any", vec![]),
            wf: true,
            values: vec![
                vc(a(b"x"), true, "an atom"),
                vc(tup(vec![]), true, "a tuple"),
                vc(encode_int(1), true, "an int"),
            ],
        },
        Case {
            name: "atom",
            descriptor: d("atom", vec![]),
            wf: true,
            values: vec![
                vc(a(b"x"), true, "an atom"),
                vc(tup(vec![]), false, "a tuple"),
            ],
        },
        Case {
            name: "atom-exact",
            descriptor: d("atom-exact", vec![a(b"hi")]),
            wf: true,
            values: vec![
                vc(a(b"hi"), true, "exact match"),
                vc(a(b"ho"), false, "different bytes"),
                vc(tup(vec![a(b"hi")]), false, "a tuple, not an atom"),
            ],
        },
        Case {
            name: "atom-len-1-3",
            descriptor: d("atom-len", vec![encode_int(1), encode_int(3)]),
            wf: true,
            values: vec![
                vc(a(b"x"), true, "len 1"),
                vc(a(b"abc"), true, "len 3"),
                vc(a(b""), false, "len 0 below lo"),
                vc(a(b"abcd"), false, "len 4 above hi"),
            ],
        },
        Case {
            name: "atom-in",
            descriptor: d("atom-in", vec![encode_set(&[a(b"x"), a(b"y")]).unwrap()]),
            wf: true,
            values: vec![
                vc(a(b"x"), true, "member"),
                vc(a(b"z"), false, "non-member"),
            ],
        },
        // ----- (#68) data-encoded refinements: the four named grammar-class predicates the
        // visual-atlas consumer needs (atlas#533 Q4 / visual-atlas#534). Each measures
        // Unicode CHARACTERS (code points), not bytes; the multi-byte cases below (where
        // char-count ≠ byte-count, and an astral emoji counts as ONE char yet two UTF-16
        // units) are the acceptance load-bearers that pin char-counting in all three cores.
        Case {
            name: "text-len-1-3",
            descriptor: d("text-len", vec![encode_int(1), encode_int(3)]),
            wf: true,
            values: vec![
                vc(encode_text("a"), true, "1 char (1 byte)"),
                vc(encode_text("abc"), true, "3 chars (3 bytes)"),
                vc(encode_text(""), false, "0 chars below lo"),
                vc(encode_text("abcd"), false, "4 chars above hi"),
                // Multi-byte: "ééé" is 3 chars but 6 bytes — char-count 3 ≤ 3 CONFORMS;
                // a byte-count reading (6 > 3) would wrongly reject. The key #68 case.
                vc(
                    encode_text("ééé"),
                    true,
                    "3 multi-byte chars (6 bytes) — char-count 3 ≤ 3, so conforms (byte-count 6 would reject)",
                ),
                // "éééé" is 4 chars (8 bytes) — over hi by CHARACTER count.
                vc(
                    encode_text("éééé"),
                    false,
                    "4 multi-byte chars (8 bytes) — char-count 4 > 3, rejected",
                ),
                // An astral emoji is ONE code point (4 bytes; two UTF-16 units in JS) —
                // pins that TS counts code points, not `.length`, matching rs/go.
                vc(
                    encode_text("😀"),
                    true,
                    "1 astral char (4 bytes, 2 UTF-16 units) — char-count 1 within [1,3]",
                ),
                vc(encode_int(1), false, "not utf8-text"),
            ],
        },
        Case {
            name: "text-len-0-0-empty",
            descriptor: d("text-len", vec![encode_int(0), encode_int(0)]),
            wf: true,
            values: vec![
                vc(encode_text(""), true, "the empty string has 0 chars"),
                vc(encode_text("a"), false, "1 char above hi=0"),
            ],
        },
        Case {
            name: "key-grammar",
            descriptor: d("key-grammar", vec![]),
            wf: true,
            values: vec![
                vc(encode_text("a"), true, "single lowercase letter"),
                vc(encode_text("0"), true, "single digit (leading class allows 0-9)"),
                vc(encode_text("a.b_c-1"), true, "interior . _ - and alnum"),
                vc(encode_text("abc123"), true, "alnum mix"),
                vc(encode_text(""), false, "empty fails the leading [a-z0-9]"),
                vc(encode_text("A"), false, "uppercase not allowed"),
                vc(encode_text(".ab"), false, "leading '.' is not in [a-z0-9]"),
                vc(encode_text("-ab"), false, "leading '-' is not in [a-z0-9]"),
                vc(encode_text("a b"), false, "space is not in the interior class"),
                vc(encode_text("a/b"), false, "'/' is not in the interior class"),
                // Boundary: 64 chars (1 + 63) conforms; 65 chars exceeds {0,63}.
                vc(
                    encode_text(&format!("a{}", "b".repeat(63))),
                    true,
                    "64 chars (1 leading + 63 tail) — the upper boundary",
                ),
                vc(
                    encode_text(&format!("a{}", "b".repeat(64))),
                    false,
                    "65 chars — one past the {0,63} tail bound",
                ),
                // Multi-byte: "é" is not in [a-z0-9] (and a non-ASCII char never is).
                vc(
                    encode_text("café"),
                    false,
                    "the multi-byte 'é' is not in [a-z0-9._-]",
                ),
                vc(encode_int(1), false, "not utf8-text"),
            ],
        },
        Case {
            name: "text-non-blank",
            descriptor: d("text-non-blank", vec![]),
            wf: true,
            values: vec![
                vc(encode_text("a"), true, "a non-whitespace char"),
                vc(encode_text("  x  "), true, "has a non-whitespace char among spaces"),
                vc(encode_text(""), false, "empty has no non-whitespace char"),
                vc(encode_text("   "), false, "all spaces"),
                vc(encode_text("\t\n\r"), false, "tab/newline/CR are all whitespace"),
                // A non-ASCII char is never whitespace → counts as non-blank.
                vc(
                    encode_text("é"),
                    true,
                    "a multi-byte non-ASCII char is never whitespace → non-blank",
                ),
                // A no-break space (U+00A0, 2 bytes) is NOT in the ASCII whitespace set,
                // so it counts as non-blank — pins the deliberately ASCII-only ws set.
                vc(
                    encode_text("\u{00A0}"),
                    true,
                    "U+00A0 NO-BREAK SPACE is not in the ASCII ws set → non-blank in all cores",
                ),
                vc(encode_int(1), false, "not utf8-text"),
            ],
        },
        Case {
            name: "lowercase-uuid",
            descriptor: d("lowercase-uuid", vec![]),
            wf: true,
            values: vec![
                vc(
                    encode_text("00000000-0000-0000-0000-000000000000"),
                    true,
                    "the nil UUID",
                ),
                vc(
                    encode_text("0123456789abcdef-89ab-cdef-0123-456789abcdef"),
                    false,
                    "first group has 16 hex chars, not 8 — bad shape",
                ),
                vc(
                    encode_text("9b2e4c1a-7d3f-4e8b-a1c2-d3e4f5061728"),
                    true,
                    "a canonical lowercase UUID",
                ),
                vc(
                    encode_text("9B2E4C1A-7D3F-4E8B-A1C2-D3E4F5061728"),
                    false,
                    "uppercase hex is not the LOWERCASE grammar",
                ),
                vc(
                    encode_text("9b2e4c1a7d3f4e8ba1c2d3e4f5061728"),
                    false,
                    "missing the four hyphens",
                ),
                vc(
                    encode_text("9b2e4c1a-7d3f-4e8b-a1c2-d3e4f506172"),
                    false,
                    "last group has 11 hex chars, not 12 — too short",
                ),
                vc(
                    encode_text("9b2e4c1a-7d3f-4e8b-a1c2-d3e4f50617289"),
                    false,
                    "last group has 13 hex chars — too long",
                ),
                vc(
                    encode_text("9b2e4c1g-7d3f-4e8b-a1c2-d3e4f5061728"),
                    false,
                    "'g' is not a hex digit",
                ),
                vc(encode_text(""), false, "empty is not a UUID"),
                vc(encode_int(1), false, "not utf8-text"),
            ],
        },
        // ----- (#68) wf rejections for the new refinements -----
        Case {
            name: "wf-bad-text-len-lo-gt-hi",
            descriptor: d("text-len", vec![encode_int(3), encode_int(1)]),
            wf: false,
            values: vec![vc(
                encode_text("ab"),
                false,
                "lo=3 > hi=1 is non-wf (mirrors atom-len), so nothing conforms",
            )],
        },
        Case {
            name: "wf-bad-text-len-neg-lo",
            descriptor: d("text-len", vec![encode_int(-1), encode_int(3)]),
            wf: false,
            values: vec![vc(
                encode_text("a"),
                false,
                "lo<0 is non-wf; len 1 would lie in [-1,3] locally but the wf gate rejects it",
            )],
        },
        Case {
            name: "wf-text-len-bound-2pow127",
            descriptor: d("text-len", vec![encode_int(0), int_two_pow_127()]),
            wf: false,
            values: vec![vc(
                encode_text("x"),
                false,
                "a hi bound of 2^127 is past i128::MAX → non-wf everywhere (the #18 cap, shared with atom-len)",
            )],
        },
        Case {
            name: "wf-bad-key-grammar-arity",
            descriptor: d("key-grammar", vec![a(b"x")]),
            wf: false,
            values: vec![vc(
                encode_text("a"),
                false,
                "key-grammar takes no args; an extra arg is non-wf",
            )],
        },
        Case {
            name: "scalar-int",
            descriptor: scalar(b"int"),
            wf: true,
            values: vec![
                vc(encode_int(5), true, "positive"),
                vc(encode_int(-1), true, "negative"),
                vc(encode_text("5"), false, "text, not int"),
                vc(a(b"x"), false, "bare atom"),
            ],
        },
        Case {
            name: "scalar-text",
            descriptor: scalar(b"utf8-text"),
            wf: true,
            values: vec![
                vc(encode_text("hi"), true, "text"),
                vc(encode_int(1), false, "int, not text"),
            ],
        },
        Case {
            name: "scalar-bool",
            descriptor: scalar(b"bool"),
            wf: true,
            values: vec![
                vc(encode_bool(true), true, "bool"),
                vc(encode_int(1), false, "int, not bool"),
            ],
        },
        Case {
            name: "tuple-int-atom",
            descriptor: d("tuple", vec![scalar(b"int"), d("atom", vec![])]),
            wf: true,
            values: vec![
                vc(tup(vec![encode_int(1), a(b"x")]), true, "int then atom"),
                vc(tup(vec![encode_int(1)]), false, "wrong arity"),
                vc(tup(vec![a(b"x"), a(b"x")]), false, "child 0 not an int"),
                vc(a(b"x"), false, "not a tuple"),
            ],
        },
        Case {
            name: "seq-int",
            descriptor: d("seq", vec![scalar(b"int")]),
            wf: true,
            values: vec![
                vc(tup(vec![encode_int(1), encode_int(2)]), true, "all ints"),
                vc(tup(vec![]), true, "empty (vacuous)"),
                vc(tup(vec![encode_int(1), a(b"x")]), false, "a non-int child"),
            ],
        },
        Case {
            name: "compound-point",
            descriptor: d(
                "compound",
                vec![a(b"point"), scalar(b"int"), scalar(b"int")],
            ),
            wf: true,
            values: vec![
                vc(
                    tup(vec![a(b"point"), encode_int(1), encode_int(2)]),
                    true,
                    "point(1, 2)",
                ),
                vc(
                    tup(vec![a(b"line"), encode_int(1), encode_int(2)]),
                    false,
                    "wrong label",
                ),
                vc(tup(vec![a(b"point"), encode_int(1)]), false, "wrong arity"),
            ],
        },
        Case {
            name: "list-of-int",
            descriptor: d("list-of", vec![scalar(b"int")]),
            wf: true,
            values: vec![
                vc(encode_list(&[encode_int(1), encode_int(2)]), true, "ints"),
                vc(encode_list(&[]), true, "empty list"),
                vc(
                    encode_list(&[encode_int(1), encode_text("x")]),
                    false,
                    "a non-int element",
                ),
                vc(encode_int(1), false, "not a list"),
            ],
        },
        Case {
            name: "set-of-atom",
            descriptor: d("set-of", vec![d("atom", vec![])]),
            wf: true,
            values: vec![
                vc(encode_set(&[a(b"x"), a(b"y")]).unwrap(), true, "atoms"),
                vc(encode_list(&[a(b"x")]), false, "a list is not a set"),
            ],
        },
        Case {
            name: "map-of-text-int",
            descriptor: d("map-of", vec![scalar(b"utf8-text"), scalar(b"int")]),
            wf: true,
            values: vec![
                vc(
                    map_v(&[("a", encode_int(1)), ("b", encode_int(2))]),
                    true,
                    "text->int",
                ),
                vc(
                    map_v(&[("a", encode_text("x"))]),
                    false,
                    "a value is not an int",
                ),
            ],
        },
        Case {
            name: "record-closed",
            descriptor: d(
                "record",
                vec![
                    a(b"closed"),
                    field("name", true, scalar(b"utf8-text")),
                    field("age", false, scalar(b"int")),
                ],
            ),
            wf: true,
            values: vec![
                vc(
                    map_v(&[("name", encode_text("ada"))]),
                    true,
                    "required only",
                ),
                vc(
                    map_v(&[("age", encode_int(36)), ("name", encode_text("ada"))]),
                    true,
                    "required + optional",
                ),
                vc(
                    map_v(&[("age", encode_int(36))]),
                    false,
                    "required name missing",
                ),
                vc(map_v(&[("name", encode_int(1))]), false, "name is not text"),
                vc(
                    map_v(&[("name", encode_text("ada")), ("x", encode_int(1))]),
                    false,
                    "extra key under closed",
                ),
            ],
        },
        Case {
            name: "record-open",
            descriptor: d(
                "record",
                vec![a(b"open"), field("name", true, scalar(b"utf8-text"))],
            ),
            wf: true,
            values: vec![vc(
                map_v(&[("name", encode_text("ada")), ("x", encode_int(1))]),
                true,
                "extra key allowed under open",
            )],
        },
        Case {
            name: "union-int-bool",
            descriptor: d("union", vec![scalar(b"int"), scalar(b"bool")]),
            wf: true,
            values: vec![
                vc(encode_int(1), true, "int alternative"),
                vc(encode_bool(true), true, "bool alternative"),
                vc(encode_text("x"), false, "neither"),
            ],
        },
        Case {
            name: "and-atom-len",
            descriptor: d(
                "and",
                vec![
                    d("atom", vec![]),
                    d("atom-len", vec![encode_int(1), encode_int(3)]),
                ],
            ),
            wf: true,
            values: vec![
                vc(a(b"ab"), true, "atom of len 2"),
                vc(a(b""), false, "fails the length bound"),
                vc(tup(vec![]), false, "not an atom"),
            ],
        },
        Case {
            name: "not-int",
            descriptor: d("not", vec![scalar(b"int")]),
            wf: true,
            values: vec![
                vc(encode_int(1), false, "is an int"),
                vc(encode_text("x"), true, "not an int"),
                vc(a(b"x"), true, "bare atom is not an int"),
            ],
        },
        Case {
            name: "exact-7",
            descriptor: d("exact", vec![encode_int(7)]),
            wf: true,
            values: vec![
                vc(encode_int(7), true, "equal"),
                vc(encode_int(8), false, "different value"),
            ],
        },
        Case {
            name: "rec-cons-list",
            descriptor: cons_list(),
            wf: true,
            values: vec![
                vc(a(b"nil"), true, "[]"),
                vc(tup(vec![encode_int(1), a(b"nil")]), true, "[1]"),
                vc(
                    tup(vec![encode_int(1), tup(vec![encode_int(2), a(b"nil")])]),
                    true,
                    "[1, 2]",
                ),
                vc(
                    tup(vec![encode_text("x"), a(b"nil")]),
                    false,
                    "head is not an int",
                ),
                vc(a(b"other"), false, "not nil, not a cons cell"),
            ],
        },
        // (a, #19) int_tree: a recursive `compound` with a DOUBLE `ref` (multi-descent /
        // non-tail recursion). Only `rec-cons-list` (single tail ref) was in the oracle.
        Case {
            name: "rec-int-tree",
            descriptor: int_tree(),
            wf: true,
            values: vec![
                vc(encode_int(5), true, "a leaf int"),
                vc(
                    tup(vec![a(b"node"), encode_int(1), encode_int(2)]),
                    true,
                    "node(1, 2) — two leaves",
                ),
                vc(
                    tup(vec![
                        a(b"node"),
                        encode_int(1),
                        tup(vec![a(b"node"), encode_int(2), encode_int(3)]),
                    ]),
                    true,
                    "node(1, node(2, 3)) — right subtree descends",
                ),
                vc(
                    tup(vec![
                        a(b"node"),
                        tup(vec![a(b"node"), encode_int(1), encode_int(2)]),
                        encode_int(3),
                    ]),
                    true,
                    "node(node(1, 2), 3) — left subtree descends",
                ),
                vc(
                    tup(vec![a(b"node"), encode_text("x"), encode_int(2)]),
                    false,
                    "left subtree is neither an int nor a node",
                ),
            ],
        },
        // (b, #19) nested/shadowing `rec`: an inner `rec x` rebinds the outer `rec x`.
        // Exercises the innermost-binding-wins lookup (env.iter().rev() / lookupLast).
        Case {
            name: "rec-nested-shadow",
            descriptor: nested_shadow_rec(),
            wf: true,
            values: vec![
                vc(a(b"onil"), true, "outer base case"),
                vc(
                    tup(vec![a(b"inil"), a(b"onil")]),
                    true,
                    "(inner-nil, outer-nil) — inner ref resolves to the inner cons-list",
                ),
                vc(
                    tup(vec![
                        tup(vec![encode_int(1), a(b"inil")]),
                        a(b"onil"),
                    ]),
                    true,
                    "((1, inil), onil) — first element is an inner int-list",
                ),
                vc(
                    tup(vec![a(b"onil"), a(b"onil")]),
                    false,
                    "(onil, onil) — inner expects inil/(int,…), so the inner ref did NOT leak to the outer binding",
                ),
            ],
        },
        // (d, #19) map-of with a non-conforming KEY: the key-descriptor branch. The key
        // must be an int; a text key fails the KEY descriptor → conforms=false.
        Case {
            name: "map-of-int-key",
            descriptor: d("map-of", vec![scalar(b"int"), scalar(b"utf8-text")]),
            wf: true,
            values: vec![
                vc(
                    map_int_key(&[(encode_int(1), encode_text("a"))]),
                    true,
                    "int->text, key conforms",
                ),
                vc(
                    map_v(&[("k", encode_text("a"))]),
                    false,
                    "a text key fails the int KEY descriptor",
                ),
            ],
        },
        // ----- wf rejections (a non-wf descriptor; conforms over it is false) -----
        Case {
            name: "wf-bad-scalar-kind",
            descriptor: d("scalar", vec![a(b"float")]),
            wf: false,
            values: vec![vc(encode_int(1), false, "non-wf descriptor never conforms")],
        },
        Case {
            name: "wf-bad-atom-len-lo-gt-hi",
            descriptor: d("atom-len", vec![encode_int(3), encode_int(1)]),
            wf: false,
            values: vec![vc(a(b"x"), false, "non-wf descriptor never conforms")],
        },
        Case {
            name: "wf-bad-record-mode",
            descriptor: d(
                "record",
                vec![a(b"maybe"), field("n", true, scalar(b"int"))],
            ),
            wf: false,
            values: vec![vc(
                map_v(&[("n", encode_int(1))]),
                false,
                "the field would conform, but the bad mode makes the descriptor non-wf",
            )],
        },
        Case {
            name: "wf-unguarded-rec",
            descriptor: d("rec", vec![a(b"x"), d("ref", vec![a(b"x")])]),
            wf: false,
            values: vec![vc(
                a(b"anything"),
                false,
                "an unguarded rec is non-wf, so nothing conforms",
            )],
        },
        Case {
            name: "wf-unbound-ref",
            descriptor: d("ref", vec![a(b"y")]),
            wf: false,
            values: vec![vc(
                a(b"anything"),
                false,
                "an unbound ref is non-wf, so nothing conforms",
            )],
        },
        Case {
            name: "wf-not-a-descriptor",
            descriptor: tup(vec![a(b"int")]),
            wf: false,
            values: vec![vc(
                encode_int(1),
                false,
                "unreserved label is not a descriptor",
            )],
        },
        // (c, #19) duplicate record field → wf=false. Two fields named "name".
        Case {
            name: "wf-dup-record-field",
            descriptor: d(
                "record",
                vec![
                    a(b"open"),
                    field("name", true, scalar(b"utf8-text")),
                    field("name", false, scalar(b"int")),
                ],
            ),
            wf: false,
            values: vec![vc(
                map_v(&[("name", encode_text("ada"))]),
                false,
                "non-wf descriptor never conforms",
            )],
        },
        // (e, #19) non-UTF-8 descriptor label → wf=false. The head atom is
        // `b"horos/\xff"`: it sits under the reserved prefix but byte 0xFF is not valid
        // UTF-8, so the label gate (str::from_utf8 / utf8.Valid / fatal TextDecoder)
        // rejects it — it is not a horos descriptor at all, in all three cores.
        Case {
            name: "wf-non-utf8-label",
            descriptor: tup(vec![a(b"horos/\xff"), a(b"x")]),
            wf: false,
            values: vec![vc(
                encode_int(1),
                false,
                "an invalid-UTF-8 label is not a descriptor",
            )],
        },
        // (f, #18) the atom-len cap boundary: a bound = 2^127 (one past i128::MAX) makes
        // the descriptor non-wf in ALL THREE cores — Rust's read_int errors, and Go/TS
        // apply the same explicit cap. Proves the three agree exactly at the cap.
        Case {
            name: "wf-atom-len-bound-2pow127",
            descriptor: d("atom-len", vec![encode_int(0), int_two_pow_127()]),
            wf: false,
            values: vec![vc(
                a(b"x"),
                false,
                "a bound of 2^127 is past i128::MAX → non-wf everywhere",
            )],
        },
        // ----- soundness gate (#69, spec §3 MUST): conforms() presupposes wf() -----
        // Each of the six probes is a non-wf descriptor paired with a value that WOULD
        // match the descriptor's body locally; only the top-level wf() gate flips
        // conforms to false. These pin that the gate — not arm-by-arm validation — is
        // what rejects them, identically in all three cores.
        Case {
            // atom-len with a negative lo (lo > hi check is `0 <= lo <= hi`, so lo=-1 is
            // non-wf). "x" has len 1, which lies in the *evaluated* range [-1, 2].
            name: "wf-gate-atom-len-neg-lo",
            descriptor: d("atom-len", vec![encode_int(-1), encode_int(2)]),
            wf: false,
            values: vec![vc(
                a(b"x"),
                false,
                "len 1 is in [-1, 2] locally, but lo<0 is non-wf → conforms gated false",
            )],
        },
        Case {
            // union with a non-descriptor (garbage) alternative is non-wf; int 7 would
            // conform via the `any` alternative locally.
            name: "wf-gate-union-garbage-alt",
            descriptor: d("union", vec![d("any", vec![]), tup(vec![a(b"garbage")])]),
            wf: false,
            values: vec![vc(
                encode_int(7),
                false,
                "7 matches the `any` alternative locally, but a non-wf alternative gates conforms false",
            )],
        },
        Case {
            // atom-in whose set carries a non-atom element (a tuple) is non-wf; "x" is a
            // member, so the membership test would pass locally.
            name: "wf-gate-atom-in-non-atom-elem",
            descriptor: d(
                "atom-in",
                vec![encode_set(&[a(b"x"), tup(vec![a(b"t")])]).unwrap()],
            ),
            wf: false,
            values: vec![vc(
                a(b"x"),
                false,
                "x is a set member locally, but a non-atom set element is non-wf → gated false",
            )],
        },
        Case {
            // record with a duplicate field name "a" (req text then opt text) is non-wf;
            // {"a": text} satisfies both field clauses locally.
            name: "wf-gate-record-dup-field",
            descriptor: d(
                "record",
                vec![
                    a(b"open"),
                    field("a", true, scalar(b"utf8-text")),
                    field("a", false, scalar(b"utf8-text")),
                ],
            ),
            wf: false,
            values: vec![vc(
                map_v(&[("a", encode_text("hi"))]),
                false,
                "{a: text} satisfies both clauses locally, but the duplicate field is non-wf → gated false",
            )],
        },
        Case {
            // rec whose body reaches `ref x` with no descent — unguarded, non-wf. Any
            // value conforms via the `any` alternative locally.
            name: "wf-gate-unguarded-rec-union",
            descriptor: d(
                "rec",
                vec![
                    a(b"x"),
                    d("union", vec![d("any", vec![]), d("ref", vec![a(b"x")])]),
                ],
            ),
            wf: false,
            values: vec![vc(
                a(b"anything"),
                false,
                "the `any` alternative matches locally, but the unguarded ref is non-wf → gated false",
            )],
        },
        Case {
            // seq whose element descriptor is garbage (a non-descriptor) is non-wf; the
            // empty tuple `()` conforms vacuously (no element is ever checked) locally.
            name: "wf-gate-seq-garbage-elem",
            descriptor: d("seq", vec![tup(vec![a(b"garbage")])]),
            wf: false,
            values: vec![vc(
                tup(vec![]),
                false,
                "() conforms vacuously locally, but a non-wf element descriptor gates conforms false",
            )],
        },
        // ----- (#71) canonical-form negatives delegated to ontos-data -----
        // Each descriptor is wf; the VALUE is a hand-built NON-canonical ontos-data value
        // (the `encode_*` builders only ever emit the canonical form). The recognizer
        // (read_set/read_map/recognize_int) rejects it, so conforms is `false` — pinning the
        // rs/go/ts ontos-data lanes at exactly the seam a future ontos tag roll could drift.
        Case {
            name: "canon-set-out-of-order",
            descriptor: d("set-of", vec![d("atom", vec![])]),
            wf: true,
            values: vec![
                vc(
                    set_out_of_order(),
                    false,
                    "elements (\"b\", \"a\") are not strictly ascending → read_set rejects (UnsortedElements)",
                ),
                vc(
                    encode_set(&[a(b"a"), a(b"b")]).unwrap(),
                    true,
                    "the canonical (\"a\", \"b\") spelling DOES conform — pins that only order, not content, differs",
                ),
            ],
        },
        Case {
            name: "canon-set-duplicate-elem",
            descriptor: d("set-of", vec![d("atom", vec![])]),
            wf: true,
            values: vec![vc(
                set_duplicate_elem(),
                false,
                "elements (\"a\", \"a\") repeat → read_set rejects (DuplicateElement)",
            )],
        },
        Case {
            name: "canon-map-out-of-order",
            descriptor: d("map-of", vec![scalar(b"utf8-text"), scalar(b"int")]),
            wf: true,
            values: vec![
                vc(
                    map_out_of_order(),
                    false,
                    "keys (\"b\", \"a\") are not strictly ascending → read_map rejects (UnsortedEntries)",
                ),
                vc(
                    map_v(&[("a", encode_int(1)), ("b", encode_int(2))]),
                    true,
                    "the canonical key order DOES conform — pins that only ordering, not content, differs",
                ),
            ],
        },
        Case {
            name: "canon-map-duplicate-key",
            descriptor: d("map-of", vec![scalar(b"utf8-text"), scalar(b"int")]),
            wf: true,
            values: vec![vc(
                map_duplicate_key(),
                false,
                "key \"a\" repeats → read_map rejects (DuplicateKey)",
            )],
        },
        Case {
            name: "canon-int-non-minimal",
            descriptor: scalar(b"int"),
            wf: true,
            values: vec![
                vc(
                    int_non_minimal(),
                    false,
                    "magnitude [0x00, 0x05] carries a leading zero → recognize_int rejects (non-minimal)",
                ),
                vc(
                    encode_int(5),
                    true,
                    "the minimal spelling of 5 ([0x00, 0x05]) DOES conform — pins recognition is on the canonical form",
                ),
            ],
        },
        // ----- (#71) empty-arity composition -----
        // `union()`/`and()` are the n=0 edges of the §4 list-arity combinators (spec writes
        // `T₀…Tₙ₋₁` without authorizing n=0). All three cores fold them as the empty
        // any()/all(): `union()` is wf yet accepts NOTHING (bottom / uninhabited), `and()` is
        // wf and accepts EVERYTHING (top). This pins the CURRENT tri-core behavior — a
        // vectors PR changes no behavior; were n=0 to be ruled non-wf instead, that is a
        // separate spec/core decision out of scope here (issue #71 acceptance note).
        Case {
            name: "empty-union-is-bottom",
            descriptor: d("union", vec![]),
            wf: true,
            values: vec![
                vc(encode_int(1), false, "union() has no alternative to satisfy → bottom (uninhabited)"),
                vc(a(b"x"), false, "even a bare atom does not conform to the empty union"),
                vc(tup(vec![]), false, "the empty tuple does not conform either"),
            ],
        },
        Case {
            name: "empty-and-is-top",
            descriptor: d("and", vec![]),
            wf: true,
            values: vec![
                vc(encode_int(1), true, "and() has no conjunct to fail → top (accepts everything)"),
                vc(a(b"x"), true, "a bare atom conforms to the empty and"),
                vc(tup(vec![encode_int(1), a(b"x")]), true, "an arbitrary tuple conforms to the empty and"),
            ],
        },
        // ----- (#71/#37) depth cap at MAX_RECURSION_DEPTH = 512 -----
        // A deep VALUE over the SMALL `cons_list` descriptor. The boundary is exact and
        // identical across cores: a 170-deep cons value still conforms (frames stay <= 512),
        // a 171-deep one trips `depth > 512` at the same frame in rs/go/ts and returns a
        // deterministic `false`. Pinning BOTH sides makes the cap's tri-core agreement point
        // a theorem rather than a "way over trips" approximation — and pins that nothing
        // below the cap changed (170 conforms exactly as a shallow list would).
        Case {
            name: "depth-170-in-bound-conforms",
            descriptor: cons_list(),
            wf: true,
            values: vec![vc(
                deep_cons_value(170),
                true,
                "a 170-deep cons value stays within the depth cap → conforms, as a shallow list does",
            )],
        },
        Case {
            name: "depth-171-over-cap-false",
            descriptor: cons_list(),
            wf: true,
            values: vec![vc(
                deep_cons_value(171),
                false,
                "a 171-deep cons value trips depth > MAX_RECURSION_DEPTH (512) at the same frame in all three cores → deterministic false, not a crash",
            )],
        },
        // ----- (#29) worker-1 hardening: the decimal/null FAIL-CLOSED boundary (horos#103),
        // decidable complement (§6), and nested combinators. Additive; authored by hand from
        // docs/spec/horos.md. horos classifies 6 of ontos-data's 8 recognizers
        // (int/utf8-text/bool/list/map/set; NOT decimal/null). These pin that a *canonical*
        // decimal/null value is REJECTED with a clean `false` (never mis-read) by the
        // recognizer-delegating descriptors — while `horos/any` (any ontos value) still accepts
        // it. Locking the fail-closed behaviour in the oracle keeps the #103 v2 decision safe
        // whichever way it lands.
        Case {
            name: "scalar-int-vs-decimal-null",
            descriptor: scalar(b"int"),
            wf: true,
            values: vec![
                vc(encode_int(7), true, "an int conforms — sanity"),
                vc(
                    encode_decimal(0, 0).unwrap(),
                    false,
                    "the canonical decimal `decimal(0,0)` is NOT an int — recognize_int rejects the decimal embedding, so scalar(int) is a clean false, never a mis-read (horos#103: decimal is one of the two recognizers horos does not classify)",
                ),
                vc(
                    encode_null(),
                    false,
                    "`null` is NOT an int — fail-closed (horos#103: the other unclassified recognizer)",
                ),
            ],
        },
        Case {
            name: "any-accepts-decimal-null",
            descriptor: d("any", vec![]),
            wf: true,
            values: vec![
                vc(
                    encode_decimal(0, 0).unwrap(),
                    true,
                    "`horos/any` accepts ANY ontos value — including an ontos-data decimal; the fail-closed boundary is at the recognizer-delegating descriptors (scalar/list-of/...), not at `any`",
                ),
                vc(encode_null(), true, "`any` accepts `null` too — the contrast that scopes the fail-closed pins above"),
            ],
        },
        Case {
            name: "list-of-int-decimal-null-elem",
            descriptor: d("list-of", vec![scalar(b"int")]),
            wf: true,
            values: vec![
                vc(
                    encode_list(&[encode_int(1), encode_decimal(0, 0).unwrap()]),
                    false,
                    "a well-formed list with a decimal element — fail-closed at the element recognizer, not a mis-accept (horos#103)",
                ),
                vc(
                    encode_list(&[encode_int(1), encode_null()]),
                    false,
                    "a list with a `null` element — fail-closed the same way",
                ),
            ],
        },
        Case {
            name: "not-union-int-bool",
            descriptor: d("not", vec![d("union", vec![scalar(b"int"), scalar(b"bool")])]),
            wf: true,
            values: vec![
                vc(encode_int(1), false, "an int IS in the union → its complement rejects"),
                vc(encode_bool(true), false, "a bool IS in the union → complement rejects"),
                vc(
                    encode_text("x"),
                    true,
                    "text is neither int nor bool → does not conform to the union → decidable complement holds (§6, not negation-as-failure)",
                ),
                vc(a(b"x"), true, "a bare atom is neither → complement holds"),
            ],
        },
        Case {
            name: "not-text-len-1-3",
            descriptor: d("not", vec![d("text-len", vec![encode_int(1), encode_int(3)])]),
            wf: true,
            values: vec![
                vc(encode_text("ab"), false, "a 2-char text IS within [1,3] → complement rejects"),
                vc(encode_text("abcde"), true, "a 5-char text is outside [1,3] → complement holds"),
                vc(
                    encode_text(""),
                    true,
                    "empty text is below lo → does not conform to text-len → complement holds",
                ),
                vc(
                    encode_int(1),
                    true,
                    "a non-text value does not conform to text-len at all → decidable complement holds (§6: complement of a refinement is total over wrong-kind values, not NAF)",
                ),
            ],
        },
        Case {
            name: "not-not-int",
            descriptor: d("not", vec![d("not", vec![scalar(b"int")])]),
            wf: true,
            values: vec![
                vc(
                    encode_int(5),
                    true,
                    "double complement over a recognized value: not(not(int)) classifies an int as int does",
                ),
                vc(a(b"x"), false, "a bare atom is not an int → not(not(int)) rejects it"),
                vc(encode_text("x"), false, "text is not an int → not(not(int)) rejects it"),
            ],
        },
        Case {
            name: "list-of-union-int-bool",
            descriptor: d("list-of", vec![d("union", vec![scalar(b"int"), scalar(b"bool")])]),
            wf: true,
            values: vec![
                vc(
                    encode_list(&[encode_int(1), encode_bool(true), encode_int(2)]),
                    true,
                    "every element is int-or-bool → the nested list-of(union) conforms",
                ),
                vc(encode_list(&[]), true, "an empty list vacuously conforms"),
                vc(
                    encode_list(&[encode_int(1), encode_text("x")]),
                    false,
                    "a text element is neither int nor bool → the list rejects",
                ),
            ],
        },
        Case {
            name: "map-of-record-closed",
            descriptor: d(
                "map-of",
                vec![
                    scalar(b"utf8-text"),
                    d("record", vec![a(b"closed"), field("k", true, scalar(b"int"))]),
                ],
            ),
            wf: true,
            values: vec![
                vc(
                    map_v(&[("a", map_v(&[("k", encode_int(1))]))]),
                    true,
                    "text -> a valid closed record {k: int}",
                ),
                vc(
                    map_v(&[("a", map_v(&[("k", encode_text("x"))]))]),
                    false,
                    "the inner record's `k` is not an int → the nested map-of(record) rejects",
                ),
                vc(
                    map_v(&[("a", map_v(&[("k", encode_int(1)), ("x", encode_int(2))]))]),
                    false,
                    "the inner record is closed; an extra key is rejected through the map",
                ),
            ],
        },
        // ----- (#29, worker-1 slice 2) guarded-recursion edges + canonicalization NESTING.
        // Additive; hand-authored from docs/spec/horos.md §6-§7. The refinement negatives
        // (key-grammar / lowercase-uuid / text-non-blank) were already thorough, so this slice
        // targets the genuine gaps: a `ref` is guarded ONLY by a structural descent into a value
        // position (tuple/list/set/map/record); `union`/`and`/`not` do NOT guard. `union` is
        // already pinned (wf-gate-unguarded-rec-union) — these add `and`/`not`, plus a POSITIVE
        // list-of guard, and non-canonical values NESTED below the top level.
        Case {
            name: "wf-unguarded-rec-and",
            descriptor: d(
                "rec",
                vec![
                    a(b"x"),
                    d("and", vec![d("any", vec![]), d("ref", vec![a(b"x")])]),
                ],
            ),
            wf: false,
            values: vec![vc(
                a(b"anything"),
                false,
                "`and` is not a structural descent, so the ref under it is unguarded → non-wf (same rule as the union case)",
            )],
        },
        Case {
            name: "wf-unguarded-rec-not",
            descriptor: d("rec", vec![a(b"x"), d("not", vec![d("ref", vec![a(b"x")])])]),
            wf: false,
            values: vec![vc(
                a(b"anything"),
                false,
                "`not` is not a structural descent → the ref under it is unguarded → non-wf",
            )],
        },
        Case {
            // POSITIVE guarded recursion via a DIFFERENT guard than cons_list's `tuple`: a
            // `ref` under `list-of` IS a structural descent, so `rec x. list-of(ref x)` is wf.
            // Its values are the finitely-nested empty lists: [], [[]], [[],[[]]], ...
            name: "rec-list-of-guarded",
            descriptor: d("rec", vec![a(b"x"), d("list-of", vec![d("ref", vec![a(b"x")])])]),
            wf: true,
            values: vec![
                vc(encode_list(&[]), true, "the empty list is the base case (list-of vacuous)"),
                vc(
                    encode_list(&[encode_list(&[])]),
                    true,
                    "[[]] — one element, itself an empty list, conforms to `ref x`",
                ),
                vc(
                    encode_list(&[encode_list(&[]), encode_list(&[encode_list(&[])])]),
                    true,
                    "[[], [[]]] — every element is itself a list-of-x",
                ),
                vc(
                    encode_list(&[encode_int(1)]),
                    false,
                    "[1] — an int element is not a `list-of(ref x)`, so the list rejects",
                ),
                vc(encode_int(1), false, "a bare int is not a list at all"),
            ],
        },
        Case {
            // Canonicalization NESTING: the existing canon negatives are all top-level; this
            // pins that a non-canonical ontos-data value is rejected at a NESTED (element)
            // position too. The outer `list` is canonical; its single element is an
            // out-of-order `set`, which read_set rejects → the whole conforms is false.
            name: "canon-nested-set-in-list",
            descriptor: d("list-of", vec![d("set-of", vec![d("atom", vec![])])]),
            wf: true,
            values: vec![
                vc(
                    encode_list(&[encode_set(&[a(b"a"), a(b"b")]).unwrap()]),
                    true,
                    "a list of one CANONICAL set of atoms",
                ),
                vc(
                    encode_list(&[set_out_of_order()]),
                    false,
                    "a canonical list whose element is a NON-canonical (out-of-order) set → read_set rejects the nested value → false in all three cores",
                ),
            ],
        },
        Case {
            // Canon nesting on the map-VALUE axis: a canonical map (single entry, sorted key)
            // whose VALUE is a non-minimal int. read_map accepts the map shape, then the value
            // is checked against scalar(int); recognize_int rejects the non-minimal spelling.
            name: "canon-nested-int-in-map-value",
            descriptor: d("map-of", vec![scalar(b"utf8-text"), scalar(b"int")]),
            wf: true,
            values: vec![
                vc(
                    map_v(&[("a", encode_int(5))]),
                    true,
                    "text -> a canonical int value",
                ),
                vc(
                    map_v(&[("a", int_non_minimal())]),
                    false,
                    "a canonical map whose VALUE is a non-minimal int — recognize_int rejects the nested value, so scalar(int) fails through the map",
                ),
            ],
        },
        Case {
            // Refinement edge: `atom-in` over the EMPTY set is a wf descriptor that accepts
            // nothing (membership in ∅ is always false). Pins the empty-alternatives boundary,
            // the atom-in analogue of `empty-union-is-bottom`.
            name: "atom-in-empty",
            descriptor: d("atom-in", vec![encode_set(&[]).unwrap()]),
            wf: true,
            values: vec![
                vc(a(b"x"), false, "no atom is a member of the empty set"),
                vc(a(b""), false, "not even the empty atom is a member of ∅"),
                vc(encode_int(1), false, "a non-atom is rejected regardless"),
            ],
        },
        // ===================================================================
        // Leg B — the op-D asset_id PREIMAGE (economy shape-provider).
        //
        // SUPERSEDES the PIN-2 preimage (PR #126 @54f99fb): that 6-tuple carried
        // `exact_space` IN the identity and `realm_id : atom-len(32,32)`. PIN-2 was a
        // TRANSIENT double-embodiment ghost (07:28Z), superseded the same morning by
        // the 07:38Z UNIFIED ruling and made of-record by the independent authority
        // pair d86216ae (law-side, 11:34Z) + abb0fe02 (store-side, 11:35Z). Engineering
        // ground: ADR-0006 store-unrepresentability — `exact_space` IN the identity
        // broke cross-space moves (the content-address changed when an asset moved).
        //
        // GOVERNING (re-factored) form. The asset_id is the content-address of a
        // `record(closed, …)` 6-tuple in the law-of-record field order
        //   { realm_id, genesis, symbol, scale, divisible?, supply_mode }.
        // `exact_space` is OUT of the identity; `genesis` is IN (uniqueness =
        // issuer-lineage-genesis); home-space is OUT of the preimage entirely
        // (a separate `asset_home` fact, NOT a horos-preimage field). Leg A (the coin
        // BODY) still types asset_id OPAQUELY as `atom-len(32,32)` — that atom is the
        // hash of THIS preimage, UNTOUCHED by this re-freeze. Each field's typing:
        //   realm_id     : scalar(utf8-text) -- content-address STRING, unbounded at
        //                                       the form layer. lowercase-hex + width-64
        //                                       is a CANONICALIZATION invariant owned
        //                                       single-owner by ontos-codec/producer,
        //                                       NOT a horos recognizer bound: form
        //                                       classifies SHAPE (is-text/present/arity)
        //                                       and does NOT recognize the hex grammar.
        //                                       (form/codec boundary co-signed + hex
        //                                       grammar ruled codec-single-owner by
        //                                       aa920669/cca@ontos, 11:47Z; rides the
        //                                       symbol precedent — text atom, no form
        //                                       grammar, grammar-is-policy.)
        //   genesis      : scalar(utf8-text) -- issuer-lineage-genesis content-address;
        //                                       same utf8-text rule as realm_id.
        //   symbol       : scalar(utf8-text) -- utf8-text symbol (charset/len is
        //                                       economy admissibility, not form)
        //   scale        : scalar(int)       -- BARE int; magnitude is NOT a form
        //                                       bound (int-range §5-principled-out
        //                                       to logos — CAA ruling 2026-07-24)
        //   divisible?   : scalar(bool)      -- ontos bool embedding (golden-byte pin)
        //   supply_mode  : union(exact "fixed", exact "elastic") -- utf8-text symbol,
        //                                       NOT an int discriminant
        // The public surface is a single tri-core-identical BOOLEAN; there is no
        // rich-violation type. Author BY HAND from the spec (never emitter output).
        // ===================================================================
        Case {
            name: "asset-id-preimage",
            descriptor: d(
                "record",
                vec![
                    a(b"closed"),
                    field("realm_id", true, scalar(b"utf8-text")),
                    field("genesis", true, scalar(b"utf8-text")),
                    field("symbol", true, scalar(b"utf8-text")),
                    field("scale", true, scalar(b"int")),
                    field("divisible?", true, scalar(b"bool")),
                    field(
                        "supply_mode",
                        true,
                        d(
                            "union",
                            vec![
                                d("exact", vec![encode_text("fixed")]),
                                d("exact", vec![encode_text("elastic")]),
                            ],
                        ),
                    ),
                ],
            ),
            wf: true,
            values: vec![
                // -- positives: two canonical preimages spanning the flag/mode axes.
                //    realm_id/genesis are the producer's canonical content-address text
                //    (lowercase hex, width-64); the FORM only requires utf8-text, so these
                //    ride the scalar(utf8-text) accept-rule (symbol precedent) — any future
                //    codec-side hex tightening stays additive against these positives. --
                vc(
                    map_v(&[
                        ("divisible?", encode_bool(true)),
                        ("genesis", encode_text(&"a0".repeat(32))),
                        ("realm_id", encode_text(&"11".repeat(32))),
                        ("scale", encode_int(8)),
                        ("supply_mode", encode_text("fixed")),
                        ("symbol", encode_text("GOLD")),
                    ]),
                    true,
                    "canonical op-D asset_id preimage — divisible fixed-supply asset; realm_id/genesis as width-64 lowercase-hex content-address text",
                ),
                vc(
                    map_v(&[
                        ("divisible?", encode_bool(false)),
                        ("genesis", encode_text(&"cd".repeat(32))),
                        ("realm_id", encode_text(&"ab".repeat(32))),
                        ("scale", encode_int(1_000_000_000)),
                        ("supply_mode", encode_text("elastic")),
                        ("symbol", encode_text("KILO")),
                    ]),
                    true,
                    "indivisible elastic asset with a large (1e9) scale — magnitude is NOT a form bound (scale ruled bare int, int-range §5-principled-out)",
                ),
                // -- placement-in-preimage regression: every field is form-load-bearing.
                //    genesis is IN the re-factored identity (uniqueness = issuer-lineage
                //    -genesis); its absence is a form-negative. --
                vc(
                    map_v(&[
                        ("divisible?", encode_bool(true)),
                        ("realm_id", encode_text(&"11".repeat(32))),
                        ("scale", encode_int(8)),
                        ("supply_mode", encode_text("fixed")),
                        ("symbol", encode_text("GOLD")),
                    ]),
                    false,
                    "required genesis missing — genesis is a form-load-bearing identity field in the re-factored preimage",
                ),
                vc(
                    map_v(&[
                        ("genesis", encode_text(&"a0".repeat(32))),
                        ("realm_id", encode_text(&"11".repeat(32))),
                        ("scale", encode_int(8)),
                        ("supply_mode", encode_text("fixed")),
                        ("symbol", encode_text("GOLD")),
                    ]),
                    false,
                    "required divisible? missing",
                ),
                vc(
                    map_v(&[
                        ("divisible?", encode_bool(true)),
                        ("genesis", encode_text(&"a0".repeat(32))),
                        ("realm_id", encode_text(&"11".repeat(32))),
                        ("scale", encode_int(8)),
                        ("supply_mode", encode_text("fixed")),
                        ("symbol", encode_text("GOLD")),
                        ("zzz_extra", encode_int(1)),
                    ]),
                    false,
                    "extra key under closed preimage — a 7th field is not the frozen record",
                ),
                // -- supply_mode: utf8-text symbol, NOT an int discriminant --
                vc(
                    map_v(&[
                        ("divisible?", encode_bool(true)),
                        ("genesis", encode_text(&"a0".repeat(32))),
                        ("realm_id", encode_text(&"11".repeat(32))),
                        ("scale", encode_int(8)),
                        ("supply_mode", encode_int(0)),
                        ("symbol", encode_text("GOLD")),
                    ]),
                    false,
                    "supply_mode as an int discriminant — rejected; the symbol set is utf8-text",
                ),
                vc(
                    map_v(&[
                        ("divisible?", encode_bool(true)),
                        ("genesis", encode_text(&"a0".repeat(32))),
                        ("realm_id", encode_text(&"11".repeat(32))),
                        ("scale", encode_int(8)),
                        ("supply_mode", encode_text("burn")),
                        ("symbol", encode_text("GOLD")),
                    ]),
                    false,
                    "supply_mode text outside the closed {fixed, elastic} set — rejected",
                ),
                // -- realm_id/genesis are utf8-text content-address STRINGS, not atoms.
                //    The voided PIN-2 form typed realm_id as atom-len(32,32); under the
                //    re-factored governing form that 32-byte atom is a form-NEGATIVE — this
                //    pins the supersession (d86216ae/abb0fe02, 11:34/11:35Z). --
                vc(
                    map_v(&[
                        ("divisible?", encode_bool(true)),
                        ("genesis", encode_text(&"a0".repeat(32))),
                        ("realm_id", a(&[0x11u8; 32])),
                        ("scale", encode_int(8)),
                        ("supply_mode", encode_text("fixed")),
                        ("symbol", encode_text("GOLD")),
                    ]),
                    false,
                    "realm_id as a 32-byte atom — rejected; the re-factored form carries realm_id as utf8-text, not the voided PIN-2 atom-len(32,32)",
                ),
                vc(
                    map_v(&[
                        ("divisible?", encode_bool(true)),
                        ("genesis", encode_int(1)),
                        ("realm_id", encode_text(&"11".repeat(32))),
                        ("scale", encode_int(8)),
                        ("supply_mode", encode_text("fixed")),
                        ("symbol", encode_text("GOLD")),
                    ]),
                    false,
                    "genesis as a bare int — rejected; genesis is a utf8-text content-address embedding",
                ),
                // -- symbol: a utf8-text embedding, not a bare int --
                vc(
                    map_v(&[
                        ("divisible?", encode_bool(true)),
                        ("genesis", encode_text(&"a0".repeat(32))),
                        ("realm_id", encode_text(&"11".repeat(32))),
                        ("scale", encode_int(8)),
                        ("supply_mode", encode_text("fixed")),
                        ("symbol", encode_int(1)),
                    ]),
                    false,
                    "symbol as a bare int — rejected; symbol is a utf8-text embedding",
                ),
                // -- divisible? malformed bool payload, rejected THROUGH the record --
                vc(
                    map_v(&[
                        ("divisible?", tup(vec![a(b"bool"), a(&[])])),
                        ("genesis", encode_text(&"a0".repeat(32))),
                        ("realm_id", encode_text(&"11".repeat(32))),
                        ("scale", encode_int(8)),
                        ("supply_mode", encode_text("fixed")),
                        ("symbol", encode_text("GOLD")),
                    ]),
                    false,
                    "divisible? with an empty bool payload — rejected; scalar(bool) fails through the record",
                ),
            ],
        },
        // Standalone divisible? atom — the ontos bool embedding, pinned sharply.
        // Canonical bool = Tuple(Atom(\"bool\"), Atom([0x00|0x01])); the payload MUST be
        // exactly one byte in {0x00, 0x01}. Malformed payloads are NEGATIVES.
        Case {
            name: "divisible-flag-bool",
            descriptor: scalar(b"bool"),
            wf: true,
            values: vec![
                vc(encode_bool(true), true, "canonical true"),
                vc(encode_bool(false), true, "canonical false"),
                vc(tup(vec![a(b"bool"), a(&[])]), false, "empty payload — not a 1-byte bool"),
                vc(
                    tup(vec![a(b"bool"), a(&[0x01, 0x00])]),
                    false,
                    "two-byte payload — a bool payload is exactly one byte",
                ),
                vc(
                    tup(vec![a(b"bool"), a(&[0x02])]),
                    false,
                    "payload byte 0x02 — outside the {0x00, 0x01} range",
                ),
                vc(encode_int(1), false, "a bare int is not the bool embedding"),
            ],
        },
        // Standalone supply_mode symbol — a closed utf8-text set, NO int discriminant.
        Case {
            name: "supply-mode-symbol",
            descriptor: d(
                "union",
                vec![
                    d("exact", vec![encode_text("fixed")]),
                    d("exact", vec![encode_text("elastic")]),
                ],
            ),
            wf: true,
            values: vec![
                vc(encode_text("fixed"), true, "the fixed symbol"),
                vc(encode_text("elastic"), true, "the elastic symbol"),
                vc(encode_text("burn"), false, "a utf8-text outside the closed set"),
                vc(encode_int(0), false, "an int discriminant — supply_mode is a utf8-text symbol, not an int"),
                vc(a(b"fixed"), false, "a bare atom — not the utf8-text embedding of \"fixed\""),
            ],
        },
    ]
}

// ----- hex (avoid a dep; total over the even-length hex we emit) -----

fn to_hex(bytes: &[u8]) -> String {
    bytes.iter().map(|b| format!("{b:02x}")).collect()
}
fn from_hex(s: &str) -> Vec<u8> {
    (0..s.len())
        .step_by(2)
        .map(|i| u8::from_str_radix(&s[i..i + 2], 16).unwrap())
        .collect()
}

// ----- the two tests -----

/// This core agrees with the hand-authored oracle. With `HOROS_EMIT_VECTORS=1`, also
/// (re)writes `vectors/conformance.json` from the authored cases.
#[test]
fn vectors_authored_selfcheck() {
    let cases = authored();
    for c in &cases {
        assert_eq!(wf(&c.descriptor), c.wf, "wf({})", c.name);
        for v in &c.values {
            assert_eq!(
                conforms(&v.value, &c.descriptor),
                v.conforms,
                "{} / {}",
                c.name,
                v.note
            );
        }
    }
    if std::env::var("HOROS_EMIT_VECTORS").is_ok() {
        emit(&cases);
    }
}

fn emit(cases: &[Case]) {
    let cases_json: Vec<serde_json::Value> = cases
        .iter()
        .map(|c| {
            serde_json::json!({
                "name": c.name,
                "descriptor": to_hex(&encode(&c.descriptor)),
                "wf": c.wf,
                "values": c.values.iter().map(|v| serde_json::json!({
                    "value": to_hex(&encode(&v.value)),
                    "conforms": v.conforms,
                    "note": v.note,
                })).collect::<Vec<_>>(),
            })
        })
        .collect();
    let doc = serde_json::json!({
        "version": "horos-v1",
        "note": "Hand-authored conformance oracle for horos. `descriptor` and `value` are ontos-codec-v1 lowercase hex; `wf`/`conforms` are the expected judgments, authored from docs/spec/horos.md. Every core decodes the hex and must agree byte-for-byte.",
        "cases": cases_json,
    });
    let path = format!(
        "{}/../../vectors/conformance.json",
        env!("CARGO_MANIFEST_DIR")
    );
    std::fs::write(&path, serde_json::to_string_pretty(&doc).unwrap() + "\n")
        .expect("write vectors/conformance.json");
}

/// Replay the committed `vectors/conformance.json` — decode each descriptor/value from
/// `ontos-codec-v1` hex and re-check `wf`/`conforms`. This is the exact replay the Go
/// and TS cores run, so the three agree on one oracle.
#[test]
fn vectors_replay() {
    let path = format!(
        "{}/testdata/conformance.json",
        env!("CARGO_MANIFEST_DIR")
    );
    let txt = std::fs::read_to_string(&path)
        .expect("vectors/conformance.json missing — run with HOROS_EMIT_VECTORS=1 first");
    let doc: serde_json::Value = serde_json::from_str(&txt).unwrap();
    for c in doc["cases"].as_array().unwrap() {
        let name = c["name"].as_str().unwrap();
        let desc = decode(&from_hex(c["descriptor"].as_str().unwrap())).unwrap();
        assert_eq!(wf(&desc), c["wf"].as_bool().unwrap(), "wf {name}");
        for v in c["values"].as_array().unwrap() {
            let val = decode(&from_hex(v["value"].as_str().unwrap())).unwrap();
            assert_eq!(
                conforms(&val, &desc),
                v["conforms"].as_bool().unwrap(),
                "{name} / {}",
                v["note"].as_str().unwrap_or("")
            );
        }
    }
}
