package declmodel

import (
	"slices"
	"strconv"
	"strings"

	"github.com/Bitspark/horos/core/go/internal/kindmodel"
	core "github.com/bitspark/ontos/core/go"
)

type environment struct {
	bindings map[ParameterID]*semanticValue
}
type semanticValue struct {
	sort            Sort
	guarantee       *kindTerm
	expression      *Expression
	env             *environment
	declaration     *Declaration
	rigid, abstract bool
	parameter       ParameterID
	descriptor      core.Value
}
type valueKey struct {
	expression *Expression
	env        *environment
}
type invocationKey struct {
	declaration *Declaration
	arguments   string
}
type inclusionKey struct{ actual, expected *kindTerm }
type constructorKey struct {
	declaration *Declaration
	expected    *kindTerm
}
type kindInstanceKey struct {
	declaration *Declaration
	arguments   string
}
type checker struct {
	program                *Program
	work                   *budget
	compiled               map[*kindTerm]*kindmodel.Kind
	values                 map[valueKey]*semanticValue
	invocations            map[invocationKey]*semanticValue
	inclusions             map[inclusionKey]bool
	constructors           map[constructorKey]bool
	ids                    map[*semanticValue]uint64
	nextID                 uint64
	refinements            map[*semanticValue]*kindTerm
	contracts              map[string]*kindTerm
	kindObligations        []*kindTerm
	checkedKindObligations int
	kindInstances          map[kindInstanceKey]*kindTerm
	kindIDs                map[*kindTerm]uint64
	nextKindID             uint64
}

func newChecker(p *Program, work *budget) *checker {
	return &checker{program: p, work: work, compiled: map[*kindTerm]*kindmodel.Kind{}, values: map[valueKey]*semanticValue{}, invocations: map[invocationKey]*semanticValue{}, inclusions: map[inclusionKey]bool{}, constructors: map[constructorKey]bool{}, ids: map[*semanticValue]uint64{}, refinements: map[*semanticValue]*kindTerm{}, contracts: map[string]*kindTerm{}, kindInstances: map[kindInstanceKey]*kindTerm{}, kindIDs: map[*kindTerm]uint64{}}
}
func (c *checker) allocate(depth int) error {
	if err := c.work.step(depth); err != nil {
		return err
	}
	return c.work.reserveBytes(64)
}
func (c *checker) bound(v *semanticValue) *kindTerm {
	if k := c.refinements[v]; k != nil {
		return k
	}
	return v.guarantee
}
func (c *checker) rigid(k *kindTerm, id ParameterID, depth int) (*semanticValue, error) {
	if err := c.allocate(depth); err != nil {
		return nil, err
	}
	return &semanticValue{sort: k.sort, guarantee: k, rigid: true, parameter: id}, nil
}
func (c *checker) formKind(t *kindTerm, depth int) error {
	if err := c.work.step(depth); err != nil {
		return err
	}
	if t.sort.IsProper() {
		_, err := c.properKind(t)
		return err
	}
	switch t.form {
	case KindTop, KindBottom:
		return nil
	case KindUnion, KindIntersection:
		for _, x := range t.children {
			if err := c.formKind(x, depth+1); err != nil {
				return err
			}
		}
	case KindArrow:
		for _, x := range t.inputs {
			if err := c.formKind(x, depth+1); err != nil {
				return err
			}
		}
		return c.formKind(t.result, depth+1)
	default:
		return failure(Invalid, "invalid constructor kind")
	}
	return nil
}
func (c *checker) admitKind(t *kindTerm, depth int) error {
	if err := c.formKind(t, depth); err != nil {
		return err
	}
	for c.checkedKindObligations < len(c.kindObligations) {
		v := c.kindObligations[c.checkedKindObligations]
		c.checkedKindObligations++
		if err := c.formKind(v, depth+1); err != nil {
			return err
		}
	}
	return nil
}
func (c *checker) namedKind(d *Declaration, args []*kindTerm, origin *Kind, depth int) (*kindTerm, error) {
	var key strings.Builder
	for _, arg := range args {
		if err := c.work.step(depth); err != nil {
			return nil, err
		}
		id, ok := c.kindIDs[arg]
		if !ok {
			if err := c.allocate(depth); err != nil {
				return nil, err
			}
			c.nextKindID++
			id = c.nextKindID
			c.kindIDs[arg] = id
		}
		s := strconv.FormatUint(id, 10)
		if err := c.work.reserveBytes(len(s) + 1); err != nil {
			return nil, err
		}
		key.WriteString(s)
		key.WriteByte('/')
	}
	k := kindInstanceKey{declaration: d, arguments: key.String()}
	v := c.kindInstances[k]
	if v == nil {
		var err error
		v, err = c.instantiate(d.kindBody, args, nil, depth+1)
		if err != nil {
			return nil, err
		}
		if err := c.allocate(depth); err != nil {
			return nil, err
		}
		c.kindInstances[k] = v
	}
	if err := c.allocate(depth); err != nil {
		return nil, err
	}
	copy := *v
	copy.origin = origin
	return &copy, nil
}

