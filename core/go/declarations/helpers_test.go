package declarations_test

// Canonical revision-4 constructors following the notation of
// docs/spec/declarations.md §1: A, T, I, L, M, H and D.

import (
	"errors"
	"math/big"
	"testing"

	decl "github.com/Bitspark/horos/core/go/declarations"
	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

func a(s string) core.Value      { return core.NewAtom([]byte(s)) }
func text(s string) core.Value   { return data.MustEncodeText(s) }
func integer(i int64) core.Value { return data.EncodeInt(big.NewInt(i)) }
func list(v ...core.Value) core.Value {
	if v == nil {
		v = []core.Value{}
	}
	return data.EncodeList(v)
}
func h(tag string, v ...core.Value) core.Value {
	return core.NewTuple(append([]core.Value{a("horos/" + tag)}, v...)...)
}
func d(tag string, v ...core.Value) core.Value {
	return core.NewTuple(append([]core.Value{a("horos/decl/" + tag)}, v...)...)
}
func mapping(entries ...[2]core.Value) core.Value {
	v, err := data.EncodeMap(entries)
	if err != nil {
		panic(err)
	}
	return v
}
func entry(name string, v core.Value) [2]core.Value { return [2]core.Value{text(name), v} }
func module(name string, imports core.Value, entries ...[2]core.Value) core.Value {
	return d("module", integer(4), text(name), imports, mapping(entries...))
}
func param(i int64) core.Value { return d("param", integer(i)) }
func local(name string, args ...core.Value) core.Value {
	if len(args) == 0 {
		return d("local", text(name))
	}
	return d("apply", d("local", text(name)), list(args...))
}
func imported(alias, name string, args ...core.Value) core.Value {
	if len(args) == 0 {
		return d("imported", text(alias), text(name))
	}
	return d("apply", d("imported", text(alias), text(name)), list(args...))
}
func pin(t *testing.T, v core.Value) decl.Digest {
	t.Helper()
	p, err := decl.Pin(v)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func importValue(t *testing.T, name string, v core.Value) core.Value {
	t.Helper()
	p := pin(t, v)
	return d("import", text(name), core.NewAtom(p[:]))
}
func id(p *decl.Program, name string) decl.DeclarationID {
	return decl.DeclarationID{Module: p.Root(), Name: name}
}
func resolve(t *testing.T, root core.Value, deps ...core.Value) *decl.Program {
	t.Helper()
	p, err := decl.Resolve(root, deps)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func expectCode(t *testing.T, err error, code decl.Code) *decl.Error {
	t.Helper()
	var detail *decl.Error
	if !errors.As(err, &detail) || detail.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
	return detail
}

// Kind and declaration constructors of docs/spec/declarations.md §2.
func properSort() core.Value { return d("sort-proper") }
func kType() core.Value      { return d("kind-imported", text("k"), text("Type")) }
func kData() core.Value      { return d("kind-imported", text("k"), text("Data")) }
func kPrelude(name string, args ...core.Value) core.Value {
	ref := d("kind-imported", text("k"), text(name))
	if len(args) == 0 {
		return ref
	}
	return d("kind-apply", ref, list(args...))
}
func kLocal(name string) core.Value { return d("kind-local", text(name)) }
func kOr(kinds ...core.Value) core.Value {
	return d("or", properSort(), list(kinds...))
}
func kArrow(result core.Value, inputs ...core.Value) core.Value {
	return d("arrow", list(inputs...), result)
}
func tparam(name string, kind core.Value) core.Value { return d("parameter", text(name), kind) }
func preludeImports() core.Value                     { return mapping(entry("k", decl.PreludeImport())) }

// The pagination example of docs/spec/declarations.md §5.
func person() core.Value {
	return h("record", a("closed"), h("field", text("name"), a("req"), h("scalar", a("utf8-text"))))
}
func page(element core.Value) core.Value {
	return h("record", a("closed"),
		h("field", text("items"), a("req"), h("list-of", element)),
		h("field", text("next"), a("opt"), h("scalar", a("utf8-text"))))
}

// kTop is proper top written directly, for modules that import nothing.
func kTop() core.Value { return d("top", properSort()) }
