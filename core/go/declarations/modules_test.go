package declarations_test

// Module admission, pins, imports, the bundled horos.kinds module and
// diagnostic subjects: docs/spec/declarations.md §1, §2, §3, §4 and §8, and
// core/go/declarations/README.md "Resolution and identity".

import (
	"bytes"
	"crypto/sha256"
	"os"
	"strings"
	"testing"

	decl "github.com/Bitspark/horos/core/go/declarations"
	codec "github.com/bitspark/ontos/codec/go"
	core "github.com/bitspark/ontos/core/go"
)

// §3: the bundled module is the decoded canonical asset; its pin is the plain
// SHA-256 of its ontos-codec-v1 bytes, and the import names horos.kinds.
func TestPreludeIsTheCanonicalAsset(t *testing.T) {
	asset, err := os.ReadFile("standard/horos.kinds.ontos")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(codec.Encode(decl.Prelude()), asset) {
		t.Fatal("Prelude() is not the canonical value of the packaged asset")
	}
	if decl.PreludePin() != decl.Digest(sha256.Sum256(asset)) || pin(t, decl.Prelude()) != decl.PreludePin() {
		t.Fatal("prelude pin is not SHA-256 of its canonical bytes")
	}
	digest := decl.PreludePin()
	if !core.Equal(decl.PreludeImport(), d("import", text("horos.kinds"), core.NewAtom(digest[:]))) {
		t.Fatal("prelude import value")
	}
	// The module loads through the ordinary loader as a root of its own.
	p := resolve(t, decl.Prelude())
	m, ok := p.Module(p.Root())
	if !ok || m.Name() != "horos.kinds" || p.Root() != decl.PreludePin() || len(p.Modules()) != 1 {
		t.Fatal("standard module did not load as an ordinary module")
	}
	names := map[string]decl.DeclarationForm{}
	for _, declaration := range m.Declarations() {
		names[declaration.ID().Name] = declaration.Form()
	}
	for _, name := range []string{"Type", "Any", "AtomFamily", "Int", "Text", "Bool", "Scalar", "Exact", "List", "Seq", "Set", "Not", "Wire", "Tuple", "Compound", "UnionOf", "AndOf", "Map", "Record", "ClosedRecord", "OpenRecord", "Data"} {
		if names[name] != decl.KindDefinition {
			t.Errorf("standard kind %s missing", name)
		}
	}
	if len(names) != 22 {
		t.Errorf("standard module has %d declarations, the source defines 22", len(names))
	}
	typeKind, _ := p.Declaration(decl.DeclarationID{Module: p.Root(), Name: "Type"})
	if typeKind.KindBody().Form() != decl.KindTop || !typeKind.Sort().IsProper() {
		t.Fatal("Type is proper top")
	}
	list, _ := p.Declaration(decl.DeclarationID{Module: p.Root(), Name: "List"})
	if params := list.KindParameters(); len(params) != 1 || params[0].Name != "K" || !params[0].Sort.IsProper() {
		t.Fatal("List<K> has one proper kind parameter")
	}
	dataKind, _ := p.Declaration(decl.DeclarationID{Module: p.Root(), Name: "Data"})
	if dataKind.KindBody().Form() != decl.KindRecursive {
		t.Fatal("Data is an ordinary nu group")
	}
}

// §3/§4: names need an explicit import; the resolver supplies the prelude for
// formation, so a pinned import resolves without passing it as a dependency.
func TestPreludeNamesRequireAnExplicitImport(t *testing.T) {
	body := d("type", list(), kData(), h("scalar", a("int")))
	_, err := decl.Resolve(module("no.import", mapping(), entry("X", body)), nil)
	expectCode(t, err, decl.UnknownImport)
	p := resolve(t, module("with.import", preludeImports(), entry("X", body)))
	if len(p.Modules()) != 2 {
		t.Fatalf("closure has %d modules, want root and horos.kinds", len(p.Modules()))
	}
}

// §4: a user declaration called Data does not change the bound formation
// contracts; set-of still needs the bundled Data kind.
func TestLocalDataDoesNotReplaceFormationContracts(t *testing.T) {
	root := module("shadow", preludeImports(),
		entry("Data", d("kind", list(), properSort(), d("top", properSort()))),
		entry("Bad", d("type", list(), d("kind-local", text("Data")), h("set-of", h("wire", h("any"))))))
	_, err := decl.Resolve(root, nil)
	expectCode(t, err, decl.BoundViolation)
	// The same Wire child is admitted in a proper-type position.
	resolve(t, module("proper", preludeImports(),
		entry("Good", d("type", list(), kType(), h("list-of", h("wire", h("any")))))))
}

