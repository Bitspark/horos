package horos

import (
	"math/big"
	"testing"

	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

// Unit tests for the horos recognizer: Conforms (v : T) and WF over the full
// descriptor DSL (incl. rec/ref). These mirror core/rs/src/tests.rs, building
// descriptors with d/a/tup helpers and scalar/collection payloads via the real
// ontos/data encoders, so they exercise the actual delegation path.

// ----- builders -----

func a(bytes []byte) Value { return core.NewAtom(bytes) }

func tup(items ...Value) Value { return core.NewTuple(items...) }

// d builds a descriptor Tuple(Atom("horos/<ctor>"), args...).
func d(ctor string, args ...Value) Value {
	items := make([]Value, 0, 1+len(args))
	items = append(items, a([]byte("horos/"+ctor)))
	items = append(items, args...)
	return core.NewTuple(items...)
}

func encInt(n int64) Value   { return data.EncodeInt(big.NewInt(n)) }
func encText(s string) Value { return data.MustEncodeText(s) }
func encBool(b bool) Value   { return data.EncodeBool(b) }

func encList(elems ...Value) Value { return data.EncodeList(elems) }

func encSet(t *testing.T, elems ...Value) Value {
	t.Helper()
	v, err := data.EncodeSet(elems)
	if err != nil {
		t.Fatalf("EncodeSet: %v", err)
	}
	return v
}

// field builds a horos/field descriptor for horos/record.
func field(name string, required bool, ty Value) Value {
	presence := "opt"
	if required {
		presence = "req"
	}
	return d("field", encText(name), a([]byte(presence)), ty)
}

func scalar(kind string) Value { return d("scalar", a([]byte(kind))) }

// mapOf builds a canonical ontos/data map from text-keyed pairs.
func mapOf(t *testing.T, pairs ...[2]Value) Value {
	t.Helper()
	entries := make([][2]core.Value, len(pairs))
	copy(entries, pairs)
	v, err := data.EncodeMap(entries)
	if err != nil {
		t.Fatalf("EncodeMap: %v", err)
	}
	return v
}

func pair(k string, v Value) [2]Value { return [2]Value{encText(k), v} }

func assertConforms(t *testing.T, v, ty Value, want bool, msg string) {
	t.Helper()
	if got := Conforms(v, ty); got != want {
		t.Errorf("Conforms = %v, want %v: %s", got, want, msg)
	}
}

func assertWF(t *testing.T, ty Value, want bool, msg string) {
	t.Helper()
	if got := WF(ty); got != want {
		t.Errorf("WF = %v, want %v: %s", got, want, msg)
	}
}

// ----- top / atoms -----

func TestAnyAcceptsEverything(t *testing.T) {
	any := d("any")
	assertConforms(t, a([]byte("x")), any, true, "atom")
	assertConforms(t, tup(), any, true, "empty tuple")
	assertConforms(t, encInt(5), any, true, "int")
}

func TestAtomCombinators(t *testing.T) {
	assertConforms(t, a([]byte("x")), d("atom"), true, "atom")
	assertConforms(t, tup(), d("atom"), false, "a tuple is not an atom")

	exact := d("atom-exact", a([]byte("hi")))
	assertConforms(t, a([]byte("hi")), exact, true, "exact match")
	assertConforms(t, a([]byte("ho")), exact, false, "different bytes")
	assertConforms(t, tup(a([]byte("hi"))), exact, false, "a tuple, not an atom")

	len13 := d("atom-len", encInt(1), encInt(3))
	assertConforms(t, a([]byte("x")), len13, true, "len 1")
	assertConforms(t, a([]byte("abc")), len13, true, "len 3")
	assertConforms(t, a([]byte("")), len13, false, "len 0 below lo")
	assertConforms(t, a([]byte("abcd")), len13, false, "len 4 above hi")

	inXY := d("atom-in", encSet(t, a([]byte("x")), a([]byte("y"))))
	assertConforms(t, a([]byte("x")), inXY, true, "member x")
	assertConforms(t, a([]byte("y")), inXY, true, "member y")
	assertConforms(t, a([]byte("z")), inXY, false, "non-member")
}

// ----- text refinements (horos-internal#68): char-counted bounds + grammar classes -----

func TestTextLenCountsCharsNotBytes(t *testing.T) {
	len13 := d("text-len", encInt(1), encInt(3))
	assertConforms(t, encText("a"), len13, true, "1 char")
	assertConforms(t, encText("abc"), len13, true, "3 chars")
	assertConforms(t, encText(""), len13, false, "0 chars below lo")
	assertConforms(t, encText("abcd"), len13, false, "4 chars above hi")
	// "ééé": 3 chars but 6 bytes — char-count is in range (a byte count would reject).
	assertConforms(t, encText("ééé"), len13, true, "3 multi-byte chars (6 bytes)")
	assertConforms(t, encText("éééé"), len13, false, "4 multi-byte chars over hi")
	// An astral emoji is ONE rune (4 bytes).
	assertConforms(t, encText("😀"), len13, true, "1 astral char")
	assertConforms(t, encInt(1), len13, false, "not utf8-text")

	assertWF(t, d("text-len", encInt(0), encInt(0)), true, "0<=0<=0")
	assertWF(t, d("text-len", encInt(3), encInt(1)), false, "lo>hi")
	assertWF(t, d("text-len", encInt(-1), encInt(3)), false, "lo<0")
}

func TestKeyGrammar(t *testing.T) {
	kg := d("key-grammar")
	assertConforms(t, encText("a"), kg, true, "single lowercase")
	assertConforms(t, encText("0"), kg, true, "single digit")
	assertConforms(t, encText("a.b_c-1"), kg, true, "interior . _ - and alnum")
	assertConforms(t, encText(""), kg, false, "empty")
	assertConforms(t, encText("A"), kg, false, "uppercase")
	assertConforms(t, encText(".ab"), kg, false, "bad leading char")
	assertConforms(t, encText("a b"), kg, false, "space")
	assertConforms(t, encText("café"), kg, false, "multi-byte char not in class")
	assertConforms(t, encText("a"+repeat("b", 63)), kg, true, "64 chars (boundary)")
	assertConforms(t, encText("a"+repeat("b", 64)), kg, false, "65 chars over")
	assertConforms(t, encInt(1), kg, false, "not utf8-text")
}

func TestTextNonBlank(t *testing.T) {
	nb := d("text-non-blank")
	assertConforms(t, encText("a"), nb, true, "non-ws char")
	assertConforms(t, encText("  x  "), nb, true, "ws around a non-ws char")
	assertConforms(t, encText(""), nb, false, "empty")
	assertConforms(t, encText("   "), nb, false, "all spaces")
	assertConforms(t, encText("\t\n\r"), nb, false, "tab/newline/CR")
	assertConforms(t, encText("é"), nb, true, "non-ASCII char is non-blank")
	assertConforms(t, encText(" "), nb, true, "U+00A0 not in ASCII ws set")
	assertConforms(t, encInt(1), nb, false, "not utf8-text")
}

func TestLowercaseUUID(t *testing.T) {
	u := d("lowercase-uuid")
	assertConforms(t, encText("00000000-0000-0000-0000-000000000000"), u, true, "nil UUID")
	assertConforms(t, encText("9b2e4c1a-7d3f-4e8b-a1c2-d3e4f5061728"), u, true, "canonical lowercase")
	assertConforms(t, encText("9B2E4C1A-7D3F-4E8B-A1C2-D3E4F5061728"), u, false, "uppercase")
	assertConforms(t, encText("9b2e4c1a7d3f4e8ba1c2d3e4f5061728"), u, false, "no hyphens")
	assertConforms(t, encText("9b2e4c1g-7d3f-4e8b-a1c2-d3e4f5061728"), u, false, "'g' not hex")
	assertConforms(t, encText(""), u, false, "empty")
	assertConforms(t, encInt(1), u, false, "not utf8-text")
}

// repeat returns s concatenated n times (a tiny strings.Repeat to avoid an import).
func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}

