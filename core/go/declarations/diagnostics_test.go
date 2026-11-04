package declarations_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	decl "github.com/Bitspark/horos/core/go/declarations"
	core "github.com/bitspark/ontos/core/go"
)

func TestFailureCategoriesSeparateContextAndSource(t *testing.T) {
	_, people := pagination(t)
	missingProgram, missingErr := decl.Resolve(people, nil)
	unknown := module("unknown", mapping(), entry("X", definition(local("Missing"))))
	unknownProgram, unknownErr := decl.Resolve(unknown, nil)
	var missingDetail, unknownDetail *decl.Error
	if missingProgram != nil || unknownProgram != nil ||
		!errors.As(missingErr, &missingDetail) || !errors.As(unknownErr, &unknownDetail) {
		t.Fatalf("expected structured failures without programs: %v, %v", missingErr, unknownErr)
	}
	if missingDetail.Code == unknownDetail.Code {
		t.Fatalf("missing dependency and unknown declaration have the same machine-readable category: %s", missingDetail.Code)
	}
}

func TestInvalidOptionsAreNotMalformedDeclarations(t *testing.T) {
	valid := module("options", mapping())
	optionsProgram, optionsErr := decl.ResolveWithLimits(valid, nil, decl.Limits{})
	malformedProgram, malformedErr := decl.Resolve(h("any"), nil)
	var optionsDetail, malformedDetail *decl.Error
	if optionsProgram != nil || malformedProgram != nil ||
		!errors.As(optionsErr, &optionsDetail) || !errors.As(malformedErr, &malformedDetail) {
		t.Fatalf("expected structured failures without programs: %v, %v", optionsErr, malformedErr)
	}
	if optionsDetail.Code == malformedDetail.Code {
		t.Fatalf("invalid options and malformed source have the same machine-readable category: %s", optionsDetail.Code)
	}
}

func diagnostic(t *testing.T, err error, code decl.Code, subject decl.Subject) *decl.Error {
	t.Helper()
	var detail *decl.Error
	if !errors.As(fmt.Errorf("consumer: %w", err), &detail) || detail.Code != code || detail.Subject != subject {
		t.Fatalf("want %s about %s, got %v", code, subject, err)
	}
	return detail
}

func TestDependencyDiagnosticSubjectsAndLocations(t *testing.T) {
	pages, people := pagination(t)
	_, err := decl.Resolve(people, nil)
	missing := diagnostic(t, err, decl.MissingDependency, decl.SubjectContext)
	if missing.Module != pin(t, people) || missing.ImportAlias != "pagination" ||
		missing.Dependency != pin(t, pages) || missing.ExpectedName != "example.pagination" {
		t.Fatalf("missing dependency lost its importing location or expectation: %+v", missing)
	}

	// An unrelated unlabelled pool value does not contradict an import assertion.
	other := module("other", mapping())
	_, err = decl.Resolve(people, []value{other})
	diagnostic(t, err, decl.MissingDependency, decl.SubjectContext)
	if _, err := decl.Resolve(people, []value{other, pages}); err != nil {
		t.Fatalf("unreferenced pool value changed the meaning of a complete context: %v", err)
	}

	bridge := module("bridge", mapping(entry("p", importValue(t, "example.pagination", pages))))
	root := module("root", mapping(entry("b", importValue(t, "bridge", bridge))))
	_, err = decl.Resolve(root, []value{bridge})
	transitive := diagnostic(t, err, decl.MissingDependency, decl.SubjectContext)
	if transitive.Module != pin(t, bridge) || transitive.ImportAlias != "p" || transitive.Dependency != pin(t, pages) {
		t.Fatalf("outer import overwrote the missing transitive import location: %+v", transitive)
	}

	wrong := module("wrong", mapping(entry("p", importValue(t, "expected.name", pages))))
	_, err = decl.Resolve(wrong, []value{pages})
	mismatch := diagnostic(t, err, decl.ImportMismatch, decl.SubjectContext)
	if mismatch.Module != pin(t, wrong) || mismatch.ImportAlias != "p" || mismatch.Dependency != pin(t, pages) ||
		mismatch.ExpectedName != "expected.name" || mismatch.ActualName != "example.pagination" {
		t.Fatalf("import mismatch lost authoritative details: %+v", mismatch)
	}

	malformed := h("any")
	_, err = decl.Resolve(malformed, nil)
	source := diagnostic(t, err, decl.Invalid, decl.SubjectSource)
	if source.Module != pin(t, malformed) {
		t.Fatal("malformed root lost its known document pin")
	}
	root = module("bad_context", mapping(entry("bad", importValue(t, "bad", malformed))))
	_, err = decl.Resolve(root, []value{malformed})
	context := diagnostic(t, err, decl.Invalid, decl.SubjectContext)
	if context.Module != pin(t, root) || context.ImportAlias != "bad" || context.Dependency != pin(t, malformed) {
		t.Fatalf("malformed dependency was confused with root source: %+v", context)
	}
	_, err = decl.Resolve(module("root", mapping()), []value{nil})
	diagnostic(t, err, decl.Invalid, decl.SubjectContext)
}

