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
 *
 * Every outcome other than `conforms` carries one LOCATION (ADR 0009, docs/spec/horos.md
 * "Diagnostic locations"): the value path, the descriptor position and a reason, chosen in a
 * fixed order so the Go, Rust and TypeScript cores report the identical location. A location
 * never changes which verdict is reported.
 */

import { type Value } from "@bitspark/ontos-core";
import { readInt, readList, readMap, readSet, readText } from "@bitspark/ontos-data";

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

/** The kinds of value path step that select by a 0-based index in canonical order. */
export type StepKind = "item" | "elem" | "key" | "value";

/**
 * One step of a value path: `item` of a bare tuple, `elem` of a list or set, `key` or
 * `value` of a map entry, or the record `field` with this name.
 */
export type Step =
  | { readonly kind: StepKind; readonly index: number }
  | { readonly kind: "field"; readonly name: string };

/** Why the check failed at a location (ADR 0009). */
export type Reason =
  | "refused"
  | "missing-field"
  | "extra-field"
  | "no-alternative"
  | "complement"
  | "limit"
  | "not-descriptor"
  | "unknown-constructor"
  | "arity"
  | "argument"
  | "duplicate-field"
  | "unbound-ref"
  | "unguarded-ref";

/**
 * Where a check failed: the path into the value, the position of the descriptor at fault
 * (tuple-item indices from the root descriptor), and the reason. `field` names the record
 * field for `missing-field` and `extra-field`; `arg` is the item index of the faulty argument
 * for `argument`, `duplicate-field`, `unbound-ref` and `unguarded-ref`.
 */
export interface Location {
  readonly path: readonly Step[];
  readonly at: readonly number[];
  readonly reason: Reason;
  readonly field?: string;
  readonly arg?: number;
}

/**
 * A checking outcome that distinguishes a completed decision from a reached bound.
 *
 * `exhausted` is INCOMPLETE: a statement about THIS bounded attempt, never a claim that the
 * value does or does not have the form, and never a refutation. Every outcome other than
 * `conforms` names its location.
 */
export type Diagnosis =
  | { readonly verdict: "conforms" }
  | { readonly verdict: "mismatch"; readonly location: Location }
  | { readonly verdict: "not_well_formed"; readonly location: Location }
  | { readonly verdict: "exhausted"; readonly limit: Limit; readonly location: Location };

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

// ----- locations, built lazily -----

/** A value path as a persistent chain, innermost step first; `undefined` is the root. */
interface PathNode {
  readonly parent: PathNode | undefined;
  readonly step: Step;
  readonly length: number;
}

/** A descriptor position as a persistent chain, innermost index first. */
interface PosNode {
  readonly parent: PosNode | undefined;
  readonly index: number;
}

function stepInto(path: PathNode | undefined, step: Step): PathNode {
  return { parent: path, step, length: pathLength(path) + 1 };
}

function pathLength(path: PathNode | undefined): number {
  return path === undefined ? 0 : path.length;
}

function at(pos: PosNode | undefined, index: number): PosNode {
  return { parent: pos, index };
}

/** A failure point, materialized into a {@link Location} only when it is reported. */
interface Fault {
  readonly path: PathNode | undefined;
  readonly at: PosNode | undefined;
  readonly reason: Reason;
  readonly field?: string;
  readonly arg?: number;
}

/** The internal outcome: a verdict with its unmaterialized location. */
type Outcome =
  | { readonly verdict: "conforms" }
  | { readonly verdict: "mismatch" | "not_well_formed"; readonly fault: Fault }
  | { readonly verdict: "exhausted"; readonly limit: Limit; readonly fault: Fault };

const CONFORMS: Outcome = { verdict: "conforms" };
const CONFORMS_D: Diagnosis = { verdict: "conforms" };

