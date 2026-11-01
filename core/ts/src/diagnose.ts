/*
 * Diagnostic checking outcomes (horos-internal#211, docs/architecture/decisions/0008-diagnostic-outcomes-kleene.md).
 *
 * A byte-for-byte port of core/rs/src/diagnose.rs (and core/go/diagnose.go). An ADDITIVE
 * companion to the frozen boolean surface: `wf` and `conforms` are unchanged,
 * vectors/conformance.json is unchanged, and nothing here reinterprets what today's `false`
 * means.
 *
 * What it adds is the distinction the boolean cannot carry: a completed structural MISMATCH, a
 * MALFORMED descriptor, and a computation that reached the declared MAX_RECURSION_DEPTH bound
 * are three different pieces of evidence, and the boolean renders all three as `false`.
 *
 * The composition rules are the content (ADR 0008 D2/D3), not the extra enum:
 *
 *  - Complement PRESERVES exhaustion: not(Exhausted) = Exhausted, never Conforms. The frozen
 *    recognizer computes !conformsIn(…), so an inner descent that trips the cap yields false and
 *    `not` reports an unqualified `true` — a completed positive out of a computation that never
 *    finished. That is the defect this module exists to name.
 *  - union/and are KLEENE: a DECISIVE alternative beats an exhausted sibling, so the evaluator
 *    short-circuits on a decisive result but NEVER on an Exhausted. The answer is a function of
 *    the SET of sub-outcomes, never of traversal order.
 */

import { type Value } from "@bitspark/ontos-core";
import { readList, readMap, readSet, readText } from "@bitspark/ontos-data";

import {
  ATOM_LEN_BOUND_MAX,
  ATOM_LEN_BOUND_MIN,
  isLeafCtor,
  leafClassifies,
  leafWellFormed,
  MAX_RECURSION_DEPTH,
  NAMESPACE,
  readFieldDescriptor,
} from "./core.js";

/** The declared bound an `exhausted` outcome reached. */
export type Limit = "recursion_depth";

/** The effective value of a limit in this build. */
export function limitValue(l: Limit): number {
  return l === "recursion_depth" ? MAX_RECURSION_DEPTH : 0;
}

/** The completed/incomplete classification of a checking attempt. */
export type Verdict = "conforms" | "mismatch" | "not_well_formed" | "exhausted";

/**
 * A checking outcome that distinguishes a completed decision from a reached bound.
 *
 * `exhausted` is INCOMPLETE: a statement about THIS bounded attempt, never a claim that the
 * value does or does not have the form, and never a refutation.
 */
export type Diagnosis =
  | { readonly verdict: "conforms" }
  | { readonly verdict: "mismatch" }
  | { readonly verdict: "not_well_formed" }
  | { readonly verdict: "exhausted"; readonly limit: Limit };

const CONFORMS: Diagnosis = { verdict: "conforms" };
const MISMATCH: Diagnosis = { verdict: "mismatch" };
const NOT_WF: Diagnosis = { verdict: "not_well_formed" };
const exhausted = (limit: Limit): Diagnosis => ({ verdict: "exhausted", limit });

/** Whether this outcome is a completed decision. */
export function isComplete(d: Diagnosis): boolean {
  return d.verdict !== "exhausted";
}

/**
 * The legacy boolean this outcome corresponds to WHERE ONE IS DETERMINED (ADR 0008 D4):
 * `true` for conforms, `false` for mismatch/not_well_formed, and **`undefined` for exhausted**
 * — the boolean is unconstrained there, and collapsing the outcome into it is exactly what
 * horos-internal#211 asks not to do.
 */
export function completedBool(d: Diagnosis): boolean | undefined {
  switch (d.verdict) {
    case "conforms":
      return true;
    case "mismatch":
    case "not_well_formed":
      return false;
    default:
      return undefined;
  }
}

/**
 * The checking profile a Diagnosis is stated under. A completed negative is completed UNDER
 * THIS PROFILE, never absolutely — so the profile travels with the verdict.
 */
export interface Profile {
  readonly maxRecursionDepth: number;
  readonly atomLenBoundMin: bigint;
  readonly atomLenBoundMax: bigint;
}

