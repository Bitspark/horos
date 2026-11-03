package wire_test

import (
	"fmt"
	"testing"

	hor "github.com/Bitspark/horos/core/go"
	ground "github.com/Bitspark/horos/core/go/declarations"
	wire "github.com/Bitspark/horos/core/go/wire"
	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

func TestLiftChargesAggregateClosedExpansion(t *testing.T) {
	var declarations [][2]value
	for i := 0; i < 200; i++ {
		body := h("scalar", a("int"))
		if i < 199 {
			body = glocal(fmt.Sprintf("Alias%d", i+1))
		}
		declarations = append(declarations, entry(fmt.Sprintf("Alias%d", i), gdefinition(body)))
	}
	parts := make([]value, 1000)
	for i := range parts {
		parts[i] = h("not", glocal("Alias0"))
	}
	declarations = append(declarations, entry("Root", gdefinition(h("tuple", parts...))))
	source := gresolve(t, gmodule("example.expansion", mapping(), declarations...))
	id := ground.DeclarationID{Module: source.Root(), Name: "Root"}
	declaration, _ := source.Declaration(id)
	// Each subtree individually closes under the same default limits.
	if _, err := source.Close(declaration.Expression().Operands()[0].Expression()); err != nil {
		t.Fatal(err)
	}
	got, err := wire.Lift(source, []ground.DeclarationID{id}, wire.DefaultLiftOptions())
	if got != nil {
		t.Fatal("aggregate expansion exhaustion published a derivation")
	}
	expect(t, err, wire.Exhausted)
	larger := wire.DefaultLiftOptions()
	larger.Limits.MaxNodes = 300000
	if _, err := wire.Lift(source, []ground.DeclarationID{id}, larger); err != nil {
		t.Fatalf("larger request budget should allow the same semantics: %v", err)
	}
}

func gdefinition(body value, params ...string) value {
	names := make([]value, len(params))
	for i, name := range params {
		names[i] = txt(name)
	}
	return d("type", list(names...), body)
}
func gmodule(name string, imports value, declarations ...[2]value) value {
	return d("module", num(1), txt(name), imports, mapping(declarations...))
}
func gp(i int64) value { return d("param", num(i)) }
func glocal(name string, args ...value) value {
	return d("apply", d("local", txt(name)), list(args...))
}
func gimport(name string, v value) value {
	p := pin(v)
	return d("import", txt(name), core.NewAtom(p[:]))
}
func gimported(alias, name string, args ...value) value {
	return d("apply", d("imported", txt(alias), txt(name)), list(args...))
}
func gresolve(t *testing.T, root value, deps ...value) *ground.Program {
	t.Helper()
	p, err := ground.Resolve(root, deps)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func gpage(element value) value {
	return h("record", a("closed"), h("field", txt("items"), a("req"), h("list-of", element)), h("field", txt("next"), a("opt"), h("scalar", a("utf8-text"))))
}

func TestLiftPaginationHasDistinctIdentityAndAcceptsWireArgument(t *testing.T) {
	root := gmodule("example.pagination", mapping(), entry("Page", gdefinition(gpage(gp(0)), "T")))
	source := gresolve(t, root)
	id := ground.DeclarationID{Module: source.Root(), Name: "Page"}
	derived, err := wire.Lift(source, []ground.DeclarationID{id}, wire.DefaultLiftOptions())
	if err != nil {
		t.Fatal(err)
	}
	want := module("example.pagination.wire", mapping(), entry("Page", definition(page(param(0)), "T")))
	if len(derived.Modules()) != 1 || !core.Equal(derived.Modules()[0].Value(), want) {
		t.Fatal("lift differs from independently authored canonical module")
	}
	ref := derived.Roots()[0]
	if ref.Declaration == id || ref.Profile != wire.Profile || ref.Declaration.Module != pin(want) {
		t.Fatal("derived identity/provenance collapsed")
	}
	params := derived.Parameters()
	if len(params) != 1 || params[0].Source.Declaration != id || params[0].Target != ref || params[0].Index != 0 {
		t.Fatal("missing binder correspondence")
	}
	p, err := derived.Program(ref.Declaration.Module, wire.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Specialize(ref.Declaration, []value{w("wire", textType())})
	if err != nil || !core.Equal(got, page(w("wire", textType()))) {
		t.Fatal("derived Page did not accept mixed argument", err)
	}
	if v, err := source.Specialize(id, []value{w("wire", textType())}); v != nil || err == nil {
		t.Fatal("ground slot widened")
	}
	decl, _ := source.Declaration(id)
	if !core.Equal(decl.Expression().Value(), gpage(gp(0))) {
		t.Fatal("source template changed")
	}
	// Repeated derivation has identical canonical bytes and pins.
	again, err := wire.Lift(source, []ground.DeclarationID{id}, wire.DefaultLiftOptions())
	if err != nil || again.Roots()[0] != ref || !core.Equal(again.Modules()[0].Value(), want) {
		t.Fatal("nonreproducible lifting", err)
	}
	origins := derived.Origins()
	if len(origins) == 0 {
		t.Fatal("no source origins")
	}
	for i := range origins {
		if len(origins[i].SourcePath) > 0 {
			origins[i].SourcePath[0] = 999
		}
	}
	for _, origin := range derived.Origins() {
		if len(origin.SourcePath) > 0 && origin.SourcePath[0] == 999 {
			t.Fatal("origins expose mutable storage")
		}
	}
}

func TestOrderedLiftingAndClosureStopsAtData(t *testing.T) {
	other := gmodule("other", mapping(), entry("Other", gdefinition(h("not", gp(0)), "T")))
	root := gmodule("root", mapping(entry("other", gimport("other", other))),
		entry("Root", gdefinition(h("not", gimported("other", "Other", h("scalar", a("int")))))),
		entry("Alias", gdefinition(gimported("other", "Other", h("scalar", a("int"))))))
	source := gresolve(t, root, other)
	rootID := ground.DeclarationID{Module: source.Root(), Name: "Root"}
	derived, err := wire.Lift(source, []ground.DeclarationID{rootID}, wire.DefaultLiftOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(derived.Modules()) != 1 || len(derived.Sources()) != 2 {
		t.Fatal("pruned closure lost validated dependency provenance")
	}
	want := module("root.wire", mapping(), entry("Root", definition(w("data", h("not", h("not", h("scalar", a("int"))))))))
	if !core.Equal(derived.Modules()[0].Value(), want) {
		t.Fatal("closed whole body did not embed exactly")
	}
	alias := ground.DeclarationID{Module: source.Root(), Name: "Alias"}
	for _, selected := range []ground.DeclarationID{alias, {Module: pin(other), Name: "Other"}} {
		partial, err := wire.Lift(source, []ground.DeclarationID{selected}, wire.DefaultLiftOptions())
		if partial != nil {
			t.Fatal("unsupported lift returned partial derivation")
		}
		expect(t, err, wire.UnsupportedLift)
	}
	embedded, err := wire.Embed(source, alias, nil, wire.DefaultLimits())
	if err != nil || !core.Equal(embedded, w("data", h("not", h("scalar", a("int"))))) {
		t.Fatal("explicit closed embedding failed", err)
	}
	if _, err := ground.Resolve(root, nil); err == nil {
		t.Fatal("missing original dependency did not fail")
	}
}

func TestLiftingRetainsApplicationsAndCallerSlots(t *testing.T) {
	pair := gmodule("pair", mapping(), entry("Pair", gdefinition(h("tuple", gp(1), gp(0)), "T", "U")))
	root := gmodule("swap", mapping(entry("pair", gimport("pair", pair))), entry("Swap", gdefinition(gimported("pair", "Pair", gp(1), gp(0)), "T", "U")))
	source := gresolve(t, root, pair)
	id := ground.DeclarationID{Module: source.Root(), Name: "Swap"}
	derived, err := wire.Lift(source, []ground.DeclarationID{id}, wire.DefaultLiftOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(derived.Modules()) != 2 || len(derived.Parameters()) != 4 {
		t.Fatal("dependency or binders erased")
	}
	ref := derived.Roots()[0]
	p, err := derived.Program(ref.Declaration.Module, wire.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	d, _ := p.Declaration(ref.Declaration)
	if d.Expression().Kind() != wire.Application {
		t.Fatal("application eagerly expanded")
	}
	got, err := p.Specialize(ref.Declaration, []value{textType(), w("wire", textType())})
	if err != nil || !core.Equal(got, w("tuple", textType(), w("wire", textType()))) {
		t.Fatal("caller binding lost", err)
	}
	// Independent roots in separate source modules remain separately addressable.
	many, err := wire.Lift(source, []ground.DeclarationID{id, {Module: pin(pair), Name: "Pair"}}, wire.DefaultLiftOptions())
	if err != nil || len(many.Roots()) != 2 {
		t.Fatal("multiple roots failed", err)
	}
}

func TestUnionLiteralAndUnsupportedBoundaries(t *testing.T) {
	literal := d("param", num(77))
	root := gmodule("boundaries", mapping(),
		entry("Empty", gdefinition(h("union"))),
		entry("One", gdefinition(h("union", gp(0)), "T")),
		entry("Exact", gdefinition(h("exact", literal))),
		entry("OpenSet", gdefinition(h("set-of", gp(0)), "T")))
	source := gresolve(t, root)
	for _, tc := range []struct {
		name string
		want value
	}{
		{"Empty", w("data", h("union"))},
		{"One", param(0)},
		{"Exact", w("data", h("exact", literal))},
	} {
		id := ground.DeclarationID{Module: source.Root(), Name: tc.name}
		d, err := wire.Lift(source, []ground.DeclarationID{id}, wire.DefaultLiftOptions())
		if err != nil {
			t.Fatal(err)
		}
		p, err := d.Program(d.Roots()[0].Declaration.Module, wire.DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		decl, _ := p.Declaration(d.Roots()[0].Declaration)
		if !core.Equal(decl.Expression().Value(), tc.want) {
			t.Fatal("wrong union/literal rule", tc.name)
		}
	}
	id := ground.DeclarationID{Module: source.Root(), Name: "OpenSet"}
	if d, err := wire.Lift(source, []ground.DeclarationID{id}, wire.DefaultLiftOptions()); d != nil || err == nil {
		t.Fatal("unsupported open set lifted")
	} else {
		expect(t, err, wire.UnsupportedLift)
	}
}

// This independent ground-instance interpreter uses the published layouts. It
// tests denotation, not AST equality, and is not a production mixed value checker.
func admitsGround(t value, v value) bool {
	tuple, ok := t.(core.Tuple)
	if !ok || tuple.Len() == 0 {
		return false
	}
	tag, ok := tuple.At(0).(core.Atom)
	if !ok {
		return false
	}
	switch string(tag.Bytes()) {
	case "horos/wire/data":
		return hor.Conforms(v, tuple.At(1))
	case "horos/wire/wire":
		return false
	case "horos/wire/union":
		for i := 1; i < tuple.Len(); i++ {
			if admitsGround(tuple.At(i), v) {
				return true
			}
		}
		return false
	case "horos/wire/tuple":
		items, ok := v.(core.Tuple)
		if !ok || items.Len() != tuple.Len()-1 {
			return false
		}
		for i := 0; i < items.Len(); i++ {
			if !admitsGround(tuple.At(i+1), items.At(i)) {
				return false
			}
		}
		return true
	case "horos/wire/list-of":
		items, err := data.ReadList(v)
		if err != nil {
			return false
		}
		for _, item := range items {
			if !admitsGround(tuple.At(1), item) {
				return false
			}
		}
		return true
	case "horos/wire/record":
		entries, err := data.ReadMap(v)
		if err != nil {
			return false
		}
		present := make(map[string]value)
		for _, entry := range entries {
			name, err := data.ReadText(entry[0])
			if err != nil {
				return false
			}
			present[name] = entry[1]
		}
		for i := 2; i < tuple.Len(); i++ {
			field := tuple.At(i).(core.Tuple)
			name, _ := data.ReadText(field.At(1))
			item, exists := present[name]
			if !exists {
				if core.Equal(field.At(2), a("req")) {
					return false
				}
				continue
			}
			if !admitsGround(field.At(3), item) {
				return false
			}
			delete(present, name)
		}
		return len(present) == 0
	}
	return false
}

func TestGroundMembershipAndSubstitutionCoherence(t *testing.T) {
	root := gmodule("pages", mapping(), entry("Page", gdefinition(gpage(gp(0)), "T")))
	source := gresolve(t, root)
	id := ground.DeclarationID{Module: source.Root(), Name: "Page"}
	person := h("record", a("closed"), h("field", txt("name"), a("req"), h("scalar", a("utf8-text"))))
	groundType, err := source.Specialize(id, []value{person})
	if err != nil {
		t.Fatal(err)
	}
	d, err := wire.Lift(source, []ground.DeclarationID{id}, wire.DefaultLiftOptions())
	if err != nil {
		t.Fatal(err)
	}
	p, err := d.Program(d.Roots()[0].Declaration.Module, wire.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	lifted, err := p.Specialize(d.Roots()[0].Declaration, []value{w("data", person)})
	if err != nil {
		t.Fatal(err)
	}
	closedSource := gresolve(t, gmodule("closed", mapping(), entry("Concrete", gdefinition(groundType))))
	closedID := ground.DeclarationID{Module: closedSource.Root(), Name: "Concrete"}
	other, err := wire.Lift(closedSource, []ground.DeclarationID{closedID}, wire.DefaultLiftOptions())
	if err != nil {
		t.Fatal(err)
	}
	op, err := other.Program(other.Roots()[0].Declaration.Module, wire.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	otherType, err := op.Specialize(other.Roots()[0].Declaration, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []value{
		mapping(entry("items", list())),
		mapping(entry("items", list(mapping(entry("name", txt("Ada")))))),
		mapping(entry("items", list()), entry("next", txt("cursor"))),
		mapping(entry("items", list()), entry("next", data.EncodeNull())),
		mapping(entry("items", list(txt("wrong")))), mapping(),
		mapping(entry("items", list()), entry("extra", txt("no"))),
		core.NewAtom(nil), core.NewTuple(),
	} {
		want := hor.Conforms(sample, groundType)
		if admitsGround(lifted, sample) != want || admitsGround(otherType, sample) != want {
			t.Fatal("membership/substitution changed", sample)
		}
	}
}

func TestLiftingConfigurationAndExhaustion(t *testing.T) {
	root := gmodule("names", mapping(), entry("A", gdefinition(h("any"))), entry("B", gdefinition(h("any"))))
	source := gresolve(t, root)
	aID := ground.DeclarationID{Module: source.Root(), Name: "A"}
	bID := ground.DeclarationID{Module: source.Root(), Name: "B"}
	options := wire.DefaultLiftOptions()
	options.DeclarationNames = map[ground.DeclarationID]string{aID: "Same", bID: "Same"}
	if d, err := wire.Lift(source, []ground.DeclarationID{aID, bID}, options); d != nil || err == nil {
		t.Fatal("output collision admitted")
	} else {
		expect(t, err, wire.InvalidOptions)
	}
	options = wire.DefaultLiftOptions()
	options.Limits.MaxNodes = 1
	if d, err := wire.Lift(source, []ground.DeclarationID{aID}, options); d != nil || err == nil {
		t.Fatal("exhaustion returned output")
	} else {
		expect(t, err, wire.Exhausted)
	}
	options = wire.DefaultLiftOptions()
	options.ModuleNames = map[ground.Digest]string{source.Root(): "Chosen"}
	d, err := wire.Lift(source, []ground.DeclarationID{aID}, options)
	if err != nil {
		t.Fatal(err)
	}
	options.ModuleNames[source.Root()] = "Changed"
	want := module("Chosen", mapping(), entry("A", definition(w("data", h("any")))))
	if !core.Equal(d.Modules()[0].Value(), want) {
		t.Fatal("configuration was retained by reference")
	}
}
