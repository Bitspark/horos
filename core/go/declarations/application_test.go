package declarations_test

// Application, saturation, constructor contracts, membership verdicts and
// specialisation to ground descriptors: docs/spec/declarations.md §2, §4-§8
// and docs/spec/kinds.md §1-§5. Cases marked "ported" come from the revision-3
// tests deleted in 8096dc0, rewritten for the revision-4 vocabulary.

import (
	"testing"

	decl "github.com/Bitspark/horos/core/go/declarations"
	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

func value(t *testing.T, p *decl.Program, name string) decl.TypeValue {
	t.Helper()
	v, err := p.Value(id(p, name))
	if err != nil {
		t.Fatalf("value %s: %v", name, err)
	}
	return v
}

func proper(t *testing.T, p *decl.Program, descriptor core.Value) decl.TypeValue {
	t.Helper()
	v, err := p.Proper(descriptor)
	if err != nil {
		t.Fatalf("proper: %v", err)
	}
	return v
}

func apply(t *testing.T, p *decl.Program, callee decl.TypeValue, args ...decl.TypeValue) decl.TypeValue {
	t.Helper()
	v, err := p.Apply(callee, args)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	return v
}

func ground(t *testing.T, p *decl.Program, v decl.TypeValue) core.Value {
	t.Helper()
	out, err := p.GroundDescriptor(v)
	if err != nil {
		t.Fatalf("ground descriptor: %v", err)
	}
	return out
}

func kind(t *testing.T, p *decl.Program, expr core.Value) decl.KindHandle {
	t.Helper()
	k, err := p.CompileKind(p.Root(), expr)
	if err != nil {
		t.Fatalf("compile kind: %v", err)
	}
	return k
}

func satisfies(t *testing.T, p *decl.Program, v decl.TypeValue, k decl.KindHandle) decl.Verdict {
	t.Helper()
	verdict, err := p.Satisfies(v, k)
	if err != nil {
		t.Fatalf("satisfies: %v", err)
	}
	return verdict
}

// §5: Page<Person> and DataPage<Person> yield the same ground structure, from
// the declaration, from explicit application, and across modules (ported).
func TestPaginationSpecialisation(t *testing.T) {
	pages, people := pagination(t)
	p := resolve(t, people, pages)
	pageID := decl.DeclarationID{Module: pin(t, pages), Name: "Page"}
	open, ok := p.Declaration(pageID)
	if !ok || len(open.Parameters()) != 1 || open.Parameters()[0].Name != "T" || open.Form() != decl.TypeDefinition {
		t.Fatal("open generic declaration was lost")
	}
	if got := ground(t, p, value(t, p, "PeoplePage")); !core.Equal(got, page(person())) {
		t.Fatal("PeoplePage")
	}
	pageValue, err := p.Value(pageID)
	if err != nil {
		t.Fatal(err)
	}
	if pageValue.Sort().IsProper() {
		t.Fatal("Page denotes its constructor")
	}
	dataPage, err := p.Value(decl.DeclarationID{Module: pin(t, pages), Name: "DataPage"})
	if err != nil {
		t.Fatal(err)
	}
	personValue := value(t, p, "Person")
	for _, callee := range []decl.TypeValue{pageValue, dataPage} {
		if got := ground(t, p, apply(t, p, callee, personValue)); !core.Equal(got, page(person())) {
			t.Fatal("application changed the ground structure")
		}
	}

	// Page<Wire<Person>> is a valid proper type outside Data; DataPage cannot
	// discharge its argument contract. Descriptor keeps the Wire form.
	wire := proper(t, p, h("wire", person()))
	wirePage := apply(t, p, pageValue, wire)
	if got, err := p.Descriptor(wirePage); err != nil || !core.Equal(got, page(h("wire", person()))) {
		t.Fatalf("descriptor of a Wire page: %v", err)
	}
	got, err := p.GroundDescriptor(wirePage)
	if got != nil {
		t.Fatal("a Wire page produced a ground descriptor")
	}
	if detail := expectCode(t, err, decl.BoundViolation); detail.Subject != decl.SubjectRequest {
		t.Fatalf("ground boundary failure: %+v", detail)
	}
	_, err = p.Apply(dataPage, []decl.TypeValue{wire})
	if detail := expectCode(t, err, decl.BoundViolation); detail.Subject != decl.SubjectRequest {
		t.Fatalf("argument contract failure: %+v", detail)
	}
}

// §6: constructor values, aliases, constructor results and saturation.
func TestConstructorValuesAndSaturation(t *testing.T) {
	anyConstructor := kArrow(kType(), kType())
	text_ := h("scalar", a("utf8-text"))
	root := module("constructors", preludeImports(),
		entry("Page", d("type", list(tparam("T", kType())), kType(), page(param(0)))),
		entry("Alias", d("type", list(), anyConstructor, local("Page"))),
		entry("Pick", d("type", list(tparam("F", anyConstructor)), anyConstructor, param(0))),
		entry("PickedText", d("type", list(), kType(), d("apply", local("Pick", local("Page")), list(text_)))))
	p := resolve(t, root)
	alias := value(t, p, "Alias")
	if alias.Sort().IsProper() {
		t.Fatal("a zero-parameter alias of Page is still a constructor")
	}
	_, err := p.GroundDescriptor(alias)
	expectCode(t, err, decl.KindMismatch)
	textValue := proper(t, p, text_)
	if got := ground(t, p, apply(t, p, alias, textValue)); !core.Equal(got, page(text_)) {
		t.Fatal("Alias<text>")
	}
	if got := ground(t, p, value(t, p, "PickedText")); !core.Equal(got, page(text_)) {
		t.Fatal("Pick<Page><text>")
	}
	picked := apply(t, p, value(t, p, "Pick"), value(t, p, "Page"))
	if picked.Sort().IsProper() || !core.Equal(ground(t, p, apply(t, p, picked, textValue)), page(text_)) {
		t.Fatal("Pick's constructor result is applied at a separate level")
	}

	// Every application is saturated: no empty, partial or extra arguments,
	// and a proper value cannot be applied.
	pageValue := value(t, p, "Page")
	for name, args := range map[string][]decl.TypeValue{"none": nil, "extra": {textValue, textValue}} {
		if _, err := p.Apply(pageValue, args); err == nil {
			t.Fatalf("%s arguments were accepted", name)
		} else if detail := expectCode(t, err, decl.KindMismatch); detail.Subject != decl.SubjectRequest {
			t.Fatalf("%s: %+v", name, detail)
		}
	}
	_, err = p.Apply(textValue, []decl.TypeValue{textValue})
	expectCode(t, err, decl.KindMismatch)
	// A constructor argument where a proper one is required is ill-sorted.
	_, err = p.Apply(pageValue, []decl.TypeValue{pageValue})
	expectCode(t, err, decl.KindMismatch)

	// In source, argument lists are nonempty, and a constructor-valued alias
	// needs a constructor-sorted annotation.
	for name, entries := range map[string][2]core.Value{
		"empty argument list":       entry("Bad", d("type", list(), kType(), d("apply", local("Page"), list()))),
		"proper annotation on Page": entry("Bad", d("type", list(), kType(), local("Page"))),
		"extra source argument":     entry("Bad", d("type", list(), kType(), local("Page", text_, text_))),
	} {
		bad := module("constructors", preludeImports(),
			entry("Page", d("type", list(tparam("T", kType())), kType(), page(param(0)))), entries)
		if p, err := decl.Resolve(bad, nil); p != nil || err == nil {
			t.Fatalf("%s was admitted", name)
		}
	}
}

func shapeModule() core.Value {
	scalar, flat := kPrelude("Scalar"), kLocal("Flat")
	return module("shapes", preludeImports(),
		entry("Flat", d("kind", list(), properSort(), kOr(scalar, kPrelude("List", scalar)))),
		entry("Rows", d("kind", list(), properSort(), kPrelude("Record", flat))),
		entry("Page", d("type", list(tparam("T", kType())), kType(), page(param(0)))),
		entry("Person", d("type", list(), kData(), person())))
}

// kinds.md §1-§3 and §6: closed proper membership is exact, so a failure is
// Refuted; inclusion and constructor guarantees are sufficient rules whose
// failure is Unproved, never a false membership claim.
func TestMembershipAndInclusionVerdicts(t *testing.T) {
	p := resolve(t, shapeModule())
	data_, type_ := kind(t, p, kData()), kind(t, p, kType())
	scalar, exact := kind(t, p, kPrelude("Scalar")), kind(t, p, kPrelude("Exact"))
	flat, rows := kind(t, p, kLocal("Flat")), kind(t, p, kLocal("Rows"))
	recordOfScalar := kind(t, p, kPrelude("Record", kPrelude("Scalar")))
	scalarOrRecord := kind(t, p, kOr(kPrelude("Scalar"), kPrelude("Record", kType())))
	text_ := h("scalar", a("utf8-text"))
	cases := []struct {
		name       string
		descriptor core.Value
		kind       decl.KindHandle
		want       decl.Verdict
	}{
		{"person is data", person(), data_, decl.Proven},
		{"person is rows", person(), rows, decl.Proven},
		{"any type is a type", h("wire", h("any")), type_, decl.Proven},
		{"record with a Wire field is not data", h("record", a("closed"), h("field", text("w"), a("opt"), h("wire", h("any")))), data_, decl.Refuted},
		{"open record has an any tail", h("record", a("open"), h("field", text("x"), a("req"), text_)), recordOfScalar, decl.Refuted},
		{"closed record of scalars", h("record", a("closed"), h("field", text("x"), a("req"), text_)), recordOfScalar, decl.Proven},
		{"exact text is Exact", h("exact", text("x")), exact, decl.Proven},
		{"exact text is not Scalar", h("exact", text("x")), scalar, decl.Refuted},
		{"type union has a union root", h("union", text_, person()), scalarOrRecord, decl.Refuted},
		{"nested list is not flat", h("list-of", h("list-of", text_)), flat, decl.Refuted},
		{"recursive list is data", h("rec", a("x"), h("list-of", h("ref", a("x")))), data_, decl.Proven},
	}
	for _, c := range cases {
		if got := satisfies(t, p, proper(t, p, c.descriptor), c.kind); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}

	for _, c := range []struct {
		name             string
		actual, expected decl.KindHandle
		want             decl.Verdict
	}{
		{"data within type", data_, type_, decl.Proven},
		{"rows within data", rows, data_, decl.Proven},
		{"scalar within flat", scalar, flat, decl.Proven},
		{"type is not shown within data", type_, data_, decl.Unproved},
		{"flat is not shown within scalar", flat, scalar, decl.Unproved},
	} {
		got, err := p.Includes(c.actual, c.expected)
		if err != nil || got != c.want {
			t.Errorf("%s: %s %v, want %s", c.name, got, err, c.want)
		}
	}

	// Page establishes Data => Data and Scalar => Rows; Flat => Rows does not
	// hold (a list element nests a list in items), and Satisfies may not
	// claim it. The closed instance is refuted exactly.
	pageValue := value(t, p, "Page")
	for name, c := range map[string]struct {
		arrow core.Value
		want  decl.Verdict
	}{
		"data to data":   {kArrow(kData(), kData()), decl.Proven},
		"scalar to rows": {kArrow(kLocal("Rows"), kPrelude("Scalar")), decl.Proven},
	} {
		if got := satisfies(t, p, pageValue, kind(t, p, c.arrow)); got != c.want {
			t.Errorf("%s: %s, want %s", name, got, c.want)
		}
	}
	if got := satisfies(t, p, pageValue, kind(t, p, kArrow(kLocal("Rows"), kLocal("Flat")))); got == decl.Proven {
		t.Error("Page established Flat => Rows")
	}
	nested := apply(t, p, pageValue, proper(t, p, h("list-of", text_)))
	if got := satisfies(t, p, nested, rows); got != decl.Refuted {
		t.Errorf("Page<List<text>> in Rows: %s, want refuted", got)
	}
	// A sort mismatch is an error, not a verdict.
	if _, err := p.Satisfies(pageValue, data_); err == nil {
		t.Error("a constructor was judged against a proper kind")
	} else {
		expectCode(t, err, decl.KindMismatch)
	}
}

// kinds.md §5: abstract constructors expose their declared contracts.
// Required obligations that stay Unproved block the Program.
//
// These run with MaxNodes raised to 2,000,000: under the default 100,000 work
// units, contracts that mention Data exhaust (F: Data => Data, A: Data needs
// about 544,000 units; the joint case below about 262,000). The documented
// defaults stay (declarations.md §8); task 5b937075 makes the engine cheaper,
// and that fix moves these cases back to DefaultLimits.
func TestAbstractConstructorContracts(t *testing.T) {
	limits := decl.DefaultLimits()
	limits.MaxNodes = 2_000_000
	flat, rows := kLocal("Flat"), kLocal("Rows")
	unary := d("sort-arrow", list(properSort()), properSort())
	base := func(entries ...[2]core.Value) core.Value {
		scalar := kPrelude("Scalar")
		return module("abstract", preludeImports(), append([][2]core.Value{
			entry("Flat", d("kind", list(), properSort(), kOr(scalar, kPrelude("List", scalar)))),
			entry("Rows", d("kind", list(), properSort(), kPrelude("Record", flat))),
		}, entries...)...)
	}
	call := d("apply", param(0), list(param(1)))
	flatToRows, flatToData, flatToFlat := kArrow(rows, flat), kArrow(kData(), flat), kArrow(flat, flat)
	p, err := decl.ResolveWithLimits(base(
		// F: Flat => Rows and A: Flat establish F<A> : Rows ...
		entry("Use", d("type", list(tparam("F", flatToRows), tparam("A", flat)), rows, call)),
		// ... and a record containing it has Record<Rows> structure.
		entry("Wrapped", d("type", list(tparam("F", flatToRows), tparam("A", flat)), kPrelude("Record", rows),
			h("record", a("closed"), h("field", text("value"), a("req"), call)))),
		// (A => B) & (A => C) applied to A establishes B & C.
		entry("Joint", d("type", list(tparam("F", d("and", unary, list(flatToRows, flatToData))), tparam("A", flat)),
			d("and", properSort(), list(rows, kData())), call))),
		nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	// F: Data => Data and A: Data establish F<A> : Data.
	if _, err := decl.ResolveWithLimits(base(
		entry("Higher", d("type", list(tparam("F", kArrow(kData(), kData())), tparam("A", kData())), kData(), call))),
		nil, limits); err != nil {
		t.Fatal(err)
	}
	if _, ok := p.Declaration(id(p, "Joint")); !ok {
		t.Fatal("joint arrow contracts")
	}
	for name, entries := range map[string][2]core.Value{
		// F: Flat => Flat might return text, which is not Rows.
		"weaker promise": entry("Weak", d("type", list(tparam("F", flatToFlat), tparam("A", flat)), rows, call)),
		// An open body must hold under its original contracts.
		"open body under Type": entry("Open", d("type", list(tparam("T", kType())), kData(), h("list-of", param(0)))),
		// (A => B) | (A => C) applied to A establishes only B | C.
		"union of promises": entry("Either", d("type", list(tparam("F", d("or", unary, list(flatToRows, flatToFlat))), tparam("A", flat)),
			rows, call)),
		// No arrow admits the List<Scalar> case of A.
		"argument case not admitted": entry("Narrow", d("type", list(tparam("F", kArrow(flat, kPrelude("Scalar"))), tparam("A", flat)), flat, call)),
	} {
		_, err := decl.ResolveWithLimits(base(entries), nil, limits)
		if detail := expectCode(t, err, decl.UnprovedObligation); detail.Subject != decl.SubjectSource {
			t.Errorf("%s: %+v", name, detail)
		}
	}
}

// §6: all argument contracts apply even when the body ignores the argument.
func TestIgnoredArgumentsAreChecked(t *testing.T) {
	ignore := entry("Ignore", d("type", list(tparam("T", kData())), kType(), h("scalar", a("int"))))
	wire := h("wire", h("any"))
	_, err := decl.Resolve(module("ignored", preludeImports(), ignore,
		entry("Bad", d("type", list(), kType(), local("Ignore", wire)))), nil)
	detail := expectCode(t, err, decl.BoundViolation)
	if detail.Subject != decl.SubjectSource || detail.Declaration != "Bad" {
		t.Fatalf("ignored argument failure: %+v", detail)
	}
	p := resolve(t, module("ignored", preludeImports(), ignore))
	_, err = p.Apply(value(t, p, "Ignore"), []decl.TypeValue{proper(t, p, wire)})
	expectCode(t, err, decl.BoundViolation)
	if got := ground(t, p, apply(t, p, value(t, p, "Ignore"), proper(t, p, person()))); !core.Equal(got, h("scalar", a("int"))) {
		t.Fatal("ignored argument result")
	}
}

// §4: parameters are substituted only in schema-designated type positions
// (ported TestLiteralBoundaryAndDescriptorPositions).
func TestSpecialisationPositionsAndLiteralOpacity(t *testing.T) {
	null := h("exact", data.EncodeNull())
	for _, test := range []struct {
		name       string
		body, want core.Value
	}{
		{"tuple", h("tuple", param(0)), h("tuple", person())},
		{"seq", h("seq", param(0)), h("seq", person())},
		{"compound", h("compound", a("item"), param(0)), h("compound", a("item"), person())},
		{"list", h("list-of", param(0)), h("list-of", person())},
		{"set", h("set-of", param(0)), h("set-of", person())},
		{"map", h("map-of", param(0), param(0)), h("map-of", person(), person())},
		{"record", page(param(0)), page(person())},
		{"open record", h("record", a("open"), h("field", text("x"), a("opt"), param(0))), h("record", a("open"), h("field", text("x"), a("opt"), person()))},
		{"union", h("union", param(0), null), h("union", person(), null)},
		{"intersection", h("and", param(0), h("any")), h("and", person(), h("any"))},
		{"complement", h("not", param(0)), h("not", person())},
		{"recursion", h("rec", a("x"), h("tuple", param(0), h("ref", a("x")))), h("rec", a("x"), h("tuple", person(), h("ref", a("x"))))},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := resolve(t, module("positions", preludeImports(), entry("Generic", d("type", list(tparam("T", kData())), kData(), test.body))))
			if got := ground(t, p, apply(t, p, value(t, p, "Generic"), proper(t, p, person()))); !core.Equal(got, test.want) {
				t.Fatalf("wrong parameterised position")
			}
		})
	}
	// Literals resembling parameters, references or applications stay data.
	for name, literal := range map[string]core.Value{
		"parameter":   h("exact", param(0)),
		"application": h("exact", d("apply", d("imported", text("missing"), text("Absent")), list(param(999)))),
		"kind syntax": h("exact", kData()),
	} {
		p := resolve(t, module("literal", preludeImports(), entry("Literal", d("type", list(), kData(), literal))))
		if got := ground(t, p, value(t, p, "Literal")); !core.Equal(got, literal) {
			t.Fatalf("%s literal was interpreted", name)
		}
	}
}

