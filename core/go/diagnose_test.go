package horos

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	codec "github.com/bitspark/ontos/codec/go"
	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

// Diagnostic checking outcomes (horos-internal#211 / ADR 0008). Mirrors core/rs/src/diagnose.rs tests and
// core/ts/test/diagnose.test.ts.

func dAtom(s string) Value { return core.NewAtom([]byte(s)) }

func desc(ctor string, args ...Value) Value {
	items := append([]Value{dAtom("horos/" + ctor)}, args...)
	return core.NewTuple(items...)
}

func dScalarInt() Value { return desc("scalar", dAtom("int")) }

// The oracle's own cons-list descriptor: wf, guarded, and a value nested past the cap trips
// the descent exactly as depth-171-over-cap-false does.
func dConsList() Value {
	return desc("rec", dAtom("L"),
		desc("union",
			desc("exact", dAtom("[]")),
			desc("tuple", dScalarInt(), desc("ref", dAtom("L"))),
		),
	)
}

func dDeepCons(n int) Value {
	v := dAtom("[]")
	one := data.EncodeInt(big.NewInt(1))
	for i := 0; i < n; i++ {
		v = core.NewTuple(one, v)
	}
	return v
}

// ADR 0008 D4, SWEPT rather than asserted: on every oracle judgment where Diagnose completes,
// it agrees with the frozen boolean. Where it reports Exhausted the boolean is unconstrained.
// This is also the anti-drift gate between the two recursions.
func TestDiagnosisAgreesWithTheBooleanOnCompleted(t *testing.T) {
	path := filepath.Join("testdata", "conformance.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc vectorDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	dec := func(h string) Value {
		b, err := hex.DecodeString(h)
		if err != nil {
			t.Fatalf("hex: %v", err)
		}
		v, err := codec.Decode(b)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		return v
	}
	completed, exhausted := 0, 0
	for _, c := range doc.Cases {
		dv := dec(c.Descriptor)
		if b, ok := DiagnoseWF(dv).CompletedBool(); ok {
			if b != WF(dv) {
				t.Errorf("wf disagrees for %s", c.Name)
			}
			completed++
		} else {
			exhausted++
		}
		for i, v := range c.Values {
			val := dec(v.Value)
			if b, ok := Diagnose(val, dv).CompletedBool(); ok {
				if b != Conforms(val, dv) {
					t.Errorf("conforms disagrees for %s / value[%d]", c.Name, i)
				}
				completed++
			} else {
				exhausted++
			}
		}
	}
	if completed < 100 {
		t.Errorf("expected >=100 completed, got %d", completed)
	}
	if exhausted != 1 {
		t.Errorf("expected exactly the one in-corpus exhaustion (depth-171), got %d", exhausted)
	}
}

// The oracle's depth boundary, now SEPARATED: 170 completes, 171 is an exhaustion — where the
// frozen boolean says a flat false for both.
func TestTheDepthBoundarySeparatesExhaustionFromMismatch(t *testing.T) {
	tt := dConsList()
	if got := Diagnose(dDeepCons(170), tt); got.Verdict != VerdictConforms {
		t.Errorf("170-deep: got %v, want conforms", got)
	}
	got := Diagnose(dDeepCons(171), tt)
	if got.Verdict != VerdictExhausted || got.Limit != LimitRecursionDepth {
		t.Errorf("171-deep: got %v, want exhausted/recursion_depth", got)
	}
	// the frozen boolean is UNCHANGED
	if !Conforms(dDeepCons(170), tt) || Conforms(dDeepCons(171), tt) {
		t.Errorf("the frozen boolean moved")
	}
	// a plain mismatch at the same descriptor is COMPLETED, not exhausted
	if got := Diagnose(dAtom("nope"), tt); got.Verdict != VerdictMismatch {
		t.Errorf("plain mismatch: got %v", got)
	}
}

