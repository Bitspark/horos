package declarations_test

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"testing"

	horos "github.com/Bitspark/horos/core/go"
	decl "github.com/Bitspark/horos/core/go/declarations"
	codec "github.com/bitspark/ontos/codec/go"
	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

type value = core.Value

func a(s string) value      { return core.NewAtom([]byte(s)) }
func text(s string) value   { return data.MustEncodeText(s) }
func integer(i int64) value { return data.EncodeInt(big.NewInt(i)) }
func list(v ...value) value { return data.EncodeList(v) }
func h(tag string, v ...value) value {
	return core.NewTuple(append([]value{a("horos/" + tag)}, v...)...)
}
func d(tag string, v ...value) value {
	return core.NewTuple(append([]value{a("horos/decl/" + tag)}, v...)...)
}
func mapping(entries ...[2]value) value {
	v, err := data.EncodeMap(entries)
	if err != nil {
		panic(err)
	}
	return v
}
func entry(name string, v value) [2]value { return [2]value{text(name), v} }
func definition(body value, parameters ...string) value {
	var names []value
	for _, name := range parameters {
		names = append(names, text(name))
	}
	return d("type", list(names...), body)
}
func module(name string, imports value, entries ...[2]value) value {
	return d("module", integer(1), text(name), imports, mapping(entries...))
}
func param(i int64) value { return d("param", integer(i)) }
func local(name string, args ...value) value {
	return d("apply", d("local", text(name)), list(args...))
}
func imported(alias, name string, args ...value) value {
	return d("apply", d("imported", text(alias), text(name)), list(args...))
}
func pin(t *testing.T, v value) decl.Digest {
	t.Helper()
	p, err := decl.Pin(v)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func importValue(t *testing.T, name string, v value) value {
	p := pin(t, v)
	return d("import", text(name), core.NewAtom(p[:]))
}
func id(p *decl.Program, name string) decl.DeclarationID {
	return decl.DeclarationID{Module: p.Root(), Name: name}
}
func resolve(t *testing.T, root value, deps ...value) *decl.Program {
	t.Helper()
	p, err := decl.Resolve(root, deps)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func expectCode(t *testing.T, err error, code decl.Code) {
	t.Helper()
	var detail *decl.Error
	if !errors.As(err, &detail) || detail.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
}

func person() value {
	return h("record", a("closed"), h("field", text("name"), a("req"), h("scalar", a("utf8-text"))))
}
func page(element value) value {
	return h("record", a("closed"),
		h("field", text("items"), a("req"), h("list-of", element)),
		h("field", text("next"), a("opt"), h("scalar", a("utf8-text"))))
}

func pagination(t *testing.T) (value, value) {
	t.Helper()
	pages := module("example.pagination", mapping(), entry("Page", definition(page(param(0)), "T")))
	people := module("example.people", mapping(entry("pagination", importValue(t, "example.pagination", pages))),
		entry("Person", definition(person())),
		entry("PeoplePage", definition(imported("pagination", "Page", local("Person")))))
	return pages, people
}

func TestPagination(t *testing.T) {
	pages, people := pagination(t)
	p := resolve(t, people, pages)
	if p.Root() != pin(t, people) || len(p.Modules()) != 2 {
		t.Fatal("module closure or root changed")
	}
	pageID := decl.DeclarationID{Module: pin(t, pages), Name: "Page"}
	open, ok := p.Declaration(pageID)
	if !ok || len(open.Parameters()) != 1 || open.Parameters()[0] != "T" {
		t.Fatal("open generic declaration was lost")
	}
	slot := open.Expression().Operands()[0].Expression().Operands()[0].Expression()
	binder, ok := slot.Parameter()
	if !ok || binder.Declaration != pageID || binder.Index != 0 {
		t.Fatal("Page binder origin was lost")
	}
	applied, _ := p.Declaration(id(p, "PeoplePage"))
	target, ok := applied.Expression().Target()
	if !ok || target != pageID {
		t.Fatal("imported application target was lost")
	}
	argument, ok := applied.Expression().Arguments()[0].Target()
	if !ok || argument != id(p, "Person") {
		t.Fatal("argument did not bind in caller module")
	}
	actual, err := p.Specialize(id(p, "PeoplePage"), nil)
	if err != nil || !core.Equal(actual, page(person())) {
		t.Fatalf("wrong pagination specialization: %v", err)
	}
	if !horos.WF(actual) {
		t.Fatal("specialization is not an existing Horos descriptor")
	}
	personValue := mapping(entry("name", text("Ada")))
	for _, test := range []struct {
		name string
		next value
		want bool
	}{
		{"absent", nil, true},
		{"text", text("cursor"), true},
		{"null", data.EncodeNull(), false},
		{"integer", integer(1), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			entries := [][2]value{entry("items", list(personValue))}
			if test.next != nil {
				entries = append(entries, entry("next", test.next))
			}
			if got := horos.Conforms(mapping(entries...), actual); got != test.want {
				t.Fatalf("conformance %v, want %v", got, test.want)
			}
		})
	}
	again, err := p.Specialize(pageID, []value{person()})
	if err != nil || !core.Equal(actual, again) || slot.Kind() != decl.Parameter {
		t.Fatal("direct application differs or mutated open declaration")
	}
}

func TestCallerBindingNestedGenericsAndIdentity(t *testing.T) {
	pages := module("pages", mapping(),
		entry("Person", definition(h("scalar", a("int")))),
		entry("Page", definition(page(param(0)), "T")))
	root := module("people", mapping(entry("pages", importValue(t, "pages", pages))),
		entry("Person", definition(person())),
		entry("PeoplePage", definition(imported("pages", "Page", local("Person")))),
		entry("Nested", definition(imported("pages", "Page", imported("pages", "Page", param(0))), "T")),
		entry("First", definition(param(0), "T")),
		entry("Second", definition(param(0), "T")),
		entry("Swap", definition(h("tuple", param(1), param(0)), "A", "B")))
	p := resolve(t, root, pages)
	actual, err := p.Specialize(id(p, "PeoplePage"), nil)
	if err != nil || !core.Equal(actual, page(person())) {
		t.Fatal("callee's same-spelled Person captured the caller argument")
	}
	nested, err := p.Specialize(id(p, "Nested"), []value{person()})
	if err != nil || !core.Equal(nested, page(page(person()))) {
		t.Fatal("nested application lost its original binder")
	}
	first, _ := p.Declaration(id(p, "First"))
	second, _ := p.Declaration(id(p, "Second"))
	b1, _ := first.Expression().Parameter()
	b2, _ := second.Expression().Parameter()
	if b1 == b2 || b1.Index != 0 || b2.Index != 0 {
		t.Fatal("same-spelled binders were merged")
	}
	swapped, err := p.Specialize(id(p, "Swap"), []value{h("scalar", a("int")), person()})
	if err != nil || !core.Equal(swapped, h("tuple", person(), h("scalar", a("int")))) {
		t.Fatal("parameter order was not preserved")
	}
}

func TestLiteralBoundaryAndDescriptorPositions(t *testing.T) {
	literal := h("exact", d("apply", d("imported", text("missing"), text("Absent")), list(param(999))))
	p := resolve(t, module("literal", mapping(), entry("Data", definition(literal))))
	got, err := p.Specialize(id(p, "Data"), nil)
	if err != nil || !core.Equal(got, literal) {
		t.Fatal("literal declaration-shaped data was interpreted")
	}
	for _, test := range []struct {
		name string
		body value
		want value
	}{
		{"tuple", h("tuple", param(0)), h("tuple", person())},
		{"seq", h("seq", param(0)), h("seq", person())},
		{"compound", h("compound", a("item"), param(0)), h("compound", a("item"), person())},
		{"list", h("list-of", param(0)), h("list-of", person())},
		{"set", h("set-of", param(0)), h("set-of", person())},
		{"map", h("map-of", param(0), param(0)), h("map-of", person(), person())},
		{"record", page(param(0)), page(person())},
		{"union", h("union", param(0), h("exact", data.EncodeNull())), h("union", person(), h("exact", data.EncodeNull()))},
		{"intersection", h("and", param(0), h("any")), h("and", person(), h("any"))},
		{"complement", h("not", param(0)), h("not", person())},
		{"recursion", h("rec", a("x"), h("tuple", param(0), h("ref", a("x")))), h("rec", a("x"), h("tuple", person(), h("ref", a("x"))))},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := resolve(t, module("positions", mapping(), entry("Generic", definition(test.body, "T"))))
			got, err := p.Specialize(id(p, "Generic"), []value{person()})
			if err != nil || !core.Equal(got, test.want) {
				t.Fatalf("wrong lifted position: %v", err)
			}
		})
	}
}

