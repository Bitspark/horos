// Run only from an installed consumer (conformance/ground.py): it checks the
// Go-materialised ground descriptors with the installed @bitspark/horos package.
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { decode } from "@bitspark/ontos-codec";
import { wf, conforms } from "@bitspark/horos";
import { completedBool, diagnose, diagnoseWf } from "@bitspark/horos/diagnose";

const [path, lane] = process.argv.slice(2);

// One Ontos constructor family, at the exact version the installed package requires.
const horos = JSON.parse(readFileSync("node_modules/@bitspark/horos/package.json", "utf8"));
const ontos = horos.dependencies["@bitspark/ontos-core"];
const lock = JSON.parse(readFileSync("package-lock.json", "utf8"));
const installed = Object.entries(lock.packages).filter(([p]) => /node_modules\/@bitspark\/ontos-[a-z-]+$/.test(p));
assert.equal(installed.filter(([p]) => p.endsWith("/ontos-core")).length, 1, "one installed Ontos constructor family");
for (const [p, pkg] of installed) assert.equal(pkg.version, ontos, p);
for (const [name, version] of Object.entries(horos.dependencies)) {
  if (name.startsWith("@bitspark/ontos-")) assert.equal(version, ontos, name);
}
const doc = JSON.parse(readFileSync(path, "utf8"));
const hex = (s) => decode(Uint8Array.from(Buffer.from(s, "hex")));
let judgments = 0;
const verdicts = new Set();
for (const c of doc.cases) {
  const t = hex(c.descriptor);
  assert.equal(wf(t), true, c.declaration);
  assert.equal(diagnoseWf(t).verdict, "conforms", c.declaration);
  for (const item of c.values) {
    const label = `${c.declaration} / ${item.name}`;
    const v = hex(item.value);
    const d = diagnose(v, t);
    assert.equal(d.verdict, item.verdict, label);
    assert.equal(d.limit, item.limit, label);
    assert.equal(completedBool(d), item.verdict === "exhausted" ? undefined : item.conforms, label);
    assert.equal(conforms(v, t), item.conforms, label);
    verdicts.add(d.verdict);
    judgments += 1;
  }
}
assert.deepEqual([...verdicts].sort(), ["conforms", "exhausted", "mismatch"]);
console.log(`${lane}: @bitspark/horos ${horos.version} (Ontos ${ontos}) agrees with Go on ${judgments} judgments over ${doc.cases.length} Go-materialised ground descriptors`);
