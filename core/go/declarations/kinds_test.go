package declarations_test

import (
	"errors"
	"slices"
	"sync"
	"testing"

	decl "github.com/Bitspark/horos/core/go/declarations"
	core "github.com/bitspark/ontos/core/go"
)

func properInterface(claim string) value { return d("proper", a(claim)) }
func constructorInterface(result string, inputs ...value) value {
	return d("constructor", list(inputs...), a(result))
}
func parameter(name string, admitted value) value {
	return d("parameter", text(name), admitted)
}
func typedDefinition(body value, result string, parameters ...value) value {
	return d("type", list(parameters...), a(result), body)
}
func apply(callee value, arguments ...value) value {
	return d("apply", callee, list(arguments...))
}
func box(item value) value {
	return h("record", a("closed"), h("field", text("value"), a("req"), item))
}
func publicProper(t *testing.T, claim decl.Claim) decl.Interface {
	t.Helper()
	result, err := decl.ProperInterface(claim)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func publicConstructor(t *testing.T, result decl.Claim, inputs ...decl.Interface) decl.Interface {
	t.Helper()
	admitted, err := decl.ConstructorInterface(inputs, result)
	if err != nil {
		t.Fatal(err)
	}
	return admitted
}
func constructorArg(t *testing.T, p *decl.Program, target decl.DeclarationID) decl.Argument {
	t.Helper()
	argument, err := p.Constructor(target)
	if err != nil {
		t.Fatal(err)
	}
	return argument
}

func higherKinds() value {
	typeInput := properInterface("type")
	dataInput := properInterface("data")
	unary := constructorInterface("type", typeInput)
	preserving := constructorInterface("data", dataInput)
	binary := constructorInterface("type", typeInput, typeInput)
	higher := constructorInterface("type", unary, typeInput)
	return module("higher", mapping(),
		entry("Person", typeDefinition(person())),
		entry("Page", typeDefinition(page(param(0)), "T")),
		entry("DataPage", definition(local("Page", param(0)), "D")),
		entry("Ignore", typeDefinition(h("scalar", a("utf8-text")), "T")),
		entry("OnlyData", definition(h("scalar", a("utf8-text")), "D")),
		entry("Send", typeDefinition(h("wire", param(0)), "T")),
		entry("Pair", typeDefinition(h("tuple", param(0), param(1)), "A", "B")),
		entry("Wrapped", typedDefinition(box(apply(param(0), param(1))), "type",
			parameter("F", unary), parameter("T", typeInput))),
		entry("Compose", typedDefinition(apply(param(0), apply(param(1), param(2))), "type",
			parameter("F", unary), parameter("G", unary), parameter("T", typeInput))),
		entry("DataWrapped", typedDefinition(box(apply(param(0), param(1))), "data",
			parameter("F", preserving), parameter("D", dataInput))),
		entry("Flip", typedDefinition(apply(param(0), param(2), param(1)), "type",
			parameter("F", binary), parameter("A", typeInput), parameter("B", typeInput))),
		entry("Higher", typedDefinition(apply(param(0), param(1), param(2)), "type",
			parameter("H", higher), parameter("F", unary), parameter("T", typeInput))),
		entry("PageResult", typeDefinition(local("Wrapped", local("Page"), local("Person")))),
		entry("IgnoreResult", typeDefinition(local("Wrapped", local("Ignore"), h("wire", local("Person"))))),
		entry("SendResult", typeDefinition(local("Wrapped", local("Send"), local("Person")))),
		entry("NestedResult", typeDefinition(local("Compose", local("Page"), local("Page"), local("Person")))),
		entry("StoredResult", typeDefinition(local("DataWrapped", local("Page"), local("Person")))),
		entry("HigherResult", typeDefinition(local("Higher", local("Wrapped"), local("Page"), local("Person")))))
}

func TestHigherKindsRetainConditionalDataPrecision(t *testing.T) {
	p := resolve(t, higherKinds())
	for _, test := range []struct {
		name string
		want value
		data bool
	}{
		{"PageResult", box(page(person())), true},
		{"IgnoreResult", box(h("scalar", a("utf8-text"))), true},
		{"SendResult", box(h("wire", person())), false},
		{"NestedResult", page(page(person())), true},
		{"StoredResult", box(page(person())), true},
		{"HigherResult", box(page(person())), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := p.Specialize(id(p, test.name), nil)
			if err != nil || !core.Equal(got, test.want) {
				t.Fatalf("wrong saturated result: %v", err)
			}
			dataResult, err := p.DataDescriptor(id(p, test.name), nil)
			if test.data {
				if err != nil || !core.Equal(dataResult, test.want) {
					t.Fatalf("lost known constructor data proof: %v", err)
				}
			} else if dataResult != nil || err == nil {
				t.Fatal("Wire result acquired a data proof")
			}
		})
	}
	personArgument := properArgs(t, p, person())[0]
	capabilityArgument := properArgs(t, p, h("wire", person()))[0]
	// Repeated reverse orders detect a result cached only by Wrapped's identity.
	for _, order := range [][]string{{"Page", "Ignore", "Send"}, {"Send", "Page", "Ignore"}, {"Ignore", "Send", "Page"}} {
		for _, name := range order {
			argument := personArgument
			want := box(page(person()))
			if name == "Ignore" {
				argument = capabilityArgument
				want = box(h("scalar", a("utf8-text")))
			} else if name == "Send" {
				want = box(h("wire", person()))
			}
			got, err := p.Specialize(id(p, "Wrapped"), []decl.Argument{constructorArg(t, p, id(p, name)), argument})
			if err != nil || !core.Equal(got, want) {
				t.Fatalf("constructor %s contaminated another environment: %v", name, err)
			}
		}
	}
	wrapped, _ := p.Declaration(id(p, "Wrapped"))
	if wrapped.DataStatus() != decl.DataConditional || wrapped.Result() != decl.TypeClaim {
		t.Fatal("concrete applications mutated the open constructor's evidence")
	}
	flipped, err := p.Specialize(id(p, "Flip"), append([]decl.Argument{constructorArg(t, p, id(p, "Pair"))}, properArgs(t, p, person(), h("wire", person()))...))
	if err != nil || !core.Equal(flipped, h("tuple", h("wire", person()), person())) {
		t.Fatal("n-ary application changed argument order", err)
	}
}