func TestRejectedDeclarations(t *testing.T) {
	for _, test := range []struct {
		name string
		body value
		args []string
		code decl.Code
	}{
		{"unbound", param(0), nil, decl.Invalid},
		{"out of range", param(1), []string{"T"}, decl.Invalid},
		{"negative", param(-1), []string{"T"}, decl.Invalid},
		{"extra parameter operand", d("param", integer(0), integer(1)), []string{"T"}, decl.Invalid},
		{"duplicate parameter", param(0), []string{"T", "T"}, decl.Invalid},
		{"invalid parameter name", param(0), []string{"T.A"}, decl.Invalid},
		{"unknown target", local("Missing"), nil, decl.UnknownDeclaration},
		{"unknown import", imported("missing", "X"), nil, decl.UnknownImport},
		{"bare target", d("local", text("Generic")), nil, decl.Invalid},
		{"unknown descriptor", h("function", param(0)), []string{"T"}, decl.Invalid},
		{"literal scalar kind", h("scalar", param(0)), []string{"T"}, decl.Invalid},
		{"literal record mode", h("record", param(0)), []string{"T"}, decl.Invalid},
		{"literal field name", h("record", a("closed"), h("field", param(0), a("req"), h("any"))), []string{"T"}, decl.Invalid},
		{"field duplicate", h("record", a("closed"), h("field", text("x"), a("req"), param(0)), h("field", text("x"), a("opt"), param(0))), []string{"T"}, decl.Invalid},
		{"unguarded rec", h("rec", a("x"), h("ref", a("x"))), nil, decl.Invalid},
		{"unbound rec", h("list-of", h("ref", a("x"))), nil, decl.Invalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			p, err := decl.Resolve(module("invalid", mapping(), entry("Generic", definition(test.body, test.args...))), nil)
			if p != nil {
				t.Fatal("failure returned a partial program")
			}
			expectCode(t, err, test.code)
		})
	}
	for _, arguments := range [][]value{nil, {person(), person()}} {
		root := module("arity", mapping(), entry("Generic", definition(param(0), "T")), entry("Bad", definition(local("Generic", arguments...))))
		p, err := decl.Resolve(root, nil)
		if p != nil {
			t.Fatal("arity failure returned partial program")
		}
		expectCode(t, err, decl.Invalid)
	}
	for _, entries := range [][][2]value{
		{entry("A", definition(local("A")))},
		{entry("A", definition(local("B"))), entry("B", definition(local("A")))},
		{entry("Grow", definition(local("Grow", h("list-of", param(0))), "T"))},
	} {
		_, err := decl.Resolve(module("cycles", mapping(), entries...), nil)
		expectCode(t, err, decl.Cycle)
	}
}