func TestUnknownTargetsRetainSourceLocation(t *testing.T) {
	root := module("unknown", mapping(), entry("X", definition(h("list-of", local("Missing")))))
	_, err := decl.Resolve(root, nil)
	unknown := diagnostic(t, err, decl.UnknownDeclaration, decl.SubjectSource)
	if unknown.Module != pin(t, root) || unknown.Declaration != "X" || !slices.Equal(unknown.Path, []int{1}) ||
		unknown.Target != (decl.DeclarationID{Module: pin(t, root), Name: "Missing"}) {
		t.Fatalf("unknown declaration lost its owner, tuple path or target: %+v", unknown)
	}

	root = module("unknown_import", mapping(), entry("X", definition(imported("missing", "Missing"))))
	_, err = decl.Resolve(root, nil)
	alias := diagnostic(t, err, decl.UnknownImport, decl.SubjectSource)
	if alias.Module != pin(t, root) || alias.Declaration != "X" || alias.ImportAlias != "missing" {
		t.Fatalf("unknown alias lost its source location: %+v", alias)
	}

	pages, _ := pagination(t)
	root = module("unknown_imported_target", mapping(entry("p", importValue(t, "example.pagination", pages))),
		entry("X", definition(imported("p", "Missing"))))
	_, err = decl.Resolve(root, []value{pages})
	unknown = diagnostic(t, err, decl.UnknownDeclaration, decl.SubjectSource)
	if unknown.Module != pin(t, root) || unknown.Declaration != "X" || unknown.ImportAlias != "p" ||
		unknown.Target != (decl.DeclarationID{Module: pin(t, pages), Name: "Missing"}) {
		t.Fatalf("unknown imported target was rebound to its caller: %+v", unknown)
	}

	root = module("cycle", mapping(), entry("X", definition(local("X"))))
	_, err = decl.Resolve(root, nil)
	cycle := diagnostic(t, err, decl.Cycle, decl.SubjectSource)
	if cycle.Module != pin(t, root) || cycle.Declaration != "X" {
		t.Fatal("wrapped cycle error lost its source location")
	}
}

func TestInvalidConfigurationAndExhaustionAreNotSourceRejection(t *testing.T) {
	pages, people := pagination(t)
	_, err := decl.ResolveWithLimits(people, nil, decl.Limits{})
	diagnostic(t, err, decl.InvalidOptions, decl.SubjectConfiguration)
	_, err = decl.PinWithLimits(people, decl.Limits{})
	diagnostic(t, err, decl.InvalidOptions, decl.SubjectConfiguration)

	limits := decl.DefaultLimits()
	limits.MaxNodes = 2
	program, err := decl.ResolveWithLimits(people, []value{pages}, limits)
	if program != nil {
		t.Fatal("exhaustion published a partial program")
	}
	diagnostic(t, err, decl.Exhausted, decl.SubjectRequest)
	program, err = decl.Resolve(people, []value{pages})
	if err != nil {
		t.Fatalf("unchanged source did not succeed with sufficient resources: %v", err)
	}
	_, err = (*decl.Program)(nil).Specialize(decl.DeclarationID{}, nil)
	diagnostic(t, err, decl.Invalid, decl.SubjectRequest)
	_, err = program.Specialize(decl.DeclarationID{Module: program.Root(), Name: "Missing"}, nil)
	diagnostic(t, err, decl.UnknownDeclaration, decl.SubjectRequest)
}

func TestSpecializationArgumentsAreRequestFailures(t *testing.T) {
	pages, people := pagination(t)
	program := resolve(t, people, pages)
	pageID := decl.DeclarationID{Module: pin(t, pages), Name: "Page"}
	for _, argument := range []value{nil, a("not-a-descriptor"), param(0), h("ref", a("outside"))} {
		_, err := program.Proper(argument)
		diagnostic(t, err, decl.Invalid, decl.SubjectRequest)
		unchanged, err := program.Specialize(id(program, "PeoplePage"), nil)
		if err != nil || !core.Equal(unchanged, page(person())) {
			t.Fatalf("invalid specialization argument changed checked source: %v", err)
		}
	}
	literal := h("exact", param(0))
	result, err := program.Specialize(pageID, properArgs(t, program, literal))
	if err != nil || !core.Equal(result, page(literal)) {
		t.Fatalf("declaration-shaped literal was not a valid ground argument: %v", err)
	}
}

func TestDependencyBodyFailuresKeepContextSubject(t *testing.T) {
	tests := []struct {
		name string
		body value
		code decl.Code
		path []int
	}{
		{"unknown target", h("list-of", local("Missing")), decl.UnknownDeclaration, []int{1}},
		{"invalid type", h("list-of", a("not-a-descriptor")), decl.Invalid, []int{1}},
		{"cycle", local("Broken"), decl.Cycle, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dependency := module("dependency", mapping(), entry("Broken", definition(test.body)))
			dependencyPin := pin(t, dependency)
			_, err := decl.Resolve(dependency, nil)
			diagnostic(t, err, test.code, decl.SubjectSource)
			for _, transitive := range []bool{false, true} {
				t.Run(fmt.Sprintf("transitive=%t", transitive), func(t *testing.T) {
					importedModule := dependency
					importedName := "dependency"
					pool := []value{dependency}
					if transitive {
						importedModule = module("bridge", mapping(entry("dep", importValue(t, "dependency", dependency))))
						importedName = "bridge"
						pool = append(pool, importedModule)
					}
					root := module("root", mapping(entry("dep", importValue(t, importedName, importedModule))))
					program, err := decl.Resolve(root, pool)
					if program != nil {
						t.Fatal("bad dependency body published a partial program")
					}
					detail := diagnostic(t, err, test.code, decl.SubjectContext)
					if detail.Module != dependencyPin || detail.Declaration != "Broken" ||
						detail.Dependency != dependencyPin || !slices.Equal(detail.Path, test.path) {
						t.Fatalf("dependency failure lost its defining source: %+v", detail)
					}
					if test.code == decl.UnknownDeclaration && detail.Target != (decl.DeclarationID{Module: dependencyPin, Name: "Missing"}) {
						t.Fatal("dependency failure lost its resolved target identity")
					}
				})
			}
		})
	}
}