// ----- scalars (delegate to ontos/data) -----

func TestScalarDelegatesToOntosData(t *testing.T) {
	assertConforms(t, encInt(5), scalar("int"), true, "positive int")
	assertConforms(t, encInt(-1), scalar("int"), true, "negative int")
	assertConforms(t, encText("5"), scalar("int"), false, "text, not int")

	assertConforms(t, encText("hi"), scalar("utf8-text"), true, "text")
	assertConforms(t, encInt(1), scalar("utf8-text"), false, "int, not text")

	assertConforms(t, encBool(true), scalar("bool"), true, "bool")
	assertConforms(t, encInt(1), scalar("bool"), false, "int, not bool")

	// A non-canonical int (leading-zero magnitude) is NOT a well-formed int.
	noncanon := tup(a([]byte("int")), a([]byte{0x00, 0x00, 0x01}))
	assertConforms(t, noncanon, scalar("int"), false, "non-canonical int")
}

// ----- tuples & compounds -----

func TestTupleSeqCompound(t *testing.T) {
	p := d("tuple", scalar("int"), d("atom"))
	assertConforms(t, tup(encInt(1), a([]byte("x"))), p, true, "int then atom")
	assertConforms(t, tup(encInt(1)), p, false, "wrong arity")
	assertConforms(t, tup(a([]byte("x")), a([]byte("x"))), p, false, "child 0 not an int")
	assertConforms(t, a([]byte("x")), p, false, "not a tuple")

	seqInt := d("seq", scalar("int"))
	assertConforms(t, tup(encInt(1), encInt(2)), seqInt, true, "all ints")
	assertConforms(t, tup(), seqInt, true, "empty vacuous")
	assertConforms(t, tup(encInt(1), a([]byte("x"))), seqInt, false, "a non-int child")

	point := d("compound", a([]byte("point")), scalar("int"), scalar("int"))
	assertConforms(t, tup(a([]byte("point")), encInt(1), encInt(2)), point, true, "point(1,2)")
	assertConforms(t, tup(a([]byte("line")), encInt(1), encInt(2)), point, false, "wrong label")
	assertConforms(t, tup(a([]byte("point")), encInt(1)), point, false, "wrong arity")
}

