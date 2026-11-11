/*
 * horos — the form contract (TypeScript core).
 *
 * A byte-for-byte port of the Rust reference core (core/rs/src/lib.rs) and the Go
 * core (core/go/horos.go). It implements the v1 judgment surface — wf(T) and v : T
 * (`conforms`) — over the horos/* descriptor DSL, as TOTAL, SEARCH-FREE structural
 * recursion over the finite trees of the value v and the descriptor T (both ordinary
 * ontos-core Values). Scalar and collection recognition delegate to the frozen
 * ontos/data recognizers; horos re-mints nothing.
 *
 * See docs/spec/horos.md (the frozen v1 surface) and
 * docs/architecture/decisions/0002-form-reasoning-cut.md.
 *
 * Both `wf` and `conforms` are TOTAL (they never throw), SEARCH-FREE, and
 * byte-for-byte equivalent to the Rust reference; the shared
 * vectors/conformance.json oracle pins the three cores (go/rs/ts) together.
 *
 * rec/ref guarded recursion
 * -------------------------
 * `rec` binds a recursion name to its body descriptor; `ref` unfolds it. A `ref`
 * denotes the `rec` that lexically encloses it in the written descriptor (ADR 0011):
 * each binding closes over the environment it was written in, and a `ref` continues in
 * its binding's environment, never the unfolding site's. `wf` requires a descent into
 * a strict substructure of v between every `rec` and each `ref` that denotes it, so
 * between two descents each unfolding reaches a `rec` strictly above every descriptor
 * node checked since the last descent: a `ref` never unfolds a binding twice at one
 * v-node. Termination: unfoldings are bounded by |v| times the descriptor's nesting,
 * and every frame counts against MAX_RECURSION_DEPTH.
 */

import { Atom, Tuple, bytesEqual, type Value } from "@bitspark/ontos-core";
import {
  readBool,
  readInt,
  readList,
  readMap,
  readSet,
  readText,
  recognizeInt,
} from "@bitspark/ontos-data";

/** The ontos core value type that horos classifies (descriptors) and checks (values). */
export type { Value } from "@bitspark/ontos-core";

/**
 * The reserved descriptor-label namespace. Every descriptor combinator is a
 * `Tuple(Atom("horos/<ctor>"), args...)`; a tuple whose first child is not an atom
 * under this prefix is not a horos descriptor (per ontos-data §1.1 reservation).
 */
export const NAMESPACE = "horos/";

/**
 * The inclusive i128 range an `atom-len` bound must fit in to be well-formed (horos-internal#18):
 * `[-2^127, 2^127 - 1]`. The Rust reference reads bounds into an `i128` and errors
 * (→ non-wf / false) past this range; our `readInt` returns an unbounded `bigint`, so we
 * apply the SAME cap explicitly via {@link atomLenBoundInRange}. The three cores then
 * agree byte-for-byte at the cap — a bound = 2^127 is non-wf in all of them — and below
 * it for every real byte-length bound. This makes atom-len byte-identity a theorem rather
 * than a "never happens" argument; byte-length bounds are tiny, so it rejects nothing
 * meaningful. (The Go core mirrors this with `atomLenBound*`, the Rust core implicitly via
 * its `read_int` → i128 materialization.)
 */
export const ATOM_LEN_BOUND_MAX = (1n << 127n) - 1n; // 2^127 - 1 = i128::MAX
export const ATOM_LEN_BOUND_MIN = -(1n << 127n); // -2^127 = i128::MIN

/**
 * Whether a bound fits in the signed 128-bit range — the explicit analogue of the Rust
 * core's `read_int` succeeding (vs. returning `IntOutOfRange`). A bound outside the range
 * makes the atom-len descriptor non-wf / non-conforming, matching Rust and Go.
 */
function atomLenBoundInRange(n: bigint): boolean {
  return n >= ATOM_LEN_BOUND_MIN && n <= ATOM_LEN_BOUND_MAX;
}

