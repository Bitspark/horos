package declarations

import (
	horos "github.com/Bitspark/horos/core/go"
	model "github.com/Bitspark/horos/core/go/internal/declmodel"
	core "github.com/bitspark/ontos/core/go"
)

func grammar() model.Grammar {
	return model.Grammar{DeclarationPrefix: "horos/decl/", DescriptorPrefix: "horos/", Any: descriptorAny, Positions: descriptorPositions, WF: descriptorWF, Rule: descriptorRule, Data: descriptorData}
}

func groundWF(value core.Value) error {
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

func descriptorWF(value core.Value) error {
	_, err := descriptorData(value)
	return err
}

// The ground skeleton checks only descriptor syntax/guards. Data obligations
// are checked separately, including symbolically in open generic declarations.
func descriptorData(value core.Value) (bool, error) {
	var visit func(core.Value) (core.Value, bool, error)
	visit = func(value core.Value) (core.Value, bool, error) {
		constructor, args, ok := model.Head(value, "horos/")
		if !ok {
			return nil, false, model.Failure(Invalid, "expected a closed Horos type expression")
		}
		positions, err := descriptorPositions(constructor, args)
		if err != nil {
			return nil, false, err
		}
		rule := descriptorRule(constructor, args)
		isData := !rule.Capability
		items := value.(core.Tuple).Items()
		for _, path := range positions {
			child := value
			for _, index := range path {
				child = child.(core.Tuple).At(index)
			}
			skeleton, childData, err := visit(child)
			if err != nil {
				return nil, false, err
			}
			if rule.RequiresData && !childData {
				return nil, false, model.Failure(BoundViolation, "constructor operand requires Data")
			}
			if rule.Capability {
				// A message schema is closed under its own recursion binders.
				if err := groundWF(skeleton); err != nil {
					return nil, false, err
				}
			}
			isData = isData && childData
			if len(path) == 1 {
				items[path[0]] = skeleton
			} else {
				field := items[path[0]].(core.Tuple).Items()
				field[path[1]] = skeleton
				items[path[0]] = core.NewTuple(field...)
			}
		}
		if rule.Capability {
			return descriptorAny(), false, nil
		}
		return core.NewTuple(items...), isData, nil
	}
	skeleton, isData, err := visit(value)
	if err != nil {
		return false, err
	}
	if err := groundWF(skeleton); err != nil {
		return false, err
	}
	return isData, nil
}

func descriptorRule(constructor string, args []core.Value) model.DomainRule {
	switch constructor {
	case "wire":
		return model.DomainRule{Capability: true}
	case "tuple", "compound", "list-of", "union":
		return model.DomainRule{}
	case "record":
		if len(args) > 0 {
			if mode, ok := args[0].(core.Atom); ok && string(mode.Bytes()) == "closed" {
				return model.DomainRule{}
			}
		}
	}
	return model.DomainRule{RequiresData: true}
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
	case "wire", "seq", "list-of", "set-of", "not":
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
