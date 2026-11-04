package wire_test

import (
	"testing"

	hor "github.com/Bitspark/horos/core/go"
	ground "github.com/Bitspark/horos/core/go/declarations"
	wire "github.com/Bitspark/horos/core/go/wire"
	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

func TestEmbedGroundSpecialization(t *testing.T) {
	body := h("list-of", d("param", num(0)))
	root := d("module", num(1), txt("ground.pages"), mapping(),
		mapping(entry("Page", d("type", list(txt("T")), body))))
	source, err := ground.Resolve(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := ground.DeclarationID{Module: source.Root(), Name: "Page"}
	text := h("scalar", a("utf8-text"))
	got, err := wire.Embed(source, id, []value{text}, wire.DefaultLimits())
	if err != nil || !core.Equal(got, w("data", h("list-of", text))) {
		t.Fatal("closed data embedding changed", err)
	}
	if !hor.Conforms(data.EncodeList([]value{txt("Ada")}), got.(core.Tuple).At(1)) {
		t.Fatal("embedding changed ground membership")
	}
	decl, _ := source.Declaration(id)
	if !core.Equal(decl.Expression().Value(), body) {
		t.Fatal("embedding changed the source declaration")
	}
	for _, tc := range []struct {
		name   string
		source *ground.Program
		args   []value
		limits wire.Limits
	}{
		{"missing source", nil, []value{text}, wire.DefaultLimits()},
		{"open argument", source, []value{d("param", num(0))}, wire.DefaultLimits()},
		{"Wire argument", source, []value{w("wire", textType())}, wire.DefaultLimits()},
		{"missing argument", source, nil, wire.DefaultLimits()},
		{"invalid limits", source, []value{text}, wire.Limits{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if result, err := wire.Embed(tc.source, id, tc.args, tc.limits); err == nil || result != nil {
				t.Fatal("invalid embedding returned a descriptor", err)
			}
		})
	}
	limits := wire.DefaultLimits()
	limits.MaxNodes = 1
	if result, err := wire.Embed(source, id, []value{text}, limits); result != nil || err == nil {
		t.Fatal("embedding bypassed the supplied budget")
	} else {
		expect(t, err, wire.Exhausted)
	}
}

func TestEmbedImportedGroundDeclaration(t *testing.T) {
	otherBody := h("not", d("param", num(0)))
	otherMembers := mapping(entry("Other", d("type", list(txt("T")), otherBody)))
	other := d("module", num(1), txt("ground.other"), mapping(), otherMembers)
	dependency := pin(other)
	imports := mapping(entry("other", d("import", txt("ground.other"), core.NewAtom(dependency[:]))))
	application := d("apply", d("imported", txt("other"), txt("Other")), list(h("scalar", a("int"))))
	members := mapping(entry("Alias", d("type", list(), application)))
	root := d("module", num(1), txt("ground.root"), imports, members)
	source, err := ground.Resolve(root, []value{other})
	if err != nil {
		t.Fatal(err)
	}
	id := ground.DeclarationID{Module: source.Root(), Name: "Alias"}
	got, err := wire.Embed(source, id, nil, wire.DefaultLimits())
	if err != nil || !core.Equal(got, w("data", h("not", h("scalar", a("int"))))) {
		t.Fatal("ground import or complement changed", err)
	}
	if _, err := ground.Resolve(root, nil); err == nil {
		t.Fatal("missing ground dependency accepted")
	}
}