const mismatch = (fault: Fault): Outcome => ({ verdict: "mismatch", fault });
const notWf = (fault: Fault): Outcome => ({ verdict: "not_well_formed", fault });
const exhausted = (path: PathNode | undefined, pos: PosNode | undefined): Outcome => ({
  verdict: "exhausted",
  limit: "recursion_depth",
  fault: { path, at: pos, reason: "limit" },
});

function materialize(f: Fault): Location {
  const path: Step[] = [];
  for (let n = f.path; n !== undefined; n = n.parent) path.push(n.step);
  path.reverse();
  const pos: number[] = [];
  for (let n = f.at; n !== undefined; n = n.parent) pos.push(n.index);
  pos.reverse();
  if (f.field !== undefined) return { path, at: pos, reason: f.reason, field: f.field };
  if (f.arg !== undefined) return { path, at: pos, reason: f.reason, arg: f.arg };
  return { path, at: pos, reason: f.reason };
}

function report(o: Outcome): Diagnosis {
  switch (o.verdict) {
    case "conforms":
      return CONFORMS_D;
    case "mismatch":
      return { verdict: "mismatch", location: materialize(o.fault) };
    case "not_well_formed":
      return { verdict: "not_well_formed", location: materialize(o.fault) };
    case "exhausted":
      return { verdict: "exhausted", limit: o.limit, location: materialize(o.fault) };
  }
}

// ----- the Kleene folds (ADR 0008 D2/D3), with the location order of ADR 0009 -----

/**
 * `not`: swap the completed verdicts, PRESERVE exhaustion. Never completes an incomplete
 * subdecision — the rule this module exists for. A `not` that fails reports itself.
 */
function kleeneNot(
  inner: Outcome,
  path: PathNode | undefined,
  pos: PosNode | undefined,
): Outcome {
  switch (inner.verdict) {
    case "conforms":
      return mismatch({ path, at: pos, reason: "complement" });
    case "mismatch":
      return CONFORMS;
    default:
      // not_well_formed cannot arise below the entry wf gate; exhausted is preserved.
      return inner;
  }
}

/**
 * Kleene conjunction over `count` sub-outcomes in the fixed order: a decisive `mismatch`
 * wins over an exhausted sibling and is the first one in order. Otherwise the first
 * exhausted, then the first not_well_formed sub-outcome decides.
 */
function kleeneAll(count: number, outcome: (i: number) => Outcome): Outcome {
  let firstExhausted: Outcome | undefined;
  let firstNotWf: Outcome | undefined;
  for (let i = 0; i < count; i += 1) {
    const o = outcome(i);
    if (o.verdict === "mismatch") return o; // decisive — stop here
    if (o.verdict === "exhausted") firstExhausted ??= o;
    else if (o.verdict === "not_well_formed") firstNotWf ??= o;
  }
  return firstExhausted ?? firstNotWf ?? CONFORMS;
}

/**
 * Kleene disjunction for `union`: a decisive `conforms` wins over an exhausted sibling. A
 * union with no conforming alternative is exhausted at its first exhausted alternative. A
 * mismatching union reports the first alternative with the longest value path when that
 * path goes deeper than the union's own value, and otherwise reports itself.
 */
function kleeneAny(
  count: number,
  outcome: (i: number) => Outcome,
  path: PathNode | undefined,
  pos: PosNode | undefined,
): Outcome {
  let firstExhausted: Outcome | undefined;
  let firstNotWf: Outcome | undefined;
  let deepest: Outcome | undefined;
  let deepestLength = pathLength(path);
  for (let i = 0; i < count; i += 1) {
    const o = outcome(i);
    if (o.verdict === "conforms") return CONFORMS; // decisive — stop here
    if (o.verdict === "exhausted") firstExhausted ??= o;
    else if (o.verdict === "not_well_formed") firstNotWf ??= o;
    else if (pathLength(o.fault.path) > deepestLength) {
      deepest = o;
      deepestLength = pathLength(o.fault.path);
    }
  }
  return (
    firstExhausted ??
    firstNotWf ??
    deepest ??
    mismatch({ path, at: pos, reason: "no-alternative" })
  );
}

