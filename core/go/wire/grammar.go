package wire

import (
	"slices"

	horos "github.com/Bitspark/horos/core/go"
	model "github.com/Bitspark/horos/core/go/internal/declmodel"
	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

func tagged(tag string, args ...core.Value) core.Value {
	return core.NewTuple(append([]core.Value{core.NewAtom([]byte("horos/wire/" + tag))}, args...)...)
}
func anyData() core.Value { return tagged("data", core.NewTuple(core.NewAtom([]byte("horos/any")))) }
func grammar() model.Grammar {
	return model.Grammar{DeclarationPrefix: "horos/wire/", DescriptorPrefix: "horos/wire/", Any: anyData, Positions: positions, WF: wellFormed}
}
func atom(value core.Value, text string) bool {
	a, ok := value.(core.Atom)
	return ok && string(a.Bytes()) == text
}
func invalid(message string) error { return model.Failure(Invalid, message) }
func groundWF(value core.Value) error {
	result := horos.DiagnoseWF(value)
	switch result.Verdict {
	case horos.VerdictConforms:
		return nil
	case horos.VerdictExhausted:
		return model.Failure(Exhausted, "ground descriptor checking limit reached")
	default:
		return invalid("Data requires a closed well-formed ground descriptor")
	}
}

// Only these paths contain upper-language type expressions. Data's descriptor
// and every label/name/presence literal remain opaque to upper substitution.
func positions(constructor string, args []core.Value) ([][]int, error) {
	var result [][]int
	switch constructor {
	case "data":
		if len(args) != 1 {
			return nil, invalid("Data requires one ground descriptor")
		}
		return nil, groundWF(args[0])
	case "wire", "list-of":
		if len(args) != 1 {
			return nil, invalid("expected one type operand")
		}
		return [][]int{{1}}, nil
	case "tuple", "union":
		if constructor == "union" && len(args) < 2 {
			return nil, invalid("mixed union requires at least two alternatives")
		}
		for i := range args {
			result = append(result, []int{i + 1})
		}
	case "compound":
		if len(args) == 0 {
			return nil, invalid("missing compound label")
		}
		if _, ok := args[0].(core.Atom); !ok {
			return nil, invalid("compound label must be an atom")
		}
		for i := 1; i < len(args); i++ {
			result = append(result, []int{i + 1})
		}
	case "record":
		if len(args) == 0 || !atom(args[0], "closed") {
			return nil, invalid("mixed record mode must be closed")
		}
		names := make(map[string]bool)
		for i := 1; i < len(args); i++ {
			tag, fields, ok := model.Head(args[i], "horos/wire/")
			if !ok || tag != "field" || len(fields) != 3 {
				return nil, invalid("invalid mixed field")
			}
			name, err := data.ReadText(fields[0])
			if err != nil || names[name] || !(atom(fields[1], "req") || atom(fields[1], "opt")) {
				return nil, invalid("invalid or duplicate field name/presence")
			}
			names[name] = true
			result = append(result, []int{i + 1, 3})
		}
	default:
		return nil, invalid("unknown optional expression constructor " + constructor)
	}
	return result, nil
}

func wellFormed(value core.Value) error {
	var visit func(core.Value, []int) error
	visit = func(v core.Value, path []int) error {
		constructor, args, ok := model.Head(v, "horos/wire/")
		if !ok {
			return &Error{Code: Invalid, Subject: SubjectSource, Path: slices.Clone(path), Message: "expected a closed optional expression"}
		}
		operands, err := positions(constructor, args)
		if err != nil {
			if detail, ok := err.(*Error); ok {
				copy := *detail
				copy.Path = slices.Clone(path)
				return &copy
			}
			return err
		}
		for _, operand := range operands {
			child := v
			for _, index := range operand {
				child = child.(core.Tuple).At(index)
			}
			if err := visit(child, append(slices.Clone(path), operand...)); err != nil {
				return err
			}
		}
		return nil
	}
	return visit(value, nil)
}
