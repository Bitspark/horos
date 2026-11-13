// Package horos is the form contract (Go core): a byte-for-byte port of the Rust
// reference core. It implements the v1 judgment surface — wf(T) and v : T
// (Conforms) — over the horos/* descriptor DSL, as total, search-free structural
// recursion over the finite trees of the value v and the descriptor T (both
// ordinary ontos core.Values). Scalar and collection recognition delegate to the
// frozen ontos/data recognizers; horos re-mints nothing.
//
// See docs/spec/horos.md (the frozen v1 surface) and
// docs/architecture/decisions/0002-form-reasoning-cut.md.
//
// Both WF and Conforms are TOTAL (they never panic), SEARCH-FREE, and
// byte-for-byte equivalent to the Rust reference (core/rs/src/lib.rs); the shared
// vectors/conformance.json oracle pins the three cores (go/rs/ts) together.
//
// # rec/ref guarded recursion
//
// rec binds a recursion name to its body descriptor; ref unfolds it. A ref denotes
// the rec that lexically encloses it in the written descriptor (ADR 0011): each rec
// binding closes over the environment it was written in, and a ref continues in its
// binding's environment, never the unfolding site's. WF requires a descent into a
// strict substructure of v between every rec and each ref that denotes it, so between
// two descents each unfolding reaches a rec strictly above every descriptor node
// checked since the last descent: a ref never unfolds a binding twice at one v-node.
// Termination: unfoldings are bounded by |v| times the descriptor's nesting, and
// every frame counts against MaxRecursionDepth.
package horos

