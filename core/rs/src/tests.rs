//! Unit tests for the horos recognizer: `conforms` (`v : T`) and `wf` over the full
//! descriptor DSL (incl. `rec`/`ref`). Descriptors are ordinary ontos values built with
//! the `d`/`a`/`tup` helpers; scalar/collection payloads use the real ontos/data
//! encoders, so these exercise the actual delegation path.

use super::{conforms, wf, MAX_RECURSION_DEPTH};
use ontos_core::{Tuple, Value};
use ontos_data::{encode_bool, encode_int, encode_list, encode_map, encode_set, encode_text};

// ----- builders -----

fn a(bytes: &[u8]) -> Value {
    Value::atom(bytes.to_vec())
}
fn tup(items: Vec<Value>) -> Value {
    Value::Tuple(Tuple::new(items))
}
/// A descriptor `Tuple(Atom("horos/<ctor>"), args...)`.
fn d(ctor: &str, args: Vec<Value>) -> Value {
    let mut items = vec![a(format!("horos/{ctor}").as_bytes())];
    items.extend(args);
    tup(items)
}
/// A field descriptor for `horos/record`.
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
fn scalar(kind: &[u8]) -> Value {
    d("scalar", vec![a(kind)])
}
fn map_of(pairs: &[(&str, Value)]) -> Value {
    let entries: Vec<(Value, Value)> = pairs
        .iter()
        .map(|(k, v)| (encode_text(k), v.clone()))
        .collect();
    encode_map(&entries).unwrap()
}

// ----- top / atoms -----

#[test]
fn any_accepts_everything() {
    let any = d("any", vec![]);
    assert!(conforms(&a(b"x"), &any));
    assert!(conforms(&tup(vec![]), &any));
    assert!(conforms(&encode_int(5), &any));
}

#[test]
fn atom_combinators() {
    assert!(conforms(&a(b"x"), &d("atom", vec![])));
    assert!(!conforms(&tup(vec![]), &d("atom", vec![]))); // a tuple is not an atom

    let exact = d("atom-exact", vec![a(b"hi")]);
    assert!(conforms(&a(b"hi"), &exact));
    assert!(!conforms(&a(b"ho"), &exact));
    assert!(!conforms(&tup(vec![a(b"hi")]), &exact));

    let len_1_3 = d("atom-len", vec![encode_int(1), encode_int(3)]);
    assert!(conforms(&a(b"x"), &len_1_3));
    assert!(conforms(&a(b"abc"), &len_1_3));
    assert!(!conforms(&a(b""), &len_1_3)); // 0 < 1
    assert!(!conforms(&a(b"abcd"), &len_1_3)); // 4 > 3

    let in_xy = d("atom-in", vec![encode_set(&[a(b"x"), a(b"y")]).unwrap()]);
    assert!(conforms(&a(b"x"), &in_xy));
    assert!(conforms(&a(b"y"), &in_xy));
    assert!(!conforms(&a(b"z"), &in_xy));
}

// ----- text refinements (horos-internal#68): char-counted bounds + grammar classes -----

#[test]
fn text_len_counts_chars_not_bytes() {
    let len_1_3 = d("text-len", vec![encode_int(1), encode_int(3)]);
    assert!(conforms(&encode_text("a"), &len_1_3));
    assert!(conforms(&encode_text("abc"), &len_1_3));
    assert!(!conforms(&encode_text(""), &len_1_3)); // 0 chars < lo
    assert!(!conforms(&encode_text("abcd"), &len_1_3)); // 4 chars > hi
                                                        // "ééé": 3 chars but 6 bytes — char-count is in range, so it conforms (a byte
                                                        // count would wrongly reject). The load-bearing multi-byte case (horos-internal#68).
    assert!(conforms(&encode_text("ééé"), &len_1_3));
    assert!(!conforms(&encode_text("éééé"), &len_1_3)); // 4 chars > hi
                                                        // An astral emoji is ONE code point (4 bytes; two UTF-16 units).
    assert!(conforms(&encode_text("😀"), &len_1_3));
    assert!(!conforms(&encode_int(1), &len_1_3)); // not utf8-text

    // wf: 0 <= lo <= hi, like atom-len.
    assert!(wf(&d("text-len", vec![encode_int(0), encode_int(0)])));
    assert!(!wf(&d("text-len", vec![encode_int(3), encode_int(1)]))); // lo > hi
    assert!(!wf(&d("text-len", vec![encode_int(-1), encode_int(3)]))); // lo < 0
}

