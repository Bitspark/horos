package declmodel

import (
	"github.com/Bitspark/horos/core/go/internal/kindmodel"
	core "github.com/bitspark/ontos/core/go"
)

// bindGround records lexical rec/ref links before an expression is published.
// Application arguments and Wire messages start independent recursion scopes;
// generic parameters and resolved declaration references keep their own source
// bindings. The descriptor grammar separately establishes guardedness.
func bindGround(root *Expression, work *budget) error {
	var visit func(*Expression, map[string]*Expression, int) error
	visit = func(e *Expression, scope map[string]*Expression, depth int) error {
		if err := work.step(depth); err != nil {
			return err
		}
		if e == nil {
			return failure(Invalid, "missing expression while binding recursion")
		}
		located := func(message string) error {
			if len(e.path) > work.limits.MaxBytes/8 {
				return failure(Exhausted, "recursion diagnostic path exceeds byte limit")
			}
			if err := work.reserveBytes(len(e.path) * 8); err != nil {
				return err
			}
			return &Error{Code: Invalid, Subject: SubjectSource, Path: append([]int(nil), e.path...), Message: message}
		}
		if e.kind == Application {
			if err := visit(e.callee, nil, depth+1); err != nil {
				return err
			}
			for _, argument := range e.arguments {
				if err := work.step(depth); err != nil {
					return err
				}
				if err := visit(argument, nil, depth+1); err != nil {
					return err
				}
			}
			return nil
		}
		if e.kind != Descriptor {
			return nil
		}
		if e.constructor == "rec" || e.constructor == "ref" {
			tuple, ok := e.value.(core.Tuple)
			if !ok || tuple.Len() < 2 {
				return located("invalid ground recursion form")
			}
			atom, ok := tuple.At(1).(core.Atom)
			if !ok {
				return located("ground recursion name must be an atom")
			}
			if atom.Len() > work.limits.MaxBytes/2 {
				return failure(Exhausted, "recursion name exceeds byte limit")
			}
			if err := work.reserveBytes(atom.Len() * 2); err != nil {
				return err
			}
			name := string(atom.Bytes())
			if e.constructor == "ref" {
				binding := scope[name]
				if binding == nil {
					return located("ground recursion reference is not independently closed")
				}
				e.binding = binding
				return nil
			}
			if len(e.operands) != 1 {
				return located("ground recursion requires one body")
			}
			if scope == nil {
				if err := work.reserveBytes(64); err != nil {
					return err
				}
				scope = make(map[string]*Expression)
			}
			if err := work.reserveBytes(64); err != nil {
				return err
			}
			previous, existed := scope[name]
			scope[name] = e
			err := visit(e.operands[0].expression, scope, depth+1)
			if existed {
				scope[name] = previous
			} else {
				delete(scope, name)
			}
			return err
		}
		for _, operand := range e.operands {
			if err := work.step(depth); err != nil {
				return err
			}
			childScope := scope
			if e.constructor == "wire" {
				childScope = nil
			}
			if err := visit(operand.expression, childScope, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	return visit(root, nil, 0)
}

// A record is installed before visiting children. rec/ref records are aliases,
// while primitive records own observation nodes. Proper abstract leaves retain
// their bound kind but prevent publication of a closed observation.
type shapeRecord struct {
	term *kindTerm
	alias *shapeRecord
	form string
	selectors []kindmodel.Selector
	children []*shapeRecord
	id kindmodel.ObsID
	node bool
	resolved *shapeRecord
	resolving bool
}

func (c *checker) shapeSlice(count, size int) error {
	if count < 0 || size <= 0 || count > c.work.limits.MaxBytes/size {
		return failure(Exhausted, "observation allocation exceeds byte limit")
	}
	return c.work.reserveBytes(count * size)
}

func (c *checker) shapeMetadata(e *Expression, depth int) ([]kindmodel.Selector, error) {
	if err := c.work.step(depth); err != nil {
		return nil, err
	}
	tuple, ok := e.value.(core.Tuple)
	if !ok || tuple.Len() == 0 {
		return nil, failure(Invalid, "observation requires a primitive expression")
	}
	if err := c.shapeSlice(tuple.Len(), 16); err != nil {
		return nil, err
	}
	// Metadata only reads the scalar selector or record mode. Other operands,
	// especially exact payloads, are not traversed or interpreted here.
	if e.constructor == "scalar" || e.constructor == "record" {
		if tuple.Len() > 1 {
			if atom, ok := tuple.At(1).(core.Atom); ok {
				if err := c.shapeSlice(atom.Len(), 2); err != nil {
					return nil, err
				}
			}
		}
	}
	if err := c.allocate(depth); err != nil {
		return nil, err
	}
	metadata, err := c.program.grammar.Metadata(e.constructor, tuple.Items()[1:])
	if err != nil {
		return nil, err
	}
	for _, selector := range metadata {
		if err := c.work.step(depth); err != nil {
			return nil, err
		}
		if err := c.work.reserveBytes(len(selector.Key)); err != nil {
			return nil, err
		}
		if err := c.work.reserveBytes(len(selector.Value)); err != nil {
			return nil, err
		}
	}
	return metadata, nil
}

func (c *checker) shape(v *semanticValue, depth int) (*kindTerm, kindmodel.Observation, bool, error) {
	if err := c.allocate(depth); err != nil {
		return nil, kindmodel.Observation{}, false, err
	}
	seen := make(map[*semanticValue]*shapeRecord)
	var nodes []*shapeRecord
	var implicitAny *semanticValue
	closed := true
	var visit func(*semanticValue, int) (*shapeRecord, error)
	visit = func(value *semanticValue, depth int) (*shapeRecord, error) {
		if err := c.work.step(depth); err != nil {
			return nil, err
		}
		if value == nil || !value.sort.IsProper() {
			return nil, failure(KindMismatch, "a constructor has no proper structural observation")
		}
		if known := seen[value]; known != nil {
			return known, nil
		}
		if err := c.allocate(depth); err != nil {
			return nil, err
		}
		if err := c.work.reserveBytes(256); err != nil {
			return nil, err
		}
		record := &shapeRecord{term: &kindTerm{form: KindVariable, sort: value.sort}}
		seen[value] = record
		if value.rigid || value.abstract {
			bound := c.bound(value)
			if bound == nil || !bound.sort.IsProper() {
				return nil, failure(Invalid, "abstract proper value has no proper bound")
			}
			record.term = bound
			closed = false
			return record, nil
		}
		e := value.expression
		if e == nil || e.kind != Descriptor {
			return nil, failure(Invalid, "proper observation requires a reduced primitive expression")
		}
		if e.constructor == "rec" || e.constructor == "ref" {
			var target *Expression
			if e.constructor == "rec" {
				if len(e.operands) != 1 {
					return nil, failure(Invalid, "ground recursion requires one body")
				}
				target = e.operands[0].expression
			} else {
				target = e.binding
				if target == nil {
					return nil, failure(Invalid, "ground recursion reference has no lexical binding")
				}
			}
			next, err := c.value(target, value.env, depth+1)
			if err != nil {
				return nil, err
			}
			child, err := visit(next, depth+1)
			if err != nil {
				return nil, err
			}
			if err := c.shapeSlice(1, 8); err != nil {
				return nil, err
			}
			record.alias = child
			record.term.children = []*kindTerm{child.term}
			return record, nil
		}
		metadata, err := c.shapeMetadata(e, depth)
		if err != nil {
			return nil, err
		}
		openRecord := false
		for _, selector := range metadata {
			if err := c.work.step(depth); err != nil {
				return nil, err
			}
			if e.constructor == "record" && selector.Key == "mode" && selector.Value == "open" {
				openRecord = true
			}
		}
		count := len(e.operands)
		if openRecord {
			if count == int(^uint(0)>>1) {
				return nil, failure(Exhausted, "observation width exceeds address space")
			}
			count++
		}
		if err := c.shapeSlice(count, 8); err != nil {
			return nil, err
		}
		record.children = make([]*shapeRecord, count)
		record.form = e.constructor
		record.selectors = metadata
		record.node = true
		if uint64(len(nodes)) >= uint64(1)<<32 {
			return nil, failure(Exhausted, "observation node identifier limit reached")
		}
		record.id = kindmodel.ObsID(len(nodes))
		if err := c.shapeSlice(1, 16); err != nil {
			return nil, err
		}
		nodes = append(nodes, record)
		record.term.form = KindNode
		record.term.nodeForm = e.constructor
		record.term.selectors = metadata
		for i, operand := range e.operands {
			if err := c.work.step(depth); err != nil {
				return nil, err
			}
			childValue, err := c.value(operand.expression, value.env, depth+1)
			if err != nil {
				return nil, err
			}
			record.children[i], err = visit(childValue, depth+1)
			if err != nil {
				return nil, err
			}
		}
		if openRecord {
			if implicitAny == nil {
				if err := c.allocate(depth); err != nil {
					return nil, err
				}
				if err := c.work.reserveBytes(384); err != nil {
					return nil, err
				}
				implicitAny = &semanticValue{sort: Sort{}, expression: &Expression{
					kind: Descriptor, constructor: "any", value: c.program.grammar.Any(), sort: Sort{},
				}}
			}
			record.children[count-1], err = visit(implicitAny, depth+1)
			if err != nil {
				return nil, err
			}
		}
		if err := c.allocate(depth); err != nil {
			return nil, err
		}
		sequence := &sequenceTerm{form: SequenceEpsilon}
		if count == 1 {
			sequence.form = SequenceItem
			sequence.element = record.children[0].term
		} else if count > 1 {
			sequence.form = SequenceConcat
			if err := c.shapeSlice(count, 8); err != nil {
				return nil, err
			}
			sequence.children = make([]*sequenceTerm, count)
			for i, child := range record.children {
				if err := c.allocate(depth); err != nil {
					return nil, err
				}
				sequence.children[i] = &sequenceTerm{form: SequenceItem, element: child.term}
			}
		}
		record.term.sequence = sequence
		return record, nil
	}
	root, err := visit(v, depth)
	if err != nil {
		return nil, kindmodel.Observation{}, false, err
	}
	if !closed {
		return root.term, kindmodel.Observation{}, false, nil
	}
	// Collapse only observation aliases. Kind proxies remain graph edges so the
	// structural inclusion compiler sees the same guarded recursive unfolding.
	resolve := func(start *shapeRecord) (*shapeRecord, error) {
		var trail []*shapeRecord
		current := start
		for current != nil && !current.node && current.resolved == nil {
			if err := c.work.step(depth); err != nil {
				return nil, err
			}
			if current.resolving {
				return nil, failure(Invalid, "ground recursion has an unguarded alias cycle")
			}
			current.resolving = true
			if err := c.shapeSlice(1, 16); err != nil {
				return nil, err
			}
			trail = append(trail, current)
			current = current.alias
		}
		if current == nil {
			return nil, failure(Invalid, "closed observation has no primitive node")
		}
		if current.resolved != nil {
			current = current.resolved
		}
		for _, alias := range trail {
			if err := c.work.step(depth); err != nil {
				return nil, err
			}
			alias.resolved = current
			alias.resolving = false
		}
		return current, nil
	}
	resolvedRoot, err := resolve(root)
	if err != nil {
		return nil, kindmodel.Observation{}, false, err
	}
	if err := c.shapeSlice(len(nodes), 64); err != nil {
		return nil, kindmodel.Observation{}, false, err
	}
	observation := kindmodel.Observation{Root: resolvedRoot.id, Nodes: make([]kindmodel.ObservationNode, len(nodes))}
	for _, node := range nodes {
		if err := c.work.step(depth); err != nil {
			return nil, kindmodel.Observation{}, false, err
		}
		if err := c.shapeSlice(len(node.children), 4); err != nil {
			return nil, kindmodel.Observation{}, false, err
		}
		children := make([]kindmodel.ObsID, len(node.children))
		for i, child := range node.children {
			if err := c.work.step(depth); err != nil {
				return nil, kindmodel.Observation{}, false, err
			}
			resolved, err := resolve(child)
			if err != nil {
				return nil, kindmodel.Observation{}, false, err
			}
			children[i] = resolved.id
		}
		observation.Nodes[node.id] = kindmodel.ObservationNode{Form: node.form, Selectors: node.selectors, Children: children}
	}
	return root.term, observation, true, nil
}