func TestKnownConstructorInterfacesRespectAdmissionAndResult(t *testing.T) {
	p := resolve(t, higherKinds())
	typeInput := publicProper(t, decl.TypeClaim)
	dataInput := publicProper(t, decl.DataClaim)
	typeToType := publicConstructor(t, decl.TypeClaim, typeInput)
	typeToData := publicConstructor(t, decl.DataClaim, typeInput)
	dataToType := publicConstructor(t, decl.TypeClaim, dataInput)
	dataToData := publicConstructor(t, decl.DataClaim, dataInput)
	for _, test := range []struct {
		name string
		want decl.Interface
		ok   bool
	}{
		{"Page", typeToType, true},
		{"Page", dataToData, true},
		{"Page", typeToData, false},
		{"Page", dataToType, true},
		{"Ignore", typeToData, true},
		{"Ignore", dataToData, true},
		{"Send", dataToData, false},
		{"Send", typeToType, true},
		{"DataPage", typeToType, false},
		{"DataPage", dataToData, true},
		{"OnlyData", typeToType, false},
		{"OnlyData", typeToData, false},
		{"OnlyData", dataToData, true},
	} {
		got, err := p.Satisfies(id(p, test.name), test.want)
		if err != nil || got != test.ok {
			t.Fatalf("%s interface result = %t, want %t: %v", test.name, got, test.ok, err)
		}
	}
	if !typeToType.Kind().Equal(dataToData.Kind()) || typeToType.Equal(dataToData) {
		t.Fatal("erased shape and refined admission were conflated")
	}
	for _, claim := range []decl.Claim{"", "other"} {
		if _, err := decl.ProperInterface(claim); err == nil {
			t.Fatal("unknown public claim was accepted")
		}
	}
	if _, err := decl.ConstructorInterface(nil, decl.TypeClaim); err == nil {
		t.Fatal("zero-input constructor interface was accepted")
	}
	if _, err := decl.ConstructorInterface([]decl.Interface{{}}, decl.TypeClaim); err == nil {
		t.Fatal("zero interface became a valid constructor domain")
	}
}

