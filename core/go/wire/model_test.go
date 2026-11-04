package wire_test

import (
	"crypto/sha256"
	"errors"
	"math/big"
	"sync"
	"testing"

	hor "github.com/Bitspark/horos/core/go"
	ground "github.com/Bitspark/horos/core/go/declarations"
	wire "github.com/Bitspark/horos/core/go/wire"
	codec "github.com/bitspark/ontos/codec/go"
	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

type value = core.Value

func a(s string) value      { return core.NewAtom([]byte(s)) }
func txt(s string) value    { return data.MustEncodeText(s) }
func num(n int64) value     { return data.EncodeInt(big.NewInt(n)) }
func list(v ...value) value { return data.EncodeList(v) }
func form(prefix, tag string, v ...value) value {
	return core.NewTuple(append([]value{a(prefix + tag)}, v...)...)
}
func w(tag string, v ...value) value { return form("horos/wire/", tag, v...) }
func h(tag string, v ...value) value { return form("horos/", tag, v...) }
func d(tag string, v ...value) value { return form("horos/decl/", tag, v...) }
func mapping(entries ...[2]value) value {
	v, err := data.EncodeMap(entries)
	if err != nil {
		panic(err)
	}
	return v
}
func entry(name string, v value) [2]value { return [2]value{txt(name), v} }
func definition(body value, params ...string) value {
	names := make([]value, len(params))
	for i, n := range params {
		names[i] = txt(n)
	}
	return w("type", list(names...), body)
}
func module(name string, imports value, declarations ...[2]value) value {
	return w("module", num(1), txt(name), imports, mapping(declarations...))
}
func pin(v value) wire.Digest { return sha256.Sum256(codec.Encode(v)) }
func importOf(name string, v value) value {
	p := pin(v)
	return w("import", txt(name), core.NewAtom(p[:]))
}
func param(n int64) value                    { return w("param", num(n)) }
func local(name string, args ...value) value { return w("apply", w("local", txt(name)), list(args...)) }
func imported(alias, name string, args ...value) value {
	return w("apply", w("imported", txt(alias), txt(name)), list(args...))
}
func textType() value { return w("data", h("scalar", a("utf8-text"))) }
func page(t value) value {
	return w("record", a("closed"), w("field", txt("items"), a("req"), w("list-of", t)), w("field", txt("next"), a("opt"), textType()))
}
func resolve(t *testing.T, root value, deps ...value) *wire.Program {
	t.Helper()
	p, err := wire.Resolve(root, deps)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func expect(t *testing.T, err error, code wire.Code) *wire.Error {
	t.Helper()
	var detail *wire.Error
	if !errors.As(err, &detail) || detail.Code != code {
		t.Fatalf("got %v; want %s", err, code)
	}
	return detail
}

func TestPaginationGenericBindingAndSpecialization(t *testing.T) {
	pages := module("example.pagination.wire", mapping(), entry("Page", definition(page(param(0)), "T")))
	root := module("example.consumer", mapping(entry("pages", importOf("example.pagination.wire", pages))),
		entry("WirePage", definition(imported("pages", "Page", w("wire", textType())))))
	p := resolve(t, root, pages)
	decl, ok := p.Declaration(wire.DeclarationID{Module: pin(pages), Name: "Page"})
	if !ok {
		t.Fatal("missing Page")
	}
	child := decl.Expression().Operands()[0].Expression().Operands()[0].Expression()
	binder, ok := child.Parameter()
	if !ok || binder.Declaration != decl.ID() || binder.Index != 0 {
		t.Fatal("wrong lexical binder")
	}
	got, err := p.Specialize(wire.DeclarationID{Module: p.Root(), Name: "WirePage"}, nil)
	if err != nil || !core.Equal(got, page(w("wire", textType()))) {
		t.Fatalf("unexpected Page specialization: %v", err)
	}
	ref, ok := p.Reference(decl.ID())
	if !ok || ref.Profile != wire.Profile || ref.Revision != 1 || ref.Declaration != decl.ID() {
		t.Fatal("unqualified identity")
	}
	if hor.WF(got) {
		t.Fatal("mixed descriptor leaked into ground checker")
	}
	if err := wire.WF(got, wire.DefaultLimits()); err != nil {
		t.Fatal(err)
	}
	if _, err := ground.Resolve(root, []value{pages}); err == nil {
		t.Fatal("ground loader accepted optional envelope")
	}
	if !core.Equal(decl.Expression().Value(), page(param(0))) {
		t.Fatal("specialization erased generic source")
	}
	got, err = p.Specialize(decl.ID(), []value{textType()})
	if err != nil || !core.Equal(got, page(textType())) {
		t.Fatal("the same authored Page must accept ground data arguments", err)
	}
}

func TestOrdinaryLibraryCompositionAndLiteralOpacity(t *testing.T) {
	fn := module("library.functions", mapping(),
		entry("Unit", definition(w("data", h("tuple")))),
		entry("Function", definition(w("wire", w("tuple", param(0), w("wire", param(1)))), "A", "B")))
	cell := module("library.cells", mapping(entry("fn", importOf("library.functions", fn))),
		entry("Cell", definition(w("record", a("closed"),
			w("field", txt("get"), a("req"), imported("fn", "Function", imported("fn", "Unit"), param(0))),
			w("field", txt("set"), a("req"), imported("fn", "Function", param(0), imported("fn", "Unit")))), "T")))
	p := resolve(t, cell, fn)
	got, err := p.Specialize(wire.DeclarationID{Module: p.Root(), Name: "Cell"}, []value{textType()})
	if err != nil {
		t.Fatal(err)
	}
	unit := w("data", h("tuple"))
	want := w("record", a("closed"),
		w("field", txt("get"), a("req"), w("wire", w("tuple", unit, w("wire", textType())))),
		w("field", txt("set"), a("req"), w("wire", w("tuple", textType(), w("wire", unit)))))
	if !core.Equal(got, want) {
		t.Fatal("ordinary imported composition changed")
	}
	// No production package recognizes either library name or its expanded shape.
	literal := w("data", h("exact", w("param", num(999))))
	root := module("Unfamiliar", mapping(), entry("Envelope", definition(w("tuple", literal, param(0)), "T")))
	p = resolve(t, root)
	got, err = p.Specialize(wire.DeclarationID{Module: p.Root(), Name: "Envelope"}, []value{textType()})
	if err != nil || !core.Equal(got, w("tuple", literal, textType())) {
		t.Fatal("literal interpreted as parameter", err)
	}
}

func TestClosedGrammarAndProfileRefusals(t *testing.T) {
	invalid := []value{
		h("any"), w("any"), w("wire"), w("wire", textType(), textType()),
		w("data", param(0)), w("data", h("list-of", d("param", num(0)))),
		w("union"), w("union", textType()), w("record", a("open")),
		w("record", a("closed"), w("field", txt("x"), a("req"), textType()), w("field", txt("x"), a("opt"), textType())),
		w("compound", txt("not-an-atom")),
		form("bittype/contract/", "wire", textType()),
	}
	for _, body := range invalid {
		p, err := wire.Resolve(module("bad", mapping(), entry("T", definition(body, "X"))), nil)
		if p != nil {
			t.Fatal("invalid source published program")
		}
		expect(t, err, wire.Invalid)
	}
	for _, body := range []value{w("data", h("any")), w("data", h("not", h("any"))), w("data", h("union")), w("tuple"), w("record", a("closed"))} {
		if err := wire.WF(body, wire.DefaultLimits()); err != nil {
			t.Fatal(err)
		}
	}
	badRevision := w("module", num(2), txt("bad"), mapping(), mapping())
	_, err := wire.Resolve(badRevision, nil)
	expect(t, err, wire.Invalid)
	_, err = wire.Resolve(d("module", num(1), txt("ground"), mapping(), mapping()), nil)
	expect(t, err, wire.Invalid)
	_, err = wire.Resolve(module("recursive", mapping(), entry("T", definition(local("T")))), nil)
	expect(t, err, wire.Cycle)
}

func TestFullContextAndCallerScope(t *testing.T) {
	dep := module("dep", mapping(), entry("Pair", definition(w("tuple", param(1), param(0)), "T", "U")))
	root := module("root", mapping(entry("dep", importOf("dep", dep))),
		entry("Swap", definition(imported("dep", "Pair", param(1), param(0)), "T", "U")))
	p := resolve(t, root, dep)
	result, err := p.Specialize(wire.DeclarationID{Module: p.Root(), Name: "Swap"}, []value{textType(), w("wire", textType())})
	if err != nil || !core.Equal(result, w("tuple", textType(), w("wire", textType()))) {
		t.Fatal("caller/callee scopes mixed", err)
	}
	_, err = wire.Resolve(root, nil)
	if expect(t, err, wire.MissingDependency).Subject != wire.SubjectContext {
		t.Fatal("context failure became source rejection")
	}
	unused := module("unused", mapping(entry("dep", importOf("dep", dep))))
	_, err = wire.Resolve(unused, nil)
	expect(t, err, wire.MissingDependency)
	wrong := module("wrong", mapping(entry("dep", importOf("different", dep))))
	_, err = wire.Resolve(wrong, []value{dep})
	expect(t, err, wire.ImportMismatch)
	bad := module("bad", mapping(), entry("T", definition(local("Missing"))))
	_, err = wire.Resolve(bad, nil)
	expect(t, err, wire.UnknownDeclaration)
}

func TestBudgetsAndImmutableConcurrentPrograms(t *testing.T) {
	root := module("box", mapping(), entry("Box", definition(w("tuple", param(0), param(0)), "T")))
	l := wire.DefaultLimits()
	l.MaxNodes = 1
	p, err := wire.ResolveWithLimits(root, nil, l)
	if p != nil {
		t.Fatal("partial program escaped")
	}
	expect(t, err, wire.Exhausted)
	_, err = wire.ResolveWithLimits(root, nil, wire.Limits{})
	expect(t, err, wire.InvalidOptions)
	p = resolve(t, root)
	id := wire.DeclarationID{Module: p.Root(), Name: "Box"}
	if v, err := p.Specialize(id, []value{h("any")}); v != nil || err == nil {
		t.Fatal("ground argument bypassed Data")
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			decl, _ := p.Declaration(id)
			names := decl.Parameters()
			names[0] = "changed"
			*decl = wire.Declaration{}
			v, err := p.Specialize(id, []value{textType()})
			if err != nil || !core.Equal(v, w("tuple", textType(), textType())) {
				t.Error("mutable model", err)
			}
		}()
	}
	wg.Wait()
	decl, _ := p.Declaration(id)
	if decl.Parameters()[0] != "T" {
		t.Fatal("source overwritten")
	}
}
