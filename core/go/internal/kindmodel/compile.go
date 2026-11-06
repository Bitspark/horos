package kindmodel

// Compile admits a positive, guarded equation system and retains an immutable
// copy. Declaration expansion, lexical binding and sorts belong to the caller.
func Compile(schema Schema, definition Definition, b Budget) (*Kind, error) {
	if err := requireBudget(b); err != nil {
		return nil, err
	}
	if err := charge(b, uint64(len(schema.Forms)), 96); err != nil {
		return nil, err
	}
	k := &Kind{
		schema: Schema{Forms: make([]Form, len(schema.Forms))},
		forms:  make(map[string]int, len(schema.Forms)),
	}
	for i, form := range schema.Forms {
		if err := step(b); err != nil {
			return nil, err
		}
		if form.Name == "" {
			return nil, invalid("form %d has no name", i)
		}
		if _, exists := k.forms[form.Name]; exists {
			return nil, invalid("duplicate form %q", form.Name)
		}
		if form.MaxChildren < -1 || (form.MaxChildren >= 0 && uint64(form.MaxChildren) < uint64(form.MinChildren)) {
			return nil, invalid("invalid child width for form %q", form.Name)
		}
		if err := charge(b, uint64(len(form.Selectors)), 96); err != nil {
			return nil, err
		}
		copyForm := form
		copyForm.Selectors = make([]SelectorDomain, len(form.Selectors))
		keys := make(map[string]bool, len(form.Selectors))
		for j, domain := range form.Selectors {
			if err := step(b); err != nil {
				return nil, err
			}
			if domain.Key == "" || keys[domain.Key] {
				return nil, invalid("empty or duplicate selector in form %q", form.Name)
			}
			keys[domain.Key] = true
			if err := charge(b, uint64(len(domain.Values)), 64); err != nil {
				return nil, err
			}
			values := make(map[string]bool, len(domain.Values))
			for _, value := range domain.Values {
				if err := step(b); err != nil {
					return nil, err
				}
				if values[value] {
					return nil, invalid("duplicate value for selector %q", domain.Key)
				}
				values[value] = true
			}
			copyForm.Selectors[j] = domain
			copyForm.Selectors[j].Values = append([]string(nil), domain.Values...)
		}
		k.forms[form.Name] = i
		k.schema.Forms[i] = copyForm
	}
	if len(definition.States) == 0 || uint64(definition.Root) >= uint64(len(definition.States)) {
		return nil, invalid("missing or out-of-range root state")
	}
	if uint64(len(definition.States)) > uint64(^StateID(0)) || uint64(len(definition.Sequences)) > uint64(^SeqID(0)) {
		return nil, invalid("equation table exceeds its identifier space")
	}
	if err := charge(b, uint64(len(definition.States)), 96); err != nil {
		return nil, err
	}
	if err := charge(b, uint64(len(definition.Sequences)), 48); err != nil {
		return nil, err
	}
	k.definition = Definition{
		Root:      definition.Root,
		States:    make([]State, len(definition.States)),
		Sequences: make([]Sequence, len(definition.Sequences)),
	}
	for i, state := range definition.States {
		if err := step(b); err != nil {
			return nil, err
		}
		if err := charge(b, uint64(len(state.Args)), 4); err != nil {
			return nil, err
		}
		if err := charge(b, uint64(len(state.Node.Selectors)), 64); err != nil {
			return nil, err
		}
		for _, argument := range state.Args {
			if err := step(b); err != nil {
				return nil, err
			}
			if uint64(argument) >= uint64(len(definition.States)) {
				return nil, invalid("state %d has an out-of-range argument", i)
			}
		}
		switch state.Op {
		case Top, Bottom:
			if len(state.Args) != 0 {
				return nil, invalid("constant state %d has arguments", i)
			}
		case Or, And:
			if len(state.Args) < 2 {
				return nil, invalid("Boolean state %d requires at least two arguments", i)
			}
		case Alias:
			if len(state.Args) != 1 {
				return nil, invalid("alias state %d requires one argument", i)
			}
		case Node:
			if len(state.Args) != 0 || uint64(state.Node.Sequence) >= uint64(len(definition.Sequences)) {
				return nil, invalid("node state %d has invalid operands", i)
			}
			formIndex, found := k.forms[state.Node.Form]
			if !found {
				return nil, invalid("unknown form %q in state %d", state.Node.Form, i)
			}
			if err := admitSelectors(k.schema.Forms[formIndex], state.Node.Selectors, false, b); err != nil {
				return nil, err
			}
		default:
			return nil, invalid("unknown state operation %d", state.Op)
		}
		if state.Op != Node && (state.Node.Form != "" || len(state.Node.Selectors) != 0 || state.Node.Sequence != 0) {
			return nil, invalid("non-node state %d has a node pattern", i)
		}
		copyState := state
		copyState.Args = append([]StateID(nil), state.Args...)
		copyState.Node.Selectors = append([]Selector(nil), state.Node.Selectors...)
		k.definition.States[i] = copyState
	}
	for i, sequence := range definition.Sequences {
		if err := step(b); err != nil {
			return nil, err
		}
		if err := charge(b, uint64(len(sequence.Args)), 4); err != nil {
			return nil, err
		}
		for _, argument := range sequence.Args {
			if err := step(b); err != nil {
				return nil, err
			}
			if uint64(argument) >= uint64(len(definition.Sequences)) {
				return nil, invalid("sequence %d has an out-of-range argument", i)
			}
		}
		switch sequence.Op {
		case Never, Epsilon:
			if len(sequence.Args) != 0 {
				return nil, invalid("constant sequence %d has arguments", i)
			}
		case Item:
			if len(sequence.Args) != 0 || uint64(sequence.Item) >= uint64(len(definition.States)) {
				return nil, invalid("item sequence %d has invalid operands", i)
			}
		case Concat, Choice:
			if len(sequence.Args) < 2 {
				return nil, invalid("sequence %d requires at least two arguments", i)
			}
		case Star:
			if len(sequence.Args) != 1 {
				return nil, invalid("star sequence %d requires one argument", i)
			}
		default:
			return nil, invalid("unknown sequence operation %d", sequence.Op)
		}
		if sequence.Op != Item && sequence.Item != 0 {
			return nil, invalid("non-item sequence %d has an item state", i)
		}
		copySequence := sequence
		copySequence.Args = append([]SeqID(nil), sequence.Args...)
		k.definition.Sequences[i] = copySequence
	}
	if err := admitDAG(len(k.definition.Sequences), func(i int) int {
		return len(k.definition.Sequences[i].Args)
	}, func(i, j int) int {
		return int(k.definition.Sequences[i].Args[j])
	}, "sequence cycle", b); err != nil {
		return nil, err
	}
	if err := admitDAG(len(k.definition.States), func(i int) int {
		return len(k.definition.States[i].Args)
	}, func(i, j int) int {
		return int(k.definition.States[i].Args[j])
	}, "unguarded recursive state cycle", b); err != nil {
		return nil, err
	}
	k.valid = true
	return k, nil
}

