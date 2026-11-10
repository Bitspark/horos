package kindmodel

// Expectations follow docs/spec/kinds.md: §2 node patterns and selectors,
// §4 guarded greatest-fixed-point recursion, §6 exact closed membership and
// free-shape finite-cut inclusion, §7 budgets. The schema below is a small
// Horos-like vocabulary; the engine itself knows no form or kind names.

import (
	"errors"
	"testing"
)

var errTestBudget = errors.New("test budget spent")

type testBudget struct{ steps, bytes uint64 }

func (b *testBudget) Step(uint64) error {
	if b.steps == 0 {
		return errTestBudget
	}
	b.steps--
	return nil
}

func (b *testBudget) Bytes(n uint64) error {
	if n > b.bytes {
		return errTestBudget
	}
	b.bytes -= n
	return nil
}

func plenty() *testBudget { return &testBudget{steps: 1 << 30, bytes: 1 << 40} }

var schema = Schema{Forms: []Form{
	{Name: "scalar", Selectors: []SelectorDomain{{Key: "selector", Values: []string{"int", "utf8-text", "bool"}}}},
	{Name: "list-of", MinChildren: 1, MaxChildren: 1},
	{Name: "record", Selectors: []SelectorDomain{{Key: "mode", Values: []string{"open", "closed"}}}, MaxChildren: -1},
	{Name: "any"},
}}

func node(form string, sequence SeqID, selectors ...Selector) State {
	return State{Op: Node, Node: NodePattern{Form: form, Selectors: selectors, Sequence: sequence}}
}

func compile(t *testing.T, definition Definition) *Kind {
	t.Helper()
	k, err := Compile(schema, definition, plenty())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return k
}

var (
	epsilon = Sequence{Op: Epsilon}
	text    = Selector{Key: "selector", Value: "utf8-text"}
	integer = Selector{Key: "selector", Value: "int"}
)

// scalarKind: node("scalar", {}, epsilon); the omitted selector is a wildcard.
func scalarKind(t *testing.T) *Kind {
	return compile(t, Definition{States: []State{node("scalar", 0)}, Sequences: []Sequence{epsilon}})
}

// textKind: node("scalar", {"selector": "utf8-text"}, epsilon).
func textKind(t *testing.T) *Kind {
	return compile(t, Definition{States: []State{node("scalar", 0, text)}, Sequences: []Sequence{epsilon}})
}

// treeKind: nu X. Scalar | List<X> | Record<X>.
func treeKind(t *testing.T) *Kind {
	return compile(t, Definition{
		States: []State{
			{Op: Or, Args: []StateID{1, 2, 3}},
			node("scalar", 0),
			node("list-of", 1),
			node("record", 2),
		},
		Sequences: []Sequence{epsilon, {Op: Item, Item: 0}, {Op: Star, Args: []SeqID{1}}},
	})
}

// rowsKind: Record<Flat> with Flat = Scalar | List<Scalar>.
func rowsKind(t *testing.T) *Kind {
	return compile(t, Definition{
		States: []State{
			node("record", 2),
			{Op: Or, Args: []StateID{2, 3}},
			node("scalar", 0),
			node("list-of", 1),
		},
		Sequences: []Sequence{epsilon, {Op: Item, Item: 2}, {Op: Star, Args: []SeqID{3}}, {Op: Item, Item: 1}},
	})
}

// listsKind: nu X. List<X>, whose only observations are infinite.
func listsKind(t *testing.T) *Kind {
	return compile(t, Definition{States: []State{node("list-of", 0)}, Sequences: []Sequence{{Op: Item, Item: 0}}})
}

func constant(t *testing.T, op Op) *Kind {
	return compile(t, Definition{States: []State{{Op: op}}})
}

