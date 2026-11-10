// Diagnostic checking outcomes (horos-internal#211 / ADR 0008). Mirrors core/rs/src/diagnose.rs tests and
// core/go/diagnose_test.go.

import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { decode } from "@bitspark/ontos-codec";
import { atom, tuple, type Value } from "@bitspark/ontos-core";
import { encodeInt } from "@bitspark/ontos-data";

import { conforms, MAX_RECURSION_DEPTH, wf, ATOM_LEN_BOUND_MAX } from "../src/index.js";
import {
  checkingProfile,
  completedBool,
  diagnose,
  diagnoseWf,
  isComplete,
  limitValue,
  type Diagnosis,
} from "../src/diagnose.js";

const enc = new TextEncoder();
const a = (s: string): Value => atom(enc.encode(s));
const d = (ctor: string, ...args: Value[]): Value => tuple([a(`horos/${ctor}`), ...args]);
const scalarInt = (): Value => d("scalar", a("int"));

// The oracle's own cons-list descriptor: wf, guarded, and a value nested past the cap trips the
// descent exactly as depth-171-over-cap-false does.
const consList = (): Value =>
  d(
    "rec",
    a("L"),
    d("union", d("exact", a("[]")), d("tuple", scalarInt(), d("ref", a("L")))),
  );

function deepCons(n: number): Value {
  let v: Value = a("[]");
  const one = encodeInt(1n);
  for (let i = 0; i < n; i += 1) v = tuple([one, v]);
  return v;
}

function hexToBytes(h: string): Uint8Array {
  const out = new Uint8Array(h.length / 2);
  for (let i = 0; i < out.length; i += 1)
    out[i] = parseInt(h.slice(i * 2, i * 2 + 2), 16);
  return out;
}

function verdict(x: Diagnosis): string {
  return x.verdict;
}

// ADR 0008 D4, SWEPT rather than asserted: on every oracle judgment where diagnose completes,
// it agrees with the frozen boolean. Where it reports exhausted the boolean is unconstrained.
// This is also the anti-drift gate between the two recursions.
test("the diagnosis agrees with the boolean on every completed outcome (horos-internal#211)", () => {
  const doc = JSON.parse(
    readFileSync(resolve(process.cwd(), "../../vectors/conformance.json"), "utf8"),
  ) as {
    cases: {
      name: string;
      descriptor: string;
      values: { value: string; conforms: boolean }[];
    }[];
  };
  let completed = 0;
  let exhausted = 0;
  for (const c of doc.cases) {
    const t = decode(hexToBytes(c.descriptor));
    const w = completedBool(diagnoseWf(t));
    if (w === undefined) exhausted += 1;
    else {
      assert.equal(w, wf(t), `wf disagrees for ${c.name}`);
      completed += 1;
    }
    c.values.forEach((v, i) => {
      const val = decode(hexToBytes(v.value));
      const b = completedBool(diagnose(val, t));
      if (b === undefined) exhausted += 1;
      else {
        assert.equal(
          b,
          conforms(val, t),
          `conforms disagrees for ${c.name} / value[${i}]`,
        );
        completed += 1;
      }
    });
  }
  assert.ok(completed >= 100, `expected >=100 completed, got ${completed}`);
  assert.equal(exhausted, 1, "expected exactly the one in-corpus exhaustion (depth-171)");
});

test("the depth boundary separates exhaustion from mismatch (horos-internal#211)", () => {
  const t = consList();
  assert.equal(verdict(diagnose(deepCons(170), t)), "conforms");
  const over = diagnose(deepCons(171), t);
  assert.equal(verdict(over), "exhausted");
  assert.equal(over.verdict === "exhausted" ? over.limit : "", "recursion_depth");
  // the frozen boolean is UNCHANGED
  assert.equal(conforms(deepCons(170), t), true);
  assert.equal(conforms(deepCons(171), t), false);
  // a plain mismatch at the same descriptor is COMPLETED, not exhausted
  assert.equal(verdict(diagnose(a("nope"), t)), "mismatch");
});

