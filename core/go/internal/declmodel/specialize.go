package declmodel

import (
	"encoding/binary"

	core "github.com/bitspark/ontos/core/go"
)

// TypeValue is a checked value sealed to the Program that established it.
// A value may be proper or a saturated constructor, including a constructor
// returned by another declaration. Its captured environment is private.
type TypeValue struct {
	owner *Program
	value *semanticValue
}

func (v TypeValue) Sort() Sort {
	if v.value == nil {
		return Sort{}
	}
	return cloneSort(v.value.sort)
}

func (v TypeValue) Contract() *Kind {
	if v.value == nil || v.value.guarantee == nil {
		return nil
	}
	return copyKind(v.value.guarantee.origin)
}

func (v TypeValue) Expression() *Expression {
	if v.value == nil {
		return nil
	}
	return copyExpression(v.value.expression)
}

func specializationError(err error) error {
	if detail, ok := err.(*Error); ok {
		copy := *detail
		copy.Subject = SubjectRequest
		return &copy
	}
	return err
}

func specializationFailure(code Code, message string) error {
	return &Error{Code: code, Subject: SubjectRequest, Message: message}
}

func (p *Program) hasValue(value TypeValue) bool {
	return p != nil && value.owner == p && value.value != nil
}

func (p *Program) hasKind(kind KindHandle) bool {
	return p != nil && kind.owner == p && kind.term != nil && kind.expression != nil
}

// Each public request receives fresh raw-input and semantic-work budgets. A
// prechecked handle is still input: its source, named closure, semantic graph,
// and captured bindings consume this request's allowances.
type requestAdmission struct {
	program *Program
	raw     *budget
	work    *budget
}

type admissionScope struct {
	request      *requestAdmission
	active       map[*semanticValue]bool
	declarations map[*Declaration]bool
	kinds        map[*Kind]bool
	terms        map[*kindTerm]bool
	sequences    map[*sequenceTerm]bool
}

func newRequestAdmission(p *Program) *requestAdmission {
	return &requestAdmission{
		program: p,
		raw:     &budget{limits: p.limits},
		work:    &budget{limits: p.limits},
	}
}

func (a *requestAdmission) scope() *admissionScope {
	return &admissionScope{
		request: a, active: map[*semanticValue]bool{},
		declarations: map[*Declaration]bool{}, kinds: map[*Kind]bool{},
		terms: map[*kindTerm]bool{}, sequences: map[*sequenceTerm]bool{},
	}
}

func (a *requestAdmission) input(value core.Value, depth int) error {
	if err := a.raw.value(value, 0); err != nil {
		return err
	}
	return a.work.value(value, depth)
}

func (a *requestAdmission) typeValue(value *semanticValue) error {
	return a.scope().semantic(value, 0)
}

func (a *requestAdmission) kind(kind KindHandle) error {
	scope := a.scope()
	if err := scope.kindSource(kind.expression, 0); err != nil {
		return err
	}
	return scope.term(kind.term, 0)
}

func (s *admissionScope) record(depth int) error {
	if err := s.request.work.step(depth); err != nil {
		return err
	}
	return s.request.work.reserveBytes(64)
}