/**
 * The shared recursion-depth guard (horos-internal#37) — the SAME constant in all three cores (Rust
 * `MAX_RECURSION_DEPTH`, Go `MaxRecursionDepth`). `conformsIn`/`wfIn` recurse on the
 * native stack; termination is bounded by `|v|` (`wf`'s guardedness) but there is no
 * stack bound, so an adversarial-but-finite value (e.g. a tuple nested hundreds of
 * thousands deep) could overflow the stack — a `RangeError: Maximum call stack size
 * exceeded` — at a core-dependent depth, breaking the total/never-throws contract.
 *
 * Past `MAX_RECURSION_DEPTH` frames, `conformsIn`/`wfIn` return a DETERMINISTIC result
 * (`false` / non-wf), never a throw, at the same logical depth as Rust and Go (each
 * increments a depth counter at identical recursion points). Termination is thus bounded
 * by `min(|v|, MAX_RECURSION_DEPTH)`; the cap is a robustness guardrail, not a change to
 * the recognized language for in-bound inputs. `512` is far above any descriptor/value
 * the spec, tests, or vectors exercise (< ~30, so no judgment changes) and comfortably
 * below the smallest native stack budget of the three cores (validated empirically).
 */
export const MAX_RECURSION_DEPTH = 512;

const utf8Decoder = new TextDecoder("utf-8", { fatal: true, ignoreBOM: true });

/** A parsed descriptor head: the constructor name and its argument values. */
interface Head {
  readonly ctor: string;
  readonly args: readonly Value[];
}

/**
 * A persistent (cons-list) stack threaded through the recursion by reference. Each
 * `rec`/`ref` frame pushes ONE node that points at the parent chain instead of
 * spread-copying the whole array (`[...env, …]` / `[...pending, …]`), so a descent costs
 * O(1) and allocates one small node rather than an O(depth) array. Lookups walk
 * head→tail, i.e. innermost-first, so an inner binding shadows an outer one of the same
 * name. `undefined` is the empty chain.
 */
interface Cons<T> {
  readonly head: T;
  readonly tail: Cons<T> | undefined;
}

/** Push `head` onto `tail`, returning a new node that shares `tail` (no copy). */
function cons<T>(head: T, tail: Cons<T> | undefined): Cons<T> {
  return { head, tail };
}

/** Whether the name chain holds `name` (by content) — the analogue of `pending.contains`. */
function consContainsName(s: Cons<Uint8Array> | undefined, name: Uint8Array): boolean {
  for (let n = s; n !== undefined; n = n.tail) {
    if (bytesEqual(n.head, name)) return true;
  }
  return false;
}

/** An in-scope `rec` binding: the name (atom bytes) and its body descriptor. */
interface Binding {
  readonly name: Uint8Array;
  readonly body: Value;
}

/**
 * If `t` is `Tuple(Atom("horos/<ctor>"), args...)` with a valid-UTF-8 label under the
 * reserved namespace, return `{ ctor, args }`. Otherwise `undefined` (not a horos
 * descriptor). Total — never throws.
 *
 * The Rust core requires the label to be valid UTF-8 (`str::from_utf8`); we mirror
 * that with a fatal UTF-8 decode. Every ctor we match against is ASCII, so a non-UTF-8
 * label never matches a known ctor and falls through to a miss either way; the explicit
 * validity gate mirrors Rust/Go without changing any decision.
 */
function head(t: Value): Head | undefined {
  if (!(t instanceof Tuple)) return undefined;
  const items = t.items();
  if (items.length === 0) return undefined;
  const labelChild = items[0]!;
  if (!(labelChild instanceof Atom)) return undefined;
  let label: string;
  try {
    label = utf8Decoder.decode(labelChild.bytes());
  } catch {
    return undefined;
  }
  if (!label.startsWith(NAMESPACE)) return undefined;
  return { ctor: label.slice(NAMESPACE.length), args: items.slice(1) };
}

/** Read the bytes of an atom value, or `undefined` if it is a tuple. */
function atomBytes(v: Value): Uint8Array | undefined {
  return v instanceof Atom ? v.bytes() : undefined;
}

// ---------------------------------------------------------------------------
// data-encoded refinements (horos-internal#68) — the four named grammar-class predicates the
// visual-atlas consumer needs (atlas#533 Q4 / visual-atlas#534), each a closed,
// total, deterministic, single-value predicate (spec §5 admissibility). They
// measure Unicode CHARACTERS (code points), not bytes, so the multi-byte cases
// where char-count ≠ byte-count agree with the Rust and Go cores. Implemented as
// explicit, dependency-free code-point scans (no RegExp) so the three cores decide
// byte-for-byte. CHARACTER count means CODE POINTS, not UTF-16 units: `[...s]` /
// `Array.from(s)` iterate the string by code point (a single astral character is
// one unit), matching Rust's `chars()` and Go's runes exactly. The §5 grammar
// literals are ASCII, so a non-ASCII code point never satisfies a grammar class.