#[test]
fn key_grammar_matches_the_class() {
    let kg = d("key-grammar", vec![]);
    assert!(conforms(&encode_text("a"), &kg));
    assert!(conforms(&encode_text("0"), &kg));
    assert!(conforms(&encode_text("a.b_c-1"), &kg));
    assert!(!conforms(&encode_text(""), &kg)); // empty
    assert!(!conforms(&encode_text("A"), &kg)); // uppercase
    assert!(!conforms(&encode_text(".ab"), &kg)); // bad leading char
    assert!(!conforms(&encode_text("a b"), &kg)); // space not allowed
    assert!(!conforms(&encode_text("café"), &kg)); // multi-byte char not in class
                                                   // 64 chars (1 + 63) ok; 65 over.
    assert!(conforms(&encode_text(&format!("a{}", "b".repeat(63))), &kg));
    assert!(!conforms(
        &encode_text(&format!("a{}", "b".repeat(64))),
        &kg
    ));
    assert!(!conforms(&encode_int(1), &kg)); // not utf8-text
}

#[test]
fn text_non_blank_requires_a_non_ws_char() {
    let nb = d("text-non-blank", vec![]);
    assert!(conforms(&encode_text("a"), &nb));
    assert!(conforms(&encode_text("  x  "), &nb));
    assert!(!conforms(&encode_text(""), &nb));
    assert!(!conforms(&encode_text("   "), &nb));
    assert!(!conforms(&encode_text("\t\n\r"), &nb));
    // Non-ASCII chars (incl. U+00A0 NO-BREAK SPACE) are never ASCII-whitespace.
    assert!(conforms(&encode_text("é"), &nb));
    assert!(conforms(&encode_text("\u{00A0}"), &nb));
    assert!(!conforms(&encode_int(1), &nb)); // not utf8-text
}

#[test]
fn lowercase_uuid_matches_the_canonical_grammar() {
    let u = d("lowercase-uuid", vec![]);
    assert!(conforms(
        &encode_text("00000000-0000-0000-0000-000000000000"),
        &u
    ));
    assert!(conforms(
        &encode_text("9b2e4c1a-7d3f-4e8b-a1c2-d3e4f5061728"),
        &u
    ));
    assert!(!conforms(
        &encode_text("9B2E4C1A-7D3F-4E8B-A1C2-D3E4F5061728"),
        &u
    )); // uppercase
    assert!(!conforms(
        &encode_text("9b2e4c1a7d3f4e8ba1c2d3e4f5061728"),
        &u
    )); // no hyphens
    assert!(!conforms(
        &encode_text("9b2e4c1g-7d3f-4e8b-a1c2-d3e4f5061728"),
        &u
    )); // 'g' not hex
    assert!(!conforms(&encode_text(""), &u));
    assert!(!conforms(&encode_int(1), &u)); // not utf8-text
}

// ----- scalars (delegate to ontos/data) -----

#[test]
fn scalar_delegates_to_ontos_data() {
    assert!(conforms(&encode_int(5), &scalar(b"int")));
    assert!(conforms(&encode_int(-1), &scalar(b"int")));
    assert!(!conforms(&encode_text("5"), &scalar(b"int")));

    assert!(conforms(&encode_text("hi"), &scalar(b"utf8-text")));
    assert!(!conforms(&encode_int(1), &scalar(b"utf8-text")));

    assert!(conforms(&encode_bool(true), &scalar(b"bool")));
    assert!(!conforms(&encode_int(1), &scalar(b"bool")));

    // A non-canonical int (leading-zero magnitude) is NOT a well-formed int.
    let noncanon = tup(vec![a(b"int"), a(&[0x00, 0x00, 0x01])]);
    assert!(!conforms(&noncanon, &scalar(b"int")));
}

