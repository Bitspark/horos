// Unit tests for the horos recognizer: conforms (v : T) and wf over the full
// descriptor DSL (incl. rec/ref). These mirror core/rs/src/tests.rs and
// core/go/horos_test.go, building descriptors with d/a/tup helpers and
// scalar/collection payloads via the real ontos/data encoders, so they exercise the
// actual delegation path.

import { test } from "node:test";
import assert from "node:assert/strict";

import { atom, tuple, type Value } from "@bitspark/ontos-core";
import {
  encodeBool,
  encodeInt,
  encodeList,
  encodeMap,
  encodeSet,
  encodeText,
} from "@bitspark/ontos-data";
import { conforms, MAX_RECURSION_DEPTH, wf } from "../src/index.js";

const enc = new TextEncoder();

// ----- builders -----

function a(s: string): Value {
  return atom(enc.encode(s));
}
function aBytes(bytes: number[]): Value {
  return atom(Uint8Array.from(bytes));
}
function tup(...items: Value[]): Value {
  return tuple(items);
}
/** A descriptor Tuple(Atom("horos/<ctor>"), args...). */
function d(ctor: string, ...args: Value[]): Value {
  return tuple([a(`horos/${ctor}`), ...args]);
}
function scalar(kind: string): Value {
  return d("scalar", a(kind));
}
/** A field descriptor for horos/record. */
function field(name: string, required: boolean, ty: Value): Value {
  return d("field", encodeText(name), a(required ? "req" : "opt"), ty);
}
function mapOf(pairs: Array<[string, Value]>): Value {
  return encodeMap(pairs.map(([k, v]) => [encodeText(k), v]));
}

function assertConforms(v: Value, ty: Value, want: boolean, msg: string): void {
  assert.equal(conforms(v, ty), want, msg);
}
function assertWF(ty: Value, want: boolean, msg: string): void {
  assert.equal(wf(ty), want, msg);
}

// ----- top / atoms -----

test("any accepts everything", () => {
  const any = d("any");
  assertConforms(a("x"), any, true, "atom");
  assertConforms(tup(), any, true, "empty tuple");
  assertConforms(encodeInt(5n), any, true, "int");
});

test("atom combinators", () => {
  assertConforms(a("x"), d("atom"), true, "atom");
  assertConforms(tup(), d("atom"), false, "a tuple is not an atom");

  const exact = d("atom-exact", a("hi"));
  assertConforms(a("hi"), exact, true, "exact match");
  assertConforms(a("ho"), exact, false, "different bytes");
  assertConforms(tup(a("hi")), exact, false, "a tuple, not an atom");

  const len13 = d("atom-len", encodeInt(1n), encodeInt(3n));
  assertConforms(a("x"), len13, true, "len 1");
  assertConforms(a("abc"), len13, true, "len 3");
  assertConforms(a(""), len13, false, "len 0 below lo");
  assertConforms(a("abcd"), len13, false, "len 4 above hi");

  const inXY = d("atom-in", encodeSet([a("x"), a("y")]));
  assertConforms(a("x"), inXY, true, "member x");
  assertConforms(a("y"), inXY, true, "member y");
  assertConforms(a("z"), inXY, false, "non-member");
});

// ----- text refinements (horos-internal#68): char-counted bounds + grammar classes -----

test("text-len counts code points, not bytes or UTF-16 units", () => {
  const len13 = d("text-len", encodeInt(1n), encodeInt(3n));
  assertConforms(encodeText("a"), len13, true, "1 char");
  assertConforms(encodeText("abc"), len13, true, "3 chars");
  assertConforms(encodeText(""), len13, false, "0 chars below lo");
  assertConforms(encodeText("abcd"), len13, false, "4 chars above hi");
  // "ééé": 3 code points but 6 bytes — char-count is in range (a byte count rejects).
  assertConforms(encodeText("ééé"), len13, true, "3 multi-byte chars (6 bytes)");
  assertConforms(encodeText("éééé"), len13, false, "4 multi-byte chars over hi");
  // An astral emoji is ONE code point (4 bytes; two UTF-16 units — ".length" === 2).
  assertConforms(encodeText("😀"), len13, true, "1 astral char (2 UTF-16 units)");
  assertConforms(encodeInt(1n), len13, false, "not utf8-text");

  assertWF(d("text-len", encodeInt(0n), encodeInt(0n)), true, "0<=0<=0");
  assertWF(d("text-len", encodeInt(3n), encodeInt(1n)), false, "lo>hi");
  assertWF(d("text-len", encodeInt(-1n), encodeInt(3n)), false, "lo<0");
});

