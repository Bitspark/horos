/* global process, console, TextEncoder */
// Times conforms and diagnose on ordinary input: a list of n closed records
// {name text, age opt int, tags list-of text}, with age's type an int ("plain") or a
// three-way union of leaves ("union"). It measures the checker, not seeded conformance,
// and no check runs it; scripts/bench.py runs it beside the other cores (ADR 0013).
//
//   npm run build && node --expose-gc bench/bench.mjs 200000 plain
import { atom, tuple } from "@bitspark/ontos-core";
import {
  encodeInt,
  encodeList,
  encodeMap,
  encodeNull,
  encodeText,
} from "@bitspark/ontos-data";
import { conforms } from "../dist/src/index.js";
import { diagnose } from "../dist/src/diagnose.js";

const enc = new TextEncoder();
const a = (s) => atom(enc.encode(s));
const d = (ctor, ...args) => tuple([a(`horos/${ctor}`), ...args]);
const field = (name, presence, ty) => d("field", encodeText(name), a(presence), ty);

const [nArg, mode] = process.argv.slice(2);
const n = Number(nArg);
if (!Number.isInteger(n) || n < 0 || (mode !== "plain" && mode !== "union")) {
  console.error("usage: bench.mjs <n> plain|union");
  process.exit(2);
}
const age =
  mode === "union"
    ? d(
        "union",
        d("exact", encodeNull()),
        d("scalar", a("int")),
        d("scalar", a("utf8-text")),
      )
    : d("scalar", a("int"));
const desc = d(
  "list-of",
  d(
    "record",
    a("closed"),
    field("name", "req", d("scalar", a("utf8-text"))),
    field("age", "opt", age),
    field("tags", "req", d("list-of", d("scalar", a("utf8-text")))),
  ),
);
const items = [];
for (let i = 0; i < n; i += 1) {
  items.push(
    encodeMap([
      [encodeText("age"), encodeInt(BigInt(i))],
      [encodeText("name"), encodeText(`n${i}`)],
      [
        encodeText("tags"),
        encodeList([encodeText("a"), encodeText("b"), encodeText("c")]),
      ],
    ]),
  );
}
const v = encodeList(items);

// Time, and the heap's growth over the run (V8 reports no cumulative allocation).
const measure = (run) => {
  globalThis.gc?.();
  const before = process.memoryUsage().heapUsed;
  const start = process.hrtime.bigint();
  const out = run();
  const ms = Number((process.hrtime.bigint() - start) / 1000000n);
  const mb = (process.memoryUsage().heapUsed - before) / 1e6;
  return [out, ms, mb];
};
const [ok, cms, cmb] = measure(() => conforms(v, desc));
const [dg, dms, dmb] = measure(() => diagnose(v, desc).verdict);
console.log(
  JSON.stringify({
    core: "typescript",
    n,
    mode,
    conforms: ok,
    conforms_ms: cms,
    conforms_heap_mb: Number(cmb.toFixed(1)),
    diagnose: dg,
    diagnose_ms: dms,
    diagnose_heap_mb: Number(dmb.toFixed(1)),
  }),
);