/** The profile this build decides under. */
export function checkingProfile(): Profile {
  return {
    maxRecursionDepth: MAX_RECURSION_DEPTH,
    atomLenBoundMin: ATOM_LEN_BOUND_MIN,
    atomLenBoundMax: ATOM_LEN_BOUND_MAX,
  };
}

// ----- the Kleene folds (ADR 0008 D2/D3) -----

/**
 * `not`: swap the completed verdicts, PRESERVE exhaustion. Never completes an incomplete
 * subdecision — the rule this module exists for.
 */
function kleeneNot(inner: Diagnosis): Diagnosis {
  switch (inner.verdict) {
    case "conforms":
      return MISMATCH;
    case "mismatch":
      return CONFORMS;
    default:
      // not_well_formed cannot arise below the entry wf gate; exhausted is preserved.
      return inner;
  }
}

/** Kleene disjunction: a decisive `conforms` wins over an exhausted sibling. */
function kleeneAny(outcomes: readonly Diagnosis[]): Diagnosis {
  let limit: Limit | undefined;
  let notWF = false;
  for (const o of outcomes) {
    if (o.verdict === "conforms") return CONFORMS; // decisive — stop here
    if (o.verdict === "not_well_formed") notWF = true;
    else if (o.verdict === "exhausted" && limit === undefined) limit = o.limit;
  }
  if (limit !== undefined) return exhausted(limit);
  return notWF ? NOT_WF : MISMATCH;
}

/** Kleene conjunction: a decisive `mismatch` wins over an exhausted sibling. */
function kleeneAll(outcomes: readonly Diagnosis[]): Diagnosis {
  let limit: Limit | undefined;
  let notWF = false;
  for (const o of outcomes) {
    if (o.verdict === "mismatch") return MISMATCH; // decisive — stop here
    if (o.verdict === "not_well_formed") notWF = true;
    else if (o.verdict === "exhausted" && limit === undefined) limit = o.limit;
  }
  if (limit !== undefined) return exhausted(limit);
  return notWF ? NOT_WF : CONFORMS;
}

// ----- the public entry points -----

/**
 * Diagnose `wf(T)`: `conforms` when T is a well-formed descriptor, `not_well_formed` when it is
 * conclusively not, and `exhausted` when the well-formedness traversal itself could not finish
 * — the distinction horos-internal#211 asks for by name.
 */
export function diagnoseWf(t: Value): Diagnosis {
  return diagnoseWfIn(t, [], [], 0);
}

/**
 * Diagnose `v : T`. Mirrors `conforms`: the `wf(T)` soundness gate runs first (spec §3 MUST),
 * so a descriptor that is not well-formed yields `not_well_formed` and one whose
 * well-formedness could not be decided yields `exhausted` — neither is reported as a mismatch
 * of the value.
 */
export function diagnose(v: Value, t: Value): Diagnosis {
  const w = diagnoseWf(t);
  if (w.verdict === "conforms") return diagnoseIn(v, t, [], [], 0);
  if (w.verdict === "exhausted") return w;
  return NOT_WF;
}

// ----- the recursions: arm-for-arm mirrors of conformsIn / wfIn -----

interface Binding {
  readonly name: string;
  readonly body: Value;
}

function head(t: Value): { ctor: string; args: readonly Value[] } | undefined {
  if (t.kind !== "tuple") return undefined;
  const items = t.items();
  const first = items[0];
  if (first === undefined || first.kind !== "atom") return undefined;
  let s: string;
  try {
    s = new TextDecoder("utf-8", { fatal: true, ignoreBOM: true }).decode(first.bytes());
  } catch {
    return undefined;
  }
  if (!s.startsWith(NAMESPACE)) return undefined;
  return { ctor: s.slice(NAMESPACE.length), args: items.slice(1) };
}

function atomBytesOf(v: Value): Uint8Array | undefined {
  return v.kind === "atom" ? v.bytes() : undefined;
}

function atomStringOf(v: Value): string | undefined {
  const b = atomBytesOf(v);
  if (b === undefined) return undefined;
  // One code point per byte: arbitrary atom names must never alias through UTF-8 replacement.
  let key = "";
  for (const byte of b) key += String.fromCharCode(byte);
  return key;
}

