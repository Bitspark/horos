package declmodel

import (
	"errors"
	"slices"

	"github.com/Bitspark/horos/core/go/internal/kindmodel"
	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

// Sort distinguishes proper descriptions from finite, saturated constructors.
// Its zero value is the proper sort. Recursive kinds never make sorts recursive.
type Sort struct {
	inputs []Sort
	result *Sort
}

func (s Sort) IsProper() bool { return s.result == nil }
func (s Sort) Inputs() []Sort {
	r := make([]Sort, len(s.inputs))
	for i, x := range s.inputs {
		r[i] = cloneSort(x)
	}
	return r
}
func (s Sort) Result() (Sort, bool) {
	if s.result == nil {
		return Sort{}, false
	}
	return cloneSort(*s.result), true
}
func (s Sort) Value() core.Value {
	if s.IsProper() {
		return declValue("sort-proper")
	}
	a := make([]core.Value, len(s.inputs))
	for i, x := range s.inputs {
		a[i] = x.Value()
	}
	return declValue("sort-arrow", data.EncodeList(a), s.result.Value())
}
func cloneSort(s Sort) Sort {
	if s.IsProper() {
		return Sort{}
	}
	r := Sort{inputs: s.Inputs()}
	v := cloneSort(*s.result)
	r.result = &v
	return r
}
func sameSort(a, b Sort) bool {
	if a.IsProper() || b.IsProper() {
		return a.IsProper() && b.IsProper()
	}
	if len(a.inputs) != len(b.inputs) {
		return false
	}
	for i := range a.inputs {
		if !sameSort(a.inputs[i], b.inputs[i]) {
			return false
		}
	}
	return sameSort(*a.result, *b.result)
}
func compareSort(a, b Sort, work *budget, depth int) (bool, error) {
	if err := work.step(depth); err != nil {
		return false, err
	}
	if a.IsProper() || b.IsProper() {
		return a.IsProper() && b.IsProper(), nil
	}
	if len(a.inputs) != len(b.inputs) {
		return false, nil
	}
	for i := range a.inputs {
		ok, err := compareSort(a.inputs[i], b.inputs[i], work, depth+1)
		if err != nil || !ok {
			return ok, err
		}
	}
	return compareSort(*a.result, *b.result, work, depth+1)
}
func arrowSort(inputs []Sort, result Sort) Sort {
	r := cloneSort(result)
	return Sort{inputs: slices.Clone(inputs), result: &r}
}
func declValue(name string, args ...core.Value) core.Value {
	items := []core.Value{core.NewAtom([]byte("horos/decl/" + name))}
	return core.NewTuple(append(items, args...)...)
}
func listValue(values []core.Value) core.Value { return data.EncodeList(values) }
func readSort(value core.Value, g Grammar, work *budget, depth int) (Sort, error) {
	if err := work.step(depth); err != nil {
		return Sort{}, err
	}
	if _, ok := g.form(value, "sort-proper", 0); ok {
		return Sort{}, nil
	}
	a, ok := g.form(value, "sort-arrow", 2)
	if !ok {
		return Sort{}, failure(Invalid, "expected a finite sort")
	}
	xs, err := data.ReadList(a[0])
	if err != nil || len(xs) == 0 {
		return Sort{}, failure(Invalid, "sort arrow requires nonempty inputs")
	}
	inputs := make([]Sort, len(xs))
	for i, x := range xs {
		inputs[i], err = readSort(x, g, work, depth+1)
		if err != nil {
			return Sort{}, err
		}
	}
	r, err := readSort(a[1], g, work, depth+1)
	if err != nil {
		return Sort{}, err
	}
	return arrowSort(inputs, r), nil
}

type KindForm string

const (
	KindTop                KindForm = "top"
	KindBottom             KindForm = "bottom"
	KindParameterReference KindForm = "parameter"
	KindReference          KindForm = "reference"
	KindApplication        KindForm = "application"
	KindUnion              KindForm = "or"
	KindIntersection       KindForm = "and"
	KindNode               KindForm = "node"
	KindRecursive          KindForm = "nu"
	KindVariable           KindForm = "recursive-variable"
	KindArrow              KindForm = "arrow"
)

type Selector struct{ Key, Value string }
type KindOperand struct {
	path []int
	kind *Kind
}

func (o KindOperand) Path() []int { return slices.Clone(o.path) }
func (o KindOperand) Kind() *Kind { return copyKind(o.kind) }

type Kind struct {
	form               KindForm
	value              core.Value
	sort               Sort
	path               []int
	operands           []KindOperand
	target             DeclarationID
	parameter          ParameterID
	inputs             []*Kind
	result             *Kind
	nodeForm           string
	selectors          []Selector
	sequence           *Sequence
	equations          []*Kind
	entry, depth, slot int
}

func (k *Kind) Form() KindForm                 { return k.form }
func (k *Kind) Value() core.Value              { return k.value }
func (k *Kind) Sort() Sort                     { return cloneSort(k.sort) }
func (k *Kind) Path() []int                    { return slices.Clone(k.path) }
func (k *Kind) Operands() []KindOperand        { return slices.Clone(k.operands) }
func (k *Kind) Target() (DeclarationID, bool)  { return k.target, k.form == KindReference }
func (k *Kind) Parameter() (ParameterID, bool) { return k.parameter, k.form == KindParameterReference }
func (k *Kind) Inputs() []*Kind {
	r := make([]*Kind, len(k.inputs))
	for i, x := range k.inputs {
		r[i] = copyKind(x)
	}
	return r
}
func (k *Kind) Result() *Kind               { return copyKind(k.result) }
func (k *Kind) NodeForm() string            { return k.nodeForm }
func (k *Kind) Selectors() []Selector       { return slices.Clone(k.selectors) }
func (k *Kind) Sequence() *Sequence         { return copySequence(k.sequence) }
func (k *Kind) Recursive() (int, int, bool) { return k.depth, k.slot, k.form == KindVariable }
func (k *Kind) Equations() []*Kind {
	r := make([]*Kind, len(k.equations))
	for i, x := range k.equations {
		r[i] = copyKind(x)
	}
	return r
}
func (k *Kind) Entry() int { return k.entry }
func copyKind(k *Kind) *Kind {
	if k == nil {
		return nil
	}
	r := *k
	return &r
}

type SequenceForm string

const (
	SequenceEpsilon     SequenceForm = "epsilon"
	SequenceItem        SequenceForm = "item"
	SequenceConcat      SequenceForm = "concat"
	SequenceAlternative SequenceForm = "alternative"
	SequenceStar        SequenceForm = "star"
)

type Sequence struct {
	form     SequenceForm
	value    core.Value
	path     []int
	element  *Kind
	children []*Sequence
}

func (q *Sequence) Form() SequenceForm { return q.form }
func (q *Sequence) Value() core.Value  { return q.value }
func (q *Sequence) Path() []int        { return slices.Clone(q.path) }
func (q *Sequence) Element() *Kind     { return copyKind(q.element) }
func (q *Sequence) Children() []*Sequence {
	r := make([]*Sequence, len(q.children))
	for i, x := range q.children {
		r[i] = copySequence(x)
	}
	return r
}
func copySequence(q *Sequence) *Sequence {
	if q == nil {
		return nil
	}
	r := *q
	return &r
}

type kindTerm struct {
	form      KindForm
	sort      Sort
	origin    *Kind
	children  []*kindTerm
	inputs    []*kindTerm
	result    *kindTerm
	nodeForm  string
	selectors []kindmodel.Selector
	sequence  *sequenceTerm
}
type sequenceTerm struct {
	form     SequenceForm
	element  *kindTerm
	children []*sequenceTerm
}
type KindHandle struct {
	owner      *Program
	term       *kindTerm
	expression *Kind
}

func (h KindHandle) Sort() Sort {
	if h.term == nil {
		return Sort{}
	}
	return cloneSort(h.term.sort)
}
func (h KindHandle) Expression() *Kind { return copyKind(h.expression) }
func topKind(sort Sort) *Kind {
	return &Kind{form: KindTop, sort: sort, value: declValue("top", sort.Value())}
}
func topTerm(sort Sort) *kindTerm {
	k := topKind(sort)
	return &kindTerm{form: KindTop, sort: sort, origin: k}
}
func arrowKind(inputs []*Kind, result *Kind) *Kind {
	sorts := make([]Sort, len(inputs))
	values := make([]core.Value, len(inputs))
	k := &Kind{form: KindArrow, inputs: inputs, result: result}
	for i, x := range inputs {
		sorts[i] = x.sort
		values[i] = x.value
		k.operands = append(k.operands, KindOperand{path: []int{1, i + 1}, kind: x})
	}
	k.operands = append(k.operands, KindOperand{path: []int{2}, kind: result})
	k.sort = arrowSort(sorts, result.sort)
	k.value = declValue("arrow", data.EncodeList(values), result.value)
	return k
}

func (r *expressionReader) readKind(value core.Value, owner *Declaration, path []int, groups []int, depth int) (*Kind, error) {
	k, err := r.readKindIn(value, owner, path, groups, depth)
	if e, ok := err.(*Error); ok && len(e.Path) == 0 {
		copy := *e
		copy.Path = slices.Clone(path)
		err = &copy
	}
	return k, err
}
func (r *expressionReader) readKindIn(value core.Value, owner *Declaration, path []int, groups []int, depth int) (*Kind, error) {
	if err := r.budget.step(depth); err != nil {
		return nil, err
	}
	g := r.program.grammar
	tag, a, ok := head(value, g.DeclarationPrefix)
	if !ok {
		return nil, failure(Invalid, "expected a kind expression")
	}
	k := &Kind{value: value, path: slices.Clone(path)}
	child := func(v core.Value, p []int) (*Kind, error) {
		x, err := r.readKind(v, owner, append(slices.Clone(path), p...), groups, depth+1)
		if err == nil {
			k.operands = append(k.operands, KindOperand{path: p, kind: x})
		}
		return x, err
	}
	switch tag {
	case "top", "bottom":
		if len(a) != 1 {
			return nil, failure(Invalid, "kind extremum requires a sort")
		}
		s, err := readSort(a[0], g, r.budget, depth+1)
		if err != nil {
			return nil, err
		}
		k.sort = s
		if tag == "top" {
			k.form = KindTop
		} else {
			k.form = KindBottom
		}
	case "kind-param":
		if len(a) != 1 {
			return nil, failure(Invalid, "invalid kind parameter")
		}
		slot, ok := index(a[0])
		if !ok || slot >= len(owner.kindParameters) {
			return nil, failure(Invalid, "unbound kind parameter")
		}
		k.form = KindParameterReference
		k.parameter = ParameterID{Declaration: owner.id, Index: slot}
		k.sort = owner.kindParameters[slot].Sort
	case "kind-local", "kind-imported":
		id, err := r.targetCategory(value, owner.id, KindDefinition, true)
		if err != nil {
			return nil, err
		}
		d := r.program.declarations[id]
		if len(d.kindParameters) != 0 {
			return nil, failure(Invalid, "parameterized kind requires a saturated application")
		}
		k.form = KindReference
		k.target = id
		k.sort = d.sort
		r.edges[owner.id] = append(r.edges[owner.id], id)
	case "kind-apply":
		if len(a) != 2 {
			return nil, failure(Invalid, "invalid kind application")
		}
		id, err := r.targetCategory(a[0], owner.id, KindDefinition, true)
		if err != nil {
			return nil, err
		}
		d := r.program.declarations[id]
		xs, err := data.ReadList(a[1])
		if err != nil || len(xs) == 0 || len(xs) != len(d.kindParameters) {
			return nil, failure(Invalid, "kind application must be exactly saturated")
		}
		ref := &Kind{form: KindReference, value: a[0], target: id, sort: d.sort, path: append(slices.Clone(path), 1)}
		k.form = KindApplication
		k.sort = d.sort
		k.operands = append(k.operands, KindOperand{path: []int{1}, kind: ref})
		r.edges[owner.id] = append(r.edges[owner.id], id)
		for i, x := range xs {
			v, err := child(x, []int{2, i + 1})
			if err != nil {
				return nil, err
			}
			if !sameSort(v.sort, d.kindParameters[i].Sort) {
				return nil, failure(KindMismatch, "kind argument sort mismatch")
			}
			k.inputs = append(k.inputs, v)
		}
	case "or", "and":
		if len(a) != 2 {
			return nil, failure(Invalid, "kind Boolean form requires sort and operands")
		}
		s, err := readSort(a[0], g, r.budget, depth+1)
		if err != nil {
			return nil, err
		}
		k.sort = s
		if tag == "or" {
			k.form = KindUnion
		} else {
			k.form = KindIntersection
		}
		xs, err := data.ReadList(a[1])
		if err != nil || len(xs) < 2 {
			return nil, failure(Invalid, "kind Boolean form requires at least two operands")
		}
		for i, x := range xs {
			v, err := child(x, []int{2, i + 1})
			if err != nil {
				return nil, err
			}
			if !sameSort(v.sort, s) {
				return nil, failure(KindMismatch, "kind Boolean operand sort mismatch")
			}
			k.inputs = append(k.inputs, v)
		}
	case "arrow":
		if len(a) != 2 {
			return nil, failure(Invalid, "invalid arrow contract")
		}
		xs, err := data.ReadList(a[0])
		if err != nil || len(xs) == 0 {
			return nil, failure(Invalid, "arrow contract requires nonempty inputs")
		}
		ss := make([]Sort, len(xs))
		for i, x := range xs {
			v, err := child(x, []int{1, i + 1})
			if err != nil {
				return nil, err
			}
			ss[i] = v.sort
			k.inputs = append(k.inputs, v)
		}
		v, err := child(a[1], []int{2})
		if err != nil {
			return nil, err
		}
		k.form = KindArrow
		k.result = v
		k.sort = arrowSort(ss, v.sort)
	case "node":
		if len(a) != 3 {
			return nil, failure(Invalid, "invalid kind node")
		}
		name, err := data.ReadText(a[0])
		if err != nil {
			return nil, failure(Invalid, "kind node form must be text")
		}
		k.form = KindNode
		k.nodeForm = name
		entries, err := data.ReadMap(a[1])
		if err != nil {
			return nil, failure(Invalid, "node selectors must be a canonical map")
		}
		for _, entry := range entries {
			key, e1 := data.ReadText(entry[0])
			val, e2 := data.ReadText(entry[1])
			if e1 != nil || e2 != nil {
				return nil, failure(Invalid, "node selectors must contain text keys and values")
			}
			k.selectors = append(k.selectors, Selector{Key: key, Value: val})
		}
		q, err := r.readSequence(a[2], owner, append(slices.Clone(path), 3), groups, depth+1, k)
		if err != nil {
			return nil, err
		}
		k.sequence = q
	case "nu":
		if len(a) != 2 {
			return nil, failure(Invalid, "invalid recursive kind")
		}
		xs, err := data.ReadList(a[0])
		entry, valid := index(a[1])
		if err != nil || len(xs) == 0 || !valid || entry >= len(xs) {
			return nil, failure(Invalid, "invalid recursive equation group or entry")
		}
		k.form = KindRecursive
		k.entry = entry
		inner := append(slices.Clone(groups), len(xs))
		for i, x := range xs {
			v, err := r.readKind(x, owner, append(slices.Clone(path), 1, i+1), inner, depth+1)
			if err != nil {
				return nil, err
			}
			if !v.sort.IsProper() {
				return nil, failure(KindMismatch, "recursive kind equations must be proper")
			}
			k.equations = append(k.equations, v)
			k.operands = append(k.operands, KindOperand{path: []int{1, i + 1}, kind: v})
		}
	case "recursive-var":
		if len(a) != 2 {
			return nil, failure(Invalid, "invalid recursive variable")
		}
		d, ok1 := index(a[0])
		s, ok2 := index(a[1])
		if !ok1 || !ok2 || d >= len(groups) || s >= groups[len(groups)-1-d] {
			return nil, failure(Invalid, "unbound recursive kind variable")
		}
		k.form = KindVariable
		k.depth = d
		k.slot = s
	default:
		return nil, failure(Invalid, "unknown kind expression form")
	}
	return k, nil
}

func (r *expressionReader) readSequence(value core.Value, owner *Declaration, path []int, groups []int, depth int, node *Kind) (*Sequence, error) {
	q, err := r.readSequenceIn(value, owner, path, groups, depth, node)
	if e, ok := err.(*Error); ok && len(e.Path) == 0 {
		copy := *e
		copy.Path = slices.Clone(path)
		err = &copy
	}
	return q, err
}
func (r *expressionReader) readSequenceIn(value core.Value, owner *Declaration, path []int, groups []int, depth int, node *Kind) (*Sequence, error) {
	if err := r.budget.step(depth); err != nil {
		return nil, err
	}
	tag, a, ok := head(value, r.program.grammar.DeclarationPrefix)
	if !ok {
		return nil, failure(Invalid, "expected a finite child-sequence expression")
	}
	q := &Sequence{value: value, path: slices.Clone(path)}
	switch tag {
	case "epsilon":
		if len(a) != 0 {
			return nil, failure(Invalid, "epsilon has no operands")
		}
		q.form = SequenceEpsilon
	case "item":
		if len(a) != 1 {
			return nil, failure(Invalid, "item requires a kind")
		}
		x, err := r.readKind(a[0], owner, append(slices.Clone(path), 1), groups, depth+1)
		if err != nil {
			return nil, err
		}
		if !x.sort.IsProper() {
			return nil, failure(KindMismatch, "sequence items require proper kinds")
		}
		q.form = SequenceItem
		q.element = x
		relative := slices.Clone(x.path[len(node.path):])
		node.operands = append(node.operands, KindOperand{path: relative, kind: x})
	case "concat", "alternative":
		if len(a) != 1 {
			return nil, failure(Invalid, "invalid sequence combinator")
		}
		xs, err := data.ReadList(a[0])
		if err != nil || len(xs) < 2 {
			return nil, failure(Invalid, "sequence combinator requires at least two operands")
		}
		if tag == "concat" {
			q.form = SequenceConcat
		} else {
			q.form = SequenceAlternative
		}
		for i, v := range xs {
			x, err := r.readSequence(v, owner, append(slices.Clone(path), 1, i+1), groups, depth+1, node)
			if err != nil {
				return nil, err
			}
			q.children = append(q.children, x)
		}
	case "star":
		if len(a) != 1 {
			return nil, failure(Invalid, "star requires a sequence")
		}
		x, err := r.readSequence(a[0], owner, append(slices.Clone(path), 1), groups, depth+1, node)
		if err != nil {
			return nil, err
		}
		q.form = SequenceStar
		q.children = []*Sequence{x}
	default:
		return nil, failure(Invalid, "unknown sequence form")
	}
	return q, nil
}

func (c *checker) instantiate(k *Kind, arguments []*kindTerm, groups [][]*kindTerm, depth int) (*kindTerm, error) {
	if err := c.allocate(depth); err != nil {
		return nil, err
	}
	if k.form == KindParameterReference {
		if k.parameter.Index >= len(arguments) {
			return nil, failure(Invalid, "unbound kind argument")
		}
		return arguments[k.parameter.Index], nil
	}
	if k.form == KindVariable {
		return groups[len(groups)-1-k.depth][k.slot], nil
	}
	if k.form == KindReference || k.form == KindApplication {
		var id DeclarationID
		var args []*kindTerm
		if k.form == KindReference {
			id = k.target
		} else {
			id = k.operands[0].kind.target
			for _, x := range k.inputs {
				v, err := c.instantiate(x, arguments, groups, depth+1)
				if err != nil {
					return nil, err
				}
				if err := c.allocate(depth); err != nil {
					return nil, err
				}
				c.kindObligations = append(c.kindObligations, v)
				args = append(args, v)
			}
		}
		d := c.program.declarations[id]
		if d == nil || d.kindBody == nil {
			return nil, failure(Invalid, "unresolved kind definition")
		}
		return c.namedKind(d, args, k, depth+1)
	}
	if k.form == KindRecursive {
		vars := make([]*kindTerm, len(k.equations))
		for i := range vars {
			vars[i] = &kindTerm{form: KindVariable, origin: k}
		}
		inner := append(slices.Clone(groups), vars)
		for i, x := range k.equations {
			v, err := c.instantiate(x, arguments, inner, depth+1)
			if err != nil {
				return nil, err
			}
			vars[i].children = []*kindTerm{v}
		}
		for _, v := range vars {
			if err := c.allocate(depth); err != nil {
				return nil, err
			}
			c.kindObligations = append(c.kindObligations, v)
		}
		return vars[k.entry], nil
	}
	t := &kindTerm{form: k.form, sort: k.sort, origin: k, nodeForm: k.nodeForm}
	for _, s := range k.selectors {
		t.selectors = append(t.selectors, kindmodel.Selector{Key: s.Key, Value: s.Value})
	}
	for _, x := range k.inputs {
		v, err := c.instantiate(x, arguments, groups, depth+1)
		if err != nil {
			return nil, err
		}
		t.inputs = append(t.inputs, v)
		if k.form == KindUnion || k.form == KindIntersection {
			t.children = append(t.children, v)
		}
	}
	if k.result != nil {
		v, err := c.instantiate(k.result, arguments, groups, depth+1)
		if err != nil {
			return nil, err
		}
		t.result = v
	}
	if k.sequence != nil {
		v, err := c.instantiateSequence(k.sequence, arguments, groups, depth+1)
		if err != nil {
			return nil, err
		}
		t.sequence = v
	}
	return t, nil
}
func (c *checker) instantiateSequence(q *Sequence, args []*kindTerm, groups [][]*kindTerm, depth int) (*sequenceTerm, error) {
	if err := c.allocate(depth); err != nil {
		return nil, err
	}
	r := &sequenceTerm{form: q.form}
	if q.element != nil {
		v, err := c.instantiate(q.element, args, groups, depth+1)
		if err != nil {
			return nil, err
		}
		r.element = v
	}
	for _, x := range q.children {
		v, err := c.instantiateSequence(x, args, groups, depth+1)
		if err != nil {
			return nil, err
		}
		r.children = append(r.children, v)
	}
	return r, nil
}

func (c *checker) properKind(root *kindTerm) (*kindmodel.Kind, error) {
	if value, ok := c.compiled[root]; ok {
		return value, nil
	}
	definition := kindmodel.Definition{}
	states := map[*kindTerm]kindmodel.StateID{}
	sequences := map[*sequenceTerm]kindmodel.SeqID{}
	var state func(*kindTerm, int) (kindmodel.StateID, error)
	var sequence func(*sequenceTerm, int) (kindmodel.SeqID, error)
	sequence = func(q *sequenceTerm, depth int) (kindmodel.SeqID, error) {
		if id, ok := sequences[q]; ok {
			return id, nil
		}
		if err := c.work.step(depth); err != nil {
			return 0, err
		}
		id := kindmodel.SeqID(len(definition.Sequences))
		sequences[q] = id
		definition.Sequences = append(definition.Sequences, kindmodel.Sequence{})
		s := kindmodel.Sequence{}
		switch q.form {
		case SequenceEpsilon:
			s.Op = kindmodel.Epsilon
		case SequenceItem:
			s.Op = kindmodel.Item
			v, err := state(q.element, depth+1)
			if err != nil {
				return 0, err
			}
			s.Item = v
		case SequenceConcat:
			s.Op = kindmodel.Concat
		case SequenceAlternative:
			s.Op = kindmodel.Choice
		case SequenceStar:
			s.Op = kindmodel.Star
		}
		for _, x := range q.children {
			v, err := sequence(x, depth+1)
			if err != nil {
				return 0, err
			}
			s.Args = append(s.Args, v)
		}
		definition.Sequences[id] = s
		return id, nil
	}
	state = func(t *kindTerm, depth int) (kindmodel.StateID, error) {
		if id, ok := states[t]; ok {
			return id, nil
		}
		if err := c.work.step(depth); err != nil {
			return 0, err
		}
		if !t.sort.IsProper() {
			return 0, failure(KindMismatch, "expected a proper kind")
		}
		id := kindmodel.StateID(len(definition.States))
		states[t] = id
		definition.States = append(definition.States, kindmodel.State{})
		s := kindmodel.State{}
		switch t.form {
		case KindTop:
			s.Op = kindmodel.Top
		case KindBottom:
			s.Op = kindmodel.Bottom
		case KindUnion:
			s.Op = kindmodel.Or
		case KindIntersection:
			s.Op = kindmodel.And
		case KindVariable:
			s.Op = kindmodel.Alias
		case KindNode:
			s.Op = kindmodel.Node
			q, err := sequence(t.sequence, depth+1)
			if err != nil {
				return 0, err
			}
			s.Node = kindmodel.NodePattern{Form: t.nodeForm, Selectors: slices.Clone(t.selectors), Sequence: q}
		default:
			return 0, failure(Invalid, "unsupported proper kind state")
		}
		for _, x := range t.children {
			v, err := state(x, depth+1)
			if err != nil {
				return 0, err
			}
			s.Args = append(s.Args, v)
		}
		definition.States[id] = s
		return id, nil
	}
	id, err := state(root, 0)
	if err != nil {
		return nil, err
	}
	definition.Root = id
	k, err := kindmodel.Compile(c.program.grammar.Schema, definition, c.work)
	if err != nil {
		return nil, kindError(err)
	}
	if err := c.allocate(0); err != nil {
		return nil, err
	}
	c.compiled[root] = k
	return k, nil
}
func kindError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*Error); ok {
		return err
	}
	if errors.Is(err, kindmodel.ErrExhausted) {
		return failure(Exhausted, err.Error())
	}
	return failure(Invalid, err.Error())
}

func (p *Program) CompileKind(module Digest, value core.Value) (KindHandle, error) {
	h, err := p.compileKind(module, value)
	return h, specializationError(err)
}
func (p *Program) compileKind(module Digest, value core.Value) (KindHandle, error) {
	if p == nil || p.modules[module] == nil {
		return KindHandle{}, failure(Invalid, "unknown kind resolution module")
	}
	raw := &budget{limits: p.limits}
	if err := raw.value(value, 0); err != nil {
		return KindHandle{}, err
	}
	work := &budget{limits: p.limits}
	r := expressionReader{program: p, budget: work, edges: make(map[DeclarationID][]DeclarationID)}
	owner := &Declaration{id: DeclarationID{Module: module}}
	k, err := r.readKind(value, owner, nil, nil, 0)
	if err != nil {
		return KindHandle{}, err
	}
	c := newChecker(p, work)
	t, err := c.instantiate(k, nil, nil, 0)
	if err == nil {
		err = c.admitKind(t, 0)
	}
	if err != nil {
		return KindHandle{}, err
	}
	return KindHandle{owner: p, term: t, expression: k}, nil
}
