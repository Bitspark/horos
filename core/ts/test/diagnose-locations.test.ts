// Diagnostic locations (ADR 0009, docs/spec/horos.md "Diagnostic locations"). Replays the
// hand-authored vectors/diagnose.json corpus, which the Go and Rust cores replay too, and pins
// a few locations authored here from the specification.

import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { decode } from "@bitspark/ontos-codec";
import { atom, tuple, type Value } from "@bitspark/ontos-core";
import { encodeInt, encodeList, encodeMap, encodeText } from "@bitspark/ontos-data";

import { diagnose, diagnoseWf, type Diagnosis, type Step } from "../src/diagnose.js";

interface CorpusValue {
  readonly note?: string;
  readonly value: string;
  readonly diagnosis: unknown;
}
interface CorpusCase {
  readonly name: string;
  readonly descriptor: string;
  readonly wf: unknown;
  readonly diagnose_wf?: unknown;
  readonly values: readonly CorpusValue[];
}

function hexToBytes(h: string): Uint8Array {
  const out = new Uint8Array(h.length / 2);
  for (let i = 0; i < out.length; i += 1)
    out[i] = parseInt(h.slice(i * 2, i * 2 + 2), 16);
  return out;
}

/** The JSON form of the contract: what the corpus pins and the command-line tool prints. */
const json = (d: Diagnosis): unknown => JSON.parse(JSON.stringify(d));

/** Replays a located corpus; `wfOf` picks the expected `diagnoseWf` result of a case. */
function replayLocated(file: string, wfOf: (c: CorpusCase) => unknown): void {
  const path = resolve(process.cwd(), "../../vectors", file);
  const doc = JSON.parse(readFileSync(path, "utf8")) as { cases: readonly CorpusCase[] };
  assert.ok(doc.cases.length > 0, `${file} has no cases`);
  let checked = 0;
  for (const c of doc.cases) {
    const t = decode(hexToBytes(c.descriptor));
    assert.deepEqual(json(diagnoseWf(t)), wfOf(c), `diagnoseWf for ${file} ${c.name}`);
    checked += 1;
    c.values.forEach((v, i) => {
      const got = json(diagnose(decode(hexToBytes(v.value)), t));
      assert.deepEqual(
        got,
        v.diagnosis,
        `diagnose for ${file} ${c.name} / value[${i}] (${v.note})`,
      );
      checked += 1;
    });
  }
  assert.ok(checked > doc.cases.length, `${file} has no values`);
}

test("every diagnosis in the location corpus is reproduced exactly (ADR 0009)", () => {
  replayLocated("diagnose.json", (c) => c.wf);
});

// The ref-scoping oracle (ADR 0011) pins booleans as `wf`/`conforms`, which the vectors
// replay checks, and diagnoses as `diagnose_wf`/`diagnosis`.
test("every diagnosis in the checking-work oracle is reproduced exactly (ADR 0013)", () => {
  replayLocated("checking-work.json", (c) => c.diagnose_wf);
});

test("every diagnosis in the ref-scoping oracle is reproduced exactly (ADR 0011)", () => {
  replayLocated("recursion-scope.json", (c) => c.diagnose_wf);
});

const enc = new TextEncoder();
const a = (s: string): Value => atom(enc.encode(s));
const d = (ctor: string, ...args: Value[]): Value => tuple([a(`horos/${ctor}`), ...args]);
const scalar = (kind: string): Value => d("scalar", a(kind));
const field = (name: string, presence: "req" | "opt", ty: Value): Value =>
  d("field", encodeText(name), a(presence), ty);
const text = encodeText;
const int = (n: number): Value => encodeInt(BigInt(n));
const map = (...entries: [Value, Value][]): Value => encodeMap(entries);
const item = (index: number): Step => ({ kind: "item", index });