function diagnoseIn(
  v: Value,
  t: Value,
  env: readonly Binding[],
  pending: readonly string[],
  depth: number,
): Diagnosis {
  if (depth > MAX_RECURSION_DEPTH) return exhausted("recursion_depth");
  const h = head(t);
  if (h === undefined) return MISMATCH; // not a horos descriptor
  const { ctor, args } = h;

  // Leaves never recurse into a value position, so they are always decisive here.
  if (isLeafCtor(ctor)) return leafClassifies(v, t) ? CONFORMS : MISMATCH;

  switch (ctor) {
    // Composition on the SAME v — pending threads through unchanged.
    case "union":
      return kleeneAny(args.map((ct) => diagnoseIn(v, ct, env, pending, depth + 1)));
    case "and":
      return kleeneAll(args.map((ct) => diagnoseIn(v, ct, env, pending, depth + 1)));
    case "not": {
      if (args.length !== 1) return MISMATCH;
      return kleeneNot(diagnoseIn(v, args[0]!, env, pending, depth + 1));
    }

    // Recursion — mirrors the recognizer's guard exactly.
    case "rec": {
      if (args.length !== 2) return MISMATCH;
      const name = atomStringOf(args[0]!);
      if (name === undefined) return MISMATCH;
      const body = args[1]!;
      return diagnoseIn(v, body, [...env, { name, body }], [...pending, name], depth + 1);
    }
    case "ref": {
      if (args.length !== 1) return MISMATCH;
      const name = atomStringOf(args[0]!);
      if (name === undefined) return MISMATCH;
      if (pending.includes(name)) return MISMATCH; // unguarded — the recognizer's false
      for (let i = env.length - 1; i >= 0; i -= 1) {
        if (env[i]!.name === name) {
          return diagnoseIn(v, env[i]!.body, env, [...pending, name], depth + 1);
        }
      }
      return MISMATCH; // unbound
    }
  }

  // Structural arms: the gate is decisive, and a descent into a CHILD VALUE clears pending.
  const children = structuralChildren(v, ctor, args);
  if (children === undefined) return MISMATCH;
  return kleeneAll(children.map(([cv, ct]) => diagnoseIn(cv, ct, env, [], depth + 1)));
}

/**
 * The structural gate plus the ordered child sub-judgments, for the arms that descend into a
 * child VALUE. Mirrors the recognizer's recursive arms; `undefined` means the gate failed, so
 * `v` cannot conform regardless of any child.
 */
function structuralChildren(
  v: Value,
  ctor: string,
  args: readonly Value[],
): [Value, Value][] | undefined {
  switch (ctor) {
    case "tuple": {
      if (v.kind !== "tuple") return undefined;
      const items = v.items();
      if (items.length !== args.length) return undefined;
      return items.map((c, i) => [c, args[i]!] as [Value, Value]);
    }
    case "seq": {
      if (args.length !== 1 || v.kind !== "tuple") return undefined;
      return v.items().map((c) => [c, args[0]!] as [Value, Value]);
    }
    case "compound": {
      if (args.length < 1 || v.kind !== "tuple") return undefined;
      const label = atomBytesOf(args[0]!);
      const items = v.items();
      if (label === undefined || items.length !== args.length) return undefined;
      const got = atomBytesOf(items[0]!);
      if (
        got === undefined ||
        got.length !== label.length ||
        got.some((b, i) => b !== label[i])
      )
        return undefined;
      return args.slice(1).map((ct, i) => [items[i + 1]!, ct] as [Value, Value]);
    }
    case "list-of":
    case "set-of": {
      if (args.length !== 1) return undefined;
      try {
        const elems = ctor === "list-of" ? readList(v) : readSet(v);
        return elems.map((e) => [e, args[0]!] as [Value, Value]);
      } catch {
        return undefined;
      }
    }
    case "map-of": {
      if (args.length !== 2) return undefined;
      try {
        const out: [Value, Value][] = [];
        for (const [k, val] of readMap(v)) {
          out.push([k, args[0]!], [val, args[1]!]);
        }
        return out;
      } catch {
        return undefined;
      }
    }
    case "record":
      return args.length < 1 ? undefined : recordChildren(v, args[0]!, args.slice(1));
  }
  return undefined;
}