// ----- structural collections (delegate well-formedness to ontos/data) -----

func TestCollections(t *testing.T) {
	listInt := d("list-of", scalar("int"))
	assertConforms(t, encList(encInt(1), encInt(2)), listInt, true, "ints")
	assertConforms(t, encList(), listInt, true, "empty list")
	assertConforms(t, encList(encInt(1), encText("x")), listInt, false, "a non-int element")
	assertConforms(t, encInt(1), listInt, false, "not a list")

	setAtom := d("set-of", d("atom"))
	assertConforms(t, encSet(t, a([]byte("x")), a([]byte("y"))), setAtom, true, "atoms")
	assertConforms(t, encList(a([]byte("x"))), setAtom, false, "a list is not a set")

	mapTI := d("map-of", scalar("utf8-text"), scalar("int"))
	assertConforms(t, mapOf(t, pair("a", encInt(1)), pair("b", encInt(2))), mapTI, true, "text->int")
	assertConforms(t, mapOf(t, pair("a", encText("x"))), mapTI, false, "value not int")
}

// ----- records -----

func TestRecords(t *testing.T) {
	recClosed := d("record",
		a([]byte("closed")),
		field("name", true, scalar("utf8-text")),
		field("age", false, scalar("int")),
	)
	recOpen := d("record",
		a([]byte("open")),
		field("name", true, scalar("utf8-text")),
	)

	assertConforms(t, mapOf(t, pair("name", encText("ada"))), recClosed, true, "required only")
	assertConforms(t, mapOf(t, pair("age", encInt(36)), pair("name", encText("ada"))), recClosed, true, "required + optional")
	assertConforms(t, mapOf(t, pair("age", encInt(36))), recClosed, false, "required name missing")
	assertConforms(t, mapOf(t, pair("name", encInt(1))), recClosed, false, "name not text")

	withExtra := mapOf(t, pair("name", encText("ada")), pair("x", encInt(1)))
	assertConforms(t, withExtra, recClosed, false, "extra key under closed")
	assertConforms(t, withExtra, recOpen, true, "extra key under open")

	assertConforms(t, encList(), recClosed, false, "not a map")
}

// ----- composition -----