func (c *checker) value(e *Expression, env *environment, depth int) (*semanticValue, error) {
	if err := c.work.step(depth); err != nil {
		return nil, err
	}
	if e.kind == Parameter {
		if env == nil || env.bindings[e.parameter] == nil {
			return nil, failure(Invalid, "unbound type parameter")
		}
		return env.bindings[e.parameter], nil
	}
	key := valueKey{e, env}
	if v := c.values[key]; v != nil {
		return v, nil
	}
	var v *semanticValue
	switch e.kind {
	case Reference:
		d := c.program.declarations[e.target]
		if d == nil || d.form != TypeDefinition {
			return nil, failure(Invalid, "invalid type reference")
		}
		if len(d.parameters) > 0 {
			v = &semanticValue{sort: d.sort, guarantee: d.semantic, declaration: d, expression: e}
		} else {
			var err error
			v, err = c.value(d.expression, nil, depth+1)
			if err != nil {
				return nil, err
			}
		}
	case Application:
		callee, err := c.value(e.callee, env, depth+1)
		if err != nil {
			return nil, err
		}
		args := make([]*semanticValue, len(e.arguments))
		for i, x := range e.arguments {
			args[i], err = c.value(x, env, depth+1)
			if err != nil {
				return nil, err
			}
		}
		v, err = c.apply(callee, args, e, depth+1)
		if err != nil {
			return nil, err
		}
	case Descriptor:
		v = &semanticValue{sort: e.sort, guarantee: topTerm(e.sort), expression: e, env: env}
	default:
		return nil, failure(Invalid, "invalid expression")
	}
	if err := c.allocate(depth); err != nil {
		return nil, err
	}
	c.values[key] = v
	return v, nil
}
func (c *checker) invocation(d *Declaration, args []*semanticValue, depth int) (invocationKey, error) {
	var b strings.Builder
	for _, v := range args {
		if err := c.work.step(depth); err != nil {
			return invocationKey{}, err
		}
		id, ok := c.ids[v]
		if !ok {
			if err := c.allocate(depth); err != nil {
				return invocationKey{}, err
			}
			c.nextID++
			id = c.nextID
			c.ids[v] = id
		}
		text := strconv.FormatUint(id, 10)
		if err := c.work.reserveBytes(len(text) + 1); err != nil {
			return invocationKey{}, err
		}
		b.WriteString(text)
		b.WriteByte('/')
	}
	return invocationKey{declaration: d, arguments: b.String()}, nil
}
func (c *checker) apply(callee *semanticValue, args []*semanticValue, source *Expression, depth int) (*semanticValue, error) {
	if err := c.work.step(depth); err != nil {
		return nil, err
	}
	if callee.sort.IsProper() || len(args) != len(callee.sort.inputs) || len(args) == 0 {
		return nil, failure(KindMismatch, "application requires a saturated constructor")
	}
	for i, v := range args {
		ok, err := compareSort(v.sort, callee.sort.inputs[i], c.work, depth+1)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, failure(KindMismatch, "argument sort mismatch")
		}
	}
	if callee.declaration != nil {
		d := callee.declaration
		for i, arg := range args {
			verdict, err := c.satisfies(arg, d.semantic.inputs[i], depth+1)
			if err != nil {
				return nil, err
			}
			if verdict != Proven {
				return nil, c.argumentFailure(source, i, d.semantic.inputs[i], verdict)
			}
		}
		key, err := c.invocation(d, args, depth+1)
		if err != nil {
			return nil, err
		}
		if v := c.invocations[key]; v != nil {
			return v, nil
		}
		if err := c.allocate(depth); err != nil {
			return nil, err
		}
		env := &environment{bindings: map[ParameterID]*semanticValue{}}
		for i, v := range args {
			env.bindings[ParameterID{Declaration: d.id, Index: i}] = v
		}
		v, err := c.value(d.expression, env, depth+1)
		if err != nil {
			return nil, err
		}
		c.invocations[key] = v
		return v, nil
	}
	k, err := c.applyContracts(c.bound(callee), args, depth+1)
	if err != nil {
		if source != nil {
			if detail, ok := err.(*Error); ok && detail.Declaration == "" {
				copy := *detail
				copy.Path = append(slices.Clone(source.path), detail.Path...)
				if source.callee.kind == Parameter {
					copy.CalleeParameter = source.callee.parameter
				}
				if source.callee.kind == Reference {
					copy.Target = source.callee.target
				}
				return nil, &copy
			}
		}
		return nil, err
	}
	if err := c.allocate(depth); err != nil {
		return nil, err
	}
	return &semanticValue{sort: *callee.sort.result, guarantee: k, abstract: true, expression: source}, nil
}
func (c *checker) argumentFailure(source *Expression, index int, k *kindTerm, verdict Verdict) error {
	code := UnprovedObligation
	if verdict == Refuted {
		code = BoundViolation
	}
	if source == nil {
		return &Error{Code: code, Subject: SubjectRequest, Expected: copyKind(k.origin), Message: "argument does not establish its constructor contract"}
	}
	err := applicationFailure(code, source.callee, k.origin, "argument does not establish its constructor contract")
	if e, ok := err.(*Error); ok {
		e.Path = append(slices.Clone(source.path), 2, index+1)
	}
	return err
}