func admitSelectors(form Form, selectors []Selector, complete bool, b Budget) error {
	if err := charge(b, uint64(len(selectors)), 64); err != nil {
		return err
	}
	seen := make(map[string]bool, len(selectors))
	for _, selected := range selectors {
		if err := step(b); err != nil {
			return err
		}
		if seen[selected.Key] {
			return invalid("duplicate selector %q for form %q", selected.Key, form.Name)
		}
		seen[selected.Key] = true
		found := false
		for _, domain := range form.Selectors {
			if err := step(b); err != nil {
				return err
			}
			if domain.Key != selected.Key {
				continue
			}
			found = true
			allowed := domain.Open
			for _, value := range domain.Values {
				if err := step(b); err != nil {
					return err
				}
				allowed = allowed || value == selected.Value
			}
			if !allowed {
				return invalid("unapproved value for selector %q on form %q", selected.Key, form.Name)
			}
			break
		}
		if !found {
			return invalid("unknown selector %q for form %q", selected.Key, form.Name)
		}
	}
	if complete {
		for _, domain := range form.Selectors {
			if err := step(b); err != nil {
				return err
			}
			if !domain.Optional && !seen[domain.Key] {
				return invalid("missing selector %q for form %q", domain.Key, form.Name)
			}
		}
	}
	return nil
}

type dagFrame struct {
	node int
	next int
}

func admitDAG(n int, count func(int) int, child func(int, int) int, message string, b Budget) error {
	if err := charge(b, uint64(n), 24); err != nil {
		return err
	}
	colors := make([]uint8, n)
	stack := make([]dagFrame, 0, n)
	for start := 0; start < n; start++ {
		if err := step(b); err != nil {
			return err
		}
		if colors[start] != 0 {
			continue
		}
		colors[start] = 1
		stack = append(stack, dagFrame{node: start})
		for len(stack) != 0 {
			if err := b.Step(uint64(len(stack))); err != nil {
				return err
			}
			last := len(stack) - 1
			frame := &stack[last]
			if frame.next == count(frame.node) {
				colors[frame.node] = 2
				stack = stack[:last]
				continue
			}
			next := child(frame.node, frame.next)
			frame.next++
			if colors[next] == 1 {
				return invalid("%s at state %d", message, next)
			}
			if colors[next] == 0 {
				colors[next] = 1
				stack = append(stack, dagFrame{node: next})
			}
		}
	}
	return nil
}