func TestNestedConstructorInterfacePolarity(t *testing.T) {
	typeInput := properInterface("type")
	dataInput := properInterface("data")
	root := module("polarity", mapping(),
		entry("NeedsPreserving", typedDefinition(apply(param(0), param(1)), "type",
			parameter("F", constructorInterface("data", dataInput)), parameter("D", dataInput))),
		entry("General", typedDefinition(apply(param(0), param(1)), "type",
			parameter("F", constructorInterface("type", typeInput)), parameter("D", dataInput))))
	p := resolve(t, root)
	pt := publicProper(t, decl.TypeClaim)
	pd := publicProper(t, decl.DataClaim)
	tt := publicConstructor(t, decl.TypeClaim, pt)
	td := publicConstructor(t, decl.DataClaim, pt)
	dd := publicConstructor(t, decl.DataClaim, pd)
	for _, test := range []struct {
		name string
		want decl.Interface
		ok   bool
	}{
		{"NeedsPreserving", publicConstructor(t, decl.TypeClaim, tt, pd), false},
		{"NeedsPreserving", publicConstructor(t, decl.DataClaim, td, pd), true},
		{"General", publicConstructor(t, decl.DataClaim, td, pd), true},
		{"General", publicConstructor(t, decl.TypeClaim, dd, pd), false},
	} {
		got, err := p.Satisfies(id(p, test.name), test.want)
		if err != nil || got != test.ok {
			t.Fatalf("nested polarity for %s = %t, want %t: %v", test.name, got, test.ok, err)
		}
	}
}

func TestHigherKindOpenBodiesAndIgnoredArguments(t *testing.T) {
	typeInput := properInterface("type")
	dataInput := properInterface("data")
	unary := constructorInterface("type", typeInput)
	preserving := constructorInterface("data", dataInput)
	base := [][2]value{
		entry("Person", typeDefinition(person())),
		entry("Page", typeDefinition(page(param(0)), "T")),
		entry("DataPage", definition(page(param(0)), "D")),
		entry("Ignore", typeDefinition(h("scalar", a("utf8-text")), "T")),
		entry("Send", typeDefinition(h("wire", param(0)), "T")),
		entry("OnlyData", definition(h("scalar", a("utf8-text")), "D")),
		entry("Wrapped", typedDefinition(box(apply(param(0), param(1))), "type", parameter("F", unary), parameter("T", typeInput))),
		entry("IgnoreF", typedDefinition(h("scalar", a("utf8-text")), "type", parameter("F", preserving))),
	}
	for _, test := range []struct {
		name string
		body value
		code decl.Code
	}{
		{"unproved result", typedDefinition(local("DataPage", apply(param(0), param(1))), "type", parameter("F", unary), parameter("D", dataInput)), decl.BoundViolation},
		{"invalid abstract input", typedDefinition(apply(param(0), h("wire", local("Person"))), "type", parameter("F", preserving)), decl.BoundViolation},
		{"ignored bad proper argument", typeDefinition(local("Ignore", local("DataPage", h("wire", local("Person"))))), decl.BoundViolation},
		{"ignored bad constructor", typeDefinition(local("IgnoreF", local("Send"))), decl.BoundViolation},
		{"ignored missing declaration", typeDefinition(local("Ignore", local("Missing"))), decl.UnknownDeclaration},
		{"body cannot widen admission", typeDefinition(local("Wrapped", local("OnlyData"), local("Person"))), decl.BoundViolation},
		{"false explicit result", typedDefinition(h("wire", param(0)), "data", parameter("D", dataInput)), decl.BoundViolation},
		{"abstract explicit result", typedDefinition(apply(param(0), param(1)), "data", parameter("F", unary), parameter("D", dataInput)), decl.BoundViolation},
	} {
		t.Run(test.name, func(t *testing.T) {
			entries := append(slices.Clone(base), entry("Bad", test.body))
			p, err := decl.Resolve(module("invalid", mapping(), entries...), nil)
			if p != nil {
				t.Fatal("invalid open definition published a model")
			}
			diagnostic(t, err, test.code, decl.SubjectSource)
		})
	}
	good := typedDefinition(local("DataPage", apply(param(0), param(1))), "data", parameter("F", preserving), parameter("D", dataInput))
	p := resolve(t, module("good", mapping(), append(slices.Clone(base), entry("Good", good))...))
	declaration, _ := p.Declaration(id(p, "Good"))
	if declaration.DataStatus() != decl.DataProven || declaration.Result() != decl.DataClaim {
		t.Fatal("explicit abstract data guarantee was not established")
	}
}