// ----- tuples & compounds -----

#[test]
fn tuple_seq_compound() {
    let pair = d("tuple", vec![scalar(b"int"), d("atom", vec![])]);
    assert!(conforms(&tup(vec![encode_int(1), a(b"x")]), &pair));
    assert!(!conforms(&tup(vec![encode_int(1)]), &pair)); // wrong arity
    assert!(!conforms(&tup(vec![a(b"x"), a(b"x")]), &pair)); // child 0 not an int
    assert!(!conforms(&a(b"x"), &pair)); // not a tuple

    let seq_int = d("seq", vec![scalar(b"int")]);
    assert!(conforms(&tup(vec![encode_int(1), encode_int(2)]), &seq_int));
    assert!(conforms(&tup(vec![]), &seq_int)); // vacuously
    assert!(!conforms(&tup(vec![encode_int(1), a(b"x")]), &seq_int));

    let point = d(
        "compound",
        vec![a(b"point"), scalar(b"int"), scalar(b"int")],
    );
    assert!(conforms(
        &tup(vec![a(b"point"), encode_int(1), encode_int(2)]),
        &point
    ));
    assert!(!conforms(
        &tup(vec![a(b"line"), encode_int(1), encode_int(2)]),
        &point
    )); // label
    assert!(!conforms(&tup(vec![a(b"point"), encode_int(1)]), &point)); // arity
}

// ----- structural collections (delegate well-formedness to ontos/data) -----

#[test]
fn collections() {
    let list_int = d("list-of", vec![scalar(b"int")]);
    assert!(conforms(
        &encode_list(&[encode_int(1), encode_int(2)]),
        &list_int
    ));
    assert!(conforms(&encode_list(&[]), &list_int));
    assert!(!conforms(
        &encode_list(&[encode_int(1), encode_text("x")]),
        &list_int
    ));
    assert!(!conforms(&encode_int(1), &list_int)); // not a list

    let set_atom = d("set-of", vec![d("atom", vec![])]);
    assert!(conforms(
        &encode_set(&[a(b"x"), a(b"y")]).unwrap(),
        &set_atom
    ));
    assert!(!conforms(&encode_list(&[a(b"x")]), &set_atom)); // a list is not a set

    let map_ti = d("map-of", vec![scalar(b"utf8-text"), scalar(b"int")]);
    assert!(conforms(
        &map_of(&[("a", encode_int(1)), ("b", encode_int(2))]),
        &map_ti
    ));
    assert!(!conforms(&map_of(&[("a", encode_text("x"))]), &map_ti)); // value not int
}

// ----- records -----

#[test]
fn records() {
    let rec_closed = d(
        "record",
        vec![
            a(b"closed"),
            field("name", true, scalar(b"utf8-text")),
            field("age", false, scalar(b"int")),
        ],
    );
    let rec_open = d(
        "record",
        vec![a(b"open"), field("name", true, scalar(b"utf8-text"))],
    );

    // required present, optional absent, no extra keys -> ok (closed).
    assert!(conforms(
        &map_of(&[("name", encode_text("ada"))]),
        &rec_closed
    ));
    // required + optional present -> ok.
    assert!(conforms(
        &map_of(&[("age", encode_int(36)), ("name", encode_text("ada"))]),
        &rec_closed
    ));
    // required field missing -> reject.
    assert!(!conforms(&map_of(&[("age", encode_int(36))]), &rec_closed));
    // wrong type for a field -> reject.
    assert!(!conforms(&map_of(&[("name", encode_int(1))]), &rec_closed));
    // extra key under `closed` -> reject; under `open` -> ok.
    let with_extra = map_of(&[("name", encode_text("ada")), ("x", encode_int(1))]);
    assert!(!conforms(&with_extra, &rec_closed));
    assert!(conforms(&with_extra, &rec_open));
    // not a map -> reject.
    assert!(!conforms(&encode_list(&[]), &rec_closed));
}