// ADR 0008 D2 — THE rule. not of an exhausted subdecision stays exhausted, where the frozen
// boolean reports an unqualified true.
test("complement preserves exhaustion where the boolean says true (horos-internal#211)", () => {
  const inner = consList();
  const t = d("not", inner);
  const v = deepCons(171);

  // the boolean: !false = TRUE — a completed positive from a computation that never finished.
  assert.equal(
    conforms(v, t),
    true,
    "precondition: the frozen boolean reports true here",
  );
  assert.equal(
    verdict(diagnose(v, t)),
    "exhausted",
    "not(Exhausted) must stay Exhausted",
  );
  // a NESTED complement does not launder it either
  assert.equal(verdict(diagnose(v, d("not", d("not", inner)))), "exhausted");
  // sanity: over a SHALLOW value the same complement completes normally
  assert.equal(verdict(diagnose(deepCons(2), t)), "mismatch");
  assert.equal(verdict(diagnose(a("nope"), t)), "conforms");
});

// ADR 0008 D3 — a decisive alternative beats an exhausted sibling, in BOTH orders.
test("union and and are Kleene in both orders (horos-internal#211)", () => {
  const deep = deepCons(171);
  const exhausts = consList();
  const accepts = d("any");
  const rejects = scalarInt(); // a tuple is decisively not a scalar int

  for (const args of [
    [exhausts, accepts],
    [accepts, exhausts],
  ]) {
    assert.equal(
      verdict(diagnose(deep, d("union", ...args))),
      "conforms",
      "a decisive union branch must beat an exhausted sibling",
    );
  }
  assert.equal(
    verdict(diagnose(deep, d("union", exhausts, rejects))),
    "exhausted",
    "no decisive branch: unknown, never a completed mismatch",
  );

  for (const args of [
    [exhausts, rejects],
    [rejects, exhausts],
  ]) {
    assert.equal(
      verdict(diagnose(deep, d("and", ...args))),
      "mismatch",
      "a decisive and-branch mismatch must beat an exhausted sibling",
    );
  }
  assert.equal(
    verdict(diagnose(deep, d("and", exhausts, accepts))),
    "exhausted",
    "no decisive mismatch: unknown, never a completed conformance",
  );
});

test("a malformed descriptor is not a mismatch (horos-internal#211)", () => {
  const bad = d("scalar", a("float"));
  assert.equal(verdict(diagnoseWf(bad)), "not_well_formed");
  assert.equal(verdict(diagnose(encodeInt(1n), bad)), "not_well_formed");
  assert.equal(verdict(diagnose(a("x"), scalarInt())), "mismatch");
  // both are false to the frozen boolean — the distinction is the whole point
  assert.equal(conforms(encodeInt(1n), bad), false);
  assert.equal(conforms(a("x"), scalarInt()), false);
});

test("the profile and the limit are stated (horos-internal#211)", () => {
  const p = checkingProfile();
  assert.equal(p.maxRecursionDepth, MAX_RECURSION_DEPTH);
  assert.equal(p.atomLenBoundMax, ATOM_LEN_BOUND_MAX);
  assert.equal(limitValue("recursion_depth"), MAX_RECURSION_DEPTH);
  const location = { path: [], at: [], reason: "limit" } as const;
  assert.ok(isComplete({ verdict: "conforms" }));
  assert.ok(
    isComplete({ verdict: "mismatch", location: { ...location, reason: "refused" } }),
  );
  assert.ok(
    isComplete({
      verdict: "not_well_formed",
      location: { ...location, reason: "arity" },
    }),
  );
  assert.ok(!isComplete({ verdict: "exhausted", limit: "recursion_depth", location }));
  assert.equal(
    completedBool({ verdict: "exhausted", limit: "recursion_depth", location }),
    undefined,
    "an exhaustion must NOT collapse into a boolean",
  );
});