// ----- the public entry points -----

/**
 * Diagnose `wf(T)`: `conforms` when T is a well-formed descriptor, `not_well_formed` when it is
 * conclusively not, and `exhausted` when the well-formedness traversal itself could not finish
 * — the distinction horos-internal#211 asks for by name. A non-conforming outcome names the
 * descriptor position and, where one argument is at fault, its item index.
 */
export function diagnoseWf(t: Value): Diagnosis {
  return report(diagnoseWfIn(t, [], [], 0, undefined));
}

/**
 * Diagnose `v : T`. Mirrors `conforms`: the `wf(T)` soundness gate runs first (spec §3 MUST),
 * so a descriptor that is not well-formed yields `not_well_formed` and one whose
 * well-formedness could not be decided yields `exhausted` — neither is reported as a mismatch
 * of the value. Each carries the well-formedness location, with an empty value path.
 */
export function diagnose(v: Value, t: Value): Diagnosis {
  const w = diagnoseWfIn(t, [], [], 0, undefined);
  if (w.verdict !== "conforms") return report(w);
  return report(diagnoseIn(v, t, [], [], 0, undefined, undefined));
}

// ----- the recursions: arm-for-arm mirrors of conformsIn / wfIn -----

/** One `rec` binding; each `rec` frame creates its own, so identity is binding identity. */
interface Binding {
  readonly name: string;
  readonly body: Value;
  /** The position of the `rec` body, where a `ref` to this binding continues. */
  readonly at: PosNode;
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

/** One child sub-judgment of a structural arm: a child value, its descriptor and location. */
interface Child {
  readonly v: Value;
  readonly t: Value;
  readonly step: Step;
  readonly at: PosNode;
}

function diagnoseIn(
  v: Value,
  t: Value,
  env: readonly Binding[],
  pending: readonly Binding[],
  depth: number,
  path: PathNode | undefined,
  pos: PosNode | undefined,
): Outcome {
  if (depth > MAX_RECURSION_DEPTH) return exhausted(path, pos);
  const refused = (): Outcome => mismatch({ path, at: pos, reason: "refused" });
  const h = head(t);
  if (h === undefined) return refused(); // not a horos descriptor; unreachable past the wf gate
  const { ctor, args } = h;

  // Leaves never recurse into a value position, so they are always decisive here.
  if (isLeafCtor(ctor)) return leafClassifies(v, t) ? CONFORMS : refused();

  switch (ctor) {
    // Composition on the SAME v — pending threads through unchanged.
    case "union":
      return kleeneAny(
        args.length,
        (i) => diagnoseIn(v, args[i]!, env, pending, depth + 1, path, at(pos, i + 1)),
        path,
        pos,
      );
    case "and":
      return kleeneAll(args.length, (i) =>
        diagnoseIn(v, args[i]!, env, pending, depth + 1, path, at(pos, i + 1)),
      );
    case "not": {
      if (args.length !== 1) return refused();
      return kleeneNot(
        diagnoseIn(v, args[0]!, env, pending, depth + 1, path, at(pos, 1)),
        path,
        pos,
      );
    }

    // Recursion — mirrors the recognizer's guard exactly, and is transparent to locations.
    // A ref denotes the rec that lexically encloses it and continues in that binding's
    // environment (ADR 0011). `env` holds exactly the enclosing bindings, outermost first,
    // so a binding's closure is the prefix of `env` that ends with it.
    case "rec": {
      if (args.length !== 2) return refused();
      const name = atomStringOf(args[0]!);
      if (name === undefined) return refused();
      const body = args[1]!;
      const bodyAt = at(pos, 2);
      const binding: Binding = { name, body, at: bodyAt };
      return diagnoseIn(
        v,
        body,
        [...env, binding],
        [...pending, binding],
        depth + 1,
        path,
        bodyAt,
      );
    }
    case "ref": {
      if (args.length !== 1) return refused();
      const name = atomStringOf(args[0]!);
      if (name === undefined) return refused();
      for (let i = env.length - 1; i >= 0; i -= 1) {
        const b = env[i]!;
        if (b.name === name) {
          if (pending.includes(b)) return refused(); // unguarded — the recognizer's false
          return diagnoseIn(
            v,
            b.body,
            env.slice(0, i + 1),
            [...pending, b],
            depth + 1,
            path,
            b.at,
          );
        }
      }
      return refused(); // unbound
    }
  }

  // Structural arms: the gate is decisive, and a descent into a CHILD VALUE clears pending.
  const gate = structuralChildren(v, ctor, args, path, pos);
  if (!Array.isArray(gate)) return mismatch(gate as Fault);
  const children = gate as readonly Child[];
  return kleeneAll(children.length, (i) => {
    const c = children[i]!;
    return diagnoseIn(c.v, c.t, env, [], depth + 1, stepInto(path, c.step), c.at);
  });
}

/**
 * The structural gate plus the ordered child sub-judgments, for the arms that descend into a
 * child VALUE. Mirrors the recognizer's recursive arms; a {@link Fault} means the gate failed,
 * so `v` cannot conform regardless of any child.
 */
function structuralChildren(
  v: Value,
  ctor: string,
  args: readonly Value[],
  path: PathNode | undefined,
  pos: PosNode | undefined,
): readonly Child[] | Fault {
  const refused: Fault = { path, at: pos, reason: "refused" };
  switch (ctor) {
    case "tuple": {
      if (v.kind !== "tuple") return refused;
      const items = v.items();
      if (items.length !== args.length) return refused;
      return items.map((c, i) => ({
        v: c,
        t: args[i]!,
        step: { kind: "item", index: i },
        at: at(pos, i + 1),
      }));
    }
    case "seq": {
      if (args.length !== 1 || v.kind !== "tuple") return refused;
      const childAt = at(pos, 1);
      return v.items().map((c, i) => ({
        v: c,
        t: args[0]!,
        step: { kind: "item", index: i },
        at: childAt,
      }));
    }
    case "compound": {
      if (args.length < 1 || v.kind !== "tuple") return refused;
      const label = atomBytesOf(args[0]!);
      const items = v.items();
      if (label === undefined || items.length !== args.length) return refused;
      const got = atomBytesOf(items[0]!);
      if (
        got === undefined ||
        got.length !== label.length ||
        got.some((b, i) => b !== label[i])
      )
        return refused;
      // Value item j (j >= 1) is checked by descriptor argument j, at item index j + 1.
      return args.slice(1).map((ct, i) => ({
        v: items[i + 1]!,
        t: ct,
        step: { kind: "item", index: i + 1 },
        at: at(pos, i + 2),
      }));
    }
    case "list-of":
    case "set-of": {
      if (args.length !== 1) return refused;
      let elems: Value[];
      try {
        elems = ctor === "list-of" ? readList(v) : readSet(v);
      } catch {
        return refused;
      }
      const childAt = at(pos, 1);
      return elems.map((e, i) => ({
        v: e,
        t: args[0]!,
        step: { kind: "elem", index: i },
        at: childAt,
      }));
    }
    case "map-of": {
      if (args.length !== 2) return refused;
      let entries: readonly (readonly [Value, Value])[];
      try {
        entries = readMap(v);
      } catch {
        return refused;
      }
      const keyAt = at(pos, 1);
      const valueAt = at(pos, 2);
      const out: Child[] = [];
      entries.forEach(([k, val], i) => {
        out.push(
          { v: k, t: args[0]!, step: { kind: "key", index: i }, at: keyAt },
          { v: val, t: args[1]!, step: { kind: "value", index: i }, at: valueAt },
        );
      });
      return out;
    }
    case "record":
      return args.length < 1
        ? refused
        : recordChildren(v, args[0]!, args.slice(1), path, pos);
  }
  return refused;
}

/**
 * Mirrors the recognizer's record gate and present-field worklist, in the fixed order of
 * ADR 0009: a canonical map, text keys, missing required fields in descriptor order, extra
 * fields of a closed record in canonical key order, then the present declared fields.
 */
function recordChildren(
  v: Value,
  mode: Value,
  fields: readonly Value[],
  path: PathNode | undefined,
  pos: PosNode | undefined,
): readonly Child[] | Fault {
  const refused: Fault = { path, at: pos, reason: "refused" };
  const m = atomStringOf(mode);
  if (m !== "open" && m !== "closed") return refused;
  let entries: readonly (readonly [Value, Value])[];
  try {
    entries = readMap(v);
  } catch {
    return refused;
  }
  const keyed = new Map<string, Value>();
  for (let i = 0; i < entries.length; i += 1) {
    const [k, val] = entries[i]!;
    try {
      keyed.set(readText(k), val);
    } catch {
      return {
        path: stepInto(path, { kind: "key", index: i }),
        at: pos,
        reason: "refused",
      };
    }
  }
  const out: Child[] = [];
  const declared = new Set<string>();
  for (let j = 0; j < fields.length; j += 1) {
    const fd = readFieldDescriptor(fields[j]!);
    if (fd === undefined) return refused;
    declared.add(fd.name);
    const val = keyed.get(fd.name);
    if (val === undefined) {
      if (fd.required) return { path, at: pos, reason: "missing-field", field: fd.name };
      continue;
    }
    // Field j is descriptor item j + 2; its type is item 3 of the field form.
    out.push({
      v: val,
      t: fd.ty,
      step: { kind: "field", name: fd.name },
      at: at(at(pos, j + 2), 3),
    });
  }
  if (m === "closed") {
    for (const k of keyed.keys()) {
      if (!declared.has(k)) return { path, at: pos, reason: "extra-field", field: k };
    }
  }
  return out;
}

/** In-range check of an `atom-len`/`text-len` bound: the signed 128-bit cap. */
function boundInRange(n: bigint): boolean {
  return n >= ATOM_LEN_BOUND_MIN && n <= ATOM_LEN_BOUND_MAX;
}

/**
 * The leaf constructors' own-form check, in the fixed order of ADR 0009: arity, then each
 * argument in item order against the arguments before it. `undefined` is well formed;
 * otherwise the reason and, for an argument, its item index. Mirrors the recognizer's
 * `wfIn` leaf arms, which decide the same set.
 */
function leafWfFault(
  ctor: string,
  args: readonly Value[],
): { readonly reason: Reason; readonly arg?: number } | undefined {
  const arity = (n: number) =>
    args.length === n ? undefined : { reason: "arity" as const };
  const argument = (i: number) => ({ reason: "argument" as const, arg: i });
  switch (ctor) {
    case "any":
    case "atom":
    case "key-grammar":
    case "text-non-blank":
    case "lowercase-uuid":
      return arity(0);
    case "exact":
      return arity(1);
    case "atom-exact":
      return arity(1) ?? (atomBytesOf(args[0]!) === undefined ? argument(1) : undefined);
    case "int-min":
    case "int-max": {
      const a = arity(1);
      if (a !== undefined) return a;
      try {
        return boundInRange(readInt(args[0]!)) ? undefined : argument(1);
      } catch {
        return argument(1);
      }
    }
    case "atom-len":
    case "text-len": {
      const a = arity(2);
      if (a !== undefined) return a;
      let lo: bigint;
      try {
        lo = readInt(args[0]!);
      } catch {
        return argument(1);
      }
      if (!boundInRange(lo) || lo < 0n) return argument(1);
      let hi: bigint;
      try {
        hi = readInt(args[1]!);
      } catch {
        return argument(2);
      }
      return !boundInRange(hi) || hi < lo ? argument(2) : undefined;
    }
    case "atom-in": {
      const a = arity(1);
      if (a !== undefined) return a;
      let elems: Value[];
      try {
        elems = readSet(args[0]!);
      } catch {
        return argument(1);
      }
      return elems.every((e) => e.kind === "atom") ? undefined : argument(1);
    }
    case "scalar": {
      const a = arity(1);
      if (a !== undefined) return a;
      const kind = atomStringOf(args[0]!);
      return kind === "int" || kind === "utf8-text" || kind === "bool"
        ? undefined
        : argument(1);
    }
  }
  return { reason: "unknown-constructor" };
}

function diagnoseWfIn(
  t: Value,
  bound: readonly string[],
  pending: readonly string[],
  depth: number,
  pos: PosNode | undefined,
): Outcome {
  if (depth > MAX_RECURSION_DEPTH) return exhausted(undefined, pos);
  const fault = (reason: Reason, arg?: number): Outcome =>
    notWf(
      arg === undefined
        ? { path: undefined, at: pos, reason }
        : { path: undefined, at: pos, reason, arg },
    );
  const h = head(t);
  if (h === undefined) return fault("not-descriptor");
  const { ctor, args } = h;
  if (isLeafCtor(ctor)) {
    // The recognizer decides the verdict; leafWfFault only names the fault.
    if (leafWellFormed(t)) return CONFORMS;
    const f = leafWfFault(ctor, args) ?? { reason: "argument" as const };
    return fault(f.reason, f.arg);
  }

  // Child descriptors at the given item indices, in order, after the own form is accepted.
  const wfChildren = (items: readonly number[], p: readonly string[]): Outcome =>
    kleeneAll(items.length, (i) =>
      diagnoseWfIn(args[items[i]! - 1]!, bound, p, depth + 1, at(pos, items[i]!)),
    );
  const all = (from: number): number[] => {
    const out: number[] = [];
    for (let i = from; i <= args.length; i += 1) out.push(i);
    return out;
  };

  switch (ctor) {
    case "tuple":
      return wfChildren(all(1), []);
    case "seq":
    case "list-of":
    case "set-of":
      return args.length !== 1 ? fault("arity") : wfChildren([1], []);
    case "compound":
      if (args.length < 1) return fault("arity");
      if (atomBytesOf(args[0]!) === undefined) return fault("argument", 1);
      return wfChildren(all(2), []);
    case "map-of":
      return args.length !== 2 ? fault("arity") : wfChildren([1, 2], []);
    case "union":
    case "and":
      return wfChildren(all(1), pending);
    case "not":
      return args.length !== 1 ? fault("arity") : wfChildren([1], pending);
    case "rec": {
      if (args.length !== 2) return fault("arity");
      const name = atomStringOf(args[0]!);
      if (name === undefined) return fault("argument", 1);
      return diagnoseWfIn(
        args[1]!,
        [...bound, name],
        [...pending, name],
        depth + 1,
        at(pos, 2),
      );
    }
    case "ref": {
      if (args.length !== 1) return fault("arity");
      const name = atomStringOf(args[0]!);
      if (name === undefined) return fault("argument", 1);
      if (!bound.includes(name)) return fault("unbound-ref", 1);
      if (pending.includes(name)) return fault("unguarded-ref", 1);
      return CONFORMS;
    }
    case "record": {
      if (args.length < 1) return fault("arity");
      const mode = atomStringOf(args[0]!);
      if (mode !== "open" && mode !== "closed") return fault("argument", 1);
      const names = new Set<string>();
      const types: Value[] = [];
      for (let k = 2; k <= args.length; k += 1) {
        const fd = readFieldDescriptor(args[k - 1]!);
        if (fd === undefined) return fault("argument", k);
        if (names.has(fd.name)) return fault("duplicate-field", k);
        names.add(fd.name);
        types.push(fd.ty);
      }
      // Field k's type is item 3 of the field form at item k.
      return kleeneAll(types.length, (i) =>
        diagnoseWfIn(types[i]!, bound, [], depth + 1, at(at(pos, i + 2), 3)),
      );
    }
  }
  return fault("unknown-constructor");
}