/** Whether a code point is an ASCII lowercase letter a–z. */
function isAsciiLower(cp: number): boolean {
  return cp >= 0x61 && cp <= 0x7a;
}

/** Whether a code point is an ASCII digit 0–9. */
function isAsciiDigit(cp: number): boolean {
  return cp >= 0x30 && cp <= 0x39;
}

/** Whether a code point is an ASCII hex digit 0–9 / a–f (lowercase only). */
function isLowerHex(cp: number): boolean {
  return isAsciiDigit(cp) || (cp >= 0x61 && cp <= 0x66);
}

/**
 * The fixed ASCII whitespace set used by {@link isNonBlank} — mirrored byte-for-byte
 * in the Rust (`is_ascii_ws`) and Go (`isASCIIWS`) cores: space, tab, LF, CR, VT, FF.
 * Deliberately NOT Unicode `White_Space` (which varies by runtime and would break
 * tri-core parity).
 */
function isAsciiWs(cp: number): boolean {
  return (
    cp === 0x20 || cp === 0x09 || cp === 0x0a || cp === 0x0d || cp === 0x0b || cp === 0x0c
  );
}

/**
 * `horos/key-grammar`: the decoded text matches `^[a-z0-9][a-z0-9._-]{0,63}$` — a
 * per-code-point class test over the decoded string (1–64 code points total).
 */
function matchesKeyGrammar(s: string): boolean {
  const cps = [...s];
  if (cps.length === 0 || cps.length > 64) return false;
  const first = cps[0]!.codePointAt(0)!;
  if (!(isAsciiLower(first) || isAsciiDigit(first))) return false;
  for (let i = 1; i < cps.length; i += 1) {
    const cp = cps[i]!.codePointAt(0)!;
    if (!(
      isAsciiLower(cp) ||
      isAsciiDigit(cp) ||
      cp === 0x2e ||
      cp === 0x5f ||
      cp === 0x2d
    )) {
      return false; // . _ -
    }
  }
  return true;
}

/**
 * `horos/text-non-blank`: the decoded text has at least one non-whitespace code
 * point. A non-ASCII code point is never whitespace (so it counts as non-blank),
 * mirroring the Rust/Go cores exactly.
 */
function isNonBlank(s: string): boolean {
  for (const ch of s) {
    if (!isAsciiWs(ch.codePointAt(0)!)) return true;
  }
  return false;
}

/**
 * `horos/lowercase-uuid`: the decoded text matches the lowercase canonical UUID
 * grammar `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$` — scanned
 * by code point so a multi-byte char in a malformed input is rejected cleanly.
 */
function matchesLowercaseUuid(s: string): boolean {
  const groups = [8, 4, 4, 4, 12];
  const cps = [...s];
  let pos = 0;
  for (let gi = 0; gi < groups.length; gi += 1) {
    for (let i = 0; i < groups[gi]!; i += 1) {
      if (pos >= cps.length || !isLowerHex(cps[pos]!.codePointAt(0)!)) return false;
      pos += 1;
    }
    if (gi < groups.length - 1) {
      if (pos >= cps.length || cps[pos]!.codePointAt(0)! !== 0x2d) return false; // hyphen
      pos += 1;
    }
  }
  return pos === cps.length; // no trailing code points
}

// ===========================================================================
// conformance — v : T
// ===========================================================================

/**
 * Decide `v : T` — "does the value `v` exhibit the form described by `T`?"
 *
 * TOTAL and search-free. A value that does not conform — and a `T` that is not a
 * well-formed descriptor — both yield `false`; this never errors and never throws. A
 * value may conform to many descriptors at once (overlapping classification, not a
 * new identity).
 */
export function conforms(v: Value, t: Value): boolean {
  // Soundness gate (spec §3 MUST): `v : T` presupposes `wf(T)`. A descriptor that is
  // not well-formed admits no values, even where an individual arm would otherwise
  // match — so check `wf` once at the entry and never arm-by-arm below.
  return wf(t) && conformsIn(v, t, undefined, 0);
}

/**
 * `conforms` with the recursion environment threaded through: `env` binds each `rec` name
 * that lexically encloses `t` to its body descriptor; each node's tail is the environment
 * its `rec` was written in, so the node is the binding's closure. Callers check `wf(t)`
 * first; `conformsIn` relies on its guardedness (see the module comment).
 */