func both(t *testing.T, left, right Definition) *Kind {
	t.Helper()
	// And(left, right) built by concatenating two equation tables.
	offset := StateID(len(left.States)) + 1
	seqOffset := SeqID(len(left.Sequences))
	states := []State{{Op: And, Args: []StateID{left.Root + 1, right.Root + offset}}}
	for _, s := range left.States {
		s.Args = shift(s.Args, 1)
		states = append(states, s)
	}
	for _, s := range right.States {
		s.Args = shift(s.Args, offset)
		if s.Op == Node {
			s.Node.Sequence += seqOffset
		}
		states = append(states, s)
	}
	sequences := []Sequence{}
	for _, q := range left.Sequences {
		if q.Op == Item {
			q.Item++
		}
		sequences = append(sequences, q)
	}
	for _, q := range right.Sequences {
		if q.Op == Item {
			q.Item += offset
		}
		for i := range q.Args {
			q.Args[i] += seqOffset
		}
		sequences = append(sequences, q)
	}
	return compile(t, Definition{States: states, Sequences: sequences})
}

func shift(ids []StateID, by StateID) []StateID {
	out := make([]StateID, len(ids))
	for i, id := range ids {
		out[i] = id + by
	}
	return out
}

func obs(nodes ...ObservationNode) Observation { return Observation{Nodes: nodes} }

func scalarObs(selector Selector) ObservationNode {
	return ObservationNode{Form: "scalar", Selectors: []Selector{selector}}
}

func recordObs(children ...ObsID) ObservationNode {
	return ObservationNode{Form: "record", Selectors: []Selector{{Key: "mode", Value: "closed"}}, Children: children}
}

func member(t *testing.T, k *Kind, o Observation) bool {
	t.Helper()
	ok, err := Member(k, o, plenty())
	if err != nil {
		t.Fatalf("member: %v", err)
	}
	return ok
}

func includes(t *testing.T, left, right *Kind) bool {
	t.Helper()
	ok, err := Includes(left, right, plenty())
	if err != nil {
		t.Fatalf("includes: %v", err)
	}
	return ok
}

func TestMembershipSelectorsAndChildren(t *testing.T) {
	scalar, txt, tree, rows := scalarKind(t), textKind(t), treeKind(t), rowsKind(t)
	cases := []struct {
		name string
		kind *Kind
		obs  Observation
		want bool
	}{
		{"omitted selector is a wildcard", scalar, obs(scalarObs(integer)), true},
		{"selected value matches", txt, obs(scalarObs(text)), true},
		{"selected value differs", txt, obs(scalarObs(integer)), false},
		{"form differs", scalar, obs(ObservationNode{Form: "list-of", Children: []ObsID{1}}, scalarObs(text)), false},
		{"tree record of scalar and list", tree, obs(recordObs(1, 2), scalarObs(integer), ObservationNode{Form: "list-of", Children: []ObsID{3}}, scalarObs(text)), true},
		{"tree rejects an any child", tree, obs(recordObs(1, 2), scalarObs(integer), ObservationNode{Form: "any"}), false},
		{"empty record matches star", rows, obs(recordObs()), true},
		{"rows with flat fields", rows, obs(recordObs(1, 2), scalarObs(integer), ObservationNode{Form: "list-of", Children: []ObsID{1}}), true},
		{"rows rejects a nested list", rows, obs(recordObs(1), ObservationNode{Form: "list-of", Children: []ObsID{2}}, ObservationNode{Form: "list-of", Children: []ObsID{3}}, scalarObs(integer)), false},
		{"top admits anything", constant(t, Top), obs(ObservationNode{Form: "any"}), true},
		{"bottom admits nothing", constant(t, Bottom), obs(ObservationNode{Form: "any"}), false},
	}
	for _, c := range cases {
		if got := member(t, c.kind, c.obs); got != c.want {
			t.Errorf("%s: member %v, want %v", c.name, got, c.want)
		}
	}
}

