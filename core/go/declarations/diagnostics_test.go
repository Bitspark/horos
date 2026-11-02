package declarations_test

import (
	"errors"
	"testing"

	decl "github.com/Bitspark/horos/core/go/declarations"
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