function conformsIn(
  v: Value,
  t: Value,
  env: Cons<Binding> | undefined,
  depth: number,
): boolean {
  // Recursion-depth guard (horos-internal#37): past MAX_RECURSION_DEPTH frames, return a deterministic
  // `false` rather than risk a native-stack overflow (RangeError). Counted at the same
  // point in all three cores so the trip depth is byte-identical.
  if (depth > MAX_RECURSION_DEPTH) return false;
  const h = head(t);
  if (h === undefined) return false; // not a horos descriptor
  const { ctor, args } = h;
  switch (ctor) {
    // Top.
    case "any":
      return args.length === 0;

    // Atoms.
    case "atom":
      return args.length === 0 && v instanceof Atom;
    case "atom-exact": {
      if (args.length !== 1) return false;
      const vb = atomBytes(v);
      const bb = atomBytes(args[0]!);
      if (vb === undefined || bb === undefined) return false;
      return bytesEqual(vb, bb);
    }
    case "atom-len": {
      if (args.length !== 2) return false;
      const vb = atomBytes(v);
      if (vb === undefined) return false;
      // Bounds use readInt (materialize). Our readInt returns an unbounded `bigint`,
      // whereas the Rust reference's read_int materializes into an `i128` and errors
      // (→ false) for any bound past that width. We close that divergence (horos-internal#18) by
      // applying the SAME i128 cap explicitly via atomLenBoundInRange: a bound outside
      // [ATOM_LEN_BOUND_MIN, ATOM_LEN_BOUND_MAX] is rejected here, exactly as Rust's
      // read_int returns IntOutOfRange. So the cores agree at the cap (a bound = 2^127 is
      // non-conforming everywhere) and below it for every real byte-length bound.
      let lo: bigint;
      let hi: bigint;
      try {
        lo = readInt(args[0]!);
        hi = readInt(args[1]!);
      } catch {
        return false;
      }
      if (!atomLenBoundInRange(lo) || !atomLenBoundInRange(hi)) return false;
      const len = BigInt(vb.length);
      return lo <= len && len <= hi;
    }
    case "atom-in": {
      if (args.length !== 1) return false;
      const vb = atomBytes(v);
      if (vb === undefined) return false;
      let elems: Value[];
      try {
        elems = readSet(args[0]!);
      } catch {
        return false;
      }
      for (const e of elems) {
        const eb = atomBytes(e);
        if (eb !== undefined && bytesEqual(eb, vb)) return true;
      }
      return false;
    }

    // Text refinements (horos-internal#68) — each first recognizes v as a canonical utf8-text
    // (delegating to the frozen ontos/data recognizer, re-minting nothing) and then
    // applies a closed, total, single-value character predicate. Char-count (code
    // points, not bytes) is measured, so multi-byte text is bounded/matched
    // identically across the three cores.
    case "text-len": {
      if (args.length !== 2) return false;
      let s: string;
      let lo: bigint;
      let hi: bigint;
      try {
        s = readText(v);
        lo = readInt(args[0]!);
        hi = readInt(args[1]!);
      } catch {
        return false;
      }
      if (!atomLenBoundInRange(lo) || !atomLenBoundInRange(hi)) return false;
      const n = BigInt([...s].length); // code points, not UTF-16 units
      return lo <= n && n <= hi;
    }
    case "key-grammar": {
      if (args.length !== 0) return false;
      let s: string;
      try {
        s = readText(v);
      } catch {
        return false;
      }
      return matchesKeyGrammar(s);
    }
    case "text-non-blank": {
      if (args.length !== 0) return false;
      let s: string;
      try {
        s = readText(v);
      } catch {
        return false;
      }
      return isNonBlank(s);
    }
    case "lowercase-uuid": {
      if (args.length !== 0) return false;
      let s: string;
      try {
        s = readText(v);
      } catch {
        return false;
      }
      return matchesLowercaseUuid(s);
    }

    // Scalars — delegate to the frozen ontos/data recognizers.
    // Integer bounds (ADR 0010): a canonical int of any width, compared exactly with an
    // inclusive bound that is itself a canonical int in the i128 range.
    case "int-min":
    case "int-max": {
      if (args.length !== 1) return false;
      let bound: bigint;
      let n: bigint;
      try {
        bound = readInt(args[0]!);
        n = readInt(v);
      } catch {
        return false;
      }
      if (!atomLenBoundInRange(bound)) return false;
      return ctor === "int-min" ? n >= bound : n <= bound;
    }
    case "scalar": {
      if (args.length !== 1) return false;
      const k = atomBytes(args[0]!);
      if (k === undefined) return false;
      const kind = bytesToAscii(k);
      switch (kind) {
        // Structural recognition — a canonical value beyond a binding's host integer
        // width is still recognized (the materialization limit is not a recognition
        // miss, ontos-data §5.1), so all cores agree. horos re-mints nothing.
        case "int":
          return ok(() => recognizeInt(v));
        case "utf8-text":
          return ok(() => readText(v));
        case "bool":
          return ok(() => readBool(v));
        default:
          return false;
      }
    }

    // Tuples & compounds. Each child descriptor types a child VALUE — a descent.
    case "tuple": {
      if (!(v instanceof Tuple)) return false;
      const items = v.items();
      if (items.length !== args.length) return false;
      for (let i = 0; i < items.length; i += 1) {
        if (!conformsIn(items[i]!, args[i]!, env, depth + 1)) return false;
      }
      return true;
    }
    case "seq": {
      if (args.length !== 1) return false;
      if (!(v instanceof Tuple)) return false;
      for (const c of v.items()) {
        if (!conformsIn(c, args[0]!, env, depth + 1)) return false;
      }
      return true;
    }
    case "compound": {
      if (args.length < 1) return false;
      const label = args[0]!;
      const ts = args.slice(1);
      if (!(v instanceof Tuple)) return false;
      const lb = atomBytes(label);
      if (lb === undefined) return false;
      const items = v.items();
      if (items.length !== ts.length + 1) return false;
      const head0 = atomBytes(items[0]!);
      if (head0 === undefined || !bytesEqual(head0, lb)) return false;
      for (let i = 0; i < ts.length; i += 1) {
        if (!conformsIn(items[i + 1]!, ts[i]!, env, depth + 1)) return false;
      }
      return true;
    }

    // Structural collections — well-formedness delegates to ontos/data; the element
    // descriptors add the per-element checks (descents).
    case "list-of": {
      if (args.length !== 1) return false;
      let elems: Value[];
      try {
        elems = readList(v);
      } catch {
        return false;
      }
      for (const e of elems) {
        if (!conformsIn(e, args[0]!, env, depth + 1)) return false;
      }
      return true;
    }
    case "set-of": {
      if (args.length !== 1) return false;
      let elems: Value[];
      try {
        elems = readSet(v);
      } catch {
        return false;
      }
      for (const e of elems) {
        if (!conformsIn(e, args[0]!, env, depth + 1)) return false;
      }
      return true;
    }
    case "map-of": {
      if (args.length !== 2) return false;
      let entries: Array<[Value, Value]>;
      try {
        entries = readMap(v);
      } catch {
        return false;
      }
      for (const [k, val] of entries) {
        if (
          !conformsIn(k, args[0]!, env, depth + 1) ||
          !conformsIn(val, args[1]!, env, depth + 1)
        ) {
          return false;
        }
      }
      return true;
    }

    // Records — a well-formed map keyed by utf8-text, with required/optional fields
    // and a mandatory open/closed flag.
    case "record": {
      if (args.length < 1) return false;
      return conformsRecord(v, args[0]!, args.slice(1), env, depth);
    }

    // Composition — recurse on the SAME v.
    case "union": {
      for (const ct of args) {
        if (conformsIn(v, ct, env, depth + 1)) return true;
      }
      return false;
    }
    case "and": {
      for (const ct of args) {
        if (!conformsIn(v, ct, env, depth + 1)) return false;
      }
      return true;
    }
    case "not": {
      if (args.length !== 1) return false;
      return !conformsIn(v, args[0]!, env, depth + 1);
    }

    // Singleton — membership by ontos structural identity (defines no new equality).
    case "exact": {
      if (args.length !== 1) return false;
      return exactEqual(v, args[0]!);
    }

    // Recursion. rec binds the name; ref unfolds the binding that lexically encloses it.
    // Each unfolding is one counted frame, so the depth bound is the backstop (ADR 0011).
    case "rec": {
      if (args.length !== 2) return false;
      const nm = args[0]!;
      if (!(nm instanceof Atom)) return false;
      const nb = nm.bytes();
      const body = args[1]!;
      // Push one shared node (no per-frame spread copy); the child frame sees this
      // binding innermost-first. The node closes over `env`.
      return conformsIn(v, body, cons<Binding>({ name: nb, body }, env), depth + 1);
    }
    case "ref": {
      if (args.length !== 1) return false;
      const nm = args[0]!;
      if (!(nm instanceof Atom)) return false;
      // The nearest enclosing binding in the written descriptor: `env` holds exactly
      // the bindings that lexically enclose this `ref` (ADR 0011).
      const node = lookupBinding(env, nm.bytes());
      if (node === undefined) return false; // unbound reference
      // Unfold in the binding's own environment, so the body's free refs keep the
      // bindings they were written under.
      return conformsIn(v, node.head.body, node, depth + 1);
    }

    // Unknown ctor or wrong arity → not a recognized descriptor.
    default:
      return false;
  }
}

