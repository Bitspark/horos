import { test } from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

test("packed public package works in an isolated consumer and excludes proof APIs", () => {
  const temp = mkdtempSync(join(tmpdir(), "horos-consumer-"));
  const root = process.cwd();
  const npmCLI = process.env.npm_execpath;
  assert.ok(npmCLI, "run this test through npm test");
  const env: NodeJS.ProcessEnv = {
    ...process.env,
    npm_config_cache: join(temp, "cache"),
    npm_config_userconfig: join(temp, "npmrc"),
    npm_config_registry: "https://registry.npmjs.org/",
  };
  for (const name of [
    "NODE_AUTH_TOKEN",
    "NPM_TOKEN",
    "GH_TOKEN",
    "GITHUB_TOKEN",
    "BITSPARK_CI_TOKEN",
  ])
    delete env[name];
  writeFileSync(join(temp, "npmrc"), "registry=https://registry.npmjs.org/\n");
  const run = (args: string[], cwd: string) =>
    execFileSync(process.execPath, [npmCLI, ...args], {
      cwd,
      env,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"],
    });
  try {
    const packs = Object.values(
      JSON.parse(run(["pack", "--json", "--pack-destination", temp], root)),
    ) as { filename: string; files: { path: string }[] }[];
    assert.equal(packs.length, 1);
    for (const file of packs[0]!.files)
      assert.doesNotMatch(file.path, /semantics|witness|\/wire\./u);
    const consumer = join(temp, "consumer");
    mkdirSync(consumer);
    writeFileSync(
      join(consumer, "package.json"),
      JSON.stringify({ private: true, type: "module" }),
    );
    run(
      [
        "install",
        "--ignore-scripts",
        "--no-audit",
        "--no-fund",
        join(temp, packs[0]!.filename),
      ],
      consumer,
    );
    writeFileSync(
      join(consumer, "check.mjs"),
      `
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import * as horos from "@bitspark/horos";
import { diagnose } from "@bitspark/horos/diagnose";
import { decode } from "@bitspark/ontos-codec";
import { encodeMap } from "@bitspark/ontos-data";
const value = h => decode(Uint8Array.from(Buffer.from(h, "hex")));
const corpus = JSON.parse(readFileSync(process.argv[2], "utf8"));
for (const c of corpus.cases) {
  const t = value(c.descriptor);
  assert.equal(horos.wf(t), c.wf);
  for (const v of c.values) assert.equal(horos.conforms(value(v.value), t), v.conforms);
}
const t = value(corpus.cases[0].descriptor);
assert.equal(typeof diagnose(t, t).verdict, "string");
const emptyRecord = horos.recordForm("closed", []);
assert.equal(horos.wf(emptyRecord), true);
assert.equal(horos.conforms(encodeMap([]), emptyRecord), true);
for (const name of ["conformsWitness", "conformsViaCheck", "decompose", "FoldMode", "pushBinding", "lookupBinding", "conformsUnderEnv"]) assert.equal(name in horos, false);
for (const path of ["semantics", "wire"]) await assert.rejects(import("@bitspark/horos/" + path), { code: "ERR_PACKAGE_PATH_NOT_EXPORTED" });
console.log("installed public package replays the oracle; private entrypoints absent");
`,
    );
    const output = execFileSync(
      process.execPath,
      [join(consumer, "check.mjs"), resolve(root, "../../vectors/conformance.json")],
      { cwd: consumer, env, encoding: "utf8" },
    );
    assert.match(output, /replays the oracle/u);
    writeFileSync(
      join(consumer, "check.ts"),
      `
import { wf, conforms, recordForm, type RecordField, type RecordMode, type Value } from "@bitspark/horos";
import { diagnose, type Diagnosis } from "@bitspark/horos/diagnose";
export function check(v: Value, t: Value): Diagnosis | boolean {
  return wf(t) && conforms(v,t) ? diagnose(v,t) : false;
}
export function construct(mode: RecordMode, fields: readonly RecordField[]): Value {
  return recordForm(mode, fields);
}
`,
    );
    execFileSync(
      process.execPath,
      [
        join(root, "node_modules/typescript/bin/tsc"),
        "--noEmit",
        "--strict",
        "--target",
        "ES2022",
        "--module",
        "NodeNext",
        "--moduleResolution",
        "NodeNext",
        "check.ts",
      ],
      { cwd: consumer, env, stdio: "pipe" },
    );
  } finally {
    rmSync(temp, { recursive: true, force: true });
  }
});