import (
	"math/big"
	"unicode/utf8"

	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

// Value is the ontos core value type that horos classifies (descriptors) and
// checks (values). It is re-exported so callers depend only on this package.
type Value = core.Value

// namespace is the reserved descriptor-label prefix. Every descriptor combinator
// is a Tuple(Atom("horos/<ctor>"), args...); a tuple whose first child is not an
// atom under this prefix is not a horos descriptor (per ontos-data §1.1
// reservation).
const namespace = "horos/"

// MaxRecursionDepth is the shared recursion-depth guard (horos-internal#37) — the SAME constant in
// all three cores (Rust MAX_RECURSION_DEPTH, TS MAX_RECURSION_DEPTH). conformsIn/wfIn
// recurse on the native stack; termination is bounded by |v| (WF's guardedness) but
// there is no stack bound, so an adversarial-but-finite value (e.g. a tuple nested
// hundreds of thousands deep) could overflow the stack — a fatal "goroutine stack
// exceeds" — at a core-dependent depth, breaking the total/never-panics contract.
//
// Past MaxRecursionDepth frames, conformsIn/wfIn return a DETERMINISTIC result (false /
// non-wf), never a crash, at the same logical depth as Rust and TS (each increments a
// depth counter at identical recursion points). Termination is thus bounded by
// min(|v|, MaxRecursionDepth); the cap is a robustness guardrail, not a change to the
// recognized language for in-bound inputs. 512 is far above any descriptor/value the
// spec, tests, or vectors exercise (< ~30, so no judgment changes) and comfortably below
// the smallest native stack budget of the three cores (validated empirically per core).
const MaxRecursionDepth = 512

// head returns (ctor, args) if t is Tuple(Atom("horos/<ctor>"), args...) with a
// valid-UTF-8 label under the reserved namespace; ok is false otherwise (not a
// horos descriptor). Total — never panics.
//
// Atom bytes are valid UTF-8 trivially for the label check: Go strings are byte
// sequences, so we compare the namespace prefix on bytes; the ctor is the
// remaining bytes as a string. The Rust core requires the label to be valid UTF-8
// (str::from_utf8). Every ctor we match against is ASCII, so a non-UTF-8 label
// never matches a known ctor and falls through to false either way; the explicit
// validity gate mirrors Rust without changing any decision.
func head(t Value) (ctor string, args []Value, ok bool) {
	tup, isTup := t.(core.Tuple)
	if !isTup {
		return "", nil, false
	}
	items := tup.Items()
	if len(items) == 0 {
		return "", nil, false
	}
	label, isAtom := items[0].(core.Atom)
	if !isAtom {
		return "", nil, false
	}
	lb := label.Bytes()
	if !utf8Valid(lb) {
		return "", nil, false
	}
	s := string(lb)
	if len(s) < len(namespace) || s[:len(namespace)] != namespace {
		return "", nil, false
	}
	return s[len(namespace):], items[1:], true
}

// atomBytes returns the bytes of an atom value, or (nil, false) if it is a tuple.
func atomBytes(v Value) ([]byte, bool) {
	a, ok := v.(core.Atom)
	if !ok {
		return nil, false
	}
	return a.Bytes(), true
}

// ===========================================================================
// conformance — v : T
// ===========================================================================

// Conforms decides v : T — "does the value v exhibit the form described by T?"
//
// TOTAL and search-free. A value that does not conform — and a T that is not a
// well-formed descriptor — both yield false; this never errors and never panics. A
// value may conform to many descriptors at once (overlapping classification, not a
// new identity).
func Conforms(v, t Value) bool {
	// Soundness gate (spec §3 MUST): v : T presupposes wf(T). A descriptor that is
	// not well-formed admits no values, even where an individual arm would otherwise
	// match — so check WF once at the entry and never arm-by-arm below.
	return WF(t) && newChecker().conformsIn(v, -1, t, -1, -1, nil, 0)
}

// conformsIn is Conforms with the recursion environment threaded through: env binds
// each rec name that lexically encloses t to its body descriptor (an envScope cons list
// whose nodes are closures). Callers check WF(t) first; conformsIn relies on its
// guardedness (see the package comment).
//
// In shared mode vid names v's occurrence in the checked value, and t's position in the
// written descriptor is item index of the position parent (parent itself when index is
// -1); outside it vid and parent are -1 (checker). The position is numbered only for a
// shared composite sub-check, whose result is a function of (vid, position, depth) alone,
// so it is computed once per check (ADR 0013); a leaf is never numbered.
func (c *checker) conformsIn(v Value, vid int, t Value, parent, index int, env *envScope, depth int) bool {
	// Recursion-depth guard (horos-internal#37): past MaxRecursionDepth frames, return a deterministic
	// false rather than risk a native-stack overflow. Counted at the same point in all
	// three cores so the trip depth is byte-identical.
	if depth > MaxRecursionDepth {
		return false
	}
	ctor, args, ok := head(t)
	if !ok {
		return false // not a horos descriptor
	}
	if vid < 0 || isLeafCtor(ctor) {
		return c.node(v, vid, ctor, args, -1, env, depth)
	}
	at := c.pos(parent, index)
	key := memoKey{value: vid, position: at, depth: depth}
	if r, hit := c.memo[key]; hit {
		return r
	}
	r := c.node(v, vid, ctor, args, at, env, depth)
	c.memo[key] = r
	return r
}

// share enters shared mode at a union or intersection of two or more alternatives over
// the same value: below it one sub-check can be reached along several routes (checker).
// It returns the numbers of v and of the node, fresh ones when shared mode begins here.
func (c *checker) share(vid, at, alternatives int) (int, int) {
	if vid >= 0 || alternatives < 2 {
		return vid, at
	}
	return c.fresh(), c.fresh()
}

// node decides one descriptor node against v. Leaf arms never use c, so a nil checker
// serves leafClassifies.
func (c *checker) node(v Value, vid int, ctor string, args []Value, at int, env *envScope, depth int) bool {
	switch ctor {
	// Top.
	case "any":
		return len(args) == 0

	// Atoms.
	case "atom":
		if len(args) != 0 {
			return false
		}
		_, isAtom := v.(core.Atom)
		return isAtom
	case "atom-exact":
		if len(args) != 1 {
			return false
		}
		vb, okv := atomBytes(v)
		bb, okb := atomBytes(args[0])
		if !okv || !okb {
			return false
		}
		return bytesEqual(vb, bb)
	case "atom-len":
		if len(args) != 2 {
			return false
		}
		// Bounds use ReadInt (materialize), matching the Rust reference's read_int.
		// Go's ReadInt is unbounded (*big.Int); Rust's is i128 and errors past host
		// width. We close that gap (horos-internal#18) by applying the SAME i128 cap explicitly:
		// a bound outside [atomLenBoundMin, atomLenBoundMax] is rejected here, exactly
		// as Rust's read_int returns IntOutOfRange (→ false). So the two agree at the
		// cap — a bound = 2^127 is non-conforming in all three cores — and below it for
		// every real byte-length bound (len(vb) is a small host int).
		vb, okv := atomBytes(v)
		lo, errLo := data.ReadInt(args[0])
		hi, errHi := data.ReadInt(args[1])
		if !okv || errLo != nil || errHi != nil {
			return false
		}
		if !atomLenBoundInRange(lo) || !atomLenBoundInRange(hi) {
			return false // bound past i128 — non-wf descriptor, mirroring Rust read_int
		}
		n := big.NewInt(int64(len(vb)))
		return lo.Cmp(n) <= 0 && n.Cmp(hi) <= 0
	case "atom-in":
		if len(args) != 1 {
			return false
		}
		vb, okv := atomBytes(v)
		elems, err := data.ReadSet(args[0])
		if !okv || err != nil {
			return false
		}
		for _, e := range elems {
			if eb, oke := atomBytes(e); oke && bytesEqual(eb, vb) {
				return true
			}
		}
		return false

	// Text refinements (horos-internal#68) — each first recognizes v as a canonical utf8-text
	// (delegating to the frozen ontos/data recognizer, re-minting nothing) and then
	// applies a closed, total, single-value character predicate. Char-count (runes,
	// not bytes) is measured, so multi-byte text is bounded/matched identically
	// across the three cores.
	case "text-len":
		if len(args) != 2 {
			return false
		}
		s, errT := data.ReadText(v)
		lo, errLo := data.ReadInt(args[0])
		hi, errHi := data.ReadInt(args[1])
		if errT != nil || errLo != nil || errHi != nil {
			return false
		}
		if !atomLenBoundInRange(lo) || !atomLenBoundInRange(hi) {
			return false
		}
		n := big.NewInt(int64(utf8.RuneCountInString(s)))
		return lo.Cmp(n) <= 0 && n.Cmp(hi) <= 0
	case "key-grammar":
		if len(args) != 0 {
			return false
		}
		s, err := data.ReadText(v)
		return err == nil && matchesKeyGrammar(s)
	case "text-non-blank":
		if len(args) != 0 {
			return false
		}
		s, err := data.ReadText(v)
		return err == nil && isNonBlank(s)
	case "lowercase-uuid":
		if len(args) != 0 {
			return false
		}
		s, err := data.ReadText(v)
		return err == nil && matchesLowercaseUUID(s)

	// Scalars — delegate to the frozen ontos/data recognizers.
	// Integer bounds (ADR 0010): a canonical int of any width, compared exactly with an
	// inclusive bound that is itself a canonical int in the i128 range.
	case "int-min", "int-max":
		if len(args) != 1 {
			return false
		}
		bound, errB := data.ReadInt(args[0])
		n, errV := data.ReadInt(v)
		if errB != nil || errV != nil || !atomLenBoundInRange(bound) {
			return false
		}
		if ctor == "int-min" {
			return n.Cmp(bound) >= 0
		}
		return n.Cmp(bound) <= 0
	case "scalar":
		if len(args) != 1 {
			return false
		}
		k, okk := atomBytes(args[0])
		if !okk {
			return false
		}
		switch string(k) {
		// Structural recognition — a canonical value beyond a binding's host
		// integer width is still recognized (the materialization limit is not a
		// recognition miss, ontos-data §5.1), so all cores agree. horos re-mints
		// nothing.
		case "int":
			return data.RecognizeInt(v) == nil
		case "utf8-text":
			_, err := data.ReadText(v)
			return err == nil
		case "bool":
			_, err := data.ReadBool(v)
			return err == nil
		default:
			return false
		}

	// Tuples & compounds. Each child descriptor types a child VALUE — a descent.
	case "tuple":
		tup, isTup := v.(core.Tuple)
		if !isTup {
			return false
		}
		items := tup.Items()
		if len(items) != len(args) {
			return false
		}
		for i := range items {
			if !c.conformsIn(items[i], c.value(vid, i), args[i], at, i+1, env, depth+1) {
				return false
			}
		}
		return true
	case "seq":
		if len(args) != 1 {
			return false
		}
		tup, isTup := v.(core.Tuple)
		if !isTup {
			return false
		}
		for i, item := range tup.Items() {
			if !c.conformsIn(item, c.value(vid, i), args[0], at, 1, env, depth+1) {
				return false
			}
		}
		return true
	case "compound":
		if len(args) < 1 {
			return false
		}
		label := args[0]
		ts := args[1:]
		tup, isTup := v.(core.Tuple)
		lb, oklb := atomBytes(label)
		if !isTup || !oklb {
			return false
		}
		items := tup.Items()
		if len(items) != len(ts)+1 {
			return false
		}
		head0, okHead := atomBytes(items[0])
		if !okHead || !bytesEqual(head0, lb) {
			return false
		}
		for i := range ts {
			if !c.conformsIn(items[i+1], c.value(vid, i+1), ts[i], at, i+2, env, depth+1) {
				return false
			}
		}
		return true

	// Structural collections — well-formedness delegates to ontos/data; the
	// element descriptors add the per-element checks (descents).
	case "list-of":
		if len(args) != 1 {
			return false
		}
		elems, err := data.ReadList(v)
		if err != nil {
			return false
		}
		for i, e := range elems {
			if !c.conformsIn(e, c.value(vid, i+1), args[0], at, 1, env, depth+1) {
				return false
			}
		}
		return true
	case "set-of":
		if len(args) != 1 {
			return false
		}
		elems, err := data.ReadSet(v)
		if err != nil {
			return false
		}
		for i, e := range elems {
			if !c.conformsIn(e, c.value(vid, i+1), args[0], at, 1, env, depth+1) {
				return false
			}
		}
		return true
	case "map-of":
		if len(args) != 2 {
			return false
		}
		entries, err := data.ReadMap(v)
		if err != nil {
			return false
		}
		for j, kv := range entries {
			// Entry j is the map's item j+1, a (key, value) tuple.
			entry := c.value(vid, j+1)
			if !c.conformsIn(kv[0], c.value(entry, 0), args[0], at, 1, env, depth+1) ||
				!c.conformsIn(kv[1], c.value(entry, 1), args[1], at, 2, env, depth+1) {
				return false
			}
		}
		return true

	// Records — a well-formed map keyed by utf8-text, with required/optional
	// fields and a mandatory open/closed flag.
	case "record":
		if len(args) < 1 {
			return false
		}
		return c.conformsRecord(v, vid, args[0], args[1:], at, env, depth)

	// Composition — recurse on the SAME v.
	case "union":
		vid, at = c.share(vid, at, len(args))
		for i, ct := range args {
			if c.conformsIn(v, vid, ct, at, i+1, env, depth+1) {
				return true
			}
		}
		return false
	case "and":
		vid, at = c.share(vid, at, len(args))
		for i, ct := range args {
			if !c.conformsIn(v, vid, ct, at, i+1, env, depth+1) {
				return false
			}
		}
		return true
	case "not":
		if len(args) != 1 {
			return false
		}
		return !c.conformsIn(v, vid, args[0], at, 1, env, depth+1)

	// Singleton — membership by ontos structural identity (defines no new
	// equality).
	case "exact":
		if len(args) != 1 {
			return false
		}
		return exactEqual(v, args[0])

	// Recursion. rec binds the name; ref unfolds the binding that lexically encloses it.
	// Every unfolding is one frame, so it counts against MaxRecursionDepth: that bound,
	// not a guard here, is the backstop should an unfolding ever recur (ADR 0011).
	case "rec":
		if len(args) != 2 {
			return false
		}
		nm, okNm := args[0].(core.Atom)
		if !okNm {
			return false
		}
		nb := nm.Bytes()
		body := args[1]
		// Push one shared node (no per-frame slice copy); the child frame sees this
		// binding innermost-first. The node closes over env.
		bodyAt := c.pos(at, 2)
		return c.conformsIn(v, vid, body, bodyAt, -1, pushEnv(env, nb, body, bodyAt), depth+1)
	case "ref":
		if len(args) != 1 {
			return false
		}
		nm, okNm := args[0].(core.Atom)
		if !okNm {
			return false
		}
		// The nearest enclosing binding of the name in the written descriptor: env is
		// always the chain of rec bindings that lexically enclose this ref (ADR 0011).
		b := lookupBinding(env, nm.Bytes())
		if b == nil {
			return false // unbound reference
		}
		// Unfold in the binding's own environment, never the unfolding site's, so the
		// body's free refs keep the bindings they were written under. The body is checked
		// at its own position, which fixes its environment; a binding entered before
		// shared mode takes its body's number when a shared ref first needs it.
		if vid >= 0 && b.bodyAt < 0 {
			b.bodyAt = c.fresh()
		}
		bodyAt := b.bodyAt
		if vid < 0 {
			bodyAt = -1
		}
		return c.conformsIn(v, vid, b.body, bodyAt, -1, b, depth+1)

	// Unknown ctor or wrong arity → not a recognized descriptor.
	default:
		return false
	}
}

// readField parses a field descriptor
// Tuple(Atom("horos/field"), utf8-text(name), Atom(req|opt), T) and returns
// (name, required, fieldType, ok). ok is false if f is not a well-formed field.
func readField(f Value) (name string, required bool, ty Value, ok bool) {
	ctor, args, hok := head(f)
	if !hok || ctor != "field" {
		return "", false, nil, false
	}
	if len(args) != 3 {
		return "", false, nil, false
	}
	nameStr, err := data.ReadText(args[0])
	if err != nil {
		return "", false, nil, false
	}
	pb, okp := atomBytes(args[1])
	if !okp {
		return "", false, nil, false
	}
	switch string(pb) {
	case "req":
		required = true
	case "opt":
		required = false
	default:
		return "", false, nil, false
	}
	return nameStr, required, args[2], true
}

func (c *checker) conformsRecord(v Value, vid int, mode Value, fields []Value, at int, env *envScope, depth int) bool {
	// mode is a bare atom flag (open | closed).
	mb, okm := atomBytes(mode)
	if !okm {
		return false
	}
	var open bool
	switch string(mb) {
	case "open":
		open = true
	case "closed":
		open = false
	default:
		return false
	}
	// v must be a well-formed ontos/data map.
	entries, err := data.ReadMap(v)
	if err != nil {
		return false
	}
	// Index the map's utf8-text keys to their values in a single pass — a map-backed
	// lookup replacing the old per-field linear findPresent. ReadMap already guarantees
	// keys are sorted and dup-free by codec bytes; distinct keys therefore decode to
	// distinct text strings, so no assignment ever collides and the map preserves the
	// Rust core's "first match in map order" exactly. A non-text key makes v not a
	// record-shaped value (same early false as the linear scan).
	present := make(map[string]int, len(entries)) // field name -> entry index
	for j, kv := range entries {
		key, kerr := data.ReadText(kv[0])
		if kerr != nil {
			return false
		}
		present[key] = j
	}
	// Single pass over the declared fields drives BOTH checks (no second readField
	// sweep): per field, look the name up in present (O(1)) for the required/optional
	// verdict, and — in closed mode — count how many present keys a declared field
	// covers. Field names are pairwise-distinct (WF(record), checked at entry), so a
	// present declared key is counted at most once.
	covered := 0
	for k, f := range fields {
		name, required, ty, okf := readField(f)
		if !okf {
			return false // malformed field descriptor
		}
		if j, found := present[name]; found {
			covered++
			// Entry j is the map's item j+1; its value is that tuple's item 1. Field k is
			// the record's item k+2, and its type is the field form's item 3.
			value := c.value(c.value(vid, j+1), 1)
			if !c.conformsIn(entries[j][1], value, ty, c.pos(at, k+2), 3, env, depth+1) {
				return false
			}
		} else if required {
			return false
		}
	}
	// Closed: no present key outside the declared fields. With distinct field names,
	// covered is exactly the number of present keys that match some declared field; a
	// present key outside the declared set exists iff covered < len(present). This
	// replaces the Rust core's second readField pass + O(fields×keys) sweep.
	if !open && covered != len(present) {
		return false
	}
	return true
}

// ===========================================================================
// well-formedness — wf(T)
// ===========================================================================

// WF decides wf(T) — "is T a well-formed horos descriptor?" TOTAL and search-free;
// recurses into sub-descriptors (which are finite). A descriptor that fails WF is a
// valid ontos value, simply not a recognized descriptor; Conforms(_, T) over such a
// T is false.
//
// For rec/ref: a ref must be BOUND by an enclosing rec and GUARDED — reachable only
// after a descent into the value. An unbound or unguarded ref (e.g. rec(x, ref(x))
// or rec(x, union(ref(x), …))) is not well-formed.
func WF(t Value) bool {
	return wfIn(t, nil, nil, 0)
}

func wfIn(t Value, bound *nameScope, pending *nameScope, depth int) bool {
	// Recursion-depth guard (horos-internal#37): mirrors conformsIn — past MaxRecursionDepth frames a
	// descriptor is treated as non-wf, deterministically and at the same depth in all
	// three cores, so WF can never overflow the stack on a deep-but-finite descriptor.
	if depth > MaxRecursionDepth {
		return false
	}
	ctor, args, ok := head(t)
	if !ok {
		return false
	}
	switch ctor {
	case "any", "atom":
		return len(args) == 0
	case "atom-exact":
		if len(args) != 1 {
			return false
		}
		_, isAtom := args[0].(core.Atom)
		return isAtom
	case "atom-len":
		if len(args) != 2 {
			return false
		}
		lo, errLo := data.ReadInt(args[0])
		hi, errHi := data.ReadInt(args[1])
		if errLo != nil || errHi != nil {
			return false
		}
		// horos-internal#18 cap: a bound past i128 is non-wf, mirroring Rust's read_int (which
		// returns IntOutOfRange). Then 0 <= lo <= hi.
		if !atomLenBoundInRange(lo) || !atomLenBoundInRange(hi) {
			return false
		}
		return lo.Sign() >= 0 && lo.Cmp(hi) <= 0
	case "atom-in":
		if len(args) != 1 {
			return false
		}
		elems, err := data.ReadSet(args[0])
		if err != nil {
			return false
		}
		// Every element must be an atom (an atom-membership set).
		for _, e := range elems {
			if _, isAtom := e.(core.Atom); !isAtom {
				return false
			}
		}
		return true
	// Text refinements (horos-internal#68). text-len carries char-count bounds with the same
	// 0 <= lo <= hi + i128-cap discipline as atom-len (horos-internal#18); the three grammar-class
	// refinements are nullary named predicates.
	case "text-len":
		if len(args) != 2 {
			return false
		}
		lo, errLo := data.ReadInt(args[0])
		hi, errHi := data.ReadInt(args[1])
		if errLo != nil || errHi != nil {
			return false
		}
		if !atomLenBoundInRange(lo) || !atomLenBoundInRange(hi) {
			return false
		}
		return lo.Sign() >= 0 && lo.Cmp(hi) <= 0
	case "key-grammar", "text-non-blank", "lowercase-uuid":
		return len(args) == 0
	case "int-min", "int-max":
		if len(args) != 1 {
			return false
		}
		bound, err := data.ReadInt(args[0])
		return err == nil && atomLenBoundInRange(bound)
	case "scalar":
		if len(args) != 1 {
			return false
		}
		k, okk := atomBytes(args[0])
		if !okk {
			return false
		}
		switch string(k) {
		case "int", "utf8-text", "bool":
			return true
		default:
			return false
		}
	// Descent combinators clear pending (a value constructor is consumed).
	case "tuple":
		for _, ct := range args {
			if !wfIn(ct, bound, nil, depth+1) {
				return false
			}
		}
		return true
	case "seq":
		if len(args) != 1 {
			return false
		}
		return wfIn(args[0], bound, nil, depth+1)
	case "compound":
		if len(args) < 1 {
			return false
		}
		if _, isAtom := args[0].(core.Atom); !isAtom {
			return false
		}
		for _, ct := range args[1:] {
			if !wfIn(ct, bound, nil, depth+1) {
				return false
			}
		}
		return true
	case "list-of", "set-of":
		if len(args) != 1 {
			return false
		}
		return wfIn(args[0], bound, nil, depth+1)
	case "map-of":
		if len(args) != 2 {
			return false
		}
		return wfIn(args[0], bound, nil, depth+1) && wfIn(args[1], bound, nil, depth+1)
	case "record":
		if len(args) < 1 {
			return false
		}
		return wfRecord(args[0], args[1:], bound, depth)
	// Same-v combinators thread the guard through.
	case "union", "and":
		for _, ct := range args {
			if !wfIn(ct, bound, pending, depth+1) {
				return false
			}
		}
		return true
	case "not":
		if len(args) != 1 {
			return false
		}
		return wfIn(args[0], bound, pending, depth+1)
	case "exact":
		// Any single ontos value is a valid singleton.
		return len(args) == 1
	case "rec":
		if len(args) != 2 {
			return false
		}
		nm, okNm := args[0].(core.Atom)
		if !okNm {
			return false
		}
		nb := nm.Bytes()
		// Shared push onto each chain (no per-frame slice copy).
		b := pushName(bound, nb)
		p := pushName(pending, nb)
		return wfIn(args[1], b, p, depth+1)
	case "ref":
		if len(args) != 1 {
			return false
		}
		nm, okNm := args[0].(core.Atom)
		if !okNm {
			return false
		}
		nb := nm.Bytes()
		return nameScopeContains(bound, nb) && !nameScopeContains(pending, nb)
	default:
		return false
	}
}

// ===========================================================================
// semantics.go binds decide byte-identically to the arms above.
// ===========================================================================

// isLeafCtor reports whether ctor is a LEAF descriptor (a §4 leaf / §5 refinement /
// owns exactly these.
func isLeafCtor(ctor string) bool {
	switch ctor {
	case "any", "atom", "atom-exact", "atom-len", "atom-in", "int-min", "int-max",
		"text-len", "key-grammar", "text-non-blank", "lowercase-uuid", "scalar", "exact":
		return true
	default:
		return false
	}
}

// leafClassifies decides a LEAF descriptor t against v (the horos/leaf(T, v) builtin).
// false for any non-leaf ctor. Reuses the recognizer's leaf arms via conformsIn at depth 0
// with nil scopes — a leaf arm never recurses into a value position, so the scopes are inert
// and the result is exactly the recognizer's leaf verdict.
func leafClassifies(v, t Value) bool {
	ctor, args, ok := head(t)
	if !ok || !isLeafCtor(ctor) {
		return false
	}
	var leaf *checker // leaf arms never use the checker
	return leaf.node(v, 0, ctor, args, 0, nil, 0)
}

// leafWellFormed decides whether a LEAF descriptor t is well-formed (the horos/wf-leaf(T)
// builtin). false for any non-leaf ctor.
func leafWellFormed(t Value) bool {
	ctor, _, ok := head(t)
	if !ok || !isLeafCtor(ctor) {
		return false
	}
	return wfIn(t, nil, nil, 0)
}

func wfRecord(mode Value, fields []Value, bound *nameScope, depth int) bool {
	mb, okm := atomBytes(mode)
	if !okm {
		return false
	}
	switch string(mb) {
	case "open", "closed":
		// ok
	default:
		return false
	}
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		name, _, ty, okf := readField(f)
		if !okf {
			return false
		}
		if !wfIn(ty, bound, nil, depth+1) {
			return false
		}
		if containsString(names, name) {
			return false // duplicate field name
		}
		names = append(names, name)
	}
	return true
}
