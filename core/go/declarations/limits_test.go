package declarations_test

// Resource limits, atomic failure, sealed handles and immutable snapshots:
// docs/spec/declarations.md §7-§8 and the README sections "Checked kinds and
// type values" and "Resource bounds". Ported from the revision-3
// TestLimitsNoPartialOutputAndConcurrentSnapshots and
// TestWholeHandleOverwriteCannotMutateCheckedProgram.

import (
	"fmt"
	"sync"
	"testing"

	decl "github.com/Bitspark/horos/core/go/declarations"
	core "github.com/bitspark/ontos/core/go"
)

// §8: default limits are depth 256, 100,000 work units, 16 MiB and 256
// modules; depth is at most 512 and every limit must be positive.
func TestLimitValidation(t *testing.T) {
	if got := decl.DefaultLimits(); got != (decl.Limits{MaxDepth: 256, MaxNodes: 100000, MaxBytes: 16 << 20, MaxModules: 256}) {
		t.Fatalf("default limits %+v", got)
	}
	_, people := pagination(t)
	for name, change := range map[string]func(*decl.Limits){
		"zero":           func(l *decl.Limits) { *l = decl.Limits{} },
		"depth 513":      func(l *decl.Limits) { l.MaxDepth = 513 },
		"negative nodes": func(l *decl.Limits) { l.MaxNodes = -1 },
		"zero bytes":     func(l *decl.Limits) { l.MaxBytes = 0 },
		"zero modules":   func(l *decl.Limits) { l.MaxModules = 0 },
	} {
		limits := decl.DefaultLimits()
		change(&limits)
		p, err := decl.ResolveWithLimits(people, nil, limits)
		if p != nil {
			t.Fatalf("%s: returned a program", name)
		}
		if detail := expectCode(t, err, decl.InvalidOptions); detail.Subject != decl.SubjectConfiguration {
			t.Fatalf("%s: %+v", name, detail)
		}
		_, err = decl.PinWithLimits(people, limits)
		expectCode(t, err, decl.InvalidOptions)
	}
	limits := decl.DefaultLimits()
	limits.MaxDepth = 512
	pages, people := pagination(t)
	if p, err := decl.ResolveWithLimits(people, []core.Value{pages}, limits); err != nil || p.Limits() != limits {
		t.Fatalf("depth 512 is admitted and retained: %v", err)
	}
}

func TestExhaustionReturnsNothing(t *testing.T) {
	pages, people := pagination(t)
	limits := decl.DefaultLimits()
	limits.MaxNodes = 2
	p, err := decl.ResolveWithLimits(people, []core.Value{pages}, limits)
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
	var nilAtom *core.Atom
	for _, invalid := range []core.Value{nil, nilAtom} {
		_, err := decl.Pin(invalid)
		expectCode(t, err, decl.Invalid)
	}

	// 256 supplied modules include the root.
	pool := make([]core.Value, 255)
	for i := range pool {
		pool[i] = module(fmt.Sprintf("m%d", i), mapping())
	}
	root := module("root", mapping())
	if _, err := decl.Resolve(root, pool); err != nil {
		t.Fatalf("255 dependencies and a root: %v", err)
	}
	_, err = decl.Resolve(root, append(pool, module("extra", mapping())))
	expectCode(t, err, decl.Exhausted)
	limits = decl.DefaultLimits()
	limits.MaxModules = 2
	_, err = decl.ResolveWithLimits(people, []core.Value{pages}, limits)
	expectCode(t, err, decl.Exhausted)
}

// §8: a compact symbolic result can establish its contract while complete
// materialisation exceeds the output budget; repeated occurrences count.
func TestExpandedOutputIsChargedPerOccurrence(t *testing.T) {
	chain := [][2]core.Value{entry("N0", d("type", list(), kData(), h("scalar", a("int"))))}
	for i := 1; i <= 16; i++ {
		previous := fmt.Sprintf("N%d", i-1)
		chain = append(chain, entry(fmt.Sprintf("N%d", i), d("type", list(), kData(), h("tuple", local(previous), local(previous)))))
	}
	p := resolve(t, module("expansion", preludeImports(), chain...))
	top := value(t, p, "N16")
	for name, materialise := range map[string]func(decl.TypeValue) (core.Value, error){
		"Descriptor": p.Descriptor, "GroundDescriptor": p.GroundDescriptor,
	} {
		result, err := materialise(top)
		if result != nil {
			t.Fatalf("%s: exhaustion returned a partial descriptor", name)
		}
		if detail := expectCode(t, err, decl.Exhausted); detail.Subject != decl.SubjectRequest {
			t.Fatalf("%s: %+v", name, detail)
		}
	}
	small := h("tuple", h("tuple", h("scalar", a("int")), h("scalar", a("int"))), h("tuple", h("scalar", a("int")), h("scalar", a("int"))))
	if !core.Equal(ground(t, p, value(t, p, "N2")), small) {
		t.Fatal("shared references expand to every occurrence")
	}
}