/** Mirrors the recognizer's record gate and present-field worklist. */
function recordChildren(
  v: Value,
  mode: Value,
  fields: readonly Value[],
): [Value, Value][] | undefined {
  const m = atomStringOf(mode);
  if (m !== "open" && m !== "closed") return undefined;
  const closed = m === "closed";
  let entries: readonly (readonly [Value, Value])[];
  try {
    entries = readMap(v);
  } catch {
    return undefined;
  }
  const keyed = new Map<string, Value>();
  for (const [k, val] of entries) {
    try {
      keyed.set(readText(k), val);
    } catch {
      return undefined;
    }
  }
  const out: [Value, Value][] = [];
  const declared = new Set<string>();
  for (const f of fields) {
    const fd = readFieldDescriptor(f);
    if (fd === undefined) return undefined;
    declared.add(fd.name);
    const val = keyed.get(fd.name);
    if (val === undefined) {
      if (fd.required) return undefined; // a missing required field is decisive
      continue;
    }
    out.push([val, fd.ty]);
  }
  if (closed) {
    for (const k of keyed.keys()) {
      if (!declared.has(k)) return undefined; // an extra key in a closed record is decisive
    }
  }
  return out;
}

function diagnoseWfIn(
  t: Value,
  bound: readonly string[],
  pending: readonly string[],
  depth: number,
): Diagnosis {
  if (depth > MAX_RECURSION_DEPTH) return exhausted("recursion_depth");
  const h = head(t);
  if (h === undefined) return NOT_WF;
  const { ctor, args } = h;
  if (isLeafCtor(ctor)) return leafWellFormed(t) ? CONFORMS : NOT_WF;

  const wfAll = (ts: readonly Value[], p: readonly string[]): Diagnosis =>
    kleeneAll(ts.map((ct) => diagnoseWfIn(ct, bound, p, depth + 1)));

  switch (ctor) {
    case "tuple":
      return wfAll(args, []);
    case "seq":
      return args.length !== 1 ? NOT_WF : diagnoseWfIn(args[0]!, bound, [], depth + 1);
    case "compound":
      if (args.length < 1 || atomBytesOf(args[0]!) === undefined) return NOT_WF;
      return wfAll(args.slice(1), []);
    case "list-of":
    case "set-of":
      return args.length !== 1 ? NOT_WF : diagnoseWfIn(args[0]!, bound, [], depth + 1);
    case "map-of":
      if (args.length !== 2) return NOT_WF;
      return kleeneAll([
        diagnoseWfIn(args[0]!, bound, [], depth + 1),
        diagnoseWfIn(args[1]!, bound, [], depth + 1),
      ]);
    case "union":
    case "and":
      return wfAll(args, pending);
    case "not":
      return args.length !== 1
        ? NOT_WF
        : diagnoseWfIn(args[0]!, bound, pending, depth + 1);
    case "rec": {
      if (args.length !== 2) return NOT_WF;
      const name = atomStringOf(args[0]!);
      if (name === undefined) return NOT_WF;
      return diagnoseWfIn(args[1]!, [...bound, name], [...pending, name], depth + 1);
    }
    case "ref": {
      if (args.length !== 1) return NOT_WF;
      const name = atomStringOf(args[0]!);
      return name !== undefined && bound.includes(name) && !pending.includes(name)
        ? CONFORMS
        : NOT_WF;
    }
    case "record": {
      if (args.length < 1) return NOT_WF;
      const mode = atomStringOf(args[0]!);
      if (mode !== "open" && mode !== "closed") return NOT_WF;
      const names = new Set<string>();
      const outcomes: Diagnosis[] = [];
      for (const field of args.slice(1)) {
        const fd = readFieldDescriptor(field);
        if (fd === undefined || names.has(fd.name)) return NOT_WF;
        names.add(fd.name);
        outcomes.push(diagnoseWfIn(fd.ty, bound, [], depth + 1));
      }
      return kleeneAll(outcomes);
    }
  }
  return NOT_WF;
}