func TestRecursionClosureAtApplicationBoundaries(t *testing.T) {
	captured := h("rec", a("x"), h("list-of", local("Identity", h("ref", a("x")))))
	_, err := decl.Resolve(module("capture", mapping(),
		entry("Identity", definition(param(0), "T")), entry("Bad", definition(captured))), nil)
	expectCode(t, err, decl.Invalid)
	closed := h("rec", a("x"), h("list-of", h("ref", a("x"))))
	p := resolve(t, module("closed", mapping(),
		entry("Wrap", definition(h("rec", a("x"), h("list-of", param(0))), "T")),
		entry("Applied", definition(local("Wrap", closed)))))
	actual, err := p.Specialize(id(p, "Applied"), nil)
	if err != nil || !core.Equal(actual, h("rec", a("x"), h("list-of", closed))) {
		t.Fatal("closed nested recursion was captured or rewritten")
	}
	// Guarded but uninhabited is well-formed, not a declaration error.
	resolve(t, module("empty", mapping(), entry("Empty", definition(h("rec", a("x"), h("tuple", h("ref", a("x"))))))))
}

func TestModulePinsAndImports(t *testing.T) {
	pages, people := pagination(t)
	got := pin(t, pages)
	if got != sha256.Sum256(codec.Encode(pages)) {
		t.Fatal("pin is not plain SHA-256 of codec-v1 bytes")
	}
	_, err := decl.Resolve(people, nil)
	expectCode(t, err, decl.MissingDependency)
	wrongName := module("people", mapping(entry("p", importValue(t, "wrong.name", pages))))
	_, err = decl.Resolve(wrongName, []value{pages})
	expectCode(t, err, decl.ImportMismatch)
	other := module("unrelated", mapping(), entry("X", definition(h("any"))))
	_, err = decl.Resolve(people, []value{other})
	expectCode(t, err, decl.MissingDependency)
	bridge := module("bridge", mapping(entry("p", importValue(t, "example.pagination", pages))))
	root := module("root", mapping(entry("bridge", importValue(t, "bridge", bridge))))
	_, err = decl.Resolve(root, []value{bridge})
	expectCode(t, err, decl.MissingDependency) // Unused transitive import is still required.
	if len(resolve(t, root, bridge, pages).Modules()) != 3 {
		t.Fatal("transitive closure lost a module")
	}
	for _, invalid := range []value{
		d("module", integer(2), text("m"), mapping(), mapping()),
		d("module", integer(1), text("bad..name"), mapping(), mapping()),
		d("module", integer(1), text(strings.Repeat(".", 1<<16)), mapping(), mapping()),
		d("module", integer(1), text("m"), list(), mapping()),
		d("module", integer(1), text("m"), mapping(), mapping(), h("any")),
		module("m", mapping(entry("p", d("import", text("p"), a("short"))))),
	} {
		_, err := decl.Resolve(invalid, nil)
		expectCode(t, err, decl.Invalid)
	}
	duplicate := core.NewTuple(a("map"),
		core.NewTuple(text("X"), definition(h("any"))),
		core.NewTuple(text("X"), definition(h("any"))))
	_, err = decl.Resolve(d("module", integer(1), text("m"), mapping(), duplicate), nil)
	expectCode(t, err, decl.Invalid)
}