/** A parsed field descriptor: its name, requiredness, and field type. */
interface Field {
  readonly name: string;
  readonly required: boolean;
  readonly ty: Value;
}

/**
 * A field descriptor: `Tuple(Atom("horos/field"), utf8-text(name), Atom(req|opt), T)`.
 * Returns `{ name, required, ty }` if `f` is a well-formed field, else `undefined`.
 */
function readField(f: Value): Field | undefined {
  const h = head(f);
  if (h === undefined || h.ctor !== "field") return undefined;
  const { args } = h;
  if (args.length !== 3) return undefined;
  let name: string;
  try {
    name = readText(args[0]!);
  } catch {
    return undefined;
  }
  const presence = atomBytes(args[1]!);
  if (presence === undefined) return undefined;
  const p = bytesToAscii(presence);
  let required: boolean;
  if (p === "req") required = true;
  else if (p === "opt") required = false;
  else return undefined;
  return { name, required, ty: args[2]! };
}

function conformsRecord(
  v: Value,
  mode: Value,
  fields: readonly Value[],
  env: Cons<Binding> | undefined,
  depth: number,
): boolean {
  // mode is a bare atom flag (open | closed).
  const mb = atomBytes(mode);
  if (mb === undefined) return false;
  let open: boolean;
  const m = bytesToAscii(mb);
  if (m === "open") open = true;
  else if (m === "closed") open = false;
  else return false;
  // v must be a well-formed ontos/data map.
  let entries: Array<[Value, Value]>;
  try {
    entries = readMap(v);
  } catch {
    return false;
  }
  // Index the map's utf8-text keys to their values in a single pass — a map-backed
  // lookup replacing the old per-field linear `present.find`. `readMap` already
  // guarantees keys are sorted and dup-free by codec bytes; distinct keys therefore
  // decode to distinct text strings, so no `set` ever collides and the map preserves
  // the Rust core's "first match in map order" exactly. A non-text key makes v not a
  // record-shaped value (same early miss as the linear scan).
  const present = new Map<string, Value>();
  for (const [k, val] of entries) {
    let key: string;
    try {
      key = readText(k);
    } catch {
      return false;
    }
    present.set(key, val);
  }
  // Single pass over the declared fields drives BOTH checks (no second readField
  // sweep): per field, look the name up in `present` (O(1)) for the required/optional
  // verdict, and — in closed mode — count how many present keys a declared field
  // covers. Field names are pairwise-distinct (`wf(record)`, checked at entry), so a
  // present declared key is counted at most once.
  let covered = 0;
  for (const f of fields) {
    const fld = readField(f);
    if (fld === undefined) return false; // malformed field descriptor
    const val = present.get(fld.name);
    if (val !== undefined) {
      covered += 1;
      if (!conformsIn(val, fld.ty, env, depth + 1)) return false;
    } else if (fld.required) {
      return false;
    }
  }
  // Closed: no present key outside the declared fields. With distinct field names,
  // `covered` is exactly the number of present keys that match some declared field; a
  // present key outside the declared set exists iff `covered < present.size`. This
  // replaces the Rust core's second readField pass + O(fields×keys) sweep.
  if (!open && covered !== present.size) return false;
  return true;
}