test("key-grammar matches ^[a-z0-9][a-z0-9._-]{0,63}$", () => {
  const kg = d("key-grammar");
  assertConforms(encodeText("a"), kg, true, "single lowercase");
  assertConforms(encodeText("0"), kg, true, "single digit");
  assertConforms(encodeText("a.b_c-1"), kg, true, "interior . _ - and alnum");
  assertConforms(encodeText(""), kg, false, "empty");
  assertConforms(encodeText("A"), kg, false, "uppercase");
  assertConforms(encodeText(".ab"), kg, false, "bad leading char");
  assertConforms(encodeText("a b"), kg, false, "space");
  assertConforms(encodeText("café"), kg, false, "multi-byte char not in class");
  assertConforms(encodeText(`a${"b".repeat(63)}`), kg, true, "64 chars (boundary)");
  assertConforms(encodeText(`a${"b".repeat(64)}`), kg, false, "65 chars over");
  assertConforms(encodeInt(1n), kg, false, "not utf8-text");
});

test("text-non-blank requires a non-whitespace char", () => {
  const nb = d("text-non-blank");
  assertConforms(encodeText("a"), nb, true, "non-ws char");
  assertConforms(encodeText("  x  "), nb, true, "ws around a non-ws char");
  assertConforms(encodeText(""), nb, false, "empty");
  assertConforms(encodeText("   "), nb, false, "all spaces");
  assertConforms(encodeText("\t\n\r"), nb, false, "tab/newline/CR");
  assertConforms(encodeText("é"), nb, true, "non-ASCII char is non-blank");
  assertConforms(encodeText(" "), nb, true, "U+00A0 not in ASCII ws set");
  assertConforms(encodeInt(1n), nb, false, "not utf8-text");
});

test("lowercase-uuid matches the canonical lowercase UUID grammar", () => {
  const u = d("lowercase-uuid");
  assertConforms(encodeText("00000000-0000-0000-0000-000000000000"), u, true, "nil UUID");
  assertConforms(
    encodeText("9b2e4c1a-7d3f-4e8b-a1c2-d3e4f5061728"),
    u,
    true,
    "canonical",
  );
  assertConforms(
    encodeText("9B2E4C1A-7D3F-4E8B-A1C2-D3E4F5061728"),
    u,
    false,
    "uppercase",
  );
  assertConforms(encodeText("9b2e4c1a7d3f4e8ba1c2d3e4f5061728"), u, false, "no hyphens");
  assertConforms(
    encodeText("9b2e4c1g-7d3f-4e8b-a1c2-d3e4f5061728"),
    u,
    false,
    "'g' not hex",
  );
  assertConforms(encodeText(""), u, false, "empty");
  assertConforms(encodeInt(1n), u, false, "not utf8-text");
});

// ----- scalars (delegate to ontos/data) -----

test("scalar delegates to ontos/data", () => {
  assertConforms(encodeInt(5n), scalar("int"), true, "positive int");
  assertConforms(encodeInt(-1n), scalar("int"), true, "negative int");
  assertConforms(encodeText("5"), scalar("int"), false, "text, not int");

  assertConforms(encodeText("hi"), scalar("utf8-text"), true, "text");
  assertConforms(encodeInt(1n), scalar("utf8-text"), false, "int, not text");

  assertConforms(encodeBool(true), scalar("bool"), true, "bool");
  assertConforms(encodeInt(1n), scalar("bool"), false, "int, not bool");

  // A non-canonical int (leading-zero magnitude) is NOT a well-formed int.
  const noncanon = tup(a("int"), aBytes([0x00, 0x00, 0x01]));
  assertConforms(noncanon, scalar("int"), false, "non-canonical int");
});

