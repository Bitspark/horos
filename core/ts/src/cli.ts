#!/usr/bin/env node
/** Evaluate wf/conforms over ontos-codec-v1 hex. */
import { decode } from "@bitspark/ontos-codec";
import { conforms, wf } from "./index.js";

function value(hex: string) {
  if (!/^(?:[0-9a-f]{2})*$/u.test(hex)) throw new Error("invalid lowercase hex");
  return decode(Uint8Array.from(Buffer.from(hex, "hex")));
}

try {
  const args = process.argv.slice(2);
  if (args[0] === "wf" && args.length === 2) {
    console.log(wf(value(args[1]!)));
  } else if (args[0] === "conforms" && args.length === 3) {
    console.log(conforms(value(args[2]!), value(args[1]!)));
  } else {
    throw new Error("usage: horos-conform wf <descriptor> | conforms <descriptor> <value>");
  }
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
}
