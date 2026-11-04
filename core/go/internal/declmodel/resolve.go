package declmodel

import (
	"crypto/sha256"
	"fmt"
	"slices"

	codec "github.com/bitspark/ontos/codec/go"
	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

// Resolve checks the complete supplied closure using the owner's fixed grammar.
func Resolve(root core.Value, dependencies []core.Value, limits Limits, grammar Grammar) (*Program, error) {
	if !limits.valid() {
		return nil, failure(InvalidOptions, "invalid checking limits")
	}
	if len(dependencies) >= limits.MaxModules {
		return nil, failure(Exhausted, "supplied-module limit reached")
	}
	pool := make(map[Digest]core.Value, len(dependencies)+1)
	input := &budget{limits: limits}
	all := append([]core.Value{root}, dependencies...)
	var rootPin Digest
	for i, value := range all {
		if err := input.value(value, 0); err != nil {
			if detail, ok := err.(*Error); ok && i > 0 && detail.Code == Invalid {
				copy := *detail
				copy.Subject = SubjectContext
				return nil, &copy
			}
			return nil, err
		}
		pin := sha256.Sum256(codec.Encode(value))
		if previous, exists := pool[pin]; exists && !core.Equal(previous, value) {
			return nil, &Error{Code: Invalid, Subject: SubjectContext, Dependency: pin,
				Message: "distinct module values have the same digest"}
		}
		pool[pin] = value
		if i == 0 {
			rootPin = pin
		}
	}
	p := &Program{
		root: rootPin, limits: limits, grammar: grammar,
		modules: make(map[Digest]*Module), declarations: make(map[DeclarationID]*Declaration),
	}
	state := make(map[Digest]uint8)
	var load func(Digest, int) error
	load = func(pin Digest, depth int) error {
		if depth > limits.MaxDepth {
			return failure(Exhausted, "import depth limit reached")
		}
		if state[pin] == 1 {
			return sourceFailure(&Error{Code: Cycle, Subject: SubjectSource, Module: pin,
				Message: "cyclic module imports"}, rootPin)
		}
		if state[pin] == 2 {
			return nil
		}
		value, ok := pool[pin]
		if !ok {
			return &Error{Code: MissingDependency, Subject: SubjectContext, Dependency: pin,
				Message: "missing pinned module " + pin.String()}
		}
		m, err := readModule(pin, value, grammar)
		if err != nil {
			if detail, ok := err.(*Error); ok {
				copy := *detail
				copy.Module = pin
				if pin != rootPin {
					copy.Subject = SubjectContext
					copy.Dependency = pin
				}
				return &copy
			}
			return err
		}
		state[pin] = 1
		p.modules[pin] = m
		for _, imported := range m.imports {
			if err := load(imported.pin, depth+1); err != nil {
				if detail, ok := err.(*Error); ok && detail.Subject == SubjectContext && detail.ImportAlias == "" {
					copy := *detail
					copy.Module = m.pin
					copy.ImportAlias = imported.alias
					copy.Dependency = imported.pin
					copy.ExpectedName = imported.name
					return &copy
				}
				return err
			}
			if p.modules[imported.pin].name != imported.name {
				return &Error{Code: ImportMismatch, Subject: SubjectContext, Module: m.pin,
					ImportAlias: imported.alias, Dependency: imported.pin, ExpectedName: imported.name,
					ActualName: p.modules[imported.pin].name,
					Message:    "imported module name does not match its pin"}
			}
		}
		state[pin] = 2
		return nil
	}
	if err := load(rootPin, 0); err != nil {
		return nil, err
	}
	for _, m := range p.modules {
		p.order = append(p.order, m)
		for _, declaration := range m.declarations {
			p.declarations[declaration.id] = declaration
		}
	}
	slices.SortFunc(p.order, func(a, b *Module) int {
		return slices.Compare(a.pin[:], b.pin[:])
	})
	reader := expressionReader{program: p, budget: &budget{limits: limits}, edges: make(map[DeclarationID][]DeclarationID)}
	for _, m := range p.order {
		for _, declaration := range m.declarations {
			expression, skeleton, err := reader.read(declaration.body, declaration, nil, 0)
			if err == nil && expression.interface_.IsConstructor() {
				err = failure(KindMismatch, "declaration body must be a proper type")
			}
			if err == nil {
				err = p.grammar.WF(skeleton)
			}
			if err != nil {
				return nil, sourceFailure(locate(err, declaration.id, nil), p.root)
			}
			declaration.expression = expression
		}
	}
	if err := reader.acyclic(); err != nil {
		return nil, err
	}
	if err := p.checkBounds(reader.budget); err != nil {
		return nil, err
	}
	return p, nil
}

func locate(err error, id DeclarationID, path []int) error {
	if detail, ok := err.(*Error); ok && detail.Declaration == "" {
		copy := *detail
		copy.Module = id.Module
		copy.Declaration = id.Name
		copy.Path = append(slices.Clone(path), detail.Path...)
		return &copy
	}
	return err
}

// A located failure in supplied dependency source is about this program's
// context. Keep its defining location rather than moving it to an outer import.
// Exhaustion is a request outcome and must not become a context rejection.
func sourceFailure(err error, root Digest) error {
	if detail, ok := err.(*Error); ok && detail.Subject == SubjectSource && detail.Module != root {
		copy := *detail
		copy.Subject = SubjectContext
		copy.Dependency = detail.Module
		return &copy
	}
	return err
}

func readModule(pin Digest, value core.Value, grammar Grammar) (*Module, error) {
	args, ok := grammar.form(value, "module", 4)
	if !ok {
		return nil, failure(Invalid, "expected revision-3 module form")
	}
	revision, ok := index(args[0])
	name, nameErr := data.ReadText(args[1])
	if !ok || revision != 3 || nameErr != nil || !moduleName(name) {
		return nil, failure(Invalid, "invalid module revision or name")
	}
	imports, err := data.ReadMap(args[2])
	if err != nil {
		return nil, failure(Invalid, "imports must be a canonical map")
	}
	declarations, err := data.ReadMap(args[3])
	if err != nil {
		return nil, failure(Invalid, "declarations must be a canonical map")
	}
	m := &Module{pin: pin, name: name, value: value, importByName: make(map[string]Import)}
	for _, entry := range imports {
		alias, err := data.ReadText(entry[0])
		args, ok := grammar.form(entry[1], "import", 2)
		if err != nil || !identifier(alias) || !ok {
			return nil, failure(Invalid, "invalid import alias or form")
		}
		name, err := data.ReadText(args[0])
		pin, ok := digest(args[1])
		if err != nil || !moduleName(name) || !ok {
			return nil, failure(Invalid, "invalid import module name or digest")
		}
		imported := Import{alias: alias, name: name, pin: pin}
		m.imports = append(m.imports, imported)
		m.importByName[alias] = imported
	}
	for _, entry := range declarations {
		name, err := data.ReadText(entry[0])
		args, ok := grammar.form(entry[1], "type", 3)
		if err != nil || !identifier(name) || !ok {
			return nil, failure(Invalid, "invalid declaration name or form")
		}
		params, err := data.ReadList(args[0])
		if err != nil {
			return nil, failure(Invalid, "parameters must be a canonical list")
		}
		claim, err := readClaim(args[1])
		if err != nil {
			return nil, err
		}
		declaration := &Declaration{id: DeclarationID{Module: pin, Name: name}, result: claim, body: args[2]}
		names := make(map[string]bool)
		for _, param := range params {
			parts, ok := grammar.form(param, "parameter", 2)
			if !ok {
				return nil, failure(Invalid, "expected parameter name and explicit interface")
			}
			name, err := data.ReadText(parts[0])
			if err != nil || !identifier(name) || names[name] {
				return nil, failure(Invalid, "invalid or duplicate parameter name")
			}
			iface, err := readInterface(parts[1], grammar)
			if err != nil {
				return nil, err
			}
			names[name] = true
			declaration.parameters = append(declaration.parameters, TypeParameter{Name: name, Interface: iface})
		}
		declaration.interface_ = Interface{result: claim}
		for _, parameter := range declaration.parameters {
			declaration.interface_.inputs = append(declaration.interface_.inputs, parameter.Interface)
		}
		m.declarations = append(m.declarations, declaration)
	}
	return m, nil
}

type expressionReader struct {
	program *Program
	budget  *budget
	edges   map[DeclarationID][]DeclarationID
}

func (r *expressionReader) target(value core.Value, owner DeclarationID) (DeclarationID, error) {
	module := r.program.modules[owner.Module]
	var name string
	var importAlias string
	pin := owner.Module
	if args, ok := r.program.grammar.form(value, "local", 1); ok {
		var err error
		name, err = data.ReadText(args[0])
		if err != nil || !identifier(name) {
			return DeclarationID{}, failure(Invalid, "invalid local declaration target")
		}
	} else if args, ok := r.program.grammar.form(value, "imported", 2); ok {
		alias, err := data.ReadText(args[0])
		name, _ = data.ReadText(args[1])
		if err != nil || !identifier(alias) || !identifier(name) {
			return DeclarationID{}, failure(Invalid, "invalid imported declaration target")
		}
		imported, found := module.importByName[alias]
		if !found {
			return DeclarationID{}, &Error{Code: UnknownImport, Subject: SubjectSource,
				ImportAlias: alias, Message: "unknown import alias " + alias}
		}
		importAlias = alias
		pin = imported.pin
	} else {
		return DeclarationID{}, failure(Invalid, "expected local or imported target")
	}
	id := DeclarationID{Module: pin, Name: name}
	if _, ok := r.program.declarations[id]; !ok {
		return DeclarationID{}, &Error{Code: UnknownDeclaration, Subject: SubjectSource,
			ImportAlias: importAlias, Target: id, Message: "unknown declaration " + name}
	}
	return id, nil
}

func (r *expressionReader) read(value core.Value, owner *Declaration, path []int, depth int) (*Expression, core.Value, error) {
	expression, skeleton, err := r.readIn(value, owner, path, depth)
	if err != nil {
		return nil, nil, locate(err, owner.id, path)
	}
	expression.path = slices.Clone(path)
	return expression, skeleton, nil
}

func (r *expressionReader) readIn(value core.Value, owner *Declaration, path []int, depth int) (*Expression, core.Value, error) {
	if err := r.budget.step(depth); err != nil {
		return nil, nil, err
	}
	if args, ok := r.program.grammar.form(value, "param", 1); ok {
		slot, ok := index(args[0])
		if !ok || slot >= len(owner.parameters) {
			return nil, nil, failure(Invalid, "unbound or out-of-range type parameter")
		}
		iface := owner.parameters[slot].Interface
		var skeleton core.Value
		if !iface.IsConstructor() {
			skeleton = r.program.grammar.Any()
		}
		return &Expression{kind: Parameter, value: value, parameter: ParameterID{Declaration: owner.id, Index: slot}, interface_: iface}, skeleton, nil
	}
	_, local := r.program.grammar.form(value, "local", 1)
	_, imported := r.program.grammar.form(value, "imported", 2)
	if local || imported {
		target, err := r.target(value, owner.id)
		if err != nil {
			return nil, nil, err
		}
		iface := r.program.declarations[target].interface_
		r.edges[owner.id] = append(r.edges[owner.id], target)
		var skeleton core.Value
		if !iface.IsConstructor() {
			skeleton = r.program.grammar.Any()
		}
		return &Expression{kind: Reference, value: value, target: target, interface_: iface}, skeleton, nil
	}
	if args, ok := r.program.grammar.form(value, "apply", 2); ok {
		calleePath := append(slices.Clone(path), 1)
		callee, _, err := r.read(args[0], owner, calleePath, depth+1)
		if err != nil {
			return nil, nil, err
		}
		if !callee.interface_.IsConstructor() {
			return nil, nil, locate(applicationFailure(KindMismatch, callee, Interface{}, "application callee must be a constructor"), owner.id, calleePath)
		}
		arguments, err := data.ReadList(args[1])
		if err != nil || len(arguments) == 0 || len(arguments) != len(callee.interface_.inputs) {
			return nil, nil, applicationFailure(Invalid, callee, callee.interface_, "application argument list or arity mismatch")
		}
		result := &Expression{kind: Application, value: value, callee: callee, interface_: Interface{result: callee.interface_.result}}
		for i, argument := range arguments {
			childPath := append(slices.Clone(path), 2, i+1)
			child, skeleton, err := r.read(argument, owner, childPath, depth+1)
			if err == nil && !child.interface_.IsConstructor() {
				// Each argument is independently closed under recursion binders.
				err = r.program.grammar.WF(skeleton)
			}
			if err != nil {
				return nil, nil, locate(err, owner.id, childPath)
			}
			result.arguments = append(result.arguments, child)
		}
		return result, r.program.grammar.Any(), nil
	}
	constructor, args, ok := head(value, r.program.grammar.DescriptorPrefix)
	if !ok {
		return nil, nil, failure(Invalid, "expected a descriptor or declaration expression")
	}
	positions, err := r.program.grammar.Positions(constructor, args)
	if err != nil {
		return nil, nil, err
	}
	result := &Expression{kind: Descriptor, value: value, constructor: constructor, interface_: Interface{result: TypeClaim}}
	var children []core.Value
	for _, position := range positions {
		childPath := append(slices.Clone(path), position...)
		child, concrete, err := r.read(at(value, position), owner, childPath, depth+1)
		if err != nil {
			return nil, nil, err
		}
		if child.interface_.IsConstructor() {
			return nil, nil, locate(failure(KindMismatch, "descriptor operand must be a proper type"), owner.id, childPath)
		}
		result.operands = append(result.operands, Operand{path: position, expression: child})
		children = append(children, concrete)
	}
	return result, replaceOperands(value, result.operands, children), nil
}

func (r *expressionReader) acyclic() error {
	state := make(map[DeclarationID]uint8)
	var visit func(DeclarationID, int) error
	visit = func(id DeclarationID, depth int) error {
		if err := r.budget.step(depth); err != nil {
			return locate(err, id, nil)
		}
		if state[id] == 1 {
			return locate(failure(Cycle, "cyclic declaration references"), id, nil)
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		for _, target := range r.edges[id] {
			if err := visit(target, depth+1); err != nil {
				return err
			}
		}
		state[id] = 2
		r.program.declarationOrder = append(r.program.declarationOrder, r.program.declarations[id])
		return nil
	}
	for _, module := range r.program.order {
		for _, declaration := range module.declarations {
			if err := visit(declaration.id, 0); err != nil {
				return fmt.Errorf("declaration graph: %w", sourceFailure(err, r.program.root))
			}
		}
	}
	return nil
}