func TestLimitsNoPartialOutputAndConcurrentSnapshots(t *testing.T) {
	pages, people := pagination(t)
	limits := decl.DefaultLimits()
	limits.MaxNodes = 2
	p, err := decl.ResolveWithLimits(people, []value{pages}, limits)
	if p != nil {
		t.Fatal("exhaustion returned a partial program")
	}
	expectCode(t, err, decl.Exhausted)
	limits = decl.DefaultLimits()
	limits.MaxDepth = 2
	_, err = decl.PinWithLimits(people, limits)
	expectCode(t, err, decl.Exhausted)
	limits = decl.DefaultLimits()
	limits.MaxBytes = 2
	_, err = decl.PinWithLimits(a("large"), limits)
	expectCode(t, err, decl.Exhausted)
	_, err = decl.ResolveWithLimits(people, nil, decl.Limits{})
	expectCode(t, err, decl.InvalidOptions)
	var nilAtom *core.Atom
	for _, invalid := range []value{nil, nilAtom} {
		_, err := decl.Pin(invalid)
		expectCode(t, err, decl.Invalid)
	}
	var chain [][2]value
	chain = append(chain, entry("N0", definition(h("scalar", a("int")))))
	for i := 1; i <= 16; i++ {
		previous := fmt.Sprintf("N%d", i-1)
		chain = append(chain, entry(fmt.Sprintf("N%d", i), definition(h("tuple", local(previous), local(previous)))))
	}
	p = resolve(t, module("expansion", mapping(), chain...))
	result, err := p.Specialize(id(p, "N16"), nil)
	if result != nil {
		t.Fatal("expansion exhaustion returned a partial descriptor")
	}
	expectCode(t, err, decl.Exhausted)

	p = resolve(t, people, pages)
	pageID := decl.DeclarationID{Module: pin(t, pages), Name: "Page"}
	open, _ := p.Declaration(pageID)
	names := open.Parameters()
	names[0] = "Changed"
	operands := open.Expression().Operands()
	path := operands[0].Path()
	path[0] = 900
	operands[0] = decl.Operand{}
	modules := p.Modules()
	modules[0] = nil
	if open.Parameters()[0] != "T" || open.Expression().Operands()[0].Path()[0] != 2 || p.Modules()[0] == nil {
		t.Fatal("accessor leaked mutable storage")
	}
	var workers sync.WaitGroup
	for i := 0; i < 12; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			handle, _ := p.Declaration(id(p, "PeoplePage"))
			*handle.Expression() = decl.Expression{}
			*handle = decl.Declaration{}
			result, err := p.Specialize(id(p, "PeoplePage"), nil)
			if err != nil || !core.Equal(result, page(person())) {
				t.Error("concurrent specialization changed the program", err)
			}
		}()
	}
	workers.Wait()
	for _, args := range [][]value{nil, {param(0)}, {h("ref", a("outside"))}, {nil}} {
		result, err := p.Specialize(pageID, args)
		if result != nil || err == nil {
			t.Fatal("invalid specialization did not fail atomically")
		}
	}
	_, err = (*decl.Program)(nil).Specialize(pageID, nil)
	expectCode(t, err, decl.Invalid)
}