// §4: constructors retain their definition environment; arguments retain the
// caller's bindings (ported TestCallerBindingNestedGenericsAndIdentity).
func TestCallerBindingAndBinderIdentity(t *testing.T) {
	pages := module("pages", preludeImports(),
		entry("Person", d("type", list(), kType(), h("scalar", a("int")))),
		entry("Page", d("type", list(tparam("T", kType())), kType(), page(param(0)))))
	root := module("people", mapping(entry("k", decl.PreludeImport()), entry("pages", importValue(t, "pages", pages))),
		entry("Person", d("type", list(), kType(), person())),
		entry("PeoplePage", d("type", list(), kType(), imported("pages", "Page", local("Person")))),
		entry("Nested", d("type", list(tparam("T", kType())), kType(), imported("pages", "Page", imported("pages", "Page", param(0))))),
		entry("First", d("type", list(tparam("T", kType())), kType(), param(0))),
		entry("Second", d("type", list(tparam("T", kType())), kType(), param(0))),
		entry("Swap", d("type", list(tparam("A", kType()), tparam("B", kType())), kType(), h("tuple", param(1), param(0)))))
	p := resolve(t, root, pages)
	if !core.Equal(ground(t, p, value(t, p, "PeoplePage")), page(person())) {
		t.Fatal("callee's same-spelled Person captured the caller argument")
	}
	personValue := value(t, p, "Person")
	if !core.Equal(ground(t, p, apply(t, p, value(t, p, "Nested"), personValue)), page(page(person()))) {
		t.Fatal("nested application lost its original binder")
	}
	first, _ := p.Declaration(id(p, "First"))
	second, _ := p.Declaration(id(p, "Second"))
	b1, ok1 := first.Expression().Parameter()
	b2, ok2 := second.Expression().Parameter()
	if !ok1 || !ok2 || b1 == b2 || b1.Index != 0 || b2.Index != 0 || b1.Declaration != id(p, "First") {
		t.Fatal("same-spelled binders were merged")
	}
	swapped := apply(t, p, value(t, p, "Swap"), proper(t, p, h("scalar", a("int"))), personValue)
	if !core.Equal(ground(t, p, swapped), h("tuple", person(), h("scalar", a("int")))) {
		t.Fatal("parameter order was not preserved")
	}
}

