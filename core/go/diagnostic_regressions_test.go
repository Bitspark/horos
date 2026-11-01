package horos

import (
	"testing"

	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

func TestDiagnosticBinaryIdentity(t *testing.T) {
	x, y := core.NewAtom([]byte{0x80}), core.NewAtom([]byte{0x81})
	label := desc("compound", x)
	if !WF(label) || Conforms(core.NewTuple(y), label) || Diagnose(core.NewTuple(y), label).Verdict != VerdictMismatch {
		t.Fatal("distinct binary labels must mismatch")
	}
	ty := desc("rec", x, desc("union", desc("exact", dAtom("end")), desc("tuple", desc("rec", y, desc("tuple", desc("ref", x))))))
	v := core.NewTuple(core.NewTuple(dAtom("end")))
	if !WF(ty) || !Conforms(v, ty) || Diagnose(v, ty).Verdict != VerdictConforms {
		t.Fatal("distinct binary recursion names must not shadow each other")
	}
}

func TestExactDeepLiteral(t *testing.T) {
	v, equal, different := dAtom("end"), dAtom("end"), dAtom("other")
	for i := 0; i < 20000; i++ {
		v = core.NewTuple(v)
		equal = core.NewTuple(equal)
		different = core.NewTuple(different)
	}
	ty := desc("exact", v)
	if !WF(ty) || !Conforms(equal, ty) || Diagnose(equal, ty).Verdict != VerdictConforms {
		t.Fatal("deep equal literal rejected")
	}
	if Conforms(different, ty) || Diagnose(different, ty).Verdict != VerdictMismatch {
		t.Fatal("deep unequal literal accepted")
	}
}

func TestDiagnosticWFDepthAndScope(t *testing.T) {
	seqs := func(n int, ty Value) Value {
		for i := 0; i < n; i++ {
			ty = desc("seq", ty)
		}
		return ty
	}
	field := func(ty Value) Value { return desc("field", data.EncodeText("next"), dAtom("opt"), ty) }
	for _, inner := range []Value{
		desc("rec", dAtom("x"), seqs(2, desc("any"))),
		desc("record", dAtom("open"), field(seqs(2, desc("any")))),
	} {
		at := seqs(MaxRecursionDepth-3, inner)
		over := seqs(MaxRecursionDepth-2, inner)
		if !WF(at) || DiagnoseWF(at).Verdict != VerdictConforms {
			t.Fatal("inclusive depth limit rejected")
		}
		if WF(over) || DiagnoseWF(over).Verdict != VerdictExhausted {
			t.Fatal("nested rec/record reset the depth")
		}
		if Diagnose(core.NewTuple(), over).Verdict != VerdictExhausted {
			t.Fatal("value gate hid descriptor exhaustion")
		}
	}
	deep := desc("rec", dAtom("x"), seqs(MaxRecursionDepth+1, desc("any")))
	if DiagnoseWF(deep).Verdict != VerdictExhausted {
		t.Fatal("exhaustion reported as malformed")
	}
	guarded := desc("rec", dAtom("x"), desc("record", dAtom("open"), field(desc("ref", dAtom("x")))))
	if !WF(guarded) || DiagnoseWF(guarded).Verdict != VerdictConforms {
		t.Fatal("record lost recursion scope")
	}
	for _, bad := range []Value{desc("ref", dAtom("x")), desc("rec", dAtom("x"), desc("ref", dAtom("x")))} {
		if DiagnoseWF(bad).Verdict != VerdictNotWellFormed {
			t.Fatal("unbound or unguarded recursion accepted")
		}
	}
}
