package declmodel

import core "github.com/bitspark/ontos/core/go"

// Argument is sealed to the exact checked Program that admitted it. Its zero
// value is invalid; equal module digests do not grant cross-Program ownership.
type Argument struct {
	owner       *Program
	descriptor  core.Value
	constructor *Declaration
}

func (p *Program) Proper(value core.Value) (Argument, error) {
	if p == nil || !p.limits.valid() {
		return Argument{}, &Error{Code: Invalid, Subject: SubjectRequest, Message: "expected a resolved program"}
	}
	if err := (&budget{limits: p.limits}).value(value, 0); err != nil {
		return Argument{}, argumentFailure(err, DeclarationID{})
	}
	if err := p.grammar.WF(value); err != nil {
		return Argument{}, argumentFailure(err, DeclarationID{})
	}
	return Argument{owner: p, descriptor: value}, nil
}

func (p *Program) Constructor(id DeclarationID) (Argument, error) {
	if p == nil || !p.limits.valid() {
		return Argument{}, &Error{Code: Invalid, Subject: SubjectRequest, Message: "expected a resolved program"}
	}
	d, ok := p.declarations[id]
	if !ok {
		return Argument{}, &Error{Code: UnknownDeclaration, Subject: SubjectRequest, Target: id, Message: "declaration is outside this program"}
	}
	if !d.interface_.IsConstructor() {
		return Argument{}, &Error{Code: KindMismatch, Subject: SubjectRequest, Target: id, Message: "expected a parameterized declaration"}
	}
	return Argument{owner: p, constructor: d}, nil
}

// Specialize accepts closed proper descriptors and checked named constructors.
// All arguments are collectively re-admitted on every request. Open source and
// symbolic summaries remain unchanged; every failure returns a nil descriptor.
func (p *Program) Specialize(id DeclarationID, arguments []Argument) (core.Value, error) {
	if p == nil || !p.limits.valid() {
		return nil, &Error{Code: Invalid, Subject: SubjectRequest, Message: "expected a resolved program"}
	}
	d, ok := p.declarations[id]
	if !ok {
		return nil, &Error{Code: UnknownDeclaration, Subject: SubjectRequest, Target: id, Message: "declaration is outside this program"}
	}
	if len(arguments) != len(d.parameters) {
		return nil, &Error{Code: Invalid, Subject: SubjectRequest, Target: id, Message: "specialization argument arity mismatch"}
	}
	c := newProofChecker(p, &budget{limits: p.limits})
	values := make([]*semanticValue, len(arguments))
	for i, argument := range arguments {
		if err := c.work.step(0); err != nil {
			return nil, argumentFailure(err, id)
		}
		if argument.owner != p || (argument.descriptor == nil) == (argument.constructor == nil) {
			return nil, &Error{Code: Invalid, Subject: SubjectRequest, Target: id, Message: "argument is not owned by this checked program"}
		}
		v := semanticValue{}
		if argument.constructor != nil {
			if p.declarations[argument.constructor.id] != argument.constructor {
				return nil, &Error{Code: Invalid, Subject: SubjectRequest, Target: id, Message: "invalid constructor ownership"}
			}
			if err := validateInterface(argument.constructor.interface_, c.work, 0); err != nil {
				return nil, argumentFailure(err, id)
			}
			v.declaration = argument.constructor
			v.interface_ = argument.constructor.interface_
		} else {
			if err := c.work.value(argument.descriptor, 0); err != nil {
				return nil, argumentFailure(err, id)
			}
			isData, err := p.grammar.Data(argument.descriptor)
			if err != nil {
				return nil, argumentFailure(err, id)
			}
			v.descriptor = argument.descriptor
			v.interface_ = Interface{result: TypeClaim}
			v.status = DataFailed
			if isData {
				v.status = DataProven
			}
		}
		var err error
		values[i], err = c.fresh(v, 0)
		if err != nil {
			return nil, argumentFailure(err, id)
		}
		admitted, err := c.satisfies(values[i], d.parameters[i].Interface, 0)
		if err != nil {
			return nil, argumentFailure(err, id)
		}
		if !admitted {
			code := BoundViolation
			shape, err := sameShape(v.interface_, d.parameters[i].Interface, c.work, 0)
			if err != nil {
				return nil, argumentFailure(err, id)
			}
			if !shape {
				code = KindMismatch
			}
			return nil, &Error{Code: code, Subject: SubjectRequest, Target: id, Expected: cloneInterface(d.parameters[i].Interface), Message: "specialization argument does not satisfy its interface"}
		}
	}
	env, err := c.invocation(d, values, 0)
	if err != nil {
		return nil, argumentFailure(err, id)
	}
	result, err := c.expand(d.expression, env, 0)
	if err != nil {
		return nil, argumentFailure(locate(err, id, nil), id)
	}
	// Count the entire output tree, including repeated occurrences of shared data.
	if err := (&budget{limits: p.limits}).value(result, 0); err != nil {
		return nil, argumentFailure(locate(err, id, nil), id)
	}
	if err := p.grammar.WF(result); err != nil {
		return nil, argumentFailure(locate(err, id, nil), id)
	}
	return result, nil
}

func (c *proofChecker) expand(e *Expression, env *environment, depth int) (core.Value, error) {
	if err := c.work.step(depth); err != nil {
		return nil, err
	}
	switch e.kind {
	case Parameter:
		v := env.values[e.parameter]
		if v == nil || v.interface_.IsConstructor() || v.rigid {
			return nil, failure(Invalid, "specialization has an open or constructor-valued result")
		}
		if v.descriptor != nil {
			return v.descriptor, nil
		}
		return c.expand(v.expression, v.environment, depth+1)
	case Reference:
		d := c.program.declarations[e.target]
		if d.interface_.IsConstructor() {
			return nil, failure(KindMismatch, "constructor cannot be a closed proper result")
		}
		actual, err := c.invocation(d, nil, depth+1)
		if err != nil {
			return nil, err
		}
		return c.expand(d.expression, actual, depth+1)
	case Application:
		callee, err := c.value(e.callee, env, depth+1)
		if err != nil {
			return nil, err
		}
		if callee.rigid || callee.declaration == nil {
			return nil, failure(Invalid, "specialization has an open constructor")
		}
		values := make([]*semanticValue, len(e.arguments))
		for i, argument := range e.arguments {
			values[i], err = c.value(argument, env, depth+1)
			if err != nil {
				return nil, err
			}
			ok, err := c.satisfies(values[i], callee.declaration.parameters[i].Interface, depth+1)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, applicationFailure(BoundViolation, e.callee, callee.declaration.parameters[i].Interface, "closed application violates original admission")
			}
		}
		actual, err := c.invocation(callee.declaration, values, depth+1)
		if err != nil {
			return nil, err
		}
		return c.expand(callee.declaration.expression, actual, depth+1)
	default:
		children := make([]core.Value, len(e.operands))
		for i, operand := range e.operands {
			child, err := c.expand(operand.expression, env, depth+1)
			if err != nil {
				return nil, err
			}
			children[i] = child
		}
		return replaceOperands(e.value, e.operands, children), nil
	}
}

func argumentFailure(err error, target DeclarationID) error {
	if detail, ok := err.(*Error); ok {
		copy := *detail
		copy.Subject = SubjectRequest
		copy.Target = target
		return &copy
	}
	return err
}