// ===========================================================================
// ===========================================================================

/**
 * Whether `ctor` is a LEAF descriptor (a §4 leaf / §5 refinement / `exact`) — i.e. NOT a
 */
export function isLeafCtor(ctor: string): boolean {
  switch (ctor) {
    case "any":
    case "atom":
    case "atom-exact":
    case "atom-len":
    case "atom-in":
    case "int-min":
    case "int-max":
    case "text-len":
    case "key-grammar":
    case "text-non-blank":
    case "lowercase-uuid":
    case "scalar":
    case "exact":
      return true;
    default:
      return false;
  }
}

/**
 * Decide a LEAF descriptor `t` against `v` (the `horos/leaf(T, v)` builtin). `false` for any
 * non-leaf ctor. Reuses the recognizer's leaf arms via `conformsIn` at depth 0 with empty
 * scopes — a leaf arm never recurses into a value position, so the scopes are inert.
 */
export function leafClassifies(v: Value, t: Value): boolean {
  const h = head(t);
  if (h === undefined || !isLeafCtor(h.ctor)) return false;
  return conformsIn(v, t, undefined, 0);
}

/** Decide whether a LEAF descriptor `t` is well-formed (the `horos/wf-leaf(T)` builtin). */
export function leafWellFormed(t: Value): boolean {
  const h = head(t);
  if (h === undefined || !isLeafCtor(h.ctor)) return false;
  return wfIn(t, undefined, undefined, 0);
}