// ----- composition -----

#[test]
fn union_and_not_exact() {
    let u = d("union", vec![scalar(b"int"), scalar(b"bool")]);
    assert!(conforms(&encode_int(1), &u));
    assert!(conforms(&encode_bool(true), &u));
    assert!(!conforms(&encode_text("x"), &u));

    let both = d(
        "and",
        vec![
            d("atom", vec![]),
            d("atom-len", vec![encode_int(1), encode_int(3)]),
        ],
    );
    assert!(conforms(&a(b"ab"), &both));
    assert!(!conforms(&a(b""), &both)); // fails the length bound
    assert!(!conforms(&tup(vec![]), &both)); // not an atom

    let not_int = d("not", vec![scalar(b"int")]);
    assert!(!conforms(&encode_int(1), &not_int));
    assert!(conforms(&encode_text("x"), &not_int));
    assert!(conforms(&a(b"x"), &not_int));

    let exact7 = d("exact", vec![encode_int(7)]);
    assert!(conforms(&encode_int(7), &exact7));
    assert!(!conforms(&encode_int(8), &exact7));
}

/// ADR 0013: a union reached outside shared mode begins a region with a memo of its own.
/// Outside shared mode the check walks the copies ontos/data returns, and each record's
/// copies are freed before the next record's reuse their memory. A memo kept across
/// regions would take a failing record's element for the conforming one checked at the
/// same address before it.
#[test]
fn shared_regions_do_not_outlive_their_values() {
    let alternative = |kind: &[u8]| d("tuple", vec![scalar(kind)]);
    let element = d(
        "union",
        vec![alternative(b"int"), alternative(b"utf8-text")],
    );
    let desc = d(
        "list-of",
        vec![d(
            "record",
            vec![a(b"closed"), field("x", true, d("list-of", vec![element]))],
        )],
    );
    let good = map_of(&[("x", encode_list(&[tup(vec![encode_int(1)])]))]);
    let bad = map_of(&[("x", encode_list(&[tup(vec![a(b"y")])]))]);
    let alternating: Vec<Value> = (0..64)
        .map(|i| {
            if i % 2 == 0 {
                good.clone()
            } else {
                bad.clone()
            }
        })
        .collect();
    assert!(!conforms(&encode_list(&alternating), &desc));
    assert!(!conforms(&encode_list(&[good.clone(), bad]), &desc));
    assert!(conforms(&encode_list(&vec![good; 64]), &desc));
}

// ----- a non-descriptor is never a conformance target -----

#[test]
fn non_descriptor_target_is_false() {
    assert!(!conforms(&encode_int(1), &a(b"x"))); // bare atom
    assert!(!conforms(&encode_int(1), &tup(vec![a(b"int")]))); // unreserved label
    assert!(!conforms(&encode_int(1), &d("nope", vec![]))); // unknown ctor
}

// ----- wf -----

#[test]
fn wf_accepts_the_dsl() {
    let good = [
        d("any", vec![]),
        d("atom", vec![]),
        d("atom-exact", vec![a(b"hi")]),
        d("atom-len", vec![encode_int(0), encode_int(64)]),
        d("atom-in", vec![encode_set(&[a(b"x")]).unwrap()]),
        scalar(b"int"),
        scalar(b"utf8-text"),
        scalar(b"bool"),
        d("tuple", vec![scalar(b"int"), d("atom", vec![])]),
        d("seq", vec![scalar(b"int")]),
        d(
            "compound",
            vec![a(b"point"), scalar(b"int"), scalar(b"int")],
        ),
        d("list-of", vec![scalar(b"int")]),
        d("set-of", vec![d("atom", vec![])]),
        d("map-of", vec![scalar(b"utf8-text"), scalar(b"int")]),
        d(
            "record",
            vec![a(b"closed"), field("name", true, scalar(b"utf8-text"))],
        ),
        d("union", vec![scalar(b"int"), scalar(b"bool")]),
        d("and", vec![d("atom", vec![])]),
        d("not", vec![scalar(b"int")]),
        d("exact", vec![encode_int(7)]),
    ];
    for t in &good {
        assert!(wf(t), "expected wf: {t}");
    }
}

