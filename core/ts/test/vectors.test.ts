// Cross-language conformance: replay the committed vectors/conformance.json — decode
// each descriptor/value from ontos-codec-v1 hex and re-check wf/conforms. This proves
// TS == the oracle == Rust == Go (the three cores agree on one oracle, byte-for-byte).
// Mirrors core/rs/src/vectors.rs::vectors_replay and core/go/vectors_test.go.

import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { decode } from "@bitspark/ontos-codec";
import { conforms, wf, type Value } from "../src/index.js";

interface VectorValue {
  readonly value: string;
  readonly conforms: boolean;
  readonly note?: string;
}
interface VectorCase {
  readonly name: string;
  readonly descriptor: string;
  readonly wf: boolean;
  readonly values: readonly VectorValue[];
}
interface VectorDoc {
  readonly version?: string;
  readonly note?: string;
  readonly cases: readonly VectorCase[];
}

// `npm test` runs with cwd = core/ts; the repo-root vectors/ is two levels up
// (mirroring the Rust core's CARGO_MANIFEST_DIR/../../vectors and the Go core's
// ../../vectors from core/go).
// integer-bounds.json is the hand-authored oracle for int-min/int-max (ADR 0010) and
// recursion-scope.json the one for ref scoping (ADR 0011); the original oracle is pinned
// byte for byte.
const VEC_PATHS = [
  "conformance.json",
  "integer-bounds.json",
  "recursion-scope.json",
  "checking-work.json",
].map((f) => resolve(process.cwd(), "../../vectors", f));

function hexToBytes(h: string): Uint8Array {
  const out = new Uint8Array(h.length / 2);
  for (let i = 0; i < out.length; i += 1) {
    out[i] = parseInt(h.slice(i * 2, i * 2 + 2), 16);
  }
  return out;
}

function decodeHex(h: string): Value {
  return decode(hexToBytes(h));
}

test("vectors replay (TS == oracle == Rust == Go)", () => {
  for (const path of VEC_PATHS) {
    const doc = JSON.parse(readFileSync(path, "utf8")) as VectorDoc;
    assert.ok(doc.cases.length > 0, `${path} has no cases`);
    for (const c of doc.cases) {
      const desc = decodeHex(c.descriptor);
      assert.equal(wf(desc), c.wf, `wf(${c.name})`);
      for (const v of c.values) {
        const val = decodeHex(v.value);
        assert.equal(conforms(val, desc), v.conforms, `${c.name} / ${v.note ?? ""}`);
      }
    }
  }
});