func combine(form KindForm, sort Sort, terms []*kindTerm) *kindTerm {
	if len(terms) == 0 {
		if form == KindUnion {
			return &kindTerm{form: KindBottom, sort: sort, origin: &Kind{form: KindBottom, sort: sort, value: declValue("bottom", sort.Value())}}
		}
		return topTerm(sort)
	}
	if len(terms) == 1 {
		return terms[0]
	}
	k := &Kind{form: form, sort: sort}
	values := make([]core.Value, len(terms))
	for i, t := range terms {
		k.inputs = append(k.inputs, t.origin)
		k.operands = append(k.operands, KindOperand{path: []int{2, i + 1}, kind: t.origin})
		if t.origin != nil {
			values[i] = t.origin.value
		} else {
			values[i] = topKind(sort).value
		}
	}
	k.value = declValue(string(form), sort.Value(), listValue(values))
	return &kindTerm{form: form, sort: sort, origin: k, children: terms, inputs: terms}
}

// alternatives distributes only kind-level Boolean forms. Node children remain
// inside the node: Record<A|B> is not split into homogeneous records.
func (c *checker) alternatives(t *kindTerm, depth int) ([][]*kindTerm, error) {
	if err := c.work.step(depth); err != nil {
		return nil, err
	}
	switch t.form {
	case KindVariable:
		return c.alternatives(t.children[0], depth+1)
	case KindBottom:
		return nil, nil
	case KindTop:
		return [][]*kindTerm{{}}, nil
	case KindUnion:
		var result [][]*kindTerm
		for _, x := range t.children {
			a, err := c.alternatives(x, depth+1)
			if err != nil {
				return nil, err
			}
			if err := c.work.reserveBytes(len(a) * 24); err != nil {
				return nil, err
			}
			result = append(result, a...)
		}
		return result, nil
	case KindIntersection:
		result := [][]*kindTerm{{}}
		for _, x := range t.children {
			a, err := c.alternatives(x, depth+1)
			if err != nil {
				return nil, err
			}
			var next [][]*kindTerm
			for _, left := range result {
				for _, right := range a {
					if err := c.allocate(depth); err != nil {
						return nil, err
					}
					joint := append(slices.Clone(left), right...)
					if err := c.work.reserveBytes(len(joint) * 8); err != nil {
						return nil, err
					}
					next = append(next, joint)
				}
			}
			result = next
		}
		return result, nil
	default:
		return [][]*kindTerm{{t}}, nil
	}
}
func (c *checker) applyContracts(contract *kindTerm, args []*semanticValue, depth int) (*kindTerm, error) {
	alternatives, err := c.alternatives(contract, depth+1)
	if err != nil {
		return nil, err
	}
	resultSort := *contract.sort.result
	if len(alternatives) == 0 {
		return combine(KindUnion, resultSort, nil), nil
	}
	unique := []*semanticValue{}
	cases := map[*semanticValue][][]*kindTerm{}
	for _, v := range args {
		if _, ok := cases[v]; ok {
			continue
		}
		var bound *kindTerm
		if v.rigid || v.abstract {
			bound = c.bound(v)
		} else {
			bound = v.guarantee
		}
		a, err := c.alternatives(bound, depth+1)
		if err != nil {
			return nil, err
		}
		cases[v] = a
		unique = append(unique, v)
	}
	var results []*kindTerm
	var explore func(int, *checker) error
	explore = func(index int, branch *checker) error {
		if err := c.work.step(depth + index); err != nil {
			return err
		}
		if index < len(unique) {
			v := unique[index]
			for _, joint := range cases[v] {
				if err := c.allocate(depth); err != nil {
					return err
				}
				copy := newChecker(c.program, c.work)
				copy.compiled = c.compiled
				copy.inclusions = c.inclusions
				for key, value := range branch.refinements {
					copy.refinements[key] = value
				}
				copy.refinements[v] = combine(KindIntersection, v.sort, joint)
				if err := explore(index+1, copy); err != nil {
					return err
				}
			}
			return nil
		}
		for _, alternative := range alternatives {
			var applicable []*kindTerm
			failedIndex := -1
			var failedKind *Kind
			for _, arrow := range alternative {
				if arrow.form != KindArrow {
					continue
				}
				fits := true
				for i, arg := range args {
					v, err := branch.satisfies(arg, arrow.inputs[i], depth+1)
					if err != nil {
						return err
					}
					if v != Proven {
						fits = false
						if failedIndex < 0 {
							failedIndex = i
							failedKind = arrow.inputs[i].origin
						}
						break
					}
				}
				if fits {
					applicable = append(applicable, arrow.result)
				}
			}
			if len(applicable) == 0 {
				path := []int{1}
				if failedIndex >= 0 {
					path = []int{2, failedIndex + 1}
				}
				return &Error{Code: UnprovedObligation, Subject: SubjectSource, Path: path, Expected: copyKind(failedKind), Message: "abstract constructor has no applicable guarantee for every argument case"}
			}
			results = append(results, combine(KindIntersection, resultSort, applicable))
		}
		return nil
	}
	if err := explore(0, c); err != nil {
		return nil, err
	}
	return combine(KindUnion, resultSort, results), nil
}