// README "Checked kinds and type values": handles are sealed to the Program
// that created them, even when another Program resolved equal bytes.
func TestHandlesAreSealedToTheirProgram(t *testing.T) {
	pages, people := pagination(t)
	p := resolve(t, people, pages)
	twin := resolve(t, people, pages)
	personValue := value(t, p, "Person")
	dataKind := kind(t, p, kData())
	foreignValue := value(t, twin, "Person")
	foreignKind := kind(t, twin, kData())
	pageValue, err := p.Value(decl.DeclarationID{Module: pin(t, pages), Name: "Page"})
	if err != nil {
		t.Fatal(err)
	}
	failures := map[string]error{}
	_, failures["foreign descriptor"] = p.GroundDescriptor(foreignValue)
	_, failures["zero descriptor"] = p.Descriptor(decl.TypeValue{})
	_, failures["foreign argument"] = p.Apply(pageValue, []decl.TypeValue{foreignValue})
	_, failures["zero callee"] = p.Apply(decl.TypeValue{}, []decl.TypeValue{personValue})
	_, failures["foreign kind"] = p.Satisfies(personValue, foreignKind)
	_, failures["foreign value"] = p.Satisfies(foreignValue, dataKind)
	_, failures["zero kind"] = p.Includes(decl.KindHandle{}, dataKind)
	_, failures["nil program"] = (*decl.Program)(nil).Value(id(p, "Person"))
	_, failures["nil program proper"] = (*decl.Program)(nil).Proper(person())
	_, failures["unknown module scope"] = p.CompileKind(decl.Digest{}, kData())
	for name, err := range failures {
		if detail := expectCode(t, err, decl.Invalid); name != "unknown module scope" && detail.Subject != decl.SubjectRequest {
			t.Errorf("%s: %+v", name, detail)
		}
	}
	if verdict, err := p.Satisfies(personValue, dataKind); err != nil || verdict != decl.Proven {
		t.Fatal("own handles still work")
	}
	_, err = p.Value(decl.DeclarationID{Module: decl.PreludePin(), Name: "Data"})
	expectCode(t, err, decl.KindMismatch)
	_, err = p.Value(id(p, "Missing"))
	expectCode(t, err, decl.UnknownDeclaration)
	// Proper admits only primitive descriptors; references are not closed input.
	_, err = p.Proper(local("Person"))
	expectCode(t, err, decl.Invalid)
}

// §7/§8: checked Programs are immutable; accessors return copies, failed
// requests are atomic, and concurrent queries see one snapshot.
func TestImmutableSnapshots(t *testing.T) {
	pages, people := pagination(t)
	p := resolve(t, people, pages)
	pageID := decl.DeclarationID{Module: pin(t, pages), Name: "Page"}
	assertUnchanged := func() {
		t.Helper()
		got, err := p.GroundDescriptor(value(t, p, "PeoplePage"))
		if err != nil || !core.Equal(got, page(person())) || p.Root() != pin(t, people) {
			t.Fatal("an overwritten public handle changed the checked program", err)
		}
	}
	open, _ := p.Declaration(pageID)
	names := open.Parameters()
	names[0] = decl.TypeParameter{Name: "Changed"}
	modules := p.Modules()
	modules[0] = nil
	if open.Parameters()[0].Name != "T" || p.Modules()[0] == nil {
		t.Fatal("accessor leaked mutable storage")
	}
	for _, module := range p.Modules() {
		for _, declaration := range module.Declarations() {
			*declaration = decl.Declaration{}
		}
		*module = decl.Module{}
	}
	assertUnchanged()
	declaration, _ := p.Declaration(pageID)
	expression := declaration.Expression()
	for _, operand := range expression.Operands() {
		*operand.Expression() = decl.Expression{}
	}
	*expression = decl.Expression{}
	*declaration = decl.Declaration{}
	assertUnchanged()

	var workers sync.WaitGroup
	for i := 0; i < 12; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			handle, _ := p.Declaration(id(p, "PeoplePage"))
			*handle.Expression() = decl.Expression{}
			*handle = decl.Declaration{}
			v, err := p.Value(id(p, "PeoplePage"))
			if err != nil {
				t.Error(err)
				return
			}
			if result, err := p.GroundDescriptor(v); err != nil || !core.Equal(result, page(person())) {
				t.Error("concurrent specialisation changed the program", err)
			}
		}()
	}
	workers.Wait()
	pageValue, _ := p.Value(pageID)
	for _, args := range [][]decl.TypeValue{nil, {{}}, {value(t, p, "Person"), value(t, p, "Person")}} {
		result, err := p.Apply(pageValue, args)
		if result.Expression() != nil || err == nil {
			t.Fatal("invalid application did not fail atomically")
		}
	}
	assertUnchanged()
}