// §4/§6: recursion is a greatest fixed point over a finite graph with
// backedges; a regular infinite vertical unfolding can belong to a kind.
func TestMembershipBackedgesAtGreatestFixedPoint(t *testing.T) {
	loop := obs(ObservationNode{Form: "list-of", Children: []ObsID{0}})
	if !member(t, listsKind(t), loop) || !member(t, treeKind(t), loop) {
		t.Fatal("the infinite list observation belongs to nu X. List<X> and to Tree")
	}
	listOfScalar := compile(t, Definition{
		States:    []State{node("list-of", 1), node("scalar", 0)},
		Sequences: []Sequence{epsilon, {Op: Item, Item: 1}},
	})
	if member(t, listOfScalar, loop) {
		t.Fatal("a backedge alone established membership in List<Scalar>")
	}
	// A record whose second child loops back to the record, beside a scalar.
	recursiveRecord := obs(recordObs(1, 0), scalarObs(text))
	if !member(t, treeKind(t), recursiveRecord) || member(t, rowsKind(t), recursiveRecord) {
		t.Fatal("recursive record membership")
	}
}

func TestInclusionOverFreeShapes(t *testing.T) {
	scalar, txt, tree, rows, lists := scalarKind(t), textKind(t), treeKind(t), rowsKind(t), listsKind(t)
	bottom, top := constant(t, Bottom), constant(t, Top)
	cases := []struct {
		name        string
		left, right *Kind
		want        bool
	}{
		{"text within scalar", txt, scalar, true},
		{"scalar not within text", scalar, txt, false},
		{"rows within tree", rows, tree, true},
		{"tree not within rows", tree, rows, false},
		{"infinite lists within tree", lists, tree, true},
		{"tree not within infinite lists", tree, lists, false},
		{"bottom within everything", bottom, txt, true},
		{"top not within tree", top, tree, false},
		{"tree within top", tree, top, true},
		{"reflexive", tree, tree, true},
		{"separately compiled equal kinds", treeKind(t), tree, true},
	}
	for _, c := range cases {
		if got := includes(t, c.left, c.right); got != c.want {
			t.Errorf("%s: includes %v, want %v", c.name, got, c.want)
		}
	}
}

func TestEmptinessAndIntersection(t *testing.T) {
	scalarDef := Definition{States: []State{node("scalar", 0)}, Sequences: []Sequence{epsilon}}
	listDef := Definition{States: []State{node("list-of", 0), node("scalar", 1)}, Sequences: []Sequence{{Op: Item, Item: 1}, epsilon}}
	textDef := Definition{States: []State{node("scalar", 0, text)}, Sequences: []Sequence{epsilon}}
	cases := []struct {
		name string
		kind *Kind
		want bool
	}{
		{"bottom", constant(t, Bottom), true},
		{"top", constant(t, Top), false},
		{"infinite lists are a nonempty free shape", listsKind(t), false},
		{"scalar and list-of share no node", both(t, scalarDef, listDef), true},
		{"scalar and text overlap", both(t, scalarDef, textDef), false},
	}
	for _, c := range cases {
		got, err := Empty(c.kind, plenty())
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: empty %v, want %v", c.name, got, c.want)
		}
	}
	if !includes(t, both(t, scalarDef, textDef), textKind(t)) || includes(t, scalarKind(t), both(t, scalarDef, textDef)) {
		t.Fatal("intersection inclusion")
	}
}