func (c *checker) includes(actual, expected *kindTerm, depth int) (bool, error) {
	if err := c.work.step(depth); err != nil {
		return false, err
	}
	same, err := compareSort(actual.sort, expected.sort, c.work, depth+1)
	if err != nil {
		return false, err
	}
	if !same {
		return false, nil
	}
	if actual == expected || expected.form == KindTop || actual.form == KindBottom {
		return true, nil
	}
	key := inclusionKey{actual, expected}
	if v, ok := c.inclusions[key]; ok {
		return v, nil
	}
	if actual.sort.IsProper() {
		a, err := c.properKind(actual)
		if err != nil {
			return false, err
		}
		b, err := c.properKind(expected)
		if err != nil {
			return false, err
		}
		v, err := kindmodel.Includes(a, b, c.work)
		if err != nil {
			return false, kindError(err)
		}
		if err := c.allocate(depth); err != nil {
			return false, err
		}
		c.inclusions[key] = v
		return v, nil
	}
	sources, err := c.alternatives(actual, depth+1)
	if err != nil {
		return false, err
	}
	targets, err := c.alternatives(expected, depth+1)
	if err != nil {
		return false, err
	}
	for _, source := range sources {
		established := false
		for _, target := range targets {
			all := true
			for _, arrow := range target {
				if arrow.form != KindArrow {
					all = false
					break
				}
				args := make([]*semanticValue, len(arrow.inputs))
				for i, k := range arrow.inputs {
					args[i], err = c.rigid(k, ParameterID{}, depth+1)
					if err != nil {
						return false, err
					}
				}
				result, err := c.applyContracts(combine(KindIntersection, actual.sort, source), args, depth+1)
				if err != nil {
					if detail, ok := err.(*Error); ok && detail.Code == UnprovedObligation {
						all = false
						break
					}
					return false, err
				}
				v := &semanticValue{sort: result.sort, guarantee: result, abstract: true}
				verdict, err := c.satisfies(v, arrow.result, depth+1)
				if err != nil {
					return false, err
				}
				if verdict != Proven {
					all = false
					break
				}
			}
			if all {
				established = true
				break
			}
		}
		if !established {
			return false, nil
		}
	}
	if err := c.allocate(depth); err != nil {
		return false, err
	}
	c.inclusions[key] = true
	return true, nil
}
func (c *checker) satisfies(v *semanticValue, expected *kindTerm, depth int) (Verdict, error) {
	if err := c.work.step(depth); err != nil {
		return Unproved, err
	}
	same, err := compareSort(v.sort, expected.sort, c.work, depth+1)
	if err != nil {
		return Unproved, err
	}
	if !same {
		return Unproved, failure(KindMismatch, "membership sort mismatch")
	}
	if expected.form == KindTop {
		return Proven, nil
	}
	if !v.sort.IsProper() {
		if v.declaration != nil {
			ok, err := c.knownConstructor(v.declaration, expected, depth+1)
			if ok {
				return Proven, err
			}
			return Unproved, err
		}
		ok, err := c.includes(c.bound(v), expected, depth+1)
		if ok {
			return Proven, err
		}
		return Unproved, err
	}
	shape, observation, closed, err := c.shape(v, depth+1)
	if err != nil {
		return Unproved, err
	}
	k, err := c.properKind(expected)
	if err != nil {
		return Unproved, err
	}
	if closed {
		ok, err := kindmodel.Member(k, observation, c.work)
		if err != nil {
			return Unproved, kindError(err)
		}
		if ok {
			return Proven, nil
		}
		return Refuted, nil
	}
	ok, err := c.includes(shape, expected, depth+1)
	if ok {
		return Proven, err
	}
	return Unproved, err
}
func (c *checker) knownConstructor(d *Declaration, expected *kindTerm, depth int) (bool, error) {
	key := constructorKey{d, expected}
	if v, ok := c.constructors[key]; ok {
		return v, nil
	}
	targets, err := c.alternatives(expected, depth+1)
	if err != nil {
		return false, err
	}
	for _, target := range targets {
		all := true
		for _, arrow := range target {
			if arrow.form != KindArrow {
				all = false
				break
			}
			for i, k := range arrow.inputs {
				ok, err := c.includes(k, d.semantic.inputs[i], depth+1)
				if err != nil {
					return false, err
				}
				if !ok {
					all = false
					break
				}
			}
			if !all {
				break
			}
			args := make([]*semanticValue, len(arrow.inputs))
			for i, k := range arrow.inputs {
				args[i], err = c.rigid(k, ParameterID{Declaration: d.id, Index: i}, depth+1)
				if err != nil {
					return false, err
				}
			}
			value, err := c.apply(&semanticValue{sort: d.sort, guarantee: d.semantic, declaration: d}, args, nil, depth+1)
			if err != nil {
				return false, err
			}
			verdict, err := c.satisfies(value, arrow.result, depth+1)
			if err != nil {
				return false, err
			}
			if verdict != Proven {
				all = false
				break
			}
		}
		if all {
			if err := c.allocate(depth); err != nil {
				return false, err
			}
			c.constructors[key] = true
			return true, nil
		}
	}
	if err := c.allocate(depth); err != nil {
		return false, err
	}
	c.constructors[key] = false
	return false, nil
}

