package declmodel

import (
	"strconv"

	core "github.com/bitspark/ontos/core/go"
)

// Rigid values have operation-local identities, never fabricated declaration
// IDs. Syntax closures retain the environment where their expression was bound.
type semanticValue struct {
	id uint64
	interface_ Interface
	rigid bool
	declaration *Declaration
	expression *Expression
	environment *environment
	descriptor core.Value
	status DataStatus
}

type environment struct { values map[ParameterID]*semanticValue }
type valueKey struct { expression *Expression; environment *environment }
type evaluationKey struct { summary *DataSummary; environment *environment }
type invocationKey struct { declaration *Declaration; arguments string }
type interfaceKey struct { declaration *Declaration; expected string }

type proofChecker struct {
	program *Program
	work *budget
	nextValue uint64
	values map[valueKey]*semanticValue
	evaluations map[evaluationKey]DataStatus
	environments map[invocationKey]*environment
	interfaces map[interfaceKey]bool
}

func newProofChecker(p *Program, work *budget) *proofChecker {
	return &proofChecker{program: p, work: work, values: make(map[valueKey]*semanticValue),
		evaluations: make(map[evaluationKey]DataStatus), environments: make(map[invocationKey]*environment),
		interfaces: make(map[interfaceKey]bool)}
}

func (c *proofChecker) fresh(value semanticValue, depth int) (*semanticValue, error) {
	if err := c.work.step(depth); err != nil { return nil, err }
	c.nextValue++
	value.id = c.nextValue
	return &value, nil
}

func (c *proofChecker) rigidEnvironment(d *Declaration, inputs []Interface, depth int) (*environment, error) {
	if err := c.work.step(depth); err != nil { return nil, err }
	env := &environment{values: make(map[ParameterID]*semanticValue, len(inputs))}
	for i, iface := range inputs {
		v, err := c.fresh(semanticValue{interface_: iface, rigid: true}, depth)
		if err != nil { return nil, err }
		env.values[ParameterID{Declaration: d.id, Index: i}] = v
	}
	return env, nil
}

func (c *proofChecker) value(e *Expression, env *environment, depth int) (*semanticValue, error) {
	if err := c.work.step(depth); err != nil { return nil, err }
	if e.kind == Parameter {
		v := env.values[e.parameter]
		if v == nil { return nil, failure(Invalid, "missing parameter binding") }
		return v, nil
	}
	key := valueKey{expression: e, environment: env}
	if v, ok := c.values[key]; ok { return v, nil }
	v := semanticValue{interface_: e.interface_, expression: e, environment: env}
	if e.kind == Reference && e.interface_.IsConstructor() { v.declaration = c.program.declarations[e.target] }
	result, err := c.fresh(v, depth)
	if err != nil { return nil, err }
	c.values[key] = result
	return result, nil
}

func (c *proofChecker) invocation(d *Declaration, values []*semanticValue, depth int) (*environment, error) {
	if err := c.work.step(depth); err != nil { return nil, err }
	var keyBytes []byte
	for _, value := range values {
		if err := c.work.step(depth); err != nil { return nil, err }
		keyBytes = strconv.AppendUint(keyBytes, value.id, 10)
		keyBytes = append(keyBytes, '/')
	}
	key := invocationKey{declaration: d, arguments: string(keyBytes)}
	if env, ok := c.environments[key]; ok { return env, nil }
	if err := c.work.step(depth); err != nil { return nil, err }
	env := &environment{values: make(map[ParameterID]*semanticValue, len(values))}
	for i, value := range values { env.values[ParameterID{Declaration: d.id, Index: i}] = value }
	c.environments[key] = env
	return env, nil
}

func (c *proofChecker) evaluate(s *DataSummary, env *environment, depth int) (DataStatus, error) {
	if err := c.work.step(depth); err != nil { return "", err }
	key := evaluationKey{summary: s, environment: env}
	if status, ok := c.evaluations[key]; ok { return status, nil }
	var status DataStatus
	var err error
	switch s.kind {
	case SummaryTrue: status = DataProven
	case SummaryFalse: status = DataFailed
	case SummaryNeed:
		v := env.values[s.parameter]
		if v == nil { return "", failure(Invalid, "missing summary parameter binding") }
		status, err = c.data(v, depth+1)
	case SummaryAnd:
		status = DataProven
		for _, child := range s.children {
			var childStatus DataStatus
			childStatus, err = c.evaluate(child, env, depth+1)
			if err != nil { break }
			if childStatus == DataFailed { status = DataFailed } else if childStatus == DataConditional && status == DataProven { status = DataConditional }
		}
	case SummaryProbe:
		e := s.probe
		if e.kind == Reference {
			d := c.program.declarations[e.target]
			var actual *environment
			actual, err = c.invocation(d, nil, depth+1)
			if err == nil { status, err = c.evaluate(d.expression.summary, actual, depth+1) }
		} else {
			var callee *semanticValue
			callee, err = c.value(e.callee, env, depth+1)
			if err != nil { break }
			if callee.rigid {
				status = DataConditional
				if callee.interface_.result == DataClaim { status = DataProven }
				break
			}
			values := make([]*semanticValue, len(e.arguments))
			for i, argument := range e.arguments {
				values[i], err = c.value(argument, env, depth+1)
				if err != nil { break }
			}
			if err != nil { break }
			var actual *environment
			actual, err = c.invocation(callee.declaration, values, depth+1)
			if err == nil { status, err = c.evaluate(callee.declaration.expression.summary, actual, depth+1) }
		}
	default: return "", failure(Invalid, "unknown summary form")
	}
	if err != nil { return "", err }
	if err := c.work.step(depth); err != nil { return "", err }
	c.evaluations[key] = status
	return status, nil
}

func (c *proofChecker) data(v *semanticValue, depth int) (DataStatus, error) {
	if err := c.work.step(depth); err != nil { return "", err }
	if v.interface_.IsConstructor() { return "", failure(KindMismatch, "constructor has no proper Data judgment") }
	if v.rigid {
		if v.interface_.result == DataClaim { return DataProven, nil }
		return DataConditional, nil
	}
	if v.status != "" { return v.status, nil }
	return c.evaluate(v.expression.summary, v.environment, depth+1)
}