// ----- tuples & compounds -----

test("tuple / seq / compound", () => {
  const pair = d("tuple", scalar("int"), d("atom"));
  assertConforms(tup(encodeInt(1n), a("x")), pair, true, "int then atom");
  assertConforms(tup(encodeInt(1n)), pair, false, "wrong arity");
  assertConforms(tup(a("x"), a("x")), pair, false, "child 0 not an int");
  assertConforms(a("x"), pair, false, "not a tuple");

  const seqInt = d("seq", scalar("int"));
  assertConforms(tup(encodeInt(1n), encodeInt(2n)), seqInt, true, "all ints");
  assertConforms(tup(), seqInt, true, "empty vacuous");
  assertConforms(tup(encodeInt(1n), a("x")), seqInt, false, "a non-int child");

  const point = d("compound", a("point"), scalar("int"), scalar("int"));
  assertConforms(
    tup(a("point"), encodeInt(1n), encodeInt(2n)),
    point,
    true,
    "point(1,2)",
  );
  assertConforms(
    tup(a("line"), encodeInt(1n), encodeInt(2n)),
    point,
    false,
    "wrong label",
  );
  assertConforms(tup(a("point"), encodeInt(1n)), point, false, "wrong arity");
});

// ----- structural collections (delegate well-formedness to ontos/data) -----

test("collections", () => {
  const listInt = d("list-of", scalar("int"));
  assertConforms(encodeList([encodeInt(1n), encodeInt(2n)]), listInt, true, "ints");
  assertConforms(encodeList([]), listInt, true, "empty list");
  assertConforms(
    encodeList([encodeInt(1n), encodeText("x")]),
    listInt,
    false,
    "a non-int element",
  );
  assertConforms(encodeInt(1n), listInt, false, "not a list");

  const setAtom = d("set-of", d("atom"));
  assertConforms(encodeSet([a("x"), a("y")]), setAtom, true, "atoms");
  assertConforms(encodeList([a("x")]), setAtom, false, "a list is not a set");

  const mapTI = d("map-of", scalar("utf8-text"), scalar("int"));
  assertConforms(
    mapOf([
      ["a", encodeInt(1n)],
      ["b", encodeInt(2n)],
    ]),
    mapTI,
    true,
    "text->int",
  );
  assertConforms(mapOf([["a", encodeText("x")]]), mapTI, false, "value not int");
});

// ----- records -----

test("records", () => {
  const recClosed = d(
    "record",
    a("closed"),
    field("name", true, scalar("utf8-text")),
    field("age", false, scalar("int")),
  );
  const recOpen = d("record", a("open"), field("name", true, scalar("utf8-text")));

  assertConforms(mapOf([["name", encodeText("ada")]]), recClosed, true, "required only");
  assertConforms(
    mapOf([
      ["age", encodeInt(36n)],
      ["name", encodeText("ada")],
    ]),
    recClosed,
    true,
    "required + optional",
  );
  assertConforms(
    mapOf([["age", encodeInt(36n)]]),
    recClosed,
    false,
    "required name missing",
  );
  assertConforms(mapOf([["name", encodeInt(1n)]]), recClosed, false, "name not text");

  const withExtra = mapOf([
    ["name", encodeText("ada")],
    ["x", encodeInt(1n)],
  ]);
  assertConforms(withExtra, recClosed, false, "extra key under closed");
  assertConforms(withExtra, recOpen, true, "extra key under open");

  assertConforms(encodeList([]), recClosed, false, "not a map");
});

// ----- composition -----