func pagination(t *testing.T) (core.Value, core.Value) {
	t.Helper()
	pages := module("example.pagination", preludeImports(),
		entry("Page", d("type", list(tparam("T", kType())), kType(), page(param(0)))),
		entry("DataPage", d("type", list(tparam("D", kData())), kData(), local("Page", param(0)))))
	people := module("example.people", mapping(entry("k", decl.PreludeImport()), entry("pagination", importValue(t, "example.pagination", pages))),
		entry("Person", d("type", list(), kData(), person())),
		entry("PeoplePage", d("type", list(), kData(), imported("pagination", "Page", local("Person")))))
	return pages, people
}

// §1/§4: pins are SHA-256 of canonical bytes; imports state the expected name;
// the caller supplies every transitive dependency; no implicit lookup happens.
func TestModulePinsAndImports(t *testing.T) {
	pages, people := pagination(t)
	if pin(t, pages) != decl.Digest(sha256.Sum256(codec.Encode(pages))) {
		t.Fatal("pin is not plain SHA-256 of codec-v1 bytes")
	}
	p := resolve(t, people, pages)
	if p.Root() != pin(t, people) || len(p.Modules()) != 3 {
		t.Fatal("module closure or root changed")
	}

	_, err := decl.Resolve(people, nil)
	missing := expectCode(t, err, decl.MissingDependency)
	if missing.Subject != decl.SubjectContext || missing.ImportAlias != "pagination" || missing.Dependency != pin(t, pages) || missing.ExpectedName != "example.pagination" {
		t.Fatalf("missing dependency diagnostic: %+v", missing)
	}
	wrongName := module("people", mapping(entry("p", importValue(t, "wrong.name", pages))))
	_, err = decl.Resolve(wrongName, []core.Value{pages})
	mismatch := expectCode(t, err, decl.ImportMismatch)
	if mismatch.Subject != decl.SubjectContext || mismatch.ExpectedName != "wrong.name" || mismatch.ActualName != "example.pagination" || mismatch.ImportAlias != "p" {
		t.Fatalf("import mismatch diagnostic: %+v", mismatch)
	}
	unrelated := module("unrelated", mapping(), entry("X", d("type", list(), kTop(), h("any"))))
	_, err = decl.Resolve(people, []core.Value{unrelated})
	expectCode(t, err, decl.MissingDependency)

	// An unused transitive import is still required; unreferenced pool values
	// consume admission but do not enter the closure.
	bridge := module("bridge", mapping(entry("p", importValue(t, "example.pagination", pages))))
	root := module("root", mapping(entry("bridge", importValue(t, "bridge", bridge))))
	_, err = decl.Resolve(root, []core.Value{bridge})
	expectCode(t, err, decl.MissingDependency)
	if got := len(resolve(t, root, bridge, pages, unrelated).Modules()); got != 4 {
		t.Fatalf("closure has %d modules, want root, bridge, pagination and horos.kinds", got)
	}

	// Equal module names with different pins remain distinct.
	libA := module("lib", mapping(), entry("X", d("type", list(), kTop(), h("scalar", a("int")))))
	libB := module("lib", mapping(), entry("X", d("type", list(), kTop(), h("scalar", a("bool")))))
	both := module("both", mapping(entry("a", importValue(t, "lib", libA)), entry("b", importValue(t, "lib", libB))),
		entry("Pair", d("type", list(), kTop(), h("tuple", imported("a", "X"), imported("b", "X")))))
	pb := resolve(t, both, libA, libB)
	pair, err := pb.Value(id(pb, "Pair"))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := pb.GroundDescriptor(pair); err != nil || !core.Equal(got, h("tuple", h("scalar", a("int")), h("scalar", a("bool")))) {
		t.Fatalf("same-named modules were merged: %v", err)
	}
}

