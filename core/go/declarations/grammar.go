package declarations

import (
	"slices"

	horos "github.com/Bitspark/horos/core/go"
	model "github.com/Bitspark/horos/core/go/internal/declmodel"
	"github.com/Bitspark/horos/core/go/internal/kindmodel"
	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

func declarationForm(tag string, values ...core.Value) core.Value {
	items := make([]core.Value, len(values)+1)
	items[0] = core.NewAtom([]byte("horos/decl/" + tag))
	copy(items[1:], values)
	return core.NewTuple(items...)
}

func declarationText(value string) core.Value { return data.MustEncodeText(value) }

func localKindReference(name string) core.Value {
	return declarationForm("kind-local", declarationText(name))
}

func grammar() model.Grammar {
	return model.Grammar{
		DeclarationPrefix: "horos/decl/",
		DescriptorPrefix:  "horos/",
		Any:               descriptorAny,
		Positions:         descriptorPositions,
		WF:                descriptorWF,
		GroundWF:          groundWF,
		Metadata:          descriptorMetadata,
		Schema:            descriptorSchema(),
		Prelude:           Prelude(),
		OperandKind:       descriptorOperandKind,
		GroundKind:        localKindReference("Data"),
	}
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

// descriptorWF checks primitive syntax, literal operands and recursion guards.
// Operand membership belongs to the ordinary kinds in the formation profile.
// Each Wire message is independently closed under its own recursion binders.
func descriptorWF(value core.Value) error {
	var visit func(core.Value) (core.Value, error)
	visit = func(value core.Value) (core.Value, error) {
		constructor, args, ok := model.Head(value, "horos/")
		if !ok {
			return nil, model.Failure(Invalid, "expected a closed Horos type expression")
		}
		positions, err := descriptorPositions(constructor, args)
		if err != nil {
			return nil, err
		}
		if _, err := descriptorMetadata(constructor, args); err != nil {
			return nil, err
		}
		items := value.(core.Tuple).Items()
		for _, path := range positions {
			child := value
			for _, index := range path {
				child = child.(core.Tuple).At(index)
			}
			skeleton, err := visit(child)
			if err != nil {
				return nil, operandFailure(err, path)
			}
			if constructor == "wire" {
				if err := groundWF(skeleton); err != nil {
					return nil, operandFailure(err, path)
				}
			}
			if len(path) == 1 {
				items[path[0]] = skeleton
			} else {
				field := items[path[0]].(core.Tuple).Items()
				field[path[1]] = skeleton
				items[path[0]] = core.NewTuple(field...)
			}
		}
		if constructor == "wire" {
			return descriptorAny(), nil
		}
		return core.NewTuple(items...), nil
	}
	skeleton, err := visit(value)
	if err != nil {
		return err
	}
	return groundWF(skeleton)
}

// Grammar failures carry paths relative to their checked expression. The
// declaration resolver prefixes its body/argument location exactly once.
func operandFailure(err error, path []int) error {
	if detail, ok := err.(*model.Error); ok && detail.Declaration == "" {
		copy := *detail
		copy.Path = append(slices.Clone(path), detail.Path...)
		return &copy
	}
	return err
}

// Formation contracts refer to ordinary definitions in the supplied prelude.
// They do not interpret those definitions or dispatch on a user kind's name.
func descriptorOperandKind(constructor string, args []core.Value) core.Value {
	switch constructor {
	case "seq", "set-of", "map-of", "and", "not", "rec":
		return localKindReference("Data")
	case "record":
		if len(args) > 0 {
			if mode, ok := args[0].(core.Atom); ok && string(mode.Bytes()) == "open" {
				return localKindReference("Data")
			}
		}
	}
	return declarationForm("top", declarationForm("sort-proper"))
}

func descriptorMetadata(constructor string, args []core.Value) ([]kindmodel.Selector, error) {
	switch constructor {
	case "scalar":
		if len(args) != 1 {
			return nil, model.Failure(Invalid, "wrong scalar descriptor arity")
		}
		selector, ok := args[0].(core.Atom)
		if !ok {
			return nil, model.Failure(Invalid, "scalar selector must be an atom")
		}
		name := string(selector.Bytes())
		if name != "int" && name != "utf8-text" && name != "bool" {
			return nil, model.Failure(Invalid, "unknown scalar selector")
		}
		return []kindmodel.Selector{{Key: "selector", Value: name}}, nil
	case "record":
		if len(args) == 0 {
			return nil, model.Failure(Invalid, "missing record mode")
		}
		mode, ok := args[0].(core.Atom)
		if !ok {
			return nil, model.Failure(Invalid, "record mode must be an atom")
		}
		name := string(mode.Bytes())
		if name != "open" && name != "closed" {
			return nil, model.Failure(Invalid, "unknown record mode")
		}
		return []kindmodel.Selector{{Key: "mode", Value: name}}, nil
	}
	return nil, nil
}

func descriptorSchema() kindmodel.Schema {
	forms := make([]kindmodel.Form, 0, 24)
	for _, name := range []string{
		"any", "atom", "atom-exact", "atom-len", "atom-in", "exact",
		"int-min", "int-max", "text-len", "key-grammar", "text-non-blank", "lowercase-uuid",
	} {
		forms = append(forms, kindmodel.Form{Name: name, MaxChildren: 0})
	}
	forms = append(forms, kindmodel.Form{
		Name: "scalar",
		Selectors: []kindmodel.SelectorDomain{{
			Key: "selector", Values: []string{"int", "utf8-text", "bool"},
		}},
		MaxChildren: 0,
	})
	for _, name := range []string{"wire", "seq", "list-of", "set-of", "not"} {
		forms = append(forms, kindmodel.Form{Name: name, MinChildren: 1, MaxChildren: 1})
	}
	forms = append(forms, kindmodel.Form{Name: "map-of", MinChildren: 2, MaxChildren: 2})
	for _, name := range []string{"tuple", "compound", "union", "and"} {
		forms = append(forms, kindmodel.Form{Name: name, MaxChildren: -1})
	}
	forms = append(forms, kindmodel.Form{
		Name: "record",
		Selectors: []kindmodel.SelectorDomain{{
			Key: "mode", Values: []string{"open", "closed"},
		}},
		MaxChildren: -1,
	})
	return kindmodel.Schema{Forms: forms}
}

func descriptorAny() core.Value {
	return core.NewTuple(core.NewAtom([]byte("horos/any")))
}

// Only existing descriptor type positions admit parameters and applications.
// Literal constraints, record uniqueness and recursion guards remain the
// ground checker's responsibility. Observation lowering adds an open record's
// implicit any tail separately; there is no source operand to substitute there.
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