func TestSaturatedFeatureBoundaryAndSemanticReferenceCycles(t *testing.T) {
	typeInput := properInterface("type")
	unary := constructorInterface("type", typeInput)
	base := [][2]value{
		entry("Person", typeDefinition(person())),
		entry("Page", typeDefinition(page(param(0)), "T")),
		entry("Pair", typeDefinition(h("tuple", param(0), param(1)), "A", "B")),
		entry("Wrapped", typedDefinition(apply(param(0), param(1)), "type", parameter("F", unary), parameter("T", typeInput))),
	}
	for _, test := range []struct {
		name string
		body value
	}{
		{"proper argument for constructor", typeDefinition(local("Wrapped", local("Person"), local("Person")))},
		{"wrong constructor arity", typeDefinition(local("Wrapped", local("Pair"), local("Person")))},
		{"constructor as descriptor operand", typeDefinition(h("wire", local("Page")))},
		{"constructor alias", typeDefinition(local("Page"))},
		{"constructor-valued body", typedDefinition(param(0), "type", parameter("F", unary))},
		{"empty application", typeDefinition(apply(local("Person")))},
		{"partial application", typeDefinition(apply(local("Pair"), local("Person")))},
		{"over application", typeDefinition(apply(local("Page"), local("Person"), local("Person")))},
		{"application as callee", typeDefinition(apply(local("Page", local("Person")), local("Person")))},
		{"type lambda", typeDefinition(d("lambda", list(), local("Person")))},
		{"kind variable", typedDefinition(person(), "type", parameter("F", d("kind-variable", text("K"))))},
	} {
		t.Run(test.name, func(t *testing.T) {
			p, err := decl.Resolve(module("unsupported", mapping(), append(slices.Clone(base), entry("Bad", test.body))...), nil)
			if p != nil || err == nil {
				t.Fatal("unsupported or ill-kinded definition was admitted")
			}
			var detail *decl.Error
			if !errors.As(err, &detail) || detail.Subject != decl.SubjectSource {
				t.Fatal("source kind failure lost its subject", err)
			}
		})
	}
	cycle := module("cycle", mapping(),
		entry("IgnoreF", typedDefinition(h("scalar", a("utf8-text")), "type", parameter("F", unary), parameter("T", typeInput))),
		entry("Loop", typeDefinition(local("IgnoreF", local("Loop"), param(0)), "T")))
	p, err := decl.Resolve(cycle, nil)
	if p != nil {
		t.Fatal("ignored constructor cycle published a model")
	}
	diagnostic(t, err, decl.Cycle, decl.SubjectSource)
}

func TestCheckedArgumentOwnershipAndAtomicFailures(t *testing.T) {
	root := higherKinds()
	p := resolve(t, root)
	foreign := resolve(t, root)
	if p.Root() != foreign.Root() {
		t.Fatal("ownership fixture must have identical canonical identity")
	}
	proper := properArgs(t, p, person())[0]
	constructor := constructorArg(t, p, id(p, "Page"))
	foreignConstructor := constructorArg(t, foreign, id(foreign, "Page"))
	foreignProper := properArgs(t, foreign, person())[0]
	for _, arguments := range [][]decl.Argument{
		nil,
		{{}, proper},
		{foreignConstructor, proper},
		{constructor, foreignProper},
		{proper, proper},
		{constructor, constructor},
		{constructor, proper, proper},
	} {
		result, err := p.Specialize(id(p, "Wrapped"), arguments)
		if result != nil || err == nil {
			t.Fatal("bad typed invocation produced a usable result")
		}
		var detail *decl.Error
		if !errors.As(err, &detail) || detail.Subject != decl.SubjectRequest {
			t.Fatal("bad typed invocation rejected immutable source", err)
		}
	}
	if _, err := p.Constructor(id(p, "Person")); err == nil {
		t.Fatal("proper declaration acquired a constructor handle")
	}
	_, err := p.Constructor(id(p, "Missing"))
	diagnostic(t, err, decl.UnknownDeclaration, decl.SubjectRequest)
	if _, err := (*decl.Program)(nil).Proper(person()); err == nil {
		t.Fatal("nil program manufactured a proper argument")
	}
	if _, err := (*decl.Program)(nil).Constructor(id(p, "Page")); err == nil {
		t.Fatal("nil program manufactured a constructor argument")
	}
	for _, arguments := range [][]decl.Argument{
		{constructorArg(t, p, id(p, "Send")), proper},
		{constructorArg(t, p, id(p, "Ignore")), properArgs(t, p, h("wire", person()))[0]},
	} {
		result, err := p.Specialize(id(p, "DataWrapped"), arguments)
		if result != nil {
			t.Fatal("failed interface admission returned output")
		}
		diagnostic(t, err, decl.BoundViolation, decl.SubjectRequest)
	}
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			got, err := p.Specialize(id(p, "Wrapped"), []decl.Argument{constructor, proper})
			if err != nil || !core.Equal(got, box(page(person()))) {
				t.Error("concurrent constructor evaluation changed result", err)
			}
		}()
	}
	workers.Wait()
}