// §2/§4: malformed declarations fail with no partial Program
// (ported TestRejectedDeclarations and TestRecursionClosureAtApplicationBoundaries).
func TestRejectedDeclarations(t *testing.T) {
	type rejected struct {
		name   string
		params []core.Value
		result core.Value
		body   core.Value
		code   decl.Code
	}
	tT := tparam("T", kData())
	for _, test := range []rejected{
		{"unbound parameter", nil, kData(), param(0), decl.Invalid},
		{"parameter out of range", []core.Value{tT}, kData(), param(1), decl.Invalid},
		{"negative parameter", []core.Value{tT}, kData(), param(-1), decl.Invalid},
		{"extra parameter operand", []core.Value{tT}, kData(), d("param", integer(0), integer(1)), decl.Invalid},
		{"duplicate parameter", []core.Value{tT, tT}, kData(), param(0), decl.Invalid},
		{"invalid parameter name", []core.Value{tparam("T.A", kData())}, kData(), param(0), decl.Invalid},
		{"unknown target", nil, kData(), local("Missing"), decl.UnknownDeclaration},
		{"unknown import", nil, kData(), imported("missing", "X"), decl.UnknownImport},
		{"kind in a type position", nil, kData(), kData(), decl.Invalid},
		{"type reference in a kind position", nil, local("Other"), h("any"), decl.Invalid},
		{"unknown descriptor", []core.Value{tT}, kData(), h("function", param(0)), decl.Invalid},
		{"parameter as scalar selector", []core.Value{tT}, kData(), h("scalar", param(0)), decl.Invalid},
		{"parameter as record mode", []core.Value{tT}, kData(), h("record", param(0)), decl.Invalid},
		{"parameter as field name", []core.Value{tT}, kData(), h("record", a("closed"), h("field", param(0), a("req"), h("any"))), decl.Invalid},
		{"duplicate field", []core.Value{tT}, kData(), h("record", a("closed"), h("field", text("x"), a("req"), param(0)), h("field", text("x"), a("opt"), param(0))), decl.Invalid},
		{"unguarded rec", nil, kData(), h("rec", a("x"), h("ref", a("x"))), decl.Invalid},
		{"unbound ref", nil, kData(), h("list-of", h("ref", a("x"))), decl.Invalid},
		{"capturing an outer ref through an argument", nil, kData(), h("rec", a("x"), h("list-of", local("Identity", h("ref", a("x"))))), decl.Invalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := module("invalid", preludeImports(),
				entry("Identity", d("type", list(tparam("T", kData())), kData(), param(0))),
				entry("Other", d("type", list(), kData(), h("any"))),
				entry("Generic", d("type", list(test.params...), test.result, test.body)))
			p, err := decl.Resolve(root, nil)
			if p != nil {
				t.Fatal("failure returned a partial program")
			}
			expectCode(t, err, test.code)
		})
	}
	// Closed nested recursion passes through an application unchanged, and a
	// guarded but uninhabited recursion is well formed.
	closed := h("rec", a("x"), h("list-of", h("ref", a("x"))))
	p := resolve(t, module("closed", preludeImports(),
		entry("Wrap", d("type", list(tparam("T", kData())), kData(), h("rec", a("x"), h("list-of", param(0))))),
		entry("Applied", d("type", list(), kData(), local("Wrap", closed))),
		entry("Empty", d("type", list(), kData(), h("rec", a("x"), h("tuple", h("ref", a("x"))))))))
	if !core.Equal(ground(t, p, value(t, p, "Applied")), h("rec", a("x"), h("list-of", closed))) {
		t.Fatal("closed nested recursion was captured or rewritten")
	}
}