/** Read a field descriptor for diagnostics. */
export function readFieldDescriptor(
  f: Value,
): { name: string; required: boolean; ty: Value } | undefined {
  return readField(f);
}

// ===========================================================================
// well-formedness — wf(T)
// ===========================================================================

/**
 * Decide `wf(T)` — "is `T` a well-formed horos descriptor?" Total and search-free;
 * recurses into sub-descriptors (which are finite). A descriptor that fails `wf` is a
 * valid ontos value, simply not a recognized descriptor; `conforms(_, T)` over such a
 * `T` is `false`.
 *
 * For rec/ref: a `ref` must be BOUND by an enclosing `rec` and GUARDED — reachable
 * only after a descent into the value. An unbound or unguarded `ref` (e.g.
 * `rec(x, ref(x))` or `rec(x, union(ref(x), …))`) is not well-formed.
 */
export function wf(t: Value): boolean {
  return wfIn(t, undefined, undefined, 0);
}

function wfIn(
  t: Value,
  bound: Cons<Uint8Array> | undefined,
  pending: Cons<Uint8Array> | undefined,
  depth: number,
): boolean {
  // Recursion-depth guard (horos-internal#37): mirrors conformsIn — past MAX_RECURSION_DEPTH frames a
  // descriptor is treated as non-wf, deterministically and at the same depth in all three
  // cores, so wf can never overflow the stack (RangeError) on a deep-but-finite descriptor.
  if (depth > MAX_RECURSION_DEPTH) return false;
  const h = head(t);
  if (h === undefined) return false;
  const { ctor, args } = h;
  switch (ctor) {
    case "any":
    case "atom":
      return args.length === 0;
    case "atom-exact":
      return args.length === 1 && args[0]! instanceof Atom;
    case "atom-len": {
      if (args.length !== 2) return false;
      let lo: bigint;
      let hi: bigint;
      try {
        lo = readInt(args[0]!);
        hi = readInt(args[1]!);
      } catch {
        return false;
      }
      // horos-internal#18 cap: a bound past i128 is non-wf, mirroring Rust's read_int (which returns
      // IntOutOfRange). Then 0 <= lo <= hi.
      if (!atomLenBoundInRange(lo) || !atomLenBoundInRange(hi)) return false;
      return lo >= 0n && lo <= hi;
    }
    case "atom-in": {
      if (args.length !== 1) return false;
      let elems: Value[];
      try {
        elems = readSet(args[0]!);
      } catch {
        return false;
      }
      // Every element must be an atom (an atom-membership set).
      return elems.every((e) => e instanceof Atom);
    }
    // Text refinements (horos-internal#68). text-len carries char-count bounds with the same
    // 0 <= lo <= hi + i128-cap discipline as atom-len (horos-internal#18); the three grammar-class
    // refinements are nullary named predicates.
    case "text-len": {
      if (args.length !== 2) return false;
      let lo: bigint;
      let hi: bigint;
      try {
        lo = readInt(args[0]!);
        hi = readInt(args[1]!);
      } catch {
        return false;
      }
      if (!atomLenBoundInRange(lo) || !atomLenBoundInRange(hi)) return false;
      return lo >= 0n && lo <= hi;
    }
    case "key-grammar":
    case "text-non-blank":
    case "lowercase-uuid":
      return args.length === 0;
    case "int-min":
    case "int-max": {
      if (args.length !== 1) return false;
      try {
        return atomLenBoundInRange(readInt(args[0]!));
      } catch {
        return false;
      }
    }
    case "scalar": {
      if (args.length !== 1) return false;
      const k = atomBytes(args[0]!);
      if (k === undefined) return false;
      const kind = bytesToAscii(k);
      return kind === "int" || kind === "utf8-text" || kind === "bool";
    }
    // Descent combinators clear pending (a value constructor is consumed).
    case "tuple":
      return args.every((ct) => wfIn(ct, bound, undefined, depth + 1));
    case "seq":
      return args.length === 1 && wfIn(args[0]!, bound, undefined, depth + 1);
    case "compound": {
      if (args.length < 1) return false;
      if (!(args[0]! instanceof Atom)) return false;
      return args.slice(1).every((ct) => wfIn(ct, bound, undefined, depth + 1));
    }
    case "list-of":
    case "set-of":
      return args.length === 1 && wfIn(args[0]!, bound, undefined, depth + 1);
    case "map-of":
      return (
        args.length === 2 &&
        wfIn(args[0]!, bound, undefined, depth + 1) &&
        wfIn(args[1]!, bound, undefined, depth + 1)
      );
    case "record": {
      if (args.length < 1) return false;
      return wfRecord(args[0]!, args.slice(1), bound, depth);
    }
    // Same-v combinators thread the guard through.
    case "union":
    case "and":
      return args.every((ct) => wfIn(ct, bound, pending, depth + 1));
    case "not":
      return args.length === 1 && wfIn(args[0]!, bound, pending, depth + 1);
    case "exact":
      // Any single ontos value is a valid singleton.
      return args.length === 1;
    case "rec": {
      if (args.length !== 2) return false;
      const nm = args[0]!;
      if (!(nm instanceof Atom)) return false;
      const nb = nm.bytes();
      // Shared push onto each chain (no per-frame spread copy).
      const b = cons(nb, bound);
      const p = cons(nb, pending);
      return wfIn(args[1]!, b, p, depth + 1);
    }
    case "ref": {
      if (args.length !== 1) return false;
      const nm = args[0]!;
      if (!(nm instanceof Atom)) return false;
      const nb = nm.bytes();
      return consContainsName(bound, nb) && !consContainsName(pending, nb);
    }
    default:
      return false;
  }
}

