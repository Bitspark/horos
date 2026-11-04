// Run only from an independently installed public consumer, never via workspace links.
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { Atom, Tuple, atom, tuple } from "@bitspark/ontos-core";
import { encode, decode } from "@bitspark/ontos-codec";
import { encodeText } from "@bitspark/ontos-data";
import { NativeFactory, NativeTuple } from "@bitspark/ontos-wire/native";
import { encodeMessage, decodeMessage } from "@bitspark/bitwire";
import { wf, conforms } from "@bitspark/horos";
import { completedBool, diagnose } from "@bitspark/horos/diagnose";

const rootRequire = createRequire(import.meta.url);
const core = rootRequire.resolve("@bitspark/ontos-core");
for (const name of ["@bitspark/horos", "@bitspark/ontos-codec", "@bitspark/ontos-data",
  "@bitspark/ontos-wire/native", "@bitspark/bitwire"]) {
  assert.equal(createRequire(import.meta.resolve(name)).resolve("@bitspark/ontos-core"), core, name);
}
const lock = JSON.parse(readFileSync("package-lock.json", "utf8"));
const cores = Object.entries(lock.packages).filter(([path]) => path.endsWith("node_modules/@bitspark/ontos-core"));
assert.equal(cores.length, 1, "one installed Ontos constructor family");
assert.equal(cores[0][1].version, "0.16.0");
for (const [path, pkg] of Object.entries(lock.packages)) {
  if (/node_modules\/@bitspark\/ontos-(core|codec|data|wire)$/.test(path)) {
    assert.equal(pkg.version, "0.16.0", path);
  }
}

const tag = (s) => atom(new TextEncoder().encode(s));
const textType = tuple([tag("horos/scalar"), tag("utf8-text")]);
const anyType = tuple([tag("horos/any")]);
assert.equal(wf(textType), true);
assert.equal(wf(anyType), true);
const text = encodeText("shared Ontos value");
assert.ok(text instanceof Tuple);
assert.ok(tag("x") instanceof Atom);
for (const value of [text, decode(encode(text)), decodeMessage(encodeMessage(text))]) {
  assert.ok(value instanceof Tuple);
  assert.equal(conforms(value, textType), true);
  assert.equal(completedBool(diagnose(value, textType)), true);
}
assert.equal(conforms(tuple([]), textType), false);

// Structural qualification only: this inert leaf is not a live runtime capability.
const opaque = Object.freeze({ label: "consumer-owned opaque leaf" });
const factory = new NativeFactory((value) => {
  assert.equal(value, opaque);
  return opaque;
});
const ground = factory.tuple([text]);
assert.ok(ground instanceof Tuple, "ground collapse uses the same Ontos tuple");
assert.equal(conforms(ground, anyType), true);
const mixed = factory.tuple([text, opaque]);
assert.ok(mixed instanceof NativeTuple);
assert.equal(mixed.at(0), text);
assert.equal(mixed.at(1), opaque);
assert.equal(mixed instanceof Tuple, false, "mixed values are outside the ground checker input domain");
console.log("Ontos 0.16.0 / Bitwire 0.7.0 installed identity, conformance and ground boundary passed");