func findSummary(summary *decl.DataSummary, kind decl.SummaryKind) *decl.DataSummary {
	if summary == nil {
		return nil
	}
	if summary.Kind() == kind {
		return summary
	}
	for _, child := range summary.Children() {
		if found := findSummary(child, kind); found != nil {
			return found
		}
	}
	return nil
}

func TestCheckedKindsAndSummaryViewsPreserveSource(t *testing.T) {
	p := resolve(t, higherKinds())
	wrapped, _ := p.Declaration(id(p, "Wrapped"))
	shape := wrapped.Kind()
	if !shape.IsConstructor() || len(shape.Inputs()) != 2 || !shape.Inputs()[0].IsConstructor() || shape.Inputs()[1].IsConstructor() {
		t.Fatal("nested constructor shape was flattened")
	}
	call := wrapped.Expression().Operands()[0].Expression()
	if call.Kind() != decl.Application || call.KindShape().IsConstructor() || call.Callee().Kind() != decl.Parameter || !call.Callee().KindShape().IsConstructor() {
		t.Fatal("parameter-headed application lost its semantic shape")
	}
	if _, ok := call.Target(); ok {
		t.Fatal("application still exposes a declaration-only target")
	}
	callee, ok := call.Callee().Parameter()
	if !ok || callee != (decl.ParameterID{Declaration: id(p, "Wrapped"), Index: 0}) {
		t.Fatal("constructor parameter identity was replaced with a fabricated declaration")
	}
	if call.Callee().DataSummary() != nil || call.Callee().DataStatus() != "" {
		t.Fatal("constructor expression acquired a proper-type data judgment")
	}
	probe := findSummary(wrapped.DataSummary(), decl.SummaryProbe)
	if probe == nil || probe.Probe() == nil || probe.Probe().Kind() != decl.Application {
		t.Fatal("abstract application did not retain a structured Probe")
	}
	probeCallee, ok := probe.Probe().Callee().Parameter()
	if !ok || probeCallee != callee || len(probe.Probe().Arguments()) != 1 {
		t.Fatal("Probe erased its caller environment structure")
	}
	pageDeclaration, _ := p.Declaration(id(p, "Page"))
	need := findSummary(pageDeclaration.DataSummary(), decl.SummaryNeed)
	if need == nil {
		t.Fatal("Page lost its conditional used-slot summary")
	}
	needed, ok := need.Parameter()
	if !ok || needed != (decl.ParameterID{Declaration: id(p, "Page"), Index: 0}) {
		t.Fatal("Data requirement lost its exact binder identity")
	}
	for _, test := range []struct {
		name   string
		status decl.DataStatus
	}{
		{"Ignore", decl.DataProven},
		{"Send", decl.DataFailed},
		{"Page", decl.DataConditional},
		{"Wrapped", decl.DataConditional},
		{"DataWrapped", decl.DataProven},
		{"PageResult", decl.DataProven},
		{"SendResult", decl.DataFailed},
	} {
		declaration, _ := p.Declaration(id(p, test.name))
		if declaration.DataStatus() != test.status || declaration.Expression().DataStatus() != test.status {
			t.Fatalf("wrong established status for %s", test.name)
		}
	}
	inputs := wrapped.Interface().Inputs()
	inputs[0] = decl.Interface{}
	shapes := wrapped.Kind().Inputs()
	shapes[0] = decl.Kind{}
	*probe.Probe() = decl.Expression{}
	*probe = decl.DataSummary{}
	children := wrapped.DataSummary().Children()
	if len(children) != 0 {
		children[0] = nil
	}
	if !wrapped.Interface().Inputs()[0].IsConstructor() || !wrapped.Kind().Inputs()[0].IsConstructor() || findSummary(wrapped.DataSummary(), decl.SummaryProbe) == nil {
		t.Fatal("public kind/interface/summary view mutated the checked Program")
	}
}