function wfRecord(
  mode: Value,
  fields: readonly Value[],
  bound: Cons<Uint8Array> | undefined,
  depth: number,
): boolean {
  const mb = atomBytes(mode);
  if (mb === undefined) return false;
  const m = bytesToAscii(mb);
  if (m !== "open" && m !== "closed") return false;
  const names: string[] = [];
  for (const f of fields) {
    const fld = readField(f);
    if (fld === undefined) return false;
    if (!wfIn(fld.ty, bound, undefined, depth + 1)) return false;
    if (names.includes(fld.name)) return false; // duplicate field name
    names.push(fld.name);
  }
  return true;
}

// ===========================================================================
// internal helpers
// ===========================================================================

// Literals under `exact` are data, not descriptor recursion. Compare their finite
// trees without consuming the JavaScript call stack or changing structural identity.
function exactEqual(left: Value, right: Value): boolean {
  const work: [Value, Value][] = [[left, right]];
  while (work.length > 0) {
    const [a, b] = work.pop()!;
    if (a.kind !== b.kind) return false;
    if (a.kind === "atom") {
      if (b.kind !== "atom" || !bytesEqual(a.bytes(), b.bytes())) return false;
    } else {
      if (b.kind !== "tuple") return false;
      const aa = a.items(),
        bb = b.items();
      if (aa.length !== bb.length) return false;
      for (let i = 0; i < aa.length; i++) work.push([aa[i]!, bb[i]!]);
    }
  }
  return true;
}

/**
 * Find the binding of `name`, searching from the innermost (head) binding outward, so an
 * inner `rec` shadows an outer binding of the same name. Returns the environment node,
 * which is the binding's closure, or `undefined` if unbound.
 */
function lookupBinding(
  env: Cons<Binding> | undefined,
  name: Uint8Array,
): Cons<Binding> | undefined {
  for (let n = env; n !== undefined; n = n.tail) {
    if (bytesEqual(n.head.name, name)) return n;
  }
  return undefined;
}

/**
 * Decode bytes as a Latin-1/ASCII string for matching against the small set of fixed
 * ASCII ctor/kind/flag literals. The Rust core matches raw byte slices (e.g. `b"int"`);
 * decoding each byte to its code point reproduces that comparison exactly for ASCII and
 * round-trips any byte 1:1 (no UTF-8 multi-byte folding), so a non-ASCII kind atom never
 * spuriously matches a literal.
 */
function bytesToAscii(bytes: Uint8Array): string {
  let s = "";
  for (const b of bytes) s += String.fromCharCode(b);
  return s;
}

/** Run `fn` and report whether it returned without throwing (the recognizer succeeded). */
function ok(fn: () => unknown): boolean {
  try {
    fn();
    return true;
  } catch {
    return false;
  }
}