func TestUnionAndNotExact(t *testing.T) {
	u := d("union", scalar("int"), scalar("bool"))
	assertConforms(t, encInt(1), u, true, "int alternative")
	assertConforms(t, encBool(true), u, true, "bool alternative")
	assertConforms(t, encText("x"), u, false, "neither")

	both := d("and", d("atom"), d("atom-len", encInt(1), encInt(3)))
	assertConforms(t, a([]byte("ab")), both, true, "atom of len 2")
	assertConforms(t, a([]byte("")), both, false, "fails length bound")
	assertConforms(t, tup(), both, false, "not an atom")

	notInt := d("not", scalar("int"))
	assertConforms(t, encInt(1), notInt, false, "is an int")
	assertConforms(t, encText("x"), notInt, true, "not an int")
	assertConforms(t, a([]byte("x")), notInt, true, "bare atom not an int")

	exact7 := d("exact", encInt(7))
	assertConforms(t, encInt(7), exact7, true, "equal")
	assertConforms(t, encInt(8), exact7, false, "different value")
}

// ----- a non-descriptor is never a conformance target -----

func TestNonDescriptorTargetIsFalse(t *testing.T) {
	assertConforms(t, encInt(1), a([]byte("x")), false, "bare atom")
	assertConforms(t, encInt(1), tup(a([]byte("int"))), false, "unreserved label")
	assertConforms(t, encInt(1), d("nope"), false, "unknown ctor")
}

// ----- wf -----

func TestWFAcceptsTheDSL(t *testing.T) {
	good := []Value{
		d("any"),
		d("atom"),
		d("atom-exact", a([]byte("hi"))),
		d("atom-len", encInt(0), encInt(64)),
		d("atom-in", encSet(t, a([]byte("x")))),
		scalar("int"),
		scalar("utf8-text"),
		scalar("bool"),
		d("tuple", scalar("int"), d("atom")),
		d("seq", scalar("int")),
		d("compound", a([]byte("point")), scalar("int"), scalar("int")),
		d("list-of", scalar("int")),
		d("set-of", d("atom")),
		d("map-of", scalar("utf8-text"), scalar("int")),
		d("record", a([]byte("closed")), field("name", true, scalar("utf8-text"))),
		d("union", scalar("int"), scalar("bool")),
		d("and", d("atom")),
		d("not", scalar("int")),
		d("exact", encInt(7)),
	}
	for _, ty := range good {
		assertWF(t, ty, true, ty.String())
	}
}

func TestWFRejectsMalformed(t *testing.T) {
	bad := []Value{
		a([]byte("x")),                                                   // bare atom
		tup(a([]byte("int"))),                                            // unreserved label
		d("scalar", a([]byte("float"))),                                  // unknown scalar kind
		d("atom-len", encInt(3), encInt(1)),                              // lo > hi
		d("atom-len", encInt(-1), encInt(2)),                             // lo < 0
		d("atom-in", encSetInt(t)),                                       // set element not an atom
		d("record", a([]byte("maybe")), field("n", true, scalar("int"))), // bad mode flag
		d("record", a([]byte("closed")), field("n", true, d("scalar", a([]byte("nope"))))), // bad field type
		d("tuple", d("nope")),                              // sub-descriptor not wf
		d("rec", a([]byte("x")), d("ref", a([]byte("x")))), // rec(x, ref x): unguarded
		d("ref", a([]byte("x"))),                           // unbound ref
		field("n", true, scalar("int")),                    // a field is not a top-level descriptor
	}
	for _, ty := range bad {
		assertWF(t, ty, false, ty.String())
	}
}

// encSetInt builds a set whose single element is an int (a non-atom), for the
// atom-in wf-rejection case.
func encSetInt(t *testing.T) Value {
	t.Helper()
	v, err := data.EncodeSet([]core.Value{encInt(1)})
	if err != nil {
		t.Fatalf("EncodeSet: %v", err)
	}
	return v
}

func TestWFRejectsDuplicateRecordFields(t *testing.T) {
	dup := d("record",
		a([]byte("open")),
		field("name", true, scalar("utf8-text")),
		field("name", false, scalar("int")),
	)
	assertWF(t, dup, false, "duplicate field name")
}

// ----- rec / ref (guarded recursive descriptors) -----

// consList: rec l. (atom-exact "nil") | (int, ref l).
func consList() Value {
	return d("rec", a([]byte("l")),
		d("union",
			d("atom-exact", a([]byte("nil"))),
			d("tuple", scalar("int"), d("ref", a([]byte("l")))),
		),
	)
}

// intTree: rec t. int | node(ref t, ref t).
func intTree() Value {
	return d("rec", a([]byte("t")),
		d("union",
			scalar("int"),
			d("compound", a([]byte("node")), d("ref", a([]byte("t"))), d("ref", a([]byte("t")))),
		),
	)
}

