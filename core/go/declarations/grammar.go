package declarations

import (
	horos "github.com/Bitspark/horos/core/go"
	model "github.com/Bitspark/horos/core/go/internal/declmodel"
	core "github.com/bitspark/ontos/core/go"
)

func grammar() model.Grammar {
	return model.Grammar{DeclarationPrefix: "horos/decl/", DescriptorPrefix: "horos/", Any: descriptorAny, Positions: descriptorPositions, WF: descriptorWF}
}

func descriptorWF(value core.Value) error {
	result := horos.DiagnoseWF(value)
	switch result.Verdict {
	case horos.VerdictConforms:
		return nil
	case horos.VerdictExhausted:
		return model.Failure(Exhausted, "Horos descriptor checking limit reached")
	default:
		return model.Failure(Invalid, "malformed or unguarded Horos descriptor")
	}
}

func descriptorAny() core.Value {
	return core.NewTuple(core.NewAtom([]byte("horos/any")))
}

// Only existing descriptor type positions admit parameters/applications. Literal constraints,
// record uniqueness and recursion guards are still checked by Horos DiagnoseWF.
// Checked parameters/applications are replaced with any only in that final
// descriptor-grammar pass, after scope, arity and argument closure were checked.
func descriptorPositions(constructor string, args []core.Value) ([][]int, error) {
	var positions [][]int
	switch constructor {
	case "tuple", "union", "and":
		for i := range args {
			positions = append(positions, []int{i + 1})
		}
	case "seq", "list-of", "set-of", "not":
		if len(args) != 1 {
			return nil, model.Failure(Invalid, "wrong descriptor arity")
		}
		positions = append(positions, []int{1})
	case "map-of":
		if len(args) != 2 {
			return nil, model.Failure(Invalid, "wrong map descriptor arity")
		}
		positions = append(positions, []int{1}, []int{2})
	case "compound":
		if len(args) < 1 {
			return nil, model.Failure(Invalid, "missing compound label")
		}
		for i := 1; i < len(args); i++ {
			positions = append(positions, []int{i + 1})
		}
	case "record":
		if len(args) < 1 {
			return nil, model.Failure(Invalid, "missing record mode")
		}
		for i := 1; i < len(args); i++ {
			tag, fields, ok := model.Head(args[i], "horos/")
			if !ok || tag != "field" || len(fields) != 3 {
				return nil, model.Failure(Invalid, "invalid record field form")
			}
			positions = append(positions, []int{i + 1, 3})
		}
	case "rec":
		if len(args) != 2 {
			return nil, model.Failure(Invalid, "wrong recursion descriptor arity")
		}
		positions = append(positions, []int{2})
	}
	return positions, nil
}