#[test]
fn wf_rejects_malformed() {
    let bad = [
        a(b"x"),                                                   // bare atom, not a descriptor
        tup(vec![a(b"int")]),                                      // unreserved label
        d("scalar", vec![a(b"float")]),                            // unknown scalar kind
        d("atom-len", vec![encode_int(3), encode_int(1)]),         // lo > hi
        d("atom-len", vec![encode_int(-1), encode_int(2)]),        // lo < 0
        d("atom-in", vec![encode_set(&[encode_int(1)]).unwrap()]), // set element not an atom
        d(
            "record",
            vec![a(b"maybe"), field("n", true, scalar(b"int"))],
        ), // bad mode flag
        d(
            "record",
            vec![
                a(b"closed"),
                field("n", true, d("scalar", vec![a(b"nope")])),
            ],
        ), // bad field type
        d("tuple", vec![d("nope", vec![])]),                       // sub-descriptor not wf
        d("rec", vec![a(b"x"), d("ref", vec![a(b"x")])]),          // rec(x, ref x): unguarded
        d("ref", vec![a(b"x")]),                                   // unbound ref
        field("n", true, scalar(b"int")), // a field is not a top-level descriptor
    ];
    for t in &bad {
        assert!(!wf(t), "expected NOT wf: {t}");
    }
}

#[test]
fn wf_rejects_duplicate_record_fields() {
    let dup = d(
        "record",
        vec![
            a(b"open"),
            field("name", true, scalar(b"utf8-text")),
            field("name", false, scalar(b"int")),
        ],
    );
    assert!(!wf(&dup));
}

// ----- rec / ref (guarded recursive descriptors) -----

/// A nil-terminated cons-list of ints: `rec l. (atom-exact "nil") | (int, ref l)`.
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

/// A binary tree of ints: `rec t. int | node(ref t, ref t)`.
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

#[test]
fn recursive_conformance() {
    let l = cons_list();
    assert!(wf(&l));
    assert!(conforms(&a(b"nil"), &l)); // []
    assert!(conforms(&tup(vec![encode_int(1), a(b"nil")]), &l)); // [1]
    assert!(conforms(
        &tup(vec![encode_int(1), tup(vec![encode_int(2), a(b"nil")])]),
        &l
    )); // [1, 2]
    assert!(!conforms(&tup(vec![encode_text("x"), a(b"nil")]), &l)); // head not an int
    assert!(!conforms(&a(b"other"), &l)); // not nil and not a cons cell
    assert!(!conforms(&tup(vec![encode_int(1)]), &l)); // missing the tail

    let t = int_tree();
    assert!(wf(&t));
    assert!(conforms(&encode_int(5), &t)); // a leaf
    assert!(conforms(
        &tup(vec![a(b"node"), encode_int(1), encode_int(2)]),
        &t
    )); // node(1, 2)
    assert!(conforms(
        &tup(vec![
            a(b"node"),
            encode_int(1),
            tup(vec![a(b"node"), encode_int(2), encode_int(3)]),
        ]),
        &t
    )); // node(1, node(2, 3))
    assert!(!conforms(
        &tup(vec![a(b"node"), encode_text("x"), encode_int(2)]),
        &t
    )); // left subtree is neither an int nor a node
}