test("union / and / not / exact", () => {
  const u = d("union", scalar("int"), scalar("bool"));
  assertConforms(encodeInt(1n), u, true, "int alternative");
  assertConforms(encodeBool(true), u, true, "bool alternative");
  assertConforms(encodeText("x"), u, false, "neither");

  const both = d("and", d("atom"), d("atom-len", encodeInt(1n), encodeInt(3n)));
  assertConforms(a("ab"), both, true, "atom of len 2");
  assertConforms(a(""), both, false, "fails length bound");
  assertConforms(tup(), both, false, "not an atom");

  const notInt = d("not", scalar("int"));
  assertConforms(encodeInt(1n), notInt, false, "is an int");
  assertConforms(encodeText("x"), notInt, true, "not an int");
  assertConforms(a("x"), notInt, true, "bare atom not an int");

  const exact7 = d("exact", encodeInt(7n));
  assertConforms(encodeInt(7n), exact7, true, "equal");
  assertConforms(encodeInt(8n), exact7, false, "different value");
});

// ----- a non-descriptor is never a conformance target -----

test("non-descriptor target is false", () => {
  assertConforms(encodeInt(1n), a("x"), false, "bare atom");
  assertConforms(encodeInt(1n), tup(a("int")), false, "unreserved label");
  assertConforms(encodeInt(1n), d("nope"), false, "unknown ctor");
});

// ----- wf -----

test("wf accepts the DSL", () => {
  const good: Value[] = [
    d("any"),
    d("atom"),
    d("atom-exact", a("hi")),
    d("atom-len", encodeInt(0n), encodeInt(64n)),
    d("atom-in", encodeSet([a("x")])),
    scalar("int"),
    scalar("utf8-text"),
    scalar("bool"),
    d("tuple", scalar("int"), d("atom")),
    d("seq", scalar("int")),
    d("compound", a("point"), scalar("int"), scalar("int")),
    d("list-of", scalar("int")),
    d("set-of", d("atom")),
    d("map-of", scalar("utf8-text"), scalar("int")),
    d("record", a("closed"), field("name", true, scalar("utf8-text"))),
    d("union", scalar("int"), scalar("bool")),
    d("and", d("atom")),
    d("not", scalar("int")),
    d("exact", encodeInt(7n)),
  ];
  for (const ty of good) {
    assertWF(ty, true, ty.toString());
  }
});

test("wf rejects malformed", () => {
  const bad: Value[] = [
    a("x"), // bare atom
    tup(a("int")), // unreserved label
    d("scalar", a("float")), // unknown scalar kind
    d("atom-len", encodeInt(3n), encodeInt(1n)), // lo > hi
    d("atom-len", encodeInt(-1n), encodeInt(2n)), // lo < 0
    d("atom-in", encodeSet([encodeInt(1n)])), // set element not an atom
    d("record", a("maybe"), field("n", true, scalar("int"))), // bad mode flag
    d("record", a("closed"), field("n", true, d("scalar", a("nope")))), // bad field type
    d("tuple", d("nope")), // sub-descriptor not wf
    d("rec", a("x"), d("ref", a("x"))), // rec(x, ref x): unguarded
    d("ref", a("x")), // unbound ref
    field("n", true, scalar("int")), // a field is not a top-level descriptor
  ];
  for (const ty of bad) {
    assertWF(ty, false, ty.toString());
  }
});

test("wf rejects duplicate record fields", () => {
  const dup = d(
    "record",
    a("open"),
    field("name", true, scalar("utf8-text")),
    field("name", false, scalar("int")),
  );
  assertWF(dup, false, "duplicate field name");
});

// ----- rec / ref (guarded recursive descriptors) -----

/** consList: rec l. (atom-exact "nil") | (int, ref l). */
function consList(): Value {
  return d(
    "rec",
    a("l"),
    d("union", d("atom-exact", a("nil")), d("tuple", scalar("int"), d("ref", a("l")))),
  );
}

/** intTree: rec t. int | node(ref t, ref t). */
function intTree(): Value {
  return d(
    "rec",
    a("t"),
    d(
      "union",
      scalar("int"),
      d("compound", a("node"), d("ref", a("t")), d("ref", a("t"))),
    ),
  );
}