// §2/§4 and kinds.md §2-§4: kind definitions are sorted, guarded and use
// only the two metadata selectors.
func TestKindDefinitions(t *testing.T) {
	variable := d("recursive-var", integer(0), integer(0))
	listOf := func(k core.Value) core.Value { return d("node", text("list-of"), mapping(), d("item", k)) }
	scalarNode := func(selectors core.Value) core.Value { return d("node", text("scalar"), selectors, d("epsilon")) }
	kindDecl := func(body core.Value) core.Value { return d("kind", list(), properSort(), body) }
	resolve(t, module("kinds", mapping(),
		entry("Lists", kindDecl(d("nu", list(listOf(variable)), integer(0)))),
		entry("Text", kindDecl(scalarNode(mapping(entry("selector", text("utf8-text")))))),
		entry("Closed", kindDecl(d("node", text("record"), mapping(entry("mode", text("closed"))), d("star", d("item", d("top", properSort())))))),
		entry("Pair", d("kind", list(d("kind-parameter", text("K"), properSort())), properSort(),
			d("node", text("tuple"), mapping(), d("concat", list(d("item", d("kind-param", integer(0))), d("item", d("kind-param", integer(0))))))))))
	for name, body := range map[string]core.Value{
		"unguarded variable":       d("nu", list(variable), integer(0)),
		"Boolean-only cycle":       d("nu", list(kOr(variable, scalarNode(mapping()))), integer(0)),
		"entry out of range":       d("nu", list(listOf(variable)), integer(1)),
		"variable outside a group": listOf(variable),
		"unknown selector key":     scalarNode(mapping(entry("mode", text("open")))),
		"unknown selector value":   scalarNode(mapping(entry("selector", text("decimal")))),
		"one-operand union":        d("or", properSort(), list(scalarNode(mapping()))),
		"empty concat":             d("node", text("tuple"), mapping(), d("concat", list())),
		"unbound kind parameter":   d("kind-param", integer(0)),
		"arrow in a proper result": kArrow(d("top", properSort()), d("top", properSort())),
		"empty arrow input list":   d("arrow", list(), d("top", properSort())),
		"unknown kind reference":   kLocal("Missing"),
		"type name used as a kind": kLocal("T"),
	} {
		_, err := decl.Resolve(module("kinds", mapping(), entry("T", d("type", list(), kTop(), h("any"))), entry("K", kindDecl(body))), nil)
		if err == nil {
			t.Errorf("%s was admitted", name)
		}
	}
}
