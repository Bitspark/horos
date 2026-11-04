package declarations_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	hor "github.com/Bitspark/horos/core/go"
	decl "github.com/Bitspark/horos/core/go/declarations"
	core "github.com/bitspark/ontos/core/go"
)

func typeDefinition(body value, parameters ...string) value {
	var slots []value
	for _, name := range parameters {
		slots = append(slots, d("parameter", text(name), a("type")))
	}
	return d("type", list(slots...), body)
}

func unifiedPages() value {
	return module("example.pagination", mapping(),
		entry("Page", typeDefinition(page(param(0)), "T")),
		entry("DataPage", definition(local("Page", param(0)), "D")))
}

func TestUnifiedPaginationAndDataSpecialization(t *testing.T) {
	pages := unifiedPages()
	p := resolve(t, pages)
	capability := h("wire", person())
	for _, argument := range []value{person(), capability, page(person())} {
		got, err := p.Specialize(id(p, "Page"), []value{argument})
		if err != nil || !core.Equal(got, page(argument)) {
			t.Fatal("one Page must accept each Type argument", err)
		}
	}
	for _, argument := range []value{person(), page(person())} {
		got, err := p.DataDescriptor(id(p, "DataPage"), []value{argument})
		if err != nil || !core.Equal(got, page(argument)) || !hor.WF(got) {
			t.Fatal("data specialization changed the ground descriptor", err)
		}
	}
	for _, argument := range []value{capability, h("list-of", capability), page(capability)} {
		got, err := p.Specialize(id(p, "DataPage"), []value{argument})
		if got != nil {
			t.Fatal("failed bound returned a specialization")
		}
		detail := diagnostic(t, err, decl.BoundViolation, decl.SubjectRequest)
		if detail.Target != id(p, "DataPage") {
			t.Fatal("specialization bound lost its target")
		}
	}
	got, err := p.DataDescriptor(id(p, "Page"), []value{capability})
	if got != nil {
		t.Fatal("capability page became a ground descriptor")
	}
	expectCode(t, err, decl.BoundViolation)
	open, _ := p.Declaration(id(p, "Page"))
	if !core.Equal(open.Expression().Value(), page(param(0))) || open.Parameters()[0].Bound != decl.TypeBound {
		t.Fatal("specialization erased the original generic or its bound")
	}
	alias, _ := p.Declaration(id(p, "DataPage"))
	if target, ok := alias.Expression().Target(); !ok || target != id(p, "Page") || alias.Parameters()[0].Bound != decl.DataBound {
		t.Fatal("DataPage must remain a constrained application of the same Page")
	}
}

func TestSymbolicBoundsBeforeInstantiation(t *testing.T) {
	pages := unifiedPages()
	imports := mapping(entry("pages", importValue(t, "example.pagination", pages)))
	good := module("forward", imports,
		entry("Forward", definition(imported("pages", "DataPage", param(0)), "D")))
	resolve(t, good, pages)
	bad := module("forward", imports,
		entry("Forward", typeDefinition(imported("pages", "DataPage", param(0)), "T")))
	p, err := decl.Resolve(bad, []value{pages})
	if p != nil {
		t.Fatal("unbounded forwarding published a model")
	}
	detail := diagnostic(t, err, decl.BoundViolation, decl.SubjectSource)
	if detail.Declaration != "Forward" || len(detail.Path) != 2 || detail.Path[0] != 2 || detail.Path[1] != 1 {
		t.Fatalf("bound failure lost the argument location: %+v", detail)
	}
	for _, body := range []value{
		h("not", param(0)), h("set-of", param(0)), h("map-of", h("any"), param(0)),
		h("record", a("open"), h("field", text("value"), a("req"), param(0))),
		h("rec", a("r"), h("list-of", param(0))),
	} {
		resolve(t, module("data_operators", mapping(), entry("F", definition(body, "D"))))
		p, err := decl.Resolve(module("unbounded_operators", mapping(), entry("F", typeDefinition(body, "T"))), nil)
		if p != nil {
			t.Fatal("Type parameter was treated as data-any")
		}
		expectCode(t, err, decl.BoundViolation)
	}
}

