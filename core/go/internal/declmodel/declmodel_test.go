package declmodel

// Internal invariants of the declaration model that the public tests in
// core/go/declarations reach only indirectly: docs/spec/declarations.md §1
// (pins over canonical bytes), §2 (names), §8 (limits and budgets) and the
// diagnostic subjects of the Go API README.

import (
	"crypto/sha256"
	"errors"
	"math/big"
	"strings"
	"testing"

	codec "github.com/bitspark/ontos/codec/go"
	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

func sample() core.Value {
	text := data.MustEncodeText("name")
	return core.NewTuple(core.NewAtom([]byte("horos/record")), core.NewAtom([]byte("closed")),
		core.NewTuple(core.NewAtom([]byte("horos/field")), text, core.NewAtom([]byte("req")),
			core.NewTuple(core.NewAtom([]byte("horos/scalar")), core.NewAtom(make([]byte, 300)))))
}

func countValues(v core.Value) int {
	n := 1
	if t, ok := v.(core.Tuple); ok {
		for _, item := range t.Items() {
			n += countValues(item)
		}
	}
	return n
}

func code(t *testing.T, err error) Code {
	t.Helper()
	var detail *Error
	if !errors.As(err, &detail) {
		t.Fatalf("not a declarations error: %v", err)
	}
	return detail.Code
}

func TestLimitsValidity(t *testing.T) {
	if !DefaultLimits().valid() {
		t.Fatal("defaults are valid")
	}
	for _, l := range []Limits{
		{MaxDepth: 1, MaxNodes: 1, MaxBytes: 1, MaxModules: 1},
		{MaxDepth: 512, MaxNodes: 1, MaxBytes: 1, MaxModules: 1},
	} {
		if !l.valid() {
			t.Errorf("%+v is valid", l)
		}
	}
	for _, l := range []Limits{
		{},
		{MaxDepth: 0, MaxNodes: 1, MaxBytes: 1, MaxModules: 1},
		{MaxDepth: 513, MaxNodes: 1, MaxBytes: 1, MaxModules: 1},
		{MaxDepth: 1, MaxNodes: 0, MaxBytes: 1, MaxModules: 1},
		{MaxDepth: 1, MaxNodes: 1, MaxBytes: -1, MaxModules: 1},
		{MaxDepth: 1, MaxNodes: 1, MaxBytes: 1, MaxModules: 0},
	} {
		if l.valid() {
			t.Errorf("%+v is invalid", l)
		}
	}
}

// §1/§8: raw admission charges exactly the canonical ontos-codec-v1 bytes and
// one node per value, so the pin limits are tight at the encoded size.
func TestPinAdmissionChargesCanonicalBytes(t *testing.T) {
	v := sample()
	encoded := codec.Encode(v)
	limits := DefaultLimits()
	limits.MaxBytes = len(encoded)
	pin, err := PinWithLimits(v, limits)
	if err != nil || pin != Digest(sha256.Sum256(encoded)) {
		t.Fatalf("pin at the exact byte limit: %v", err)
	}
	limits.MaxBytes = len(encoded) - 1
	if _, err := PinWithLimits(v, limits); code(t, err) != Exhausted {
		t.Fatal("one byte under the canonical size")
	}
	limits = DefaultLimits()
	limits.MaxNodes = countValues(v)
	if _, err := PinWithLimits(v, limits); err != nil {
		t.Fatalf("pin at the exact node limit: %v", err)
	}
	limits.MaxNodes--
	if _, err := PinWithLimits(v, limits); code(t, err) != Exhausted {
		t.Fatal("one node under the value count")
	}
	limits = DefaultLimits()
	limits.MaxDepth = 3 // root tuple, field tuple, scalar tuple, atom leaf
	if _, err := PinWithLimits(v, limits); err != nil {
		t.Fatalf("pin at the exact depth: %v", err)
	}
	limits.MaxDepth = 2
	if _, err := PinWithLimits(v, limits); code(t, err) != Exhausted {
		t.Fatal("one level under the depth")
	}
	if _, err := PinWithLimits(v, Limits{}); code(t, err) != InvalidOptions {
		t.Fatal("invalid limits")
	}
	if pin.String() != strings.ToLower(pin.String()) || len(pin.String()) != 64 {
		t.Fatal("digest text is 64 lowercase hex digits")
	}
}

// §8: one aggregate budget per operation; it is never reset and reports
// exhaustion, never a verdict.
func TestBudgetAccounting(t *testing.T) {
	b := &budget{limits: Limits{MaxDepth: 2, MaxNodes: 3, MaxBytes: 10, MaxModules: 1}}
	for i := 0; i < 3; i++ {
		if err := b.Step(0); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	if code(t, b.Step(0)) != Exhausted {
		t.Fatal("fourth step")
	}
	b = &budget{limits: Limits{MaxDepth: 2, MaxNodes: 3, MaxBytes: 10, MaxModules: 1}}
	if code(t, b.Step(3)) != Exhausted || b.nodes != 0 {
		t.Fatal("depth beyond the limit consumes nothing")
	}
	if err := b.Bytes(6); err != nil {
		t.Fatal(err)
	}
	if err := b.Bytes(4); err != nil {
		t.Fatal("the remaining allowance is usable")
	}
	if code(t, b.Bytes(1)) != Exhausted || code(t, b.reserveBytes(-1)) != Exhausted || code(t, b.Bytes(11)) != Exhausted {
		t.Fatal("byte allowance")
	}
	if code(t, (&budget{limits: DefaultLimits()}).value(nil, 0)) != Invalid {
		t.Fatal("a nil value is not an Ontos value")
	}
}

func TestFailureSubjects(t *testing.T) {
	for c, want := range map[Code]Subject{
		Invalid: SubjectSource, Cycle: SubjectSource, UnknownImport: SubjectSource, UnknownDeclaration: SubjectSource,
		KindMismatch: SubjectSource, BoundViolation: SubjectSource, UnprovedObligation: SubjectSource,
		MissingDependency: SubjectContext, ImportMismatch: SubjectContext,
		InvalidOptions: SubjectConfiguration, Exhausted: SubjectRequest,
	} {
		var detail *Error
		if !errors.As(Failure(c, "x"), &detail) || detail.Code != c || detail.Subject != want {
			t.Errorf("%s: subject %v, want %s", c, detail, want)
		}
	}
	root, dependency := Digest{1}, Digest{2}
	err := sourceFailure(&Error{Code: Invalid, Subject: SubjectSource, Module: dependency}, root)
	var detail *Error
	if !errors.As(err, &detail) || detail.Subject != SubjectContext || detail.Dependency != dependency {
		t.Fatalf("a dependency's source failure is context: %+v", detail)
	}
	err = sourceFailure(&Error{Code: Invalid, Subject: SubjectSource, Module: root}, root)
	if !errors.As(err, &detail) || detail.Subject != SubjectSource {
		t.Fatalf("the root's source failure stays source: %+v", detail)
	}
	original := &Error{Code: Invalid, Subject: SubjectSource}
	if errors.As(specializationError(original), &detail); detail.Subject != SubjectRequest || original.Subject != SubjectSource {
		t.Fatal("request errors copy rather than mutate")
	}
	if !strings.Contains((&Error{Code: Cycle, Message: "m"}).Error(), "cycle") {
		t.Fatal("error text names its code")
	}
}

// §2: names and module names.
func TestNames(t *testing.T) {
	for name, want := range map[string]bool{
		"T": true, "_x": true, "Page2": true, "snake_case": true,
		"": false, "2x": false, "a-b": false, "a.b": false, "ä": false, "a b": false,
	} {
		if identifier(name) != want {
			t.Errorf("identifier(%q) != %v", name, want)
		}
	}
	for name, want := range map[string]bool{
		"horos.kinds": true, "example.people": true, "m": true, "a1.b_2": true,
		"": false, ".a": false, "a.": false, "a..b": false, "a.1b": false, "a-b": false,
	} {
		if moduleName(name) != want {
			t.Errorf("moduleName(%q) != %v", name, want)
		}
	}
	for v, want := range map[string]bool{"0": true, "7": true, "-1": false} {
		n, _ := new(big.Int).SetString(v, 10)
		if _, ok := index(data.EncodeInt(n)); ok != want {
			t.Errorf("index(%s) != %v", v, want)
		}
	}
	if _, ok := index(data.MustEncodeText("0")); ok {
		t.Error("index of text")
	}
}

func TestHead(t *testing.T) {
	label := func(s string) core.Value { return core.NewAtom([]byte(s)) }
	name, args, ok := head(core.NewTuple(label("horos/decl/param"), label("x")), "horos/decl/")
	if !ok || name != "param" || len(args) != 1 {
		t.Fatal("prefixed head")
	}
	for _, v := range []core.Value{
		label("horos/decl/param"),
		core.NewTuple(),
		core.NewTuple(core.NewTuple(), label("x")),
		core.NewTuple(label("horos/any")),
	} {
		if _, _, ok := head(v, "horos/decl/"); ok {
			t.Errorf("head(%v) matched", v)
		}
	}
}

// §2: sorts are finite; the zero value is proper and arrows keep their order.
func TestSorts(t *testing.T) {
	var proper Sort
	unary := arrowSort([]Sort{proper}, proper)
	binary := arrowSort([]Sort{proper, unary}, unary)
	if !proper.IsProper() || unary.IsProper() || len(binary.Inputs()) != 2 {
		t.Fatal("sort shape")
	}
	if !sameSort(binary, cloneSort(binary)) || sameSort(unary, binary) || sameSort(proper, unary) {
		t.Fatal("sort equality")
	}
	result, ok := binary.Result()
	if !ok || !sameSort(result, unary) {
		t.Fatal("result sort")
	}
	want := declValue("sort-arrow", data.EncodeList([]core.Value{declValue("sort-proper"), unary.Value()}), unary.Value())
	if !core.Equal(binary.Value(), want) {
		t.Fatal("canonical sort value")
	}
	inputs := binary.Inputs()
	inputs[0] = unary
	if !binary.Inputs()[0].IsProper() {
		t.Fatal("Inputs returned shared storage")
	}
	b := &budget{limits: Limits{MaxDepth: 256, MaxNodes: 2, MaxBytes: 1, MaxModules: 1}}
	if _, err := compareSort(binary, binary, b, 0); code(t, err) != Exhausted {
		t.Fatal("sort comparison is charged to the operation budget")
	}
}