func TestConstructorDefinitionScopeAndPinnedIdentity(t *testing.T) {
	first := module("same", mapping(),
		entry("Person", typeDefinition(h("scalar", a("int")))),
		entry("Select", typeDefinition(h("tuple", local("Person"), param(0)), "T")))
	second := module("same", mapping(),
		entry("Person", typeDefinition(h("scalar", a("bool")))),
		entry("Select", typeDefinition(h("tuple", local("Person"), param(0)), "T")))
	typeInput := properInterface("type")
	unary := constructorInterface("type", typeInput)
	root := module("consumer", mapping(
		entry("a", importValue(t, "same", first)), entry("b", importValue(t, "same", second))),
		entry("Person", typeDefinition(person())),
		entry("Use", typedDefinition(apply(param(0), param(1)), "type", parameter("F", unary), parameter("T", typeInput))),
		entry("First", typeDefinition(local("Use", imported("a", "Select"), local("Person")))),
		entry("Second", typeDefinition(local("Use", imported("b", "Select"), local("Person")))))
	p := resolve(t, root, first, second)
	for _, test := range []struct {
		name string
		tag  string
		pin  decl.Digest
	}{
		{"First", "int", pin(t, first)},
		{"Second", "bool", pin(t, second)},
	} {
		got, err := p.DataDescriptor(id(p, test.name), nil)
		if err != nil || !core.Equal(got, h("tuple", h("scalar", a(test.tag)), person())) {
			t.Fatal("constructor definition scope was rebound to its caller", err)
		}
		declaration, _ := p.Declaration(id(p, test.name))
		target, ok := declaration.Expression().Arguments()[0].Target()
		if !ok || target != (decl.DeclarationID{Module: test.pin, Name: "Select"}) {
			t.Fatal("same-spelled constructor identity was merged")
		}
	}
}

func TestHigherKindLiteralOpacityAndRecursionClosure(t *testing.T) {
	for _, literal := range []value{
		d("param", integer(900)),
		d("local", text("Missing")),
		d("imported", text("missing"), text("Missing")),
		apply(param(0), local("Missing")),
		constructorInterface("data", properInterface("data")),
		h("wire", param(0)),
	} {
		body := h("exact", literal)
		p := resolve(t, module("literal", mapping(), entry("Value", typedDefinition(body, "data"))))
		got, err := p.DataDescriptor(id(p, "Value"), nil)
		if err != nil || !core.Equal(got, body) {
			t.Fatal("literal resembling higher-kind syntax was interpreted", err)
		}
	}
	dataInput := properInterface("data")
	preserving := constructorInterface("data", dataInput)
	bad := typedDefinition(h("rec", a("x"), h("list-of", apply(param(0), h("ref", a("x"))))), "data", parameter("F", preserving))
	p, err := decl.Resolve(module("captured", mapping(), entry("Bad", bad)), nil)
	if p != nil {
		t.Fatal("Data-preserving constructor manufactured a recursion guard")
	}
	diagnostic(t, err, decl.Invalid, decl.SubjectSource)
	closed := h("rec", a("x"), h("list-of", h("ref", a("x"))))
	root := module("closed", mapping(),
		entry("Identity", definition(param(0), "D")),
		entry("Good", typedDefinition(h("rec", a("x"), h("tuple", apply(param(0), param(1)), h("ref", a("x")))), "data", parameter("F", preserving), parameter("D", dataInput))),
		entry("EmptyMessageWire", typeDefinition(h("wire", h("union")))))
	p = resolve(t, root)
	got, err := p.DataDescriptor(id(p, "Good"), []decl.Argument{constructorArg(t, p, id(p, "Identity")), properArgs(t, p, closed)[0]})
	if err != nil || !core.Equal(got, h("rec", a("x"), h("tuple", closed, h("ref", a("x"))))) {
		t.Fatal("independently closed recursive argument was captured", err)
	}
	got, err = p.DataDescriptor(id(p, "EmptyMessageWire"), nil)
	if got != nil || err == nil {
		t.Fatal("empty message type made a Wire data-only")
	}
}

func TestParameterCalleeDiagnosticRetainsBinderAndArgument(t *testing.T) {
	dataInput := properInterface("data")
	root := module("diagnostic", mapping(), entry("Bad",
		typedDefinition(apply(param(0), h("wire", person())), "type",
			parameter("F", constructorInterface("data", dataInput)))))
	p, err := decl.Resolve(root, nil)
	detail := diagnostic(t, err, decl.BoundViolation, decl.SubjectSource)
	owner := decl.DeclarationID{Module: pin(t, root), Name: "Bad"}
	if p != nil || detail.CalleeParameter != (decl.ParameterID{Declaration: owner, Index: 0}) ||
		!slices.Equal(detail.Path, []int{2, 1}) || detail.Expected.IsConstructor() || detail.Expected.Result() != decl.DataClaim {
		t.Fatalf("parameter application lost its interface context: %+v", detail)
	}
}
