package declarations_test

import (
	"fmt"
	"strings"
	"testing"

	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

// This test-only oracle expands a small, acyclic source fragment eagerly. It
// reads raw canonical source, not Horos's checked expressions, interfaces or
// summary facts. Every argument is expanded before entering a declaration.
// Independent hand-authored tests cover rejection and open-body admission;
// this oracle only compares already valid finite examples and their Data result.
type eagerDefinition struct {
	parameters []value
	body       value
}

type eagerValue struct {
	proper      value
	constructor *eagerDefinition
}

type eagerOracle struct {
	declarations map[string]*eagerDefinition
	steps        int
}

func oracleHead(v value) (string, []value, error) {
	tuple, ok := v.(core.Tuple)
	if !ok || tuple.Len() == 0 {
		return "", nil, fmt.Errorf("oracle expected a nonempty tuple")
	}
	tag, ok := tuple.At(0).(core.Atom)
	if !ok {
		return "", nil, fmt.Errorf("oracle expected an atom tag")
	}
	return string(tag.Bytes()), tuple.Items()[1:], nil
}

func (o *eagerOracle) eval(source value, environment []eagerValue, depth int) (eagerValue, error) {
	o.steps++
	if o.steps > 20000 || depth > 64 {
		return eagerValue{}, fmt.Errorf("oracle finite-fragment budget exhausted")
	}
	tag, args, err := oracleHead(source)
	if err != nil {
		return eagerValue{}, err
	}
	switch tag {
	case "horos/decl/param":
		index, err := data.ReadInt(args[0])
		if err != nil || !index.IsInt64() || index.Sign() < 0 || index.Int64() >= int64(len(environment)) {
			return eagerValue{}, fmt.Errorf("oracle parameter is not bound")
		}
		return environment[int(index.Int64())], nil
	case "horos/decl/local":
		name, err := data.ReadText(args[0])
		definition := o.declarations[name]
		if err != nil || definition == nil {
			return eagerValue{}, fmt.Errorf("oracle unknown declaration %q", name)
		}
		if len(definition.parameters) != 0 {
			return eagerValue{constructor: definition}, nil
		}
		return o.eval(definition.body, nil, depth+1)
	case "horos/decl/apply":
		callee, err := o.eval(args[0], environment, depth+1)
		if err != nil {
			return eagerValue{}, err
		}
		arguments, err := data.ReadList(args[1])
		if err != nil || callee.constructor == nil || len(arguments) != len(callee.constructor.parameters) {
			return eagerValue{}, fmt.Errorf("oracle expected a saturated constructor")
		}
		actual := make([]eagerValue, len(arguments))
		for i, argument := range arguments {
			actual[i], err = o.eval(argument, environment, depth+1)
			if err != nil {
				return eagerValue{}, err
			}
		}
		return o.eval(callee.constructor.body, actual, depth+1)
	}
	if !strings.HasPrefix(tag, "horos/") {
		return eagerValue{}, fmt.Errorf("oracle unsupported tag %q", tag)
	}
	var positions [][]int
	switch tag {
	case "horos/any", "horos/scalar", "horos/exact":
		// In particular exact's entire payload is a literal, even if it looks
		// like a parameter, constructor reference, application or interface.
		return eagerValue{proper: source}, nil
	case "horos/list-of", "horos/wire":
		positions = [][]int{{1}}
	case "horos/tuple", "horos/union":
		for i := range args {
			positions = append(positions, []int{i + 1})
		}
	case "horos/record":
		for i := 1; i < len(args); i++ {
			positions = append(positions, []int{i + 1, 3})
		}
	default:
		return eagerValue{}, fmt.Errorf("oracle descriptor %q is outside its fragment", tag)
	}
	items := source.(core.Tuple).Items()
	for _, position := range positions {
		child := source
		for _, index := range position {
			child = child.(core.Tuple).At(index)
		}
		expanded, err := o.eval(child, environment, depth+1)
		if err != nil {
			return eagerValue{}, err
		}
		if expanded.constructor != nil || expanded.proper == nil {
			return eagerValue{}, fmt.Errorf("oracle constructor in proper type position")
		}
		if len(position) == 1 {
			items[position[0]] = expanded.proper
		} else {
			field := items[position[0]].(core.Tuple).Items()
			field[position[1]] = expanded.proper
			items[position[0]] = core.NewTuple(field...)
		}
	}
	return eagerValue{proper: core.NewTuple(items...)}, nil
}

// oracleData independently examines a fully materialized descriptor. It never
// calls the declaration checker, its summaries, DataDescriptor, or ground WF.
func oracleData(v value, depth int) (bool, error) {
	if depth > 64 {
		return false, fmt.Errorf("oracle descriptor too deep")
	}
	tag, args, err := oracleHead(v)
	if err != nil {
		return false, err
	}
	switch tag {
	case "horos/any", "horos/scalar", "horos/exact":
		return true, nil
	case "horos/wire":
		_, err := oracleData(args[0], depth+1)
		return false, err
	case "horos/list-of", "horos/tuple", "horos/union":
		result := true
		for _, child := range args {
			childData, err := oracleData(child, depth+1)
			if err != nil {
				return false, err
			}
			result = result && childData
		}
		return result, nil
	case "horos/record":
		result := true
		for _, field := range args[1:] {
			childData, err := oracleData(field.(core.Tuple).At(3), depth+1)
			if err != nil {
				return false, err
			}
			result = result && childData
		}
		return result, nil
	default:
		return false, fmt.Errorf("oracle classification outside fragment: %s", tag)
	}
}

func TestHigherKindSummariesAgreeWithIndependentEagerOracle(t *testing.T) {
	typeInput := properInterface("type")
	unary := constructorInterface("type", typeInput)
	definitions := map[string]*eagerDefinition{
		"Page": {
			parameters: []value{parameter("T", typeInput)},
			body:       page(param(0)),
		},
		"Ignore": {
			parameters: []value{parameter("T", typeInput)},
			body:       h("scalar", a("utf8-text")),
		},
		"Send": {
			parameters: []value{parameter("T", typeInput)},
			body:       h("wire", param(0)),
		},
		"Wrapped": {
			parameters: []value{parameter("F", unary), parameter("T", typeInput)},
			body:       box(apply(param(0), param(1))),
		},
		"Compose": {
			parameters: []value{parameter("F", unary), parameter("G", unary), parameter("T", typeInput)},
			body:       apply(param(0), apply(param(1), param(2))),
		},
		"Twice": {
			parameters: []value{parameter("F", unary), parameter("T", typeInput)},
			body:       h("tuple", apply(param(0), param(1)), apply(param(0), param(1))),
		},
	}
	var roots []string
	for _, first := range []string{"Page", "Ignore", "Send"} {
		for _, second := range []string{"Page", "Ignore", "Send"} {
			for index, item := range []value{person(), h("wire", person()), h("exact", apply(param(90), local("Missing")))} {
				name := fmt.Sprintf("Case_%s_%s_%d", first, second, index)
				definitions[name] = &eagerDefinition{body: local("Compose", local(first), local(second), item)}
				roots = append(roots, name)
			}
		}
		for index, item := range []value{person(), h("wire", person())} {
			for _, outer := range []string{"Wrapped", "Twice"} {
				name := fmt.Sprintf("%s_%s_%d", outer, first, index)
				definitions[name] = &eagerDefinition{body: local(outer, local(first), item)}
				roots = append(roots, name)
			}
		}
	}
	var entries [][2]value
	for name, definition := range definitions {
		entries = append(entries, entry(name, typedDefinition(definition.body, "type", definition.parameters...)))
	}
	p := resolve(t, module("oracle", mapping(), entries...))
	for _, name := range roots {
		t.Run(name, func(t *testing.T) {
			oracle := eagerOracle{declarations: definitions}
			expected, err := oracle.eval(local(name), nil, 0)
			if err != nil || expected.proper == nil || expected.constructor != nil {
				t.Fatalf("independent fixture expansion failed: %v", err)
			}
			isData, err := oracleData(expected.proper, 0)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := p.Specialize(id(p, name), nil)
			if err != nil || !core.Equal(actual, expected.proper) {
				t.Fatalf("optimized result disagrees with independent eager substitution: %v", err)
			}
			classified, err := p.DataDescriptor(id(p, name), nil)
			if isData {
				if err != nil || !core.Equal(classified, expected.proper) {
					t.Fatal("optimized proof lost independent Data classification", err)
				}
			} else if classified != nil || err == nil {
				t.Fatal("optimized proof contradicted independently expanded Wire result")
			}
		})
	}
}