// ADR 0008 D2 — THE rule. not of an exhausted subdecision stays exhausted, where the frozen
// boolean reports an unqualified true.
func TestComplementPreservesExhaustionWhereTheBooleanSaysTrue(t *testing.T) {
	inner := dConsList()
	tt := desc("not", inner)
	v := dDeepCons(171)

	// the boolean: !false = TRUE — a completed positive from a computation that never
	// finished. This precondition is the defect being named.
	if !Conforms(v, tt) {
		t.Fatalf("precondition: the frozen boolean should report true here")
	}
	got := Diagnose(v, tt)
	if got.Verdict != VerdictExhausted || got.Limit != LimitRecursionDepth {
		t.Errorf("not(Exhausted) must stay Exhausted; got %v", got)
	}
	// a NESTED complement does not launder it either
	if got := Diagnose(v, desc("not", desc("not", inner))); got.Verdict != VerdictExhausted {
		t.Errorf("nested not: got %v", got)
	}
	// sanity: over a SHALLOW value the same complement completes normally
	if got := Diagnose(dDeepCons(2), tt); got.Verdict != VerdictMismatch {
		t.Errorf("shallow not: got %v", got)
	}
	if got := Diagnose(dAtom("nope"), tt); got.Verdict != VerdictConforms {
		t.Errorf("not of a mismatch: got %v", got)
	}
}

// ADR 0008 D3 — a decisive alternative beats an exhausted sibling, in BOTH orders, so the
// answer is a function of the SET of sub-outcomes and never of traversal order.
func TestUnionAndAndAreKleeneInBothOrders(t *testing.T) {
	deep := dDeepCons(171)
	exhausts := dConsList()
	accepts := desc("any")
	rejects := dScalarInt() // a tuple is decisively not a scalar int

	for _, args := range [][]Value{{exhausts, accepts}, {accepts, exhausts}} {
		if got := Diagnose(deep, desc("union", args...)); got.Verdict != VerdictConforms {
			t.Errorf("a decisive union branch must beat an exhausted sibling; got %v", got)
		}
	}
	if got := Diagnose(deep, desc("union", exhausts, rejects)); got.Verdict != VerdictExhausted {
		t.Errorf("no decisive union branch: want exhausted, got %v", got)
	}

	for _, args := range [][]Value{{exhausts, rejects}, {rejects, exhausts}} {
		if got := Diagnose(deep, desc("and", args...)); got.Verdict != VerdictMismatch {
			t.Errorf("a decisive and-branch mismatch must beat an exhausted sibling; got %v", got)
		}
	}
	if got := Diagnose(deep, desc("and", exhausts, accepts)); got.Verdict != VerdictExhausted {
		t.Errorf("no decisive mismatch: want exhausted, got %v", got)
	}
}

// A malformed descriptor is its OWN completed outcome, not a mismatch of the value.
func TestAMalformedDescriptorIsNotAMismatch(t *testing.T) {
	bad := desc("scalar", dAtom("float"))
	if got := DiagnoseWF(bad); got.Verdict != VerdictNotWellFormed {
		t.Errorf("wf of a bad scalar: got %v", got)
	}
	one := data.EncodeInt(big.NewInt(1))
	if got := Diagnose(one, bad); got.Verdict != VerdictNotWellFormed {
		t.Errorf("diagnose with a bad descriptor: got %v", got)
	}
	if got := Diagnose(dAtom("x"), dScalarInt()); got.Verdict != VerdictMismatch {
		t.Errorf("a real mismatch: got %v", got)
	}
	// both are false to the frozen boolean — the distinction is the whole point
	if Conforms(one, bad) || Conforms(dAtom("x"), dScalarInt()) {
		t.Errorf("the frozen boolean moved")
	}
}

// The profile travels with the verdict, and an exhaustion names its limit.
func TestTheProfileAndTheLimitAreStated(t *testing.T) {
	p := CheckingProfile()
	if p.MaxRecursionDepth != MaxRecursionDepth {
		t.Errorf("profile depth %d", p.MaxRecursionDepth)
	}
	if p.AtomLenBoundMax != atomLenBoundMax.String() {
		t.Errorf("profile atom-len max %s", p.AtomLenBoundMax)
	}
	if LimitRecursionDepth.Value() != MaxRecursionDepth {
		t.Errorf("limit value %d", LimitRecursionDepth.Value())
	}
	for _, d := range []Diagnosis{{Verdict: VerdictConforms}, {Verdict: VerdictMismatch}, {Verdict: VerdictNotWellFormed}} {
		if !d.IsComplete() {
			t.Errorf("%v should be complete", d)
		}
	}
	ex := Diagnosis{Verdict: VerdictExhausted, Limit: LimitRecursionDepth}
	if ex.IsComplete() {
		t.Errorf("exhausted should not be complete")
	}
	if _, ok := ex.CompletedBool(); ok {
		t.Errorf("an exhaustion must NOT collapse into a boolean")
	}
}