test("recursive conformance", () => {
  const l = consList();
  assertWF(l, true, "cons-list is wf");
  assertConforms(a("nil"), l, true, "[]");
  assertConforms(tup(encodeInt(1n), a("nil")), l, true, "[1]");
  assertConforms(tup(encodeInt(1n), tup(encodeInt(2n), a("nil"))), l, true, "[1, 2]");
  assertConforms(tup(encodeText("x"), a("nil")), l, false, "head not an int");
  assertConforms(a("other"), l, false, "not nil, not a cons cell");
  assertConforms(tup(encodeInt(1n)), l, false, "missing the tail");

  const tr = intTree();
  assertWF(tr, true, "int-tree is wf");
  assertConforms(encodeInt(5n), tr, true, "a leaf");
  assertConforms(tup(a("node"), encodeInt(1n), encodeInt(2n)), tr, true, "node(1,2)");
  assertConforms(
    tup(a("node"), encodeInt(1n), tup(a("node"), encodeInt(2n), encodeInt(3n))),
    tr,
    true,
    "node(1, node(2,3))",
  );
  assertConforms(
    tup(a("node"), encodeText("x"), encodeInt(2n)),
    tr,
    false,
    "left subtree neither int nor node",
  );
});

test("wf recursion guard", () => {
  assertWF(consList(), true, "guarded cons-list");
  assertWF(intTree(), true, "guarded int-tree");

  const bad: Value[] = [
    d("rec", a("x"), d("ref", a("x"))), // rec x. ref x — no descent
    d("rec", a("x"), d("union", d("ref", a("x")), scalar("int"))), // union same-v: unguarded
    d("rec", a("x"), d("and", d("atom"), d("ref", a("x")))), // and same-v: unguarded
    d("ref", a("y")), // unbound
  ];
  for (const ty of bad) {
    assertWF(ty, false, ty.toString());
  }
});

// ----- recursion-depth guard (horos-internal#37) -----

/**
 * A right-nested cons cell (0, (0, … nil)) `levels` deep, built ITERATIVELY (so building
 * it never recurses) — the shape that would overflow the native stack (RangeError) under
 * an unguarded recognizer.
 */
function deepConsValue(levels: number): Value {
  let v = a("nil");
  for (let i = 0; i < levels; i += 1) {
    v = tup(encodeInt(0n), v);
  }
  return v;
}

/** A left-nested seq descriptor seq(seq(… seq(int))) `levels` deep, built iteratively. */
function deepSeqDescriptor(levels: number): Value {
  let t = scalar("int");
  for (let i = 0; i < levels; i += 1) {
    t = d("seq", t);
  }
  return t;
}

// One value-level costs several conformsIn frames, so a value this many levels deep is
// guaranteed to drive the recognizer past MAX_RECURSION_DEPTH and trip the guard, while
// staying shallow enough that building it never blows the stack.
const pastCapLevels = MAX_RECURSION_DEPTH + 64;

test("recursion-depth guard: conforms returns false, never a RangeError", () => {
  const cons = consList();

  // A shallow, in-bound cons-list still conforms — the guard never trips below the cap,
  // so no existing behavior changes.
  assertConforms(deepConsValue(8), cons, true, "shallow list conforms");

  // A value past MAX_RECURSION_DEPTH is classified NON-conforming deterministically — the
  // guard returns false before the native stack can overflow.
  assertConforms(
    deepConsValue(pastCapLevels),
    cons,
    false,
    "a value past the depth cap is non-conforming, not a crash",
  );
});

test("recursion-depth guard: wf returns false, never a RangeError", () => {
  // A shallow seq descriptor is well-formed (guard never trips below the cap).
  assertWF(deepSeqDescriptor(8), true, "shallow seq descriptor is wf");

  // A descriptor nested past the cap is treated as NON-wf deterministically, never
  // overflowing the stack.
  assertWF(
    deepSeqDescriptor(pastCapLevels),
    false,
    "a descriptor past the depth cap is non-wf, not a crash",
  );
});
