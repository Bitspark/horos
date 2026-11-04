package declmodel

func (p *Program) checkBounds(work *budget) error {
	c := newProofChecker(p, work)
	for _, declaration := range p.declarationOrder {
		if err := validateInterface(declaration.interface_, c.work, 0); err != nil {
			return sourceFailure(locate(err, declaration.id, nil), p.root)
		}
		env, err := c.rigidEnvironment(declaration, declaration.interface_.inputs, 0)
		if err == nil { err = c.checkExpression(declaration.expression, declaration, env, 0) }
		if err == nil && declaration.result == DataClaim && declaration.expression.dataStatus != DataProven {
			err = failure(BoundViolation, "declaration body does not establish its Data result claim")
		}
		if err != nil { return sourceFailure(locate(err, declaration.id, nil), p.root) }
	}
	return nil
}

func (c *proofChecker) checkExpression(e *Expression, owner *Declaration, env *environment, depth int) error {
	if err := c.work.step(depth); err != nil { return locate(err, owner.id, e.path) }
	var err error
	switch e.kind {
	case Parameter:
		if !e.interface_.IsConstructor() {
			if e.interface_.result == DataClaim { e.summary = &DataSummary{kind: SummaryTrue} } else {
				e.summary = &DataSummary{kind: SummaryNeed, parameter: e.parameter}
			}
		}
	case Reference:
		if !e.interface_.IsConstructor() { e.summary = &DataSummary{kind: SummaryProbe, probe: e} }
	case Application:
		if err = c.checkExpression(e.callee, owner, env, depth+1); err != nil { break }
		for i, argument := range e.arguments {
			if err = c.checkExpression(argument, owner, env, depth+1); err != nil { break }
			var v *semanticValue
			v, err = c.value(argument, env, depth+1)
			if err != nil { break }
			var ok bool
			ok, err = c.satisfies(v, e.callee.interface_.inputs[i], depth+1)
			if err == nil && !ok {
				code := BoundViolation
				var shape bool
				shape, err = sameShape(v.interface_, e.callee.interface_.inputs[i], c.work, depth+1)
				if err == nil {
					if !shape { code = KindMismatch }
					err = locate(applicationFailure(code, e.callee, e.callee.interface_.inputs[i], "argument does not satisfy constructor interface"), owner.id, argument.path)
				}
			}
			if err != nil { break }
		}
		if err == nil { e.summary = &DataSummary{kind: SummaryProbe, probe: e} }
	default:
		_, args, _ := head(e.value, c.program.grammar.DescriptorPrefix)
		rule := c.program.grammar.Rule(e.constructor, args)
		children := make([]*DataSummary, 0, len(e.operands))
		for _, operand := range e.operands {
			child := operand.expression
			if err = c.checkExpression(child, owner, env, depth+1); err != nil { break }
			if rule.RequiresData && child.dataStatus != DataProven {
				err = locate(failure(BoundViolation, "constructor operand requires Data"), owner.id, child.path)
				break
			}
			children = append(children, child.summary)
		}
		if err == nil {
			if rule.Capability { e.summary = &DataSummary{kind: SummaryFalse} } else if len(children) == 0 {
				e.summary = &DataSummary{kind: SummaryTrue}
			} else { e.summary = &DataSummary{kind: SummaryAnd, children: children} }
		}
	}
	if err == nil && e.summary != nil { e.dataStatus, err = c.evaluate(e.summary, env, depth) }
	if err != nil { return locate(err, owner.id, e.path) }
	return nil
}

func applicationFailure(code Code, callee *Expression, expected Interface, message string) error {
	err := &Error{Code: code, Subject: SubjectSource, Expected: cloneInterface(expected), Message: message}
	if callee.kind == Reference { err.Target = callee.target }
	if callee.kind == Parameter { err.CalleeParameter = callee.parameter }
	return err
}

func sameShape(a, b Interface, work *budget, depth int) (bool, error) {
	if err := work.step(depth); err != nil { return false, err }
	if len(a.inputs) != len(b.inputs) { return false, nil }
	for i := range a.inputs {
		ok, err := sameShape(a.inputs[i], b.inputs[i], work, depth+1)
		if err != nil || !ok { return false, err }
	}
	return true, nil
}

func (c *proofChecker) satisfies(v *semanticValue, expected Interface, depth int) (bool, error) {
	shape, err := sameShape(v.interface_, expected, c.work, depth)
	if err != nil || !shape { return false, err }
	if !expected.IsConstructor() {
		if expected.result == TypeClaim { return true, nil }
		status, err := c.data(v, depth+1)
		return status == DataProven, err
	}
	if v.rigid { return compatible(v.interface_, expected, c.work, depth+1) }
	return c.declarationSatisfies(v.declaration, expected, depth+1)
}

func encodeInterface(iface Interface, work *budget, depth int) (string, error) {
	var output []byte
	var visit func(Interface, int) error
	visit = func(i Interface, depth int) error {
		if err := work.step(depth); err != nil { return err }
		if err := work.reserveBytes(3); err != nil { return err }
		output = append(output, '(')
		for _, child := range i.inputs { if err := visit(child, depth+1); err != nil { return err } }
		if i.result == DataClaim { output = append(output, 'd') } else { output = append(output, 't') }
		output = append(output, ')')
		return nil
	}
	if err := visit(iface, depth); err != nil { return "", err }
	return string(output), nil
}

func (c *proofChecker) declarationSatisfies(d *Declaration, expected Interface, depth int) (bool, error) {
	if err := c.work.step(depth); err != nil { return false, err }
	keyText, err := encodeInterface(expected, c.work, depth)
	if err != nil { return false, err }
	key := interfaceKey{declaration: d, expected: keyText}
	if result, ok := c.interfaces[key]; ok { return result, nil }
	shape, err := sameShape(d.interface_, expected, c.work, depth)
	if err != nil || !shape { return false, err }
	for i := range expected.inputs {
		ok, err := compatible(expected.inputs[i], d.interface_.inputs[i], c.work, depth+1)
		if err != nil || !ok { return false, err }
	}
	result := true
	if expected.result == DataClaim {
		env, err := c.rigidEnvironment(d, expected.inputs, depth+1)
		if err != nil { return false, err }
		status, err := c.evaluate(d.expression.summary, env, depth+1)
		if err != nil { return false, err }
		result = status == DataProven
	}
	if err := c.work.step(depth); err != nil { return false, err }
	c.interfaces[key] = result
	return result, nil
}

func (p *Program) Satisfies(id DeclarationID, expected Interface) (bool, error) {
	if p == nil || !p.limits.valid() { return false, &Error{Code: Invalid, Subject: SubjectRequest, Message: "expected a resolved program"} }
	d, ok := p.declarations[id]
	if !ok { return false, &Error{Code: UnknownDeclaration, Subject: SubjectRequest, Target: id, Message: "declaration is outside this program"} }
	c := newProofChecker(p, &budget{limits: p.limits})
	if err := validateInterface(expected, c.work, 0); err != nil { return false, argumentFailure(err, id) }
	ok, err := c.declarationSatisfies(d, expected, 0)
	if err != nil { return false, argumentFailure(err, id) }
	return ok, nil
}