func TestInvalidModuleEnvelopes(t *testing.T) {
	for name, invalid := range map[string]core.Value{
		"revision 3":           d("module", integer(3), text("m"), mapping(), mapping()),
		"revision 5":           d("module", integer(5), text("m"), mapping(), mapping()),
		"empty name segment":   d("module", integer(4), text("bad..name"), mapping(), mapping()),
		"name with a hyphen":   d("module", integer(4), text("bad-name"), mapping(), mapping()),
		"imports not a map":    d("module", integer(4), text("m"), list(), mapping()),
		"extra operand":        d("module", integer(4), text("m"), mapping(), mapping(), h("any")),
		"short pin":            module("m", mapping(entry("p", d("import", text("p"), a("short"))))),
		"hex text pin":         module("m", mapping(entry("p", d("import", text("p"), text(strings.Repeat("ab", 32)))))),
		"not a module":         h("any"),
		"declaration bad tag":  module("m", mapping(), entry("X", d("interface", list(), kTop(), h("any")))),
		"bad declaration name": module("m", mapping(), entry("1X", d("type", list(), kTop(), h("any")))),
	} {
		_, err := decl.Resolve(invalid, nil)
		detail := expectCode(t, err, decl.Invalid)
		if detail.Subject != decl.SubjectSource {
			t.Errorf("%s: subject %s, want source", name, detail.Subject)
		}
	}
	duplicate := core.NewTuple(a("map"),
		core.NewTuple(text("X"), d("type", list(), kTop(), h("any"))),
		core.NewTuple(text("X"), d("type", list(), kTop(), h("any"))))
	_, err := decl.Resolve(d("module", integer(4), text("m"), mapping(), duplicate), nil)
	expectCode(t, err, decl.Invalid)
}

// §8 / README "Guarantees and uncertainty": failures name their subject.
// Source problems in a dependency are context for the root.
func TestDiagnosticSubjects(t *testing.T) {
	broken := module("broken", mapping(), entry("X", d("type", list(), kTop(), local("Missing"))))
	root := module("root", mapping(entry("b", importValue(t, "broken", broken))))
	_, err := decl.Resolve(root, []core.Value{broken})
	detail := expectCode(t, err, decl.UnknownDeclaration)
	if detail.Subject != decl.SubjectContext || detail.Dependency != pin(t, broken) {
		t.Fatalf("dependency source failure: %+v", detail)
	}
	_, err = decl.Resolve(broken, nil)
	detail = expectCode(t, err, decl.UnknownDeclaration)
	if detail.Subject != decl.SubjectSource || detail.Module != pin(t, broken) || detail.Declaration != "X" || detail.Region != "body" {
		t.Fatalf("root source failure: %+v", detail)
	}
	_, err = decl.ResolveWithLimits(root, nil, decl.Limits{})
	if detail = expectCode(t, err, decl.InvalidOptions); detail.Subject != decl.SubjectConfiguration {
		t.Fatalf("invalid options: %+v", detail)
	}
	limits := decl.DefaultLimits()
	limits.MaxNodes = 2
	_, err = decl.ResolveWithLimits(root, []core.Value{broken}, limits)
	if detail = expectCode(t, err, decl.Exhausted); detail.Subject != decl.SubjectRequest {
		t.Fatalf("exhaustion: %+v", detail)
	}
	// A pinned dependency that is not a module is the caller's context.
	notModule := module("root", mapping(entry("x", importValue(t, "x", h("any")))))
	_, err = decl.Resolve(notModule, []core.Value{h("any")})
	if detail = expectCode(t, err, decl.Invalid); detail.Subject != decl.SubjectContext || detail.Dependency != pin(t, h("any")) {
		t.Fatalf("malformed supplied dependency: %+v", detail)
	}
}

// §4: named type and kind dependency cycles are rejected, including through
// ignored or growing arguments.
func TestDependencyCycles(t *testing.T) {
	typeOf := func(body core.Value, params ...core.Value) core.Value {
		return d("type", list(params...), kTop(), body)
	}
	for name, entries := range map[string][][2]core.Value{
		"self":            {entry("A", typeOf(local("A")))},
		"mutual":          {entry("A", typeOf(local("B"))), entry("B", typeOf(local("A")))},
		"growing":         {entry("Grow", typeOf(local("Grow", h("list-of", param(0))), tparam("T", kTop())))},
		"inside a record": {entry("A", typeOf(h("record", a("closed"), h("field", text("x"), a("opt"), local("A")))))},
		"kind self":       {entry("K", d("kind", list(), properSort(), d("kind-local", text("K"))))},
		"kind mutual": {
			entry("K", d("kind", list(), properSort(), d("kind-local", text("L")))),
			entry("L", d("kind", list(), properSort(), d("kind-local", text("K")))),
		},
	} {
		_, err := decl.Resolve(module("cycles", mapping(), entries...), nil)
		if detail := expectCode(t, err, decl.Cycle); detail.Subject != decl.SubjectSource {
			t.Errorf("%s: %+v", name, detail)
		}
	}
	// A module import cycle cannot be built from pins, but a guarded ground
	// recursion is not a declaration cycle.
	resolve(t, module("ok", mapping(), entry("L", typeOf(h("rec", a("x"), h("list-of", h("ref", a("x"))))))))
}