func (c *checker) operandContract(e *Expression, depth int) (*kindTerm, error) {
	_, args, _ := head(e.value, c.program.grammar.DescriptorPrefix)
	key := e.constructor
	metadata, err := c.program.grammar.Metadata(e.constructor, args)
	if err != nil {
		return nil, err
	}
	for _, x := range metadata {
		key += "/" + x.Key + "=" + x.Value
	}
	if v := c.contracts[key]; v != nil {
		return v, nil
	}
	if err := c.work.reserveBytes(len(key)); err != nil {
		return nil, err
	}
	r := expressionReader{program: c.program, budget: c.work, edges: map[DeclarationID][]DeclarationID{}}
	owner := &Declaration{id: DeclarationID{Module: c.program.prelude}}
	k, err := r.readKind(c.program.grammar.OperandKind(e.constructor, args), owner, nil, nil, depth+1)
	if err != nil {
		return nil, err
	}
	t, err := c.instantiate(k, nil, nil, depth+1)
	if err != nil {
		return nil, err
	}
	if err := c.admitKind(t, depth+1); err != nil {
		return nil, err
	}
	c.contracts[key] = t
	return t, nil
}
func (c *checker) checkDeclarations() error {
	for _, d := range c.program.declarationOrder {
		if d.form != TypeDefinition {
			continue
		}
		env := &environment{bindings: map[ParameterID]*semanticValue{}}
		var assumptions []Assumption
		for i, p := range d.parameters {
			id := ParameterID{Declaration: d.id, Index: i}
			v, err := c.rigid(d.semantic.inputs[i], id, 0)
			if err != nil {
				return err
			}
			env.bindings[id] = v
			assumptions = append(assumptions, Assumption{parameter: id, kind: p.Kind})
		}
		if err := c.checkExpression(d.expression, env, assumptions, 0); err != nil {
			return sourceFailure(regionError(err, d.id, "body", 0, nil), c.program.root)
		}
		v, err := c.value(d.expression, env, 0)
		if err != nil {
			return err
		}
		expected := d.semantic
		if len(d.parameters) > 0 {
			expected = d.semantic.result
		}
		verdict, err := c.satisfies(v, expected, 0)
		if err != nil {
			return sourceFailure(regionError(err, d.id, "body", 0, nil), c.program.root)
		}
		if verdict != Proven {
			code := UnprovedObligation
			if verdict == Refuted {
				code = BoundViolation
			}
			return sourceFailure(regionError(&Error{Code: code, Subject: SubjectSource, Expected: copyKind(d.result), Message: "declaration body does not establish its result kind"}, d.id, "body", 0, nil), c.program.root)
		}
		d.expression.memberships = append(d.expression.memberships, Membership{kind: d.result, assumptions: slices.Clone(assumptions)})
		d.memberships = []Membership{{kind: d.contract}}
	}
	return nil
}
func (c *checker) checkExpression(e *Expression, env *environment, assumptions []Assumption, depth int) error {
	if err := c.work.step(depth); err != nil {
		return err
	}
	if e.callee != nil {
		if err := c.checkExpression(e.callee, env, assumptions, depth+1); err != nil {
			return err
		}
	}
	for _, x := range e.arguments {
		if err := c.checkExpression(x, env, assumptions, depth+1); err != nil {
			return err
		}
	}
	for _, o := range e.operands {
		if err := c.checkExpression(o.expression, env, assumptions, depth+1); err != nil {
			return err
		}
	}
	if e.kind == Descriptor && len(e.operands) > 0 {
		k, err := c.operandContract(e, depth+1)
		if err != nil {
			return err
		}
		for _, o := range e.operands {
			v, err := c.value(o.expression, env, depth+1)
			if err != nil {
				return err
			}
			verdict, err := c.satisfies(v, k, depth+1)
			if err != nil {
				return err
			}
			if verdict != Proven {
				code := UnprovedObligation
				if verdict == Refuted {
					code = BoundViolation
				}
				return &Error{Code: code, Subject: SubjectSource, Path: slices.Clone(o.expression.path), Expected: copyKind(k.origin), Message: "primitive operand does not establish its formation contract"}
			}
			o.expression.memberships = append(o.expression.memberships, Membership{kind: k.origin, assumptions: slices.Clone(assumptions)})
		}
	}
	v, err := c.value(e, env, depth+1)
	if err != nil {
		return err
	}
	e.semantic = c.bound(v)
	if e.kind == Application {
		callee, err := c.value(e.callee, env, depth+1)
		if err != nil {
			return err
		}
		if callee.declaration != nil {
			e.contract = callee.declaration.result
		} else {
			e.contract = e.semantic.origin
		}
		if e.contract == nil {
			e.contract = topKind(e.sort)
		}
	}
	e.memberships = append(e.memberships, Membership{kind: e.contract, assumptions: slices.Clone(assumptions)})
	return nil
}