test("a record names its missing and extra fields, before its children", () => {
  const t = d(
    "record",
    a("closed"),
    field("name", "req", scalar("utf8-text")),
    field("age", "opt", scalar("int")),
  );
  assert.deepEqual(diagnose(map(), t), {
    verdict: "mismatch",
    location: { path: [], at: [], reason: "missing-field", field: "name" },
  });
  // missing comes before extra
  assert.deepEqual(diagnose(map([text("zzz"), int(1)]), t), {
    verdict: "mismatch",
    location: { path: [], at: [], reason: "missing-field", field: "name" },
  });
  assert.deepEqual(diagnose(map([text("name"), text("x")], [text("zzz"), int(1)]), t), {
    verdict: "mismatch",
    location: { path: [], at: [], reason: "extra-field", field: "zzz" },
  });
  // the extra field comes before a bad present field
  assert.deepEqual(diagnose(map([text("name"), int(1)], [text("zzz"), int(1)]), t), {
    verdict: "mismatch",
    location: { path: [], at: [], reason: "extra-field", field: "zzz" },
  });
  assert.deepEqual(
    diagnose(map([text("age"), text("x")], [text("name"), text("y")]), t),
    {
      verdict: "mismatch",
      location: { path: [{ kind: "field", name: "age" }], at: [3, 3], reason: "refused" },
    },
  );
  // a non-text key is refused by the record, at that key
  const open = d("record", a("open"));
  assert.deepEqual(diagnose(map([int(1), int(2)]), open), {
    verdict: "mismatch",
    location: { path: [{ kind: "key", index: 0 }], at: [], reason: "refused" },
  });
  assert.deepEqual(diagnose(encodeList([]), open), {
    verdict: "mismatch",
    location: { path: [], at: [], reason: "refused" },
  });
});

test("a map distinguishes its keys from its values", () => {
  const t = d("map-of", scalar("utf8-text"), scalar("int"));
  assert.deepEqual(diagnose(map([text("a"), int(1)], [text("b"), text("x")]), t), {
    verdict: "mismatch",
    location: { path: [{ kind: "value", index: 1 }], at: [2], reason: "refused" },
  });
  assert.deepEqual(diagnose(map([int(1), int(2)]), t), {
    verdict: "mismatch",
    location: { path: [{ kind: "key", index: 0 }], at: [1], reason: "refused" },
  });
  const list = d("list-of", d("not", scalar("int")));
  assert.deepEqual(diagnose(encodeList([text("a"), int(1)]), list), {
    verdict: "mismatch",
    location: { path: [{ kind: "elem", index: 1 }], at: [1], reason: "complement" },
  });
});

// The oracle's cons-list: every level is a union, and a ref continues at its rec body.
const consList = (): Value =>
  d(
    "rec",
    a("L"),
    d("union", d("exact", a("[]")), d("tuple", scalar("int"), d("ref", a("L")))),
  );

test("a union reports its deepest alternative, through rec and ref", () => {
  const t = consList();
  assert.deepEqual(diagnose(tuple([int(1), tuple([a("x"), a("[]")])]), t), {
    verdict: "mismatch",
    location: { path: [item(1), item(0)], at: [2, 2, 1], reason: "refused" },
  });
  assert.deepEqual(diagnose(tuple([int(1), a("zzz")]), t), {
    verdict: "mismatch",
    location: { path: [item(1)], at: [2], reason: "no-alternative" },
  });
  assert.deepEqual(diagnose(a("nope"), t), {
    verdict: "mismatch",
    location: { path: [], at: [2], reason: "no-alternative" },
  });
  assert.deepEqual(diagnose(a("x"), d("union")), {
    verdict: "mismatch",
    location: { path: [], at: [], reason: "no-alternative" },
  });
  // equally deep alternatives: the first one in order
  const tie = d("union", d("tuple", scalar("int")), d("tuple", scalar("bool")));
  assert.deepEqual(diagnose(tuple([text("x")]), tie), {
    verdict: "mismatch",
    location: { path: [item(0)], at: [1, 1], reason: "refused" },
  });
});

