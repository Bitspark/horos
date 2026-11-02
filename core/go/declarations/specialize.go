package declarations

import core "github.com/bitspark/ontos/core/go"

// Specialize accepts concrete, recursively closed descriptor arguments.
// Open declarations in the Program are unchanged; no source origin is erased.
// On any failure (including exhaustion), the returned descriptor is nil.
func (p *Program) Specialize(id DeclarationID, arguments []core.Value) (core.Value, error) {
	if p == nil || !p.limits.valid() {
		return nil, failure(Invalid, "expected a resolved program")
	}
	declaration, ok := p.declarations[id]
	if !ok {
		return nil, failure(Resolution, "declaration is outside this program")
	}
	if len(arguments) != len(declaration.parameters) {
		return nil, failure(Invalid, "specialization argument arity mismatch")
	}
	work := &budget{limits: p.limits}
	for _, argument := range arguments {
		if err := work.value(argument, 0); err != nil {
			return nil, err
		}
		if err := descriptorWF(argument); err != nil {
			return nil, err
		}
	}
	var expand func(*Expression, []core.Value, int) (core.Value, error)
	expand = func(expression *Expression, env []core.Value, depth int) (core.Value, error) {
		if err := work.step(depth); err != nil {
			return nil, err
		}
		switch expression.kind {
		case Parameter:
			return env[expression.parameter.Index], nil
		case Application:
			actual := make([]core.Value, len(expression.arguments))
			for i, argument := range expression.arguments {
				value, err := expand(argument, env, depth+1)
				if err != nil {
					return nil, err
				}
				actual[i] = value
			}
			return expand(p.declarations[expression.target].expression, actual, depth+1)
		default:
			children := make([]core.Value, len(expression.operands))
			for i, operand := range expression.operands {
				child, err := expand(operand.expression, env, depth+1)
				if err != nil {
					return nil, err
				}
				children[i] = child
			}
			return replaceOperands(expression.value, expression.operands, children), nil
		}
	}
	result, err := expand(declaration.expression, arguments, 0)
	if err != nil {
		return nil, locate(err, id, nil)
	}
	// Charge the actual result tree, including repeated parameter/literal uses,
	// before a descriptor checker or encoder traverses it.
	if err := (&budget{limits: p.limits}).value(result, 0); err != nil {
		return nil, locate(err, id, nil)
	}
	if err := descriptorWF(result); err != nil {
		return nil, locate(err, id, nil)
	}
	return result, nil
}
