package declarations_test

import (
	"fmt"
	"testing"

	decl "github.com/Bitspark/horos/core/go/declarations"
	codec "github.com/bitspark/ontos/codec/go"
	core "github.com/bitspark/ontos/core/go"
)

// Raw counts belong to the input, independent of the production semantic
// checker. They give these fixtures an exact admission ceiling without baking
// in measured implementation work counts or generated-key byte lengths.
func rawNodeCount(v value) int {
	switch v := v.(type) {
	case core.Atom:
		return 1
	case core.Tuple:
		count := 1
		for i := 0; i < v.Len(); i++ {
			count += rawNodeCount(v.At(i))
		}
		return count
	default:
		panic("budget fixture is not ground Ontos data")
	}
}

// Every use must check Poly's complete constructor admission even though
// IgnoreF's result ignores that constructor. Repeated declaration references
// keep the document compact while the request makes many interface queries.
func interfaceBudgetFixture(uses int) value {
	const arity = 128
	typeInput := properInterface("type")
	dataInput := properInterface("data")
	parameters := make([]value, arity)
	inputs := make([]value, arity)
	for i := range parameters {
		parameters[i] = parameter(fmt.Sprintf("T%d", i), typeInput)
		inputs[i] = dataInput
	}
	entries := [][2]value{
		entry("Poly", typedDefinition(param(0), "type", parameters...)),
		entry("IgnoreF", typedDefinition(h("scalar", a("utf8-text")), "type",
			parameter("F", constructorInterface("data", inputs...)))),
	}
	for i := 0; i < uses; i++ {
		entries = append(entries, entry(fmt.Sprintf("Use%04d", i), typeDefinition(local("IgnoreF", local("Poly")))))
	}
	entries = append(entries, entry("Result", typeDefinition(local(fmt.Sprintf("Use%04d", uses-1)))))
	return module("interface_budget", mapping(), entries...)
}

// Each branch passes a different proper expression in a different environment.
// The named declaration graph is acyclic and the document remains small. A
// source reference or source summary alone is not a complete invocation key.
func environmentBudgetFixture(levels int) value {
	entries := [][2]value{entry("N0", typeDefinition(param(0), "T"))}
	for i := 1; i <= levels; i++ {
		previous := fmt.Sprintf("N%d", i-1)
		body := h("tuple",
			local(previous, h("tuple", param(0))),
			local(previous, h("list-of", param(0))))
		entries = append(entries, entry(fmt.Sprintf("N%d", i), typeDefinition(body, "T")))
	}
	entries = append(entries, entry("Result", typeDefinition(local(fmt.Sprintf("N%d", levels), h("scalar", a("utf8-text"))))))
	return module("environment_budget", mapping(), entries...)
}

func checkSemanticBudgetFixture(t *testing.T, complete, isolated value) {
	t.Helper()
	roomy := decl.DefaultLimits()
	roomy.MaxNodes = 1 << 20
	program, err := decl.ResolveWithLimits(complete, nil, roomy)
	if err != nil || program == nil {
		t.Fatal("the acyclic fixture must resolve with sufficient resources", err)
	}
	result, ok := program.Declaration(id(program, "Result"))
	if !ok || result.DataStatus() != decl.DataProven {
		t.Fatal("budget fixture's closed result lost its independently expected Data proof")
	}
	for _, dimension := range []string{"work", "bytes"} {
		t.Run(dimension, func(t *testing.T) {
			limited := roomy
			if dimension == "work" {
				limited.MaxNodes = rawNodeCount(complete)
			} else {
				limited.MaxBytes = len(codec.Encode(complete))
			}
			if _, err := decl.PinWithLimits(complete, limited); err != nil {
				t.Fatal("semantic exhaustion fixture did not fit raw input admission", err)
			}
			if p, err := decl.ResolveWithLimits(isolated, nil, limited); err != nil || p == nil {
				t.Fatal("an isolated use must fit the same semantic allowance", err)
			}
			p, err := decl.ResolveWithLimits(complete, nil, limited)
			if p != nil {
				t.Fatal("aggregate semantic exhaustion exposed a partial Program")
			}
			diagnostic(t, err, decl.Exhausted, decl.SubjectRequest)
			// A fresh request receives its own allowance. An exhausted operation
			// must not retain failed proof/cache entries in a later Program.
			p, err = decl.ResolveWithLimits(complete, nil, roomy)
			if err != nil || p == nil {
				t.Fatal("failed aggregate request poisoned later resolution", err)
			}
		})
	}
}

func TestSourceInterfaceQueriesShareAggregateWorkAndByteBudgets(t *testing.T) {
	checkSemanticBudgetFixture(t, interfaceBudgetFixture(128), interfaceBudgetFixture(1))
}

func TestSourceBindingEnvironmentsShareAggregateWorkAndByteBudgets(t *testing.T) {
	checkSemanticBudgetFixture(t, environmentBudgetFixture(8), environmentBudgetFixture(1))
}