func TestDataProofUsesActualParameterUseAndCallerScope(t *testing.T) {
	pages := unifiedPages()
	root := module("consumer", mapping(entry("p", importValue(t, "example.pagination", pages))),
		entry("Ignore", typeDefinition(h("scalar", a("utf8-text")), "T")),
		entry("WireAlias", typeDefinition(h("wire", param(0)), "T")),
		entry("DataAlias", definition(imported("p", "DataPage", local("Ignore", h("wire", person()))))),
		entry("Nested", typeDefinition(imported("p", "Page", imported("p", "Page", param(0))), "T")),
		entry("Swap", typeDefinition(h("tuple", param(1), param(0)), "A", "B")),
		entry("Swapped", typeDefinition(local("Swap", param(1), param(0)), "T", "U")))
	p := resolve(t, root, pages)
	textType := h("scalar", a("utf8-text"))
	got, err := p.DataDescriptor(id(p, "DataAlias"), nil)
	if err != nil || !core.Equal(got, page(textType)) {
		t.Fatal("unused capability argument incorrectly contaminated data result", err)
	}
	capability := h("wire", person())
	got, err = p.Specialize(id(p, "Nested"), []value{capability})
	if err != nil || !core.Equal(got, page(page(capability))) {
		t.Fatal("nested mixed application lost caller slot", err)
	}
	got, err = p.Specialize(id(p, "Swapped"), []value{person(), capability})
	if err != nil || !core.Equal(got, h("tuple", person(), capability)) {
		t.Fatal("same-spelled caller/callee parameters were captured", err)
	}
	got, err = p.DataDescriptor(id(p, "WireAlias"), []value{person()})
	if got != nil || err == nil {
		t.Fatal("data argument incorrectly implied a data result")
	}
}

func TestLibraryOwnedCapabilitiesUseUnifiedPage(t *testing.T) {
	functions := module("library.functions", mapping(),
		entry("Unit", typeDefinition(h("tuple"))),
		entry("Function", typeDefinition(h("wire", h("tuple", param(0), h("wire", param(1)))), "A", "B")))
	cells := module("library.cells", mapping(entry("fn", importValue(t, "library.functions", functions))),
		entry("Cell", typeDefinition(h("record", a("closed"),
			h("field", text("get"), a("req"), imported("fn", "Function", imported("fn", "Unit"), param(0))),
			h("field", text("set"), a("req"), imported("fn", "Function", param(0), imported("fn", "Unit")))), "T")))
	pages := unifiedPages()
	imports := mapping(entry("p", importValue(t, "example.pagination", pages)), entry("c", importValue(t, "library.cells", cells)))
	root := module("consumer", imports,
		entry("Live", typeDefinition(imported("p", "Page", imported("c", "Cell", person())))))
	p := resolve(t, root, pages, cells, functions)
	got, err := p.Specialize(id(p, "Live"), nil)
	if err != nil || got == nil || hor.WF(got) {
		t.Fatal("library Cell page was rejected or admitted as ground data", err)
	}
	bad := module("consumer", imports,
		entry("Stored", typeDefinition(imported("p", "DataPage", imported("c", "Cell", person())))))
	_, err = decl.Resolve(bad, []value{pages, cells, functions})
	expectCode(t, err, decl.BoundViolation)
	// A completely unfamiliar name composes by the same ordinary rules.
	mailbox := module("library.mailboxes", mapping(), entry("Mailbox", typeDefinition(h("wire", param(0)), "Item")))
	resolve(t, mailbox)
}

func TestUnifiedGrammarRetiresOldProfilesAndPreservesLiterals(t *testing.T) {
	oldWire := core.NewTuple(a("horos/wire/module"), integer(1), text("old"), mapping(), mapping())
	for _, root := range []value{
		d("module", integer(1), text("old"), mapping(), mapping()), oldWire,
		module("bad", mapping(), entry("T", d("type", list(text("T")), param(0)))),
		module("bad", mapping(), entry("T", d("type", list(d("parameter", text("T"), a("unknown"))), param(0)))),
		module("bad", mapping(), entry("T", typeDefinition(core.NewTuple(a("horos/wire/wire"), person())))),
		module("bad", mapping(), entry("T", typeDefinition(h("data", person())))),
		module("bad", mapping(), entry("T", typeDefinition(h("wire")))),
		module("bad", mapping(), entry("T", typeDefinition(h("wire", person(), person())))),
	} {
		p, err := decl.Resolve(root, nil)
		if p != nil {
			t.Fatal("retired or malformed syntax published a model")
		}
		expectCode(t, err, decl.Invalid)
	}
	literal := h("exact", oldWire)
	p := resolve(t, module("literal", mapping(), entry("Value", definition(literal))))
	got, err := p.DataDescriptor(id(p, "Value"), nil)
	if err != nil || !core.Equal(got, literal) || !hor.Conforms(oldWire, got) {
		t.Fatal("literal payload was interpreted as declaration syntax", err)
	}
	for _, body := range []value{h("any"), h("not", h("any")), h("union"), h("tuple"), h("record", a("closed"))} {
		p := resolve(t, module("data", mapping(), entry("T", definition(body))))
		if got, err := p.DataDescriptor(id(p, "T"), nil); err != nil || !core.Equal(got, body) {
			t.Fatal("ground type meaning changed", err)
		}
	}
}