func (s *admissionScope) semantic(value *semanticValue, depth int) error {
	if err := s.record(depth); err != nil {
		return err
	}
	if value == nil || value.guarantee == nil {
		return specializationFailure(Invalid, "invalid checked type value")
	}
	if s.active[value] {
		return specializationFailure(Invalid, "cyclic captured type environment")
	}
	s.active[value] = true
	defer delete(s.active, value)
	if value.descriptor != nil {
		if err := s.request.input(value.descriptor, depth); err != nil {
			return err
		}
	}
	if value.expression != nil {
		if value.expression.value != nil {
			if err := s.request.input(value.expression.value, depth); err != nil {
				return err
			}
		}
		if err := s.expressionReferences(value.expression, depth+1); err != nil {
			return err
		}
	}
	if value.declaration != nil {
		if err := s.declaration(value.declaration, depth+1); err != nil {
			return err
		}
	}
	if value.guarantee.origin != nil {
		if err := s.kindSource(value.guarantee.origin, depth+1); err != nil {
			return err
		}
	}
	if err := s.term(value.guarantee, depth+1); err != nil {
		return err
	}
	if value.env != nil {
		if err := s.record(depth+1); err != nil {
			return err
		}
		for _, binding := range value.env.bindings {
			if err := s.semantic(binding, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *admissionScope) declaration(declaration *Declaration, depth int) error {
	if err := s.request.work.step(depth); err != nil {
		return err
	}
	if declaration == nil {
		return specializationFailure(Invalid, "unresolved declaration in a checked handle")
	}
	if s.declarations[declaration] {
		return nil
	}
	if err := s.record(depth); err != nil {
		return err
	}
	s.declarations[declaration] = true
	if err := s.request.input(declaration.body, depth); err != nil {
		return err
	}
	if declaration.form == KindDefinition {
		return s.kindReferences(declaration.kindBody, depth+1)
	}
	if err := s.kindSource(declaration.contract, depth+1); err != nil {
		return err
	}
	return s.expressionReferences(declaration.expression, depth+1)
}

func (s *admissionScope) expressionReferences(expression *Expression, depth int) error {
	if expression == nil {
		return nil
	}
	if err := s.request.work.step(depth); err != nil {
		return err
	}
	if expression.kind == Reference {
		if err := s.declaration(s.request.program.declarations[expression.target], depth+1); err != nil {
			return err
		}
	}
	if err := s.expressionReferences(expression.callee, depth+1); err != nil {
		return err
	}
	for _, argument := range expression.arguments {
		if err := s.expressionReferences(argument, depth+1); err != nil {
			return err
		}
	}
	for _, operand := range expression.operands {
		if err := s.expressionReferences(operand.expression, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (s *admissionScope) kindSource(kind *Kind, depth int) error {
	if kind == nil || s.kinds[kind] {
		return nil
	}
	if err := s.record(depth); err != nil {
		return err
	}
	s.kinds[kind] = true
	if kind.value != nil {
		if err := s.request.input(kind.value, depth); err != nil {
			return err
		}
	}
	return s.kindReferences(kind, depth+1)
}

func (s *admissionScope) kindReferences(kind *Kind, depth int) error {
	if kind == nil {
		return nil
	}
	if err := s.request.work.step(depth); err != nil {
		return err
	}
	if kind.form == KindReference {
		if err := s.declaration(s.request.program.declarations[kind.target], depth+1); err != nil {
			return err
		}
	}
	for _, operand := range kind.operands {
		if err := s.kindReferences(operand.kind, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (s *admissionScope) sort(value Sort, depth int) error {
	if err := s.request.work.step(depth); err != nil {
		return err
	}
	for _, input := range value.inputs {
		if err := s.sort(input, depth+1); err != nil {
			return err
		}
	}
	if value.result != nil {
		return s.sort(*value.result, depth+1)
	}
	return nil
}

func (s *admissionScope) term(term *kindTerm, depth int) error {
	if err := s.request.work.step(depth); err != nil {
		return err
	}
	if term == nil {
		return specializationFailure(Invalid, "invalid checked kind")
	}
	if s.terms[term] {
		return nil
	}
	if err := s.record(depth); err != nil {
		return err
	}
	s.terms[term] = true
	if err := s.sort(term.sort, depth+1); err != nil {
		return err
	}
	if err := s.request.work.reserveBytes(len(term.nodeForm)); err != nil {
		return err
	}
	for _, selector := range term.selectors {
		if err := s.request.work.reserveBytes(len(selector.Key) + len(selector.Value)); err != nil {
			return err
		}
	}
	for _, child := range term.children {
		if err := s.term(child, depth+1); err != nil {
			return err
		}
	}
	for _, input := range term.inputs {
		if err := s.term(input, depth+1); err != nil {
			return err
		}
	}
	if term.result != nil {
		if err := s.term(term.result, depth+1); err != nil {
			return err
		}
	}
	if term.sequence != nil {
		return s.sequence(term.sequence, depth+1)
	}
	return nil
}

func (s *admissionScope) sequence(sequence *sequenceTerm, depth int) error {
	if err := s.request.work.step(depth); err != nil {
		return err
	}
	if s.sequences[sequence] {
		return nil
	}
	if err := s.record(depth); err != nil {
		return err
	}
	s.sequences[sequence] = true
	if sequence.element != nil {
		if err := s.term(sequence.element, depth+1); err != nil {
			return err
		}
	}
	for _, child := range sequence.children {
		if err := s.sequence(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (p *Program) Value(id DeclarationID) (TypeValue, error) {
	if p == nil {
		return TypeValue{}, specializationFailure(Invalid, "missing checked program")
	}
	declaration := p.declarations[id]
	if declaration == nil {
		return TypeValue{}, &Error{Code: UnknownDeclaration, Subject: SubjectRequest, Target: id, Message: "unknown type declaration"}
	}
	if declaration.form != TypeDefinition {
		return TypeValue{}, specializationFailure(KindMismatch, "a kind definition is not a type value")
	}
	admission := newRequestAdmission(p)
	if err := admission.scope().declaration(declaration, 0); err != nil {
		return TypeValue{}, specializationError(err)
	}
	checker := newChecker(p, admission.work)
	if len(declaration.parameters) > 0 {
		if err := checker.allocate(0); err != nil {
			return TypeValue{}, specializationError(err)
		}
		value := &semanticValue{
			sort: declaration.sort, guarantee: declaration.semantic, declaration: declaration,
			expression: &Expression{kind: Reference, target: id, sort: declaration.sort, contract: declaration.contract},
		}
		return TypeValue{owner: p, value: value}, nil
	}
	value, err := checker.value(declaration.expression, nil, 0)
	if err != nil {
		return TypeValue{}, specializationError(err)
	}
	return TypeValue{owner: p, value: value}, nil
}

// Proper accepts only primitive descriptors in designated type positions.
// Declaration-looking data inside an opaque literal remains ordinary data.
func (p *Program) Proper(value core.Value) (TypeValue, error) {
	if p == nil {
		return TypeValue{}, specializationFailure(Invalid, "missing checked program")
	}
	admission := newRequestAdmission(p)
	if err := admission.input(value, 0); err != nil {
		return TypeValue{}, specializationError(err)
	}
	reader := expressionReader{program: p, budget: admission.work, edges: map[DeclarationID][]DeclarationID{}}
	owner := &Declaration{id: DeclarationID{Module: p.root}}
	expression, skeleton, err := reader.read(value, owner, nil, 0)
	if err == nil {
		err = primitiveExpression(expression, admission.work, 0)
	}
	if err == nil {
		err = p.grammar.WF(skeleton)
	}
	if err == nil {
		err = bindGround(expression, admission.work)
	}
	if err != nil {
		return TypeValue{}, specializationError(err)
	}
	checker := newChecker(p, admission.work)
	environment := &environment{bindings: map[ParameterID]*semanticValue{}}
	if err := checker.checkExpression(expression, environment, nil, 0); err != nil {
		return TypeValue{}, specializationError(err)
	}
	result, err := checker.value(expression, environment, 0)
	if err != nil {
		return TypeValue{}, specializationError(err)
	}
	return TypeValue{owner: p, value: result}, nil
}

func primitiveExpression(expression *Expression, work *budget, depth int) error {
	if err := work.step(depth); err != nil {
		return err
	}
	if expression == nil || expression.kind != Descriptor {
		return specializationFailure(Invalid, "Proper requires primitive descriptors in every type position")
	}
	for _, operand := range expression.operands {
		if err := primitiveExpression(operand.expression, work, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (p *Program) Apply(callee TypeValue, arguments []TypeValue) (TypeValue, error) {
	if !p.hasValue(callee) {
		return TypeValue{}, specializationFailure(Invalid, "constructor value belongs to a different or missing program")
	}
	for _, argument := range arguments {
		if !p.hasValue(argument) {
			return TypeValue{}, specializationFailure(Invalid, "argument value belongs to a different or missing program")
		}
	}
	admission := newRequestAdmission(p)
	if err := admission.typeValue(callee.value); err != nil {
		return TypeValue{}, specializationError(err)
	}
	for _, argument := range arguments {
		if err := admission.typeValue(argument.value); err != nil {
			return TypeValue{}, specializationError(err)
		}
	}
	if err := admission.work.reserveBytes(len(arguments) * 8); err != nil {
		return TypeValue{}, specializationError(err)
	}
	values := make([]*semanticValue, len(arguments))
	for i, argument := range arguments {
		values[i] = argument.value
	}
	checker := newChecker(p, admission.work)
	result, err := checker.apply(callee.value, values, nil, 0)
	if err != nil {
		return TypeValue{}, specializationError(err)
	}
	return TypeValue{owner: p, value: result}, nil
}

func (p *Program) Satisfies(value TypeValue, kind KindHandle) (Verdict, error) {
	if !p.hasValue(value) || !p.hasKind(kind) {
		return Unproved, specializationFailure(Invalid, "membership handles must belong to the same checked program")
	}
	admission := newRequestAdmission(p)
	if err := admission.typeValue(value.value); err != nil {
		return Unproved, specializationError(err)
	}
	if err := admission.kind(kind); err != nil {
		return Unproved, specializationError(err)
	}
	verdict, err := newChecker(p, admission.work).satisfies(value.value, kind.term, 0)
	return verdict, specializationError(err)
}

func (p *Program) Includes(actual KindHandle, expected KindHandle) (Verdict, error) {
	if !p.hasKind(actual) || !p.hasKind(expected) {
		return Unproved, specializationFailure(Invalid, "inclusion handles must belong to the same checked program")
	}
	admission := newRequestAdmission(p)
	if err := admission.kind(actual); err != nil {
		return Unproved, specializationError(err)
	}
	if err := admission.kind(expected); err != nil {
		return Unproved, specializationError(err)
	}
	established, err := newChecker(p, admission.work).includes(actual.term, expected.term, 0)
	if err != nil {
		return Unproved, specializationError(err)
	}
	if established {
		return Proven, nil
	}
	return Unproved, nil
}

func (p *Program) Descriptor(value TypeValue) (core.Value, error) {
	return p.descriptor(value, false)
}

func (p *Program) GroundDescriptor(value TypeValue) (core.Value, error) {
	return p.descriptor(value, true)
}

func (p *Program) descriptor(value TypeValue, ground bool) (core.Value, error) {
	if !p.hasValue(value) {
		return nil, specializationFailure(Invalid, "type value belongs to a different or missing program")
	}
	if !value.value.sort.IsProper() {
		return nil, specializationFailure(KindMismatch, "descriptor materialization requires a proper type value")
	}
	admission := newRequestAdmission(p)
	if err := admission.typeValue(value.value); err != nil {
		return nil, specializationError(err)
	}
	checker := newChecker(p, admission.work)
	if ground {
		if p.ground == nil {
			return nil, specializationFailure(Invalid, "program has no ground-kind contract")
		}
		if err := admission.scope().term(p.ground, 0); err != nil {
			return nil, specializationError(err)
		}
		verdict, err := checker.satisfies(value.value, p.ground, 0)
		if err != nil {
			return nil, specializationError(err)
		}
		if verdict != Proven {
			code := UnprovedObligation
			if verdict == Refuted {
				code = BoundViolation
			}
			return nil, &Error{Code: code, Subject: SubjectRequest, Expected: copyKind(p.ground.origin), Message: "type does not establish the ordinary ground-kind contract"}
		}
	}
	output := &budget{limits: p.limits}
	result, err := materialize(checker, value.value, output, 0, 0)
	if err == nil {
		err = p.grammar.WF(result)
	}
	if err == nil && ground {
		err = p.grammar.GroundWF(result)
	}
	if err != nil {
		return nil, specializationError(err)
	}
	return result, nil
}

// Materialization walks source operands, never observation-graph binder edges.
// Every emitted occurrence is charged separately, including shared literals and
// repeated applications; recursion remains the finite original rec/ref syntax.
func materialize(checker *checker, value *semanticValue, output *budget, depth, outputDepth int) (core.Value, error) {
	if err := checker.work.step(depth); err != nil {
		return nil, err
	}
	if value == nil || !value.sort.IsProper() {
		return nil, specializationFailure(KindMismatch, "materialization requires a proper type")
	}
	if value.rigid || value.abstract {
		return nil, specializationFailure(UnprovedObligation, "abstract type value has no closed descriptor")
	}
	if value.descriptor != nil {
		if err := output.value(value.descriptor, outputDepth); err != nil {
			return nil, err
		}
		return value.descriptor, nil
	}
	expression := value.expression
	if expression == nil {
		return nil, specializationFailure(UnprovedObligation, "type value has no materializable expression")
	}
	if expression.kind != Descriptor {
		resolved, err := checker.value(expression, value.env, depth+1)
		if err != nil {
			return nil, err
		}
		if resolved == value {
			return nil, specializationFailure(UnprovedObligation, "type value has no closed descriptor")
		}
		return materialize(checker, resolved, output, depth+1, outputDepth)
	}
	if err := chargeDescriptorShell(expression.value, expression.operands, checker.work, output, outputDepth); err != nil {
		return nil, err
	}
	if len(expression.operands) == 0 {
		return expression.value, nil
	}
	if err := checker.work.reserveBytes(len(expression.operands) * 16); err != nil {
		return nil, err
	}
	children := make([]core.Value, len(expression.operands))
	for i, operand := range expression.operands {
		child, err := checker.value(operand.expression, value.env, depth+1)
		if err != nil {
			return nil, err
		}
		children[i], err = materialize(checker, child, output, depth+1, outputDepth+len(operand.path))
		if err != nil {
			return nil, err
		}
	}
	return replaceOperands(expression.value, expression.operands, children), nil
}

func chargeOutputTuple(tuple core.Tuple, output *budget, depth int) error {
	if err := output.step(depth); err != nil {
		return err
	}
	if tuple.Len() > output.limits.MaxNodes-output.nodes {
		return failure(Exhausted, "materialized output node limit reached")
	}
	var header [binary.MaxVarintLen64]byte
	return output.reserveBytes(1 + binary.PutUvarint(header[:], uint64(tuple.Len())))
}

// Charge the exact emitted literal shell while leaving each designated operand
// for its own materialization. The grammar permits direct positions and record
// field positions only; literal payloads are never interpreted as expressions.
func chargeDescriptorShell(value core.Value, operands []Operand, work, output *budget, depth int) error {
	if len(operands) == 0 {
		return output.value(value, depth)
	}
	tuple, ok := value.(core.Tuple)
	if !ok {
		return specializationFailure(Invalid, "invalid descriptor shell")
	}
	if err := work.reserveBytes(len(operands)*64 + tuple.Len()*32); err != nil {
		return err
	}
	positions := make(map[int][]int, len(operands))
	for _, operand := range operands {
		if len(operand.path) < 1 || len(operand.path) > 2 {
			return specializationFailure(Invalid, "invalid descriptor operand path")
		}
		if _, exists := positions[operand.path[0]]; exists {
			return specializationFailure(Invalid, "overlapping descriptor operand paths")
		}
		positions[operand.path[0]] = operand.path[1:]
	}
	if err := chargeOutputTuple(tuple, output, depth); err != nil {
		return err
	}
	for i := 0; i < tuple.Len(); i++ {
		path, operand := positions[i]
		if !operand {
			if err := output.value(tuple.At(i), depth+1); err != nil {
				return err
			}
			continue
		}
		if len(path) == 0 {
			continue
		}
		field, ok := tuple.At(i).(core.Tuple)
		if !ok {
			return specializationFailure(Invalid, "invalid descriptor field shell")
		}
		if err := work.reserveBytes(field.Len() * 32); err != nil {
			return err
		}
		if err := chargeOutputTuple(field, output, depth+1); err != nil {
			return err
		}
		for j := 0; j < field.Len(); j++ {
			if j != path[0] {
				if err := output.value(field.At(j), depth+2); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