func TestRecursiveConformance(t *testing.T) {
	l := consList()
	assertWF(t, l, true, "cons-list is wf")
	assertConforms(t, a([]byte("nil")), l, true, "[]")
	assertConforms(t, tup(encInt(1), a([]byte("nil"))), l, true, "[1]")
	assertConforms(t, tup(encInt(1), tup(encInt(2), a([]byte("nil")))), l, true, "[1, 2]")
	assertConforms(t, tup(encText("x"), a([]byte("nil"))), l, false, "head not an int")
	assertConforms(t, a([]byte("other")), l, false, "not nil, not a cons cell")
	assertConforms(t, tup(encInt(1)), l, false, "missing the tail")

	tr := intTree()
	assertWF(t, tr, true, "int-tree is wf")
	assertConforms(t, encInt(5), tr, true, "a leaf")
	assertConforms(t, tup(a([]byte("node")), encInt(1), encInt(2)), tr, true, "node(1,2)")
	assertConforms(t, tup(a([]byte("node")), encInt(1), tup(a([]byte("node")), encInt(2), encInt(3))), tr, true, "node(1, node(2,3))")
	assertConforms(t, tup(a([]byte("node")), encText("x"), encInt(2)), tr, false, "left subtree neither int nor node")
}

func TestWFRecursionGuard(t *testing.T) {
	assertWF(t, consList(), true, "guarded cons-list")
	assertWF(t, intTree(), true, "guarded int-tree")

	bad := []Value{
		d("rec", a([]byte("x")), d("ref", a([]byte("x")))),                            // rec x. ref x — no descent
		d("rec", a([]byte("x")), d("union", d("ref", a([]byte("x"))), scalar("int"))), // union same-v: unguarded
		d("rec", a([]byte("x")), d("and", d("atom"), d("ref", a([]byte("x"))))),       // and same-v: unguarded
		d("ref", a([]byte("y"))), // unbound
	}
	for _, ty := range bad {
		assertWF(t, ty, false, ty.String())
	}
}

// ----- recursion-depth guard (horos-internal#37) -----

// deepConsValue builds a right-nested cons cell (0, (0, … nil)) levels deep,
// ITERATIVELY (so building it never recurses) — the shape that would overflow the
// native stack under an unguarded recognizer.
func deepConsValue(levels int) Value {
	v := a([]byte("nil"))
	for i := 0; i < levels; i++ {
		v = tup(encInt(0), v)
	}
	return v
}

// deepSeqDescriptor builds a left-nested seq descriptor seq(seq(… seq(int))) levels
// deep — a deep-but-finite DESCRIPTOR for the wf side, also built iteratively.
func deepSeqDescriptor(levels int) Value {
	t := scalar("int")
	for i := 0; i < levels; i++ {
		t = d("seq", t)
	}
	return t
}

// pastCapLevels: one value-level costs several conformsIn frames, so a value this many
// levels deep is guaranteed to drive the recognizer past MaxRecursionDepth and trip the
// guard, while staying shallow enough that building it never blows the stack.
const pastCapLevels = MaxRecursionDepth + 64

func TestRecursionDepthGuardConforms(t *testing.T) {
	cons := consList()

	// A shallow, in-bound cons-list still conforms — the guard never trips below the
	// cap, so no existing behavior changes.
	assertConforms(t, deepConsValue(8), cons, true, "shallow list conforms")

	// A value past MaxRecursionDepth is classified NON-conforming deterministically —
	// the guard returns false before the native stack can overflow (no fatal
	// "goroutine stack exceeds").
	assertConforms(t, deepConsValue(pastCapLevels), cons, false,
		"a value past the depth cap is non-conforming, not a crash")
}

func TestRecursionDepthGuardWF(t *testing.T) {
	// A shallow seq descriptor is well-formed (guard never trips below the cap).
	assertWF(t, deepSeqDescriptor(8), true, "shallow seq descriptor is wf")

	// A descriptor nested past the cap is treated as NON-wf deterministically, never
	// overflowing the stack.
	assertWF(t, deepSeqDescriptor(pastCapLevels), false,
		"a descriptor past the depth cap is non-wf, not a crash")
}