func TestBoundFailuresInDependenciesAndClosedArguments(t *testing.T) {
	pages := unifiedPages()
	broken := module("broken", mapping(entry("p", importValue(t, "example.pagination", pages))),
		entry("Bad", typeDefinition(imported("p", "DataPage", param(0)), "T")))
	root := module("root", mapping(entry("b", importValue(t, "broken", broken))))
	_, err := decl.Resolve(root, []value{broken, pages})
	detail := diagnostic(t, err, decl.BoundViolation, decl.SubjectContext)
	if detail.Module != pin(t, broken) || detail.Dependency != pin(t, broken) {
		t.Fatal("dependency bound failure was attributed to root source")
	}
	p := resolve(t, pages)
	for _, argument := range []value{param(0), h("ref", a("outside")), h("wire", h("ref", a("outside")))} {
		got, err := p.Specialize(id(p, "Page"), []value{argument})
		if got != nil || err == nil {
			t.Fatal("open argument crossed a specialization boundary")
		}
		var detail *decl.Error
		if !errors.As(err, &detail) || detail.Subject != decl.SubjectRequest {
			t.Fatal("bad invocation was attributed to immutable source", err)
		}
	}
}

func TestClosedBoundFailuresRetainNestedOperandLocations(t *testing.T) {
	bad := h("tuple", h("not", h("wire", person())))
	root := module("bad", mapping(), entry("Value", typeDefinition(bad)))
	program, err := decl.Resolve(root, nil)
	detail := diagnostic(t, err, decl.BoundViolation, decl.SubjectSource)
	if program != nil || detail.Declaration != "Value" || !slices.Equal(detail.Path, []int{1, 1}) {
		t.Fatal("closed operand lost its source location", detail)
	}
	consumer := module("consumer", mapping(entry("bad", importValue(t, "bad", root))))
	program, err = decl.Resolve(consumer, []value{root})
	detail = diagnostic(t, err, decl.BoundViolation, decl.SubjectContext)
	if program != nil || detail.Module != pin(t, root) || detail.Dependency != pin(t, root) || !slices.Equal(detail.Path, []int{1, 1}) {
		t.Fatal("closed operand lost its dependency location", detail)
	}
	caller := module("caller", mapping(), entry("Identity", typeDefinition(param(0), "T")),
		entry("Value", typeDefinition(local("Identity", bad))))
	program, err = decl.Resolve(caller, nil)
	detail = diagnostic(t, err, decl.BoundViolation, decl.SubjectSource)
	if program != nil || detail.Declaration != "Value" || !slices.Equal(detail.Path, []int{2, 1, 1, 1}) {
		t.Fatal("argument prefix was lost or repeated", detail)
	}
}

func TestSymbolicBoundPropagationHasItsOwnWorkLimit(t *testing.T) {
	var names []string
	var members []value
	for i := 0; i < 128; i++ {
		names = append(names, fmt.Sprintf("T%d", i))
		members = append(members, param(i))
	}
	body := h("tuple", members...)
	for i := 0; i < 64; i++ {
		body = h("list-of", body)
	}
	root := module("wide", mapping(), entry("Wide", typeDefinition(body, names...)))
	limits := decl.DefaultLimits()
	limits.MaxNodes = 4096
	if _, err := decl.PinWithLimits(root, limits); err != nil {
		t.Fatal("fixture must fit raw source admission before symbolic work", err)
	}
	program, err := decl.ResolveWithLimits(root, nil, limits)
	diagnostic(t, err, decl.Exhausted, decl.SubjectRequest)
	if program != nil {
		t.Fatal("incomplete bound proof exposed a partial program")
	}
	// The same structure with known Data slots needs no Type-condition sets.
	dataRoot := module("wide", mapping(), entry("Wide", definition(body, names...)))
	if program, err := decl.ResolveWithLimits(dataRoot, nil, limits); err != nil || program == nil {
		t.Fatal("syntax/graph admission exhausted before the targeted proof phase", err)
	}
	limits.MaxNodes = 16384
	if program, err := decl.ResolveWithLimits(root, nil, limits); err != nil || program == nil {
		t.Fatal("sufficient proof budget did not complete", err)
	}
}
