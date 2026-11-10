package declarations_test

// Abstract constructor application with a closed argument: declarations.md
// §8 makes closed nonmembership Refuted, which a required obligation reports
// as BoundViolation, as it already did for known constructors (§6). Ported
// from the revision-3 TestParameterCalleeDiagnosticRetainsBinderAndArgument,
// which expected BoundViolation for the same declaration.

import (
	"slices"
	"testing"

	decl "github.com/Bitspark/horos/core/go/declarations"
	core "github.com/bitspark/ontos/core/go"
)

// §6: applying a constructor parameter checks its declared input contract
// and names the parameter and the argument position.
func TestParameterCalleeDiagnosticRetainsBinderAndArgument(t *testing.T) {
	root := module("diagnostic", preludeImports(), entry("Bad",
		d("type", list(tparam("F", kArrow(kData(), kData()))), kType(), d("apply", param(0), list(h("wire", person()))))))
	p, err := decl.Resolve(root, nil)
	detail := expectCode(t, err, decl.BoundViolation)
	owner := decl.DeclarationID{Module: pin(t, root), Name: "Bad"}
	if p != nil || detail.Subject != decl.SubjectSource || detail.CalleeParameter != (decl.ParameterID{Declaration: owner, Index: 0}) ||
		!slices.Equal(detail.Path, []int{2, 1}) || detail.Expected == nil || !detail.Expected.Sort().IsProper() {
		t.Fatalf("parameter application lost its contract context: %+v", detail)
	}
}

// kinds.md §5: every alternative of a union must admit the call, so one
// refuting alternative refutes it; a joint contract needs only one arrow.
// A rigid argument that is not proven stays Unproved.
func TestAbstractApplicationRefutation(t *testing.T) {
	unary := d("sort-arrow", list(properSort()), properSort())
	dataToData, typeToType := kArrow(kData(), kData()), kArrow(kType(), kType())
	wire := h("wire", person())
	resolveBody := func(contract, argumentKind core.Value, argument core.Value) error {
		params := []core.Value{tparam("F", contract)}
		if argumentKind != nil {
			params = append(params, tparam("A", argumentKind))
		}
		_, err := decl.Resolve(module("abstract", preludeImports(),
			entry("Bad", d("type", list(params...), kType(), d("apply", param(0), list(argument))))), nil)
		return err
	}
	expectCode(t, resolveBody(d("or", unary, list(dataToData, typeToType)), nil, wire), decl.BoundViolation)
	if err := resolveBody(d("and", unary, list(dataToData, typeToType)), nil, wire); err != nil {
		t.Fatalf("a joint contract with an admitting arrow: %v", err)
	}
	expectCode(t, resolveBody(dataToData, kType(), param(1)), decl.UnprovedObligation)
	if err := resolveBody(dataToData, nil, person()); err != nil {
		t.Fatalf("a closed Data argument: %v", err)
	}
}