#[test]
fn wf_recursion_guard() {
    // Guarded recursion is well-formed.
    assert!(wf(&cons_list()));
    assert!(wf(&int_tree()));

    // Unguarded / unbound recursion is not.
    let bad = [
        d("rec", vec![a(b"x"), d("ref", vec![a(b"x")])]), // rec x. ref x — no descent
        d(
            "rec",
            vec![
                a(b"x"),
                d("union", vec![d("ref", vec![a(b"x")]), scalar(b"int")]),
            ],
        ), // union is same-v: still unguarded
        d(
            "rec",
            vec![
                a(b"x"),
                d("and", vec![d("atom", vec![]), d("ref", vec![a(b"x")])]),
            ],
        ), // and is same-v: unguarded
        d("ref", vec![a(b"y")]),                          // unbound
    ];
    for t in &bad {
        assert!(!wf(t), "expected NOT wf: {t}");
    }
}

// ----- recursion-depth guard (horos-internal#37) -----

/// Build an adversarial-but-finite value: a right-nested cons cell `(0, (0, … nil))`
/// `levels` deep, constructed ITERATIVELY (so building it never recurses) — this is the
/// shape that would overflow the native stack under an unguarded recognizer.
fn deep_cons_value(levels: usize) -> Value {
    let mut v = a(b"nil");
    for _ in 0..levels {
        v = tup(vec![encode_int(0), v]);
    }
    v
}

/// Build a left-nested `seq` descriptor `seq(seq(… seq(int)))` `levels` deep — a
/// deep-but-finite DESCRIPTOR for the `wf` side, also built iteratively.
fn deep_seq_descriptor(levels: usize) -> Value {
    let mut t = scalar(b"int");
    for _ in 0..levels {
        t = d("seq", vec![t]);
    }
    t
}

/// One value-level of the cons descriptor costs several `conforms_in` frames (rec →
/// union → tuple → ref → …), so a value this many levels deep is guaranteed to drive the
/// recognizer past `MAX_RECURSION_DEPTH` frames and trip the guard, while staying shallow
/// enough that constructing and dropping the value never overflows the test stack itself.
const PAST_CAP_LEVELS: usize = MAX_RECURSION_DEPTH + 64;

#[test]
fn recursion_depth_guard_conforms_returns_false_without_overflow() {
    let cons = cons_list();

    // A shallow, in-bound cons-list still conforms — the guard never trips below the cap,
    // so no existing behavior changes.
    assert!(
        conforms(&deep_cons_value(8), &cons),
        "shallow list conforms"
    );

    // A value deeper than MAX_RECURSION_DEPTH is classified as NON-conforming
    // deterministically — the guard returns `false` before the native stack can overflow.
    // Were the guard absent, an adversary could drive this arbitrarily deep and crash the
    // recognizer (panic / fatal / RangeError); with the guard it is a clean `false`.
    let very_deep = deep_cons_value(PAST_CAP_LEVELS);
    assert!(
        !conforms(&very_deep, &cons),
        "a value past the depth cap is non-conforming, not a crash"
    );
    // `very_deep` is ~PAST_CAP_LEVELS nodes deep; its `Value` Drop is itself recursive, so
    // leak it rather than risk overflowing the (small) test-thread stack on teardown —
    // the recognizer's guard, not Drop, is what is under test here.
    std::mem::forget(very_deep);
}

#[test]
fn recursion_depth_guard_wf_returns_false_without_overflow() {
    // A shallow seq descriptor is well-formed (guard never trips below the cap).
    assert!(wf(&deep_seq_descriptor(8)), "shallow seq descriptor is wf");

    // A descriptor nested past the cap is treated as NON-wf deterministically, never
    // overflowing the stack.
    let very_deep = deep_seq_descriptor(PAST_CAP_LEVELS);
    assert!(
        !wf(&very_deep),
        "a descriptor past the depth cap is non-wf, not a crash"
    );
    std::mem::forget(very_deep); // see the conforms test — avoid recursive Drop on teardown
}
