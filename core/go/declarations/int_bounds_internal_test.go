package declarations

import (
	"errors"
	"math/big"
	"testing"

	horos "github.com/Bitspark/horos/core/go"
	model "github.com/Bitspark/horos/core/go/internal/declmodel"
	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

// Integer bounds in declarations (ADR 0010, #36 row 30).
//
// The standard module's Scalar gains node("int-min") and node("int-max") once
// Bittype's horos-module encoder admits those constructors and regenerates
// standard/horos.kinds.ontos. Until then these tests resolve against
// boundedPrelude: the decoded standard module with exactly that Scalar edit,
// built here and never shipped. When the regenerated asset lands, the guard in
// TestIntBoundsAwaitStandardModule fails; switch these tests to Prelude().

func ibInt(n *big.Int) core.Value { return data.EncodeInt(n) }
func ibSmall(n int64) core.Value  { return ibInt(big.NewInt(n)) }
func ibH(tag string, v ...core.Value) core.Value {
	return core.NewTuple(append([]core.Value{core.NewAtom([]byte("horos/" + tag))}, v...)...)
}
func ibMap(entries ...[2]core.Value) core.Value {
	v, err := data.EncodeMap(entries)
	if err != nil {
		panic(err)
	}
	return v
}
func ibEntry(name string, v core.Value) [2]core.Value {
	return [2]core.Value{declarationText(name), v}
}

// scalarAlternatives returns the operand list of Scalar's union in a canonical
// horos.kinds module, and a function rebuilding the module with new operands.
func scalarAlternatives(t *testing.T, kinds core.Value) ([]core.Value, func([]core.Value) core.Value) {
	t.Helper()
	module := kinds.(core.Tuple).Items()
	entries, err := data.ReadMap(module[4])
	if err != nil {
		t.Fatal(err)
	}
	for i, kv := range entries {
		if name, _ := data.ReadText(kv[0]); name != "Scalar" {
			continue
		}
		kind := kv[1].(core.Tuple).Items() // D("kind", params, sort, body)
		or := kind[3].(core.Tuple).Items() // D("or", sort, L(operands...))
		operands, err := data.ReadList(or[2])
		if err != nil {
			t.Fatal(err)
		}
		rebuild := func(next []core.Value) core.Value {
			or2 := append([]core.Value{}, or...)
			or2[2] = data.EncodeList(next)
			kind2 := append([]core.Value{}, kind...)
			kind2[3] = core.NewTuple(or2...)
			entries2 := append([][2]core.Value{}, entries...)
			entries2[i] = [2]core.Value{kv[0], core.NewTuple(kind2...)}
			module2 := append([]core.Value{}, module...)
			module2[4] = ibMap(entries2...)
			return core.NewTuple(module2...)
		}
		return operands, rebuild
	}
	t.Fatal("the kinds module has no Scalar")
	return nil, nil
}

func nodeForm(v core.Value) string {
	ctor, args, ok := model.Head(v, "horos/decl/")
	if !ok || ctor != "node" || len(args) != 3 {
		return ""
	}
	name, _ := data.ReadText(args[0])
	return name
}

// boundedPrelude is the standard module with Scalar extended as
// standard/horos.kinds.horos will state it: node("int-min", {}, epsilon) and
// node("int-max", {}, epsilon) after Bool. Each node is the module's own
// text-len node with only its form name changed.
func boundedPrelude(t *testing.T) core.Value {
	t.Helper()
	operands, rebuild := scalarAlternatives(t, Prelude())
	var template core.Value
	at := -1
	for i, op := range operands {
		if nodeForm(op) == "text-len" {
			template, at = op, i
		}
	}
	if template == nil {
		t.Fatal("Scalar has no text-len node to copy")
	}
	node := func(form string) core.Value {
		items := append([]core.Value{}, template.(core.Tuple).Items()...)
		items[1] = declarationText(form)
		return core.NewTuple(items...)
	}
	next := append([]core.Value{}, operands[:at]...)
	next = append(next, node("int-min"), node("int-max"))
	next = append(next, operands[at:]...)
	return rebuild(next)
}

func resolveWith(t *testing.T, kinds, root core.Value) (*Program, error) {
	t.Helper()
	g := grammar()
	g.Prelude = kinds
	p, err := model.Resolve(root, []core.Value{kinds}, DefaultLimits(), g)
	if err != nil {
		return nil, err
	}
	return &Program{inner: p}, nil
}

// countModule declares Count as Data with the given body, importing kinds as k.
func countModule(t *testing.T, kinds, body core.Value) core.Value {
	t.Helper()
	pin, err := Pin(kinds)
	if err != nil {
		t.Fatal(err)
	}
	imports := ibMap(ibEntry("k", declarationForm("import", declarationText("horos.kinds"), core.NewAtom(pin[:]))))
	dataKind := declarationForm("kind-imported", declarationText("k"), declarationText("Data"))
	count := declarationForm("type", data.EncodeList([]core.Value{}), dataKind, body)
	return declarationForm("module", ibSmall(4), declarationText("test.counts"), imports, ibMap(ibEntry("Count", count)))
}

func countRange() core.Value {
	return ibH("and", ibH("int-min", ibSmall(0)), ibH("int-max", ibSmall(9)))
}

// With Scalar extended, a Data declaration using and(int-min, int-max) is
// admitted and materializes through GroundDescriptor unchanged, and the ground
// checkers accept and refuse as ADR 0010 says, including beyond 128 bits.
func TestIntBoundsDeclarationIsData(t *testing.T) {
	kinds := boundedPrelude(t)
	body := countRange()
	p, err := resolveWith(t, kinds, countModule(t, kinds, body))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	value, err := p.Value(DeclarationID{Module: p.Root(), Name: "Count"})
	if err != nil {
		t.Fatal(err)
	}
	ground, err := p.GroundDescriptor(value)
	if err != nil {
		t.Fatalf("GroundDescriptor: %v", err)
	}
	if !core.Equal(ground, body) {
		t.Fatal("GroundDescriptor changed the body")
	}
	wide := new(big.Int).Lsh(big.NewInt(1), 200)
	for _, c := range []struct {
		note  string
		value core.Value
		want  bool
	}{
		{"0", ibSmall(0), true},
		{"9", ibSmall(9), true},
		{"10", ibSmall(10), false},
		{"-1", ibSmall(-1), false},
		{"2^200", ibInt(wide), false},
		{"-2^200", ibInt(new(big.Int).Neg(wide)), false},
		{"text", data.MustEncodeText("5"), false},
	} {
		if got := horos.Conforms(c.value, ground); got != c.want {
			t.Errorf("Conforms(%s) = %v, want %v", c.note, got, c.want)
		}
		d := horos.Diagnose(c.value, ground)
		if done, ok := d.CompletedBool(); !ok || done != c.want {
			t.Errorf("Diagnose(%s) = %s", c.note, d.Verdict)
		}
	}
	if d := horos.Diagnose(ibSmall(10), ground); d.Location == nil || len(d.Location.At) != 1 || d.Location.At[0] != 2 {
		t.Errorf("10 should be refused by int-max at [2], got %+v", d.Location)
	}

	// A bare bound is Data too: Count = int-min(int(0)), a non-negative count.
	p, err = resolveWith(t, kinds, countModule(t, kinds, ibH("int-min", ibSmall(0))))
	if err != nil {
		t.Fatalf("resolve bare int-min: %v", err)
	}
	value, err = p.Value(DeclarationID{Module: p.Root(), Name: "Count"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.GroundDescriptor(value); err != nil {
		t.Fatalf("GroundDescriptor bare int-min: %v", err)
	}
}

// A malformed bound is refused at admission whichever kinds are selected:
// the ground well-formedness check (ADR 0010) runs on every primitive body.
func TestIntBoundsMalformedBoundIsInvalid(t *testing.T) {
	kinds := boundedPrelude(t)
	past := ibInt(new(big.Int).Lsh(big.NewInt(1), 127))
	for _, body := range []core.Value{
		ibH("int-min", past),
		ibH("int-max", core.NewAtom([]byte("9"))),
		ibH("int-min"),
	} {
		_, err := resolveWith(t, kinds, countModule(t, kinds, body))
		var detail *model.Error
		if !errors.As(err, &detail) || detail.Code != Invalid {
			t.Errorf("want %s, got %v", Invalid, err)
		}
	}
}

// Against today's standard module the same bodies are refused: Scalar does not
// list the bounds yet, so the and operand at body path [1] does not establish
// its Data formation contract, and a bare bound is not Data.
func TestIntBoundsRefusedByTodaysStandardData(t *testing.T) {
	for _, c := range []struct {
		body core.Value
		path []int
	}{
		{countRange(), []int{1}},
		{ibH("int-min", ibSmall(0)), nil},
	} {
		_, err := Resolve(countModule(t, Prelude(), c.body), []core.Value{Prelude()})
		var detail *model.Error
		if !errors.As(err, &detail) {
			t.Fatalf("want a declaration error, got %v", err)
		}
		if detail.Declaration != "Count" || detail.Region != "body" || !slicesEqual(detail.Path, c.path) {
			t.Errorf("unexpected error site: %v", err)
		}
		t.Logf("refused as expected: %v", err)
	}
}

// The guard for the switch: once the regenerated standard module lists the
// bounds, test against Prelude() directly and remove boundedPrelude.
func TestIntBoundsAwaitStandardModule(t *testing.T) {
	operands, _ := scalarAlternatives(t, Prelude())
	for _, op := range operands {
		if form := nodeForm(op); form == "int-min" || form == "int-max" {
			t.Fatal("standard Scalar now lists " + form + ": switch the int-bounds tests to Prelude() and drop boundedPrelude")
		}
	}
}

func slicesEqual(a, b []int) bool {
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