func TestCompileRejectsMalformedSystems(t *testing.T) {
	cases := []struct {
		name       string
		schema     Schema
		definition Definition
	}{
		{"no states", schema, Definition{}},
		{"root out of range", schema, Definition{Root: 1, States: []State{{Op: Top}}}},
		{"one-operand union", schema, Definition{States: []State{{Op: Or, Args: []StateID{1}}, {Op: Top}}}},
		{"constant with operands", schema, Definition{States: []State{{Op: Top, Args: []StateID{0}}}}},
		{"unknown form", schema, Definition{States: []State{node("wire", 0)}, Sequences: []Sequence{epsilon}}},
		{"unapproved selector value", schema, Definition{States: []State{node("scalar", 0, Selector{Key: "selector", Value: "decimal"})}, Sequences: []Sequence{epsilon}}},
		{"unknown selector", schema, Definition{States: []State{node("list-of", 0, Selector{Key: "mode", Value: "open"})}, Sequences: []Sequence{{Op: Item, Item: 0}}}},
		{"unguarded alias cycle", schema, Definition{States: []State{{Op: Alias, Args: []StateID{0}}}}},
		{"unguarded union cycle", schema, Definition{States: []State{{Op: Or, Args: []StateID{0, 1}}, node("scalar", 0)}, Sequences: []Sequence{epsilon}}},
		{"sequence cycle", schema, Definition{States: []State{node("record", 0)}, Sequences: []Sequence{{Op: Star, Args: []SeqID{0}}}}},
		{"item out of range", schema, Definition{States: []State{node("list-of", 0)}, Sequences: []Sequence{{Op: Item, Item: 4}}}},
		{"duplicate form", Schema{Forms: []Form{{Name: "any"}, {Name: "any"}}}, Definition{States: []State{{Op: Top}}}},
		{"inverted child width", Schema{Forms: []Form{{Name: "pair", MinChildren: 2, MaxChildren: 1}}}, Definition{States: []State{{Op: Top}}}},
	}
	for _, c := range cases {
		if _, err := Compile(c.schema, c.definition, plenty()); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", c.name, err)
		}
	}
}

func TestQueriesRejectInvalidInputs(t *testing.T) {
	tree := treeKind(t)
	if _, err := Member(tree, Observation{}, plenty()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty observation: %v", err)
	}
	if _, err := Member(tree, obs(ObservationNode{Form: "scalar"}), plenty()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("observation without its required selector: %v", err)
	}
	if _, err := Member(tree, obs(ObservationNode{Form: "list-of"}), plenty()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("observation with an invalid width: %v", err)
	}
	if _, err := Member(tree, obs(recordObs(7)), plenty()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("observation child out of range: %v", err)
	}
	if _, err := Member(&Kind{}, obs(ObservationNode{Form: "any"}), plenty()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("uncompiled kind: %v", err)
	}
	if _, err := Includes(tree, tree, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil budget: %v", err)
	}
	other, err := Compile(Schema{Forms: []Form{{Name: "any"}}}, Definition{States: []State{{Op: Top}}}, plenty())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Includes(tree, other, plenty()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("different schemas: %v", err)
	}
}

// §7: budgets belong to the caller; their errors propagate unchanged and no
// partial answer is returned. Every query is retried with a growing budget.
func TestBudgetErrorsPropagate(t *testing.T) {
	tree, rows := treeKind(t), rowsKind(t)
	loop := obs(recordObs(1, 0), scalarObs(text))
	queries := map[string]func(Budget) error{
		"compile": func(b Budget) error {
			_, err := Compile(schema, Definition{States: []State{node("scalar", 0)}, Sequences: []Sequence{epsilon}}, b)
			return err
		},
		"member":   func(b Budget) error { _, err := Member(tree, loop, b); return err },
		"includes": func(b Budget) error { _, err := Includes(rows, tree, b); return err },
		"empty":    func(b Budget) error { _, err := Empty(tree, b); return err },
	}
	for name, query := range queries {
		exhaustedOnce := false
		for steps := uint64(0); ; steps++ {
			err := query(&testBudget{steps: steps, bytes: 1 << 40})
			if err == nil {
				break
			}
			if !errors.Is(err, errTestBudget) {
				t.Fatalf("%s with %d steps: budget error replaced by %v", name, steps, err)
			}
			exhaustedOnce = true
			if steps > 1<<20 {
				t.Fatalf("%s never completed", name)
			}
		}
		if !exhaustedOnce {
			t.Fatalf("%s consumed no budget", name)
		}
		if err := query(&testBudget{steps: 1 << 30, bytes: 0}); !errors.Is(err, errTestBudget) {
			t.Fatalf("%s with no byte allowance: %v", name, err)
		}
	}
}
