import { test } from "node:test";
import assert from "node:assert/strict";
import { atom, tuple, type Value } from "@bitspark/ontos-core";
import { encodeText } from "@bitspark/ontos-data";
import { wf, conforms, MAX_RECURSION_DEPTH } from "../src/index.js";
import { diagnose, diagnoseWf } from "../src/diagnose.js";

const a = (s: string): Value => atom(new TextEncoder().encode(s));
const d = (ctor: string, ...args: Value[]): Value => tuple([a(`horos/${ctor}`), ...args]);
const seqs = (n: number, ty: Value): Value => { for (let i = 0; i < n; i++) ty = d("seq", ty); return ty; };
const field = (ty: Value): Value => d("field", encodeText("next"), a("opt"), ty);

test("diagnostics preserve arbitrary binary labels and recursion names", () => {
  const x = atom(new Uint8Array([0x80])), y = atom(new Uint8Array([0x81]));
  const label = d("compound", x);
  assert.equal(wf(label), true);
  assert.equal(conforms(tuple([y]), label), false);
  assert.equal(diagnose(tuple([y]), label).verdict, "mismatch");
  const ty = d("rec", x, d("union", d("exact", a("end")), d("tuple", d("rec", y, d("tuple", d("ref", x))))));
  const v = tuple([tuple([a("end")])]);
  assert.equal(wf(ty), true);
  assert.equal(conforms(v, ty), true);
  assert.equal(diagnose(v, ty).verdict, "conforms");
});

test("diagnostic well-formedness carries depth and scope through records and recursion", () => {
  for (const inner of [d("rec", a("x"), seqs(2, d("any"))), d("record", a("open"), field(seqs(2, d("any"))))]) {
    const at = seqs(MAX_RECURSION_DEPTH - 3, inner);
    const over = seqs(MAX_RECURSION_DEPTH - 2, inner);
    assert.equal(wf(at), true);
    assert.equal(diagnoseWf(at).verdict, "conforms");
    assert.equal(wf(over), false);
    assert.equal(diagnoseWf(over).verdict, "exhausted");
    assert.equal(diagnose(tuple([]), over).verdict, "exhausted");
  }
  assert.equal(diagnoseWf(d("rec", a("x"), seqs(MAX_RECURSION_DEPTH + 1, d("any")))).verdict, "exhausted");
  const guarded = d("rec", a("x"), d("record", a("open"), field(d("ref", a("x")))));
  assert.equal(wf(guarded), true);
  assert.equal(diagnoseWf(guarded).verdict, "conforms");
  for (const bad of [d("ref", a("x")), d("rec", a("x"), d("ref", a("x")))]) {
    assert.equal(diagnoseWf(bad).verdict, "not_well_formed");
  }
});