func TestWholeHandleOverwriteCannotMutateCheckedProgram(t *testing.T) {
	pages, people := pagination(t)
	p := resolve(t, people, pages)
	pageID := decl.DeclarationID{Module: pin(t, pages), Name: "Page"}
	assertUnchanged := func() {
		t.Helper()
		actual, err := p.Specialize(id(p, "PeoplePage"), nil)
		if err != nil || !core.Equal(actual, page(person())) || p.Root() != pin(t, people) {
			t.Fatal("an overwritten public handle changed the checked program", err)
		}
	}
	for _, module := range p.Modules() {
		for _, declaration := range module.Declarations() {
			*declaration = decl.Declaration{}
		}
		*module = decl.Module{}
	}
	assertUnchanged()
	module, _ := p.Module(p.Root())
	*module = decl.Module{}
	assertUnchanged()
	declaration, _ := p.Declaration(pageID)
	expression := declaration.Expression()
	for _, operand := range expression.Operands() {
		*operand.Expression() = decl.Expression{}
	}
	*expression = decl.Expression{}
	*declaration = decl.Declaration{}
	assertUnchanged()
	applied, _ := p.Declaration(id(p, "PeoplePage"))
	for _, argument := range applied.Expression().Arguments() {
		*argument = decl.Expression{}
	}
	*applied.Expression() = decl.Expression{}
	assertUnchanged()
}

func TestManyAliasesAndRepeatedLateTargets(t *testing.T) {
	dependency := module("dependency", mapping(), entry("Identity", definition(param(0), "T")))
	importedValue := importValue(t, "dependency", dependency)
	var imports, declarations [][2]value
	for i := 0; i < 1024; i++ {
		imports = append(imports, entry(fmt.Sprintf("shared_prefix_alias_%04d", i), importedValue))
		declarations = append(declarations, entry(fmt.Sprintf("Use%04d", i),
			definition(imported("shared_prefix_alias_1023", "Identity", h("scalar", a("int"))))))
	}
	p := resolve(t, module("wide", mapping(imports...), declarations...), dependency)
	actual, err := p.Specialize(id(p, "Use1023"), nil)
	if err != nil || !core.Equal(actual, h("scalar", a("int"))) {
		t.Fatal("late alias was not indexed correctly", err)
	}
}
