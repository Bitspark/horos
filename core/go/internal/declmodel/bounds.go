package declmodel

// A condition is a conjunction of caller-owned Type slots. Impossible denotes
// a capability-bearing type; an empty conjunction proves Data. Conditions follow
// actual parameter use, not library names or syntax hidden in literal values.
type dataCondition struct {
	impossible bool
	requires   map[ParameterID]bool
}

func (c dataCondition) proven() bool { return !c.impossible && len(c.requires) == 0 }

type boundChecker struct {
	program   *Program
	work      *budget
	summaries map[DeclarationID]dataCondition
}

func (p *Program) checkBounds() error {
	checker := boundChecker{program: p, work: &budget{limits: p.limits}, summaries: make(map[DeclarationID]dataCondition)}
	for _, module := range p.order {
		for _, declaration := range module.declarations {
			if _, err := checker.declaration(declaration, 0); err != nil {
				return sourceFailure(err, p.root)
			}
		}
	}
	return nil
}

// Summarize each source declaration once. Applying a summary substitutes its
// conditions, not its expression tree, so repeated DAG edges do not expand it.
func (c *boundChecker) declaration(declaration *Declaration, depth int) (dataCondition, error) {
	if err := c.work.step(depth); err != nil {
		return dataCondition{}, locate(err, declaration.id, nil)
	}
	if known, ok := c.summaries[declaration.id]; ok {
		return known, nil
	}
	env := make([]dataCondition, len(declaration.parameters))
	for i, parameter := range declaration.parameters {
		if parameter.Bound == TypeBound {
			env[i].requires = map[ParameterID]bool{{Declaration: declaration.id, Index: i}: true}
		}
	}
	result, err := c.domain(declaration.expression, declaration, env, depth)
	if err == nil {
		c.summaries[declaration.id] = result
	}
	return result, err
}

func (c *boundChecker) combine(result *dataCondition, condition dataCondition, depth int) error {
	result.impossible = result.impossible || condition.impossible
	for parameter := range condition.requires {
		if err := c.work.step(depth); err != nil {
			return err
		}
		if result.requires == nil {
			result.requires = make(map[ParameterID]bool)
		}
		result.requires[parameter] = true
	}
	return nil
}

func (c *boundChecker) domain(expression *Expression, owner *Declaration, env []dataCondition, depth int) (dataCondition, error) {
	result, err := c.domainIn(expression, owner, env, depth)
	if err != nil {
		return dataCondition{}, locate(err, owner.id, expression.path)
	}
	return result, nil
}

func (c *boundChecker) domainIn(expression *Expression, owner *Declaration, env []dataCondition, depth int) (dataCondition, error) {
	if err := c.work.step(depth); err != nil {
		return dataCondition{}, err
	}
	switch expression.kind {
	case Parameter:
		return env[expression.parameter.Index], nil
	case Application:
		target := c.program.declarations[expression.target]
		actual := make([]dataCondition, len(expression.arguments))
		for i, argument := range expression.arguments {
			condition, err := c.domain(argument, owner, env, depth+1)
			if err != nil {
				return dataCondition{}, err
			}
			if target.parameters[i].Bound == DataBound && !condition.proven() {
				return dataCondition{}, &Error{Code: BoundViolation, Subject: SubjectSource,
					Module: owner.id.Module, Declaration: owner.id.Name, Path: append([]int(nil), argument.path...),
					Target: target.id, Message: "application argument does not establish the required Data bound"}
			}
			actual[i] = condition
		}
		summary, err := c.declaration(target, depth+1)
		if err != nil {
			return dataCondition{}, err
		}
		result := dataCondition{impossible: summary.impossible}
		for parameter := range summary.requires {
			if err := c.combine(&result, actual[parameter.Index], depth); err != nil {
				return dataCondition{}, err
			}
		}
		return result, nil
	default:
		_, args, _ := head(expression.value, c.program.grammar.DescriptorPrefix)
		rule := c.program.grammar.Rule(expression.constructor, args)
		result := dataCondition{impossible: rule.Capability}
		for _, operand := range expression.operands {
			condition, err := c.domain(operand.expression, owner, env, depth+1)
			if err != nil {
				return dataCondition{}, err
			}
			if rule.RequiresData && !condition.proven() {
				return dataCondition{}, locate(failure(BoundViolation, "constructor operand requires Data"), owner.id, operand.expression.path)
			}
			if !rule.Capability {
				if err := c.combine(&result, condition, depth); err != nil {
					return dataCondition{}, err
				}
			}
		}
		return result, nil
	}
}
