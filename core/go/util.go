package horos

import (
	"math/big"
	"unicode/utf8"
)

// atomLenBoundMax / atomLenBoundMin are the inclusive i128 range an atom-len bound must
// fit in to be well-formed (horos-internal#18): [-2^127, 2^127 - 1]. The Rust reference reads bounds
// into an i128 and errors (→ non-wf / false) past this range; Go's data.ReadInt returns
// an unbounded *big.Int, so we apply the SAME cap explicitly via atomLenBoundInRange so
// the three cores agree byte-for-byte at the cap (a bound = 2^127 is non-wf everywhere).
// Byte-length bounds are tiny in practice, so this never rejects a meaningful descriptor;
// it makes atom-len byte-identity a theorem rather than a "never happens" argument.
var (
	atomLenBoundMax = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 127), big.NewInt(1)) // 2^127 - 1
	atomLenBoundMin = new(big.Int).Neg(new(big.Int).Lsh(big.NewInt(1), 127))                // -2^127
)

// atomLenBoundInRange reports whether n fits in the signed 128-bit range — the explicit
// analogue of the Rust core's read_int succeeding (vs. returning IntOutOfRange). A bound
// outside the range makes the atom-len descriptor non-wf / non-conforming, matching Rust.
func atomLenBoundInRange(n *big.Int) bool {
	return n.Cmp(atomLenBoundMin) >= 0 && n.Cmp(atomLenBoundMax) <= 0
}

// bytesEqual reports whether two byte slices have identical contents. Mirrors the
// Rust `vb == bb` slice comparison; used for atom-exact / atom-in / compound label.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// utf8Valid reports whether b is valid UTF-8 — the Go analogue of Rust's
// std::str::from_utf8 gate on the descriptor label.
func utf8Valid(b []byte) bool { return utf8.Valid(b) }

// nameScope is a persistent (cons-list) stack of atom-byte names threaded through the
// recursion by pointer. Each rec/ref frame pushes ONE node that points at the parent
// chain instead of copying the whole slice (the old appendName/to_vec per frame), so a
// descent costs O(1) and allocates a single small node rather than an O(depth) slice.
// nameScopeContains walks head→tail, i.e. innermost-first. WF threads its bound and
// pending names in it; WF's lexical walk never unfolds, so names identify bindings there.
type nameScope struct {
	name []byte
	tail *nameScope
}

// pushName returns a new node with name at its head and s as its tail. The Rust core
// does `pending.to_vec(); p.push(nb)`; here the tail is shared, never copied, and the
// node is immutable, so distinct frames never observe each other's pushes.
func pushName(s *nameScope, name []byte) *nameScope {
	return &nameScope{name: name, tail: s}
}

// nameScopeContains reports whether the chain holds name (by content) — the analogue of
// the Rust core's `pending.contains(&nb)` over the cons-list pending/bound.
func nameScopeContains(s *nameScope, name []byte) bool {
	for n := s; n != nil; n = n.tail {
		if bytesEqual(n.name, name) {
			return true
		}
	}
	return false
}

// envScope is the conforms-side binding environment as a cons list: each in-scope rec
// name paired with its body, threaded by pointer like nameScope (no cloneEnv per frame).
//
// A node is also a closure (ADR 0011): its tail is the environment in which its rec
// was written, so a ref that resolves to the node continues with the node itself as
// its environment.
type envScope struct {
	name []byte
	body Value
	tail *envScope
}

// pushEnv returns a new env node with (name, body) at the head and e as the tail — the
// analogue of Rust's `Scope::push((nb, body), env)`.
func pushEnv(e *envScope, name []byte, body Value) *envScope {
	return &envScope{name: name, body: body, tail: e}
}

// lookupBinding finds the binding of name, searching from the innermost (head) binding
// outward, so an inner rec shadows an outer binding of the same name. The returned node
// is the closure the ref continues in.
func lookupBinding(e *envScope, name []byte) *envScope {
	for n := e; n != nil; n = n.tail {
		if bytesEqual(n.name, name) {
			return n
		}
	}
	return nil
}

// containsString reports whether xs contains s.
func containsString(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// data-encoded refinements (horos-internal#68) — the four named grammar-class predicates the
// visual-atlas consumer needs (atlas#533 Q4 / visual-atlas#534), each a closed,
// total, deterministic, single-value predicate (spec §5 admissibility). They
// measure Unicode CHARACTERS (runes), not bytes, so the multi-byte cases where
// char-count ≠ byte-count agree with the Rust and TS cores. Implemented as
// explicit, dependency-free rune scans (no regexp) so the three cores decide
// byte-for-byte. The §5 grammar literals are ASCII, so a non-ASCII rune never
// satisfies a grammar class. These mirror the Rust core's matches_key_grammar /
// is_non_blank / matches_lowercase_uuid (core/rs/src/lib.rs) rune-for-rune.

// matchesKeyGrammar reports whether s matches ^[a-z0-9][a-z0-9._-]{0,63}$ — a
// per-rune class test over the decoded string (1–64 runes total).
func matchesKeyGrammar(s string) bool {
	runes := []rune(s)
	if len(runes) == 0 || len(runes) > 64 {
		return false
	}
	first := runes[0]
	if !(isASCIILower(first) || isASCIIDigit(first)) {
		return false
	}
	for _, c := range runes[1:] {
		if !(isASCIILower(c) || isASCIIDigit(c) || c == '.' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// isNonBlank reports whether s has at least one non-whitespace rune. Whitespace is
// the fixed ASCII set {space, tab, LF, CR, VT, FF}; a non-ASCII rune is never
// whitespace, mirroring the Rust core's is_non_blank / is_ascii_ws exactly.
func isNonBlank(s string) bool {
	for _, c := range s {
		if !isASCIIWS(c) {
			return true
		}
	}
	return false
}

// matchesLowercaseUUID reports whether s matches the lowercase canonical UUID
// grammar ^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$ — scanned
// rune-by-rune so a multi-byte rune in a malformed input is rejected cleanly.
func matchesLowercaseUUID(s string) bool {
	groups := [5]int{8, 4, 4, 4, 12}
	runes := []rune(s)
	pos := 0
	for gi, n := range groups {
		for i := 0; i < n; i++ {
			if pos >= len(runes) {
				return false
			}
			c := runes[pos]
			if !(isASCIIDigit(c) || (c >= 'a' && c <= 'f')) {
				return false
			}
			pos++
		}
		if gi < len(groups)-1 {
			if pos >= len(runes) || runes[pos] != '-' {
				return false
			}
			pos++
		}
	}
	return pos == len(runes) // no trailing runes
}

func isASCIILower(c rune) bool { return c >= 'a' && c <= 'z' }
func isASCIIDigit(c rune) bool { return c >= '0' && c <= '9' }

// isASCIIWS is the fixed ASCII whitespace set shared with the Rust/TS cores.
func isASCIIWS(c rune) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	default:
		return false
	}
}
