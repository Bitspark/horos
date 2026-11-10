package declarations_test

// Diagnostic locations: README "Resolution and identity" and declarations.md
// §4 ("Preserve ... diagnostic subjects"). Paths are tuple indexes into the
// canonical declaration region named by Region. Ported from the revision-3
// TestUnknownTargetsRetainSourceLocation,
// TestClosedBoundFailuresRetainNestedOperandLocations and
// TestSaturatedFeatureBoundaryAndSemanticReferenceCycles.

import (
	"slices"
	"testing"

	decl "github.com/Bitspark/horos/core/go/declarations"
	core "github.com/bitspark/ontos/core/go"
)

func TestUnknownTargetsRetainSourceLocation(t *testing.T) {
	root := module("unknown", mapping(), entry("X", d("type", list(), kTop(), h("list-of", local("Missing")))))
	_, err := decl.Resolve(root, nil)
	unknown := expectCode(t, err, decl.UnknownDeclaration)
	if unknown.Subject != decl.SubjectSource || unknown.Module != pin(t, root) || unknown.Declaration != "X" ||
		unknown.Region != "body" || !slices.Equal(unknown.Path, []int{1}) ||
		unknown.Target != (decl.DeclarationID{Module: pin(t, root), Name: "Missing"}) {
		t.Fatalf("unknown declaration lost its owner, path or target: %+v", unknown)
	}

	root = module("unknown_import", mapping(), entry("X", d("type", list(), kTop(), imported("missing", "Missing"))))
	_, err = decl.Resolve(root, nil)
	alias := expectCode(t, err, decl.UnknownImport)
	if alias.Module != pin(t, root) || alias.Declaration != "X" || alias.ImportAlias != "missing" {
		t.Fatalf("unknown alias lost its source location: %+v", alias)
	}

	pages, _ := pagination(t)
	root = module("unknown_imported_target", mapping(entry("p", importValue(t, "example.pagination", pages))),
		entry("X", d("type", list(), kTop(), imported("p", "Missing"))))
	_, err = decl.Resolve(root, []core.Value{pages})
	unknown = expectCode(t, err, decl.UnknownDeclaration)
	if unknown.Module != pin(t, root) || unknown.Declaration != "X" || unknown.ImportAlias != "p" ||
		unknown.Target != (decl.DeclarationID{Module: pin(t, pages), Name: "Missing"}) {
		t.Fatalf("unknown imported target was rebound to its caller: %+v", unknown)
	}

	root = module("cycle", mapping(), entry("X", d("type", list(), kTop(), local("X"))))
	_, err = decl.Resolve(root, nil)
	if cycle := expectCode(t, err, decl.Cycle); cycle.Module != pin(t, root) || cycle.Declaration != "X" {
		t.Fatalf("cycle lost its source location: %+v", cycle)
	}
}

// kinds.md §4: not requires a Data child; Wire is not Data. The failure keeps
// its nested operand path, in the root, in a dependency, and behind an
// argument.
func TestClosedBoundFailuresRetainNestedOperandLocations(t *testing.T) {
	bad := h("tuple", h("not", h("wire", person())))
	root := module("bad", preludeImports(), entry("Value", d("type", list(), kType(), bad)))
	program, err := decl.Resolve(root, nil)
	detail := expectCode(t, err, decl.BoundViolation)
	if program != nil || detail.Subject != decl.SubjectSource || detail.Declaration != "Value" || !slices.Equal(detail.Path, []int{1, 1}) {
		t.Fatalf("closed operand lost its source location: %+v", detail)
	}
	consumer := module("consumer", mapping(entry("bad", importValue(t, "bad", root))))
	program, err = decl.Resolve(consumer, []core.Value{root})
	detail = expectCode(t, err, decl.BoundViolation)
	if program != nil || detail.Subject != decl.SubjectContext || detail.Module != pin(t, root) || detail.Dependency != pin(t, root) || !slices.Equal(detail.Path, []int{1, 1}) {
		t.Fatalf("closed operand lost its dependency location: %+v", detail)
	}
	caller := module("caller", preludeImports(),
		entry("Identity", d("type", list(tparam("T", kType())), kType(), param(0))),
		entry("Value", d("type", list(), kType(), local("Identity", bad))))
	program, err = decl.Resolve(caller, nil)
	detail = expectCode(t, err, decl.BoundViolation)
	if program != nil || detail.Declaration != "Value" || !slices.Equal(detail.Path, []int{2, 1, 1, 1}) {
		t.Fatalf("argument prefix was lost or repeated: %+v", detail)
	}
}

// §4/§6: ill-sorted and unsupported forms fail as source errors; a semantic
// reference inside an ignored constructor argument still forms a cycle.
func TestSaturatedFeatureBoundaryAndSemanticReferenceCycles(t *testing.T) {
	unary := kArrow(kType(), kType())
	base := [][2]core.Value{
		entry("Person", d("type", list(), kType(), person())),
		entry("Page", d("type", list(tparam("T", kType())), kType(), page(param(0)))),
		entry("Pair", d("type", list(tparam("A", kType()), tparam("B", kType())), kType(), h("tuple", param(0), param(1)))),
		entry("Wrapped", d("type", list(tparam("F", unary), tparam("T", kType())), kType(), d("apply", param(0), list(param(1))))),
	}
	for name, body := range map[string]core.Value{
		"proper argument for constructor":   local("Wrapped", local("Person"), local("Person")),
		"wrong constructor arity":           local("Wrapped", local("Pair"), local("Person")),
		"constructor as descriptor operand": h("list-of", local("Page")),
		"application of a proper result":    d("apply", local("Page", local("Person")), list(local("Person"))),
		"partial application":               local("Pair", local("Person")),
		"type lambda":                       d("lambda", list(), local("Person")),
		"application of a proper reference": local("Person", local("Person")),
	} {
		t.Run(name, func(t *testing.T) {
			p, err := decl.Resolve(module("unsupported", preludeImports(), append(slices.Clone(base), entry("Bad", d("type", list(), kType(), body)))...), nil)
			if p != nil || err == nil {
				t.Fatal("unsupported or ill-kinded definition was admitted")
			}
			var detail *decl.Error
			if !asError(err, &detail) || detail.Subject != decl.SubjectSource || detail.Declaration != "Bad" {
				t.Fatalf("source failure lost its subject or owner: %v", err)
			}
		})
	}
	cycle := module("cycle", preludeImports(),
		entry("IgnoreF", d("type", list(tparam("F", unary), tparam("T", kType())), kType(), h("scalar", a("utf8-text")))),
		entry("Loop", d("type", list(tparam("T", kType())), kType(), local("IgnoreF", local("Loop"), param(0)))))
	p, err := decl.Resolve(cycle, nil)
	if p != nil {
		t.Fatal("ignored constructor cycle published a program")
	}
	if detail := expectCode(t, err, decl.Cycle); detail.Subject != decl.SubjectSource {
		t.Fatalf("cycle subject: %+v", detail)
	}
}