test("not reports its complement and and reports its first failing conjunct", () => {
  assert.deepEqual(diagnose(int(1), d("not", scalar("int"))), {
    verdict: "mismatch",
    location: { path: [], at: [], reason: "complement" },
  });
  const both = d("and", d("any"), d("atom-len", int(1), int(2)), d("atom-exact", a("q")));
  assert.deepEqual(diagnose(a("long"), both), {
    verdict: "mismatch",
    location: { path: [], at: [2], reason: "refused" },
  });
});

test("an exhaustion is located where the depth bound was reached", () => {
  // Each cons level costs three descents (union, tuple, child), so the children of the
  // 171st tuple are entered at depth 513. Both exceed the bound; the first in item order,
  // the head's scalar int, is reported.
  let v: Value = a("[]");
  for (let i = 0; i < 171; i += 1) v = tuple([int(1), v]);
  assert.deepEqual(diagnose(v, consList()), {
    verdict: "exhausted",
    limit: "recursion_depth",
    location: {
      path: [...Array.from({ length: 170 }, () => item(1)), item(0)],
      at: [2, 2, 1],
      reason: "limit",
    },
  });
});

test("diagnoseWf names the malformed constructor and argument", () => {
  const dup = d(
    "record",
    a("open"),
    field("a", "req", d("any")),
    field("a", "opt", d("any")),
  );
  assert.deepEqual(diagnoseWf(dup), {
    verdict: "not_well_formed",
    location: { path: [], at: [], reason: "duplicate-field", arg: 3 },
  });
  assert.deepEqual(diagnoseWf(d("union", d("atom"), dup)), {
    verdict: "not_well_formed",
    location: { path: [], at: [2], reason: "duplicate-field", arg: 3 },
  });
  const loHi = d("atom-len", int(5), int(1));
  assert.deepEqual(diagnoseWf(loHi), {
    verdict: "not_well_formed",
    location: { path: [], at: [], reason: "argument", arg: 2 },
  });
  assert.deepEqual(diagnose(a("x"), loHi), diagnoseWf(loHi));
  assert.deepEqual(diagnoseWf(d("text-len", int(-1), int(1))), {
    verdict: "not_well_formed",
    location: { path: [], at: [], reason: "argument", arg: 1 },
  });
  assert.deepEqual(diagnoseWf(d("atom", d("any"))), {
    verdict: "not_well_formed",
    location: { path: [], at: [], reason: "arity" },
  });
  assert.deepEqual(diagnoseWf(d("list-of", field("a", "req", d("any")))), {
    verdict: "not_well_formed",
    location: { path: [], at: [1], reason: "unknown-constructor" },
  });
  assert.deepEqual(diagnoseWf(d("tuple", d("any"), a("garbage"))), {
    verdict: "not_well_formed",
    location: { path: [], at: [2], reason: "not-descriptor" },
  });
  assert.deepEqual(diagnoseWf(d("ref", a("x"))), {
    verdict: "not_well_formed",
    location: { path: [], at: [], reason: "unbound-ref", arg: 1 },
  });
  assert.deepEqual(diagnoseWf(d("rec", a("x"), d("union", d("ref", a("x"))))), {
    verdict: "not_well_formed",
    location: { path: [], at: [2, 1], reason: "unguarded-ref", arg: 1 },
  });
  // an own-form fault of the record comes before a malformed field type
  const both = d("record", a("open"), field("a", "req", a("garbage")), a("not-a-field"));
  assert.deepEqual(diagnoseWf(both), {
    verdict: "not_well_formed",
    location: { path: [], at: [], reason: "argument", arg: 3 },
  });
  assert.deepEqual(diagnoseWf(d("record", a("open"), field("a", "req", a("garbage")))), {
    verdict: "not_well_formed",
    location: { path: [], at: [2, 3], reason: "not-descriptor" },
  });
});
