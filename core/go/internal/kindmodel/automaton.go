package kindmodel

// A clause is an intersection of Node states at one observation root. The
// empty clause is Top; a list containing no clauses is Bottom.
type automatonClauses [][]StateID

type automatonBuilder struct {
	k         *Kind
	labels    []label
	b         Budget
	out       automaton
	joints    map[string]int
	roots     []automatonClauses
	sequences []*wordNFA
}

func buildAutomaton(k *Kind, labels []label, b Budget) (*automaton, error) {
	if err := requireBudget(b); err != nil {
		return nil, err
	}
	if k == nil {
		return nil, invalid("nil compiled kind")
	}
	if err := charge(b, 1, 256); err != nil {
		return nil, err
	}
	c := &automatonBuilder{k: k, labels: labels, b: b, joints: make(map[string]int)}
	var err error
	c.roots, err = automatonRootClauses(k.definition.States, b)
	if err != nil {
		return nil, err
	}
	c.out.root, err = c.joint([]StateID{k.definition.Root})
	if err != nil {
		return nil, err
	}
	c.sequences, err = c.compileSequences()
	if err != nil {
		return nil, err
	}
	// Compiling a child obligation may append another vertical state. Process
	// those states through this growing worklist rather than recursive calls.
	for state := 0; state < len(c.out.states); state++ {
		if err := step(b); err != nil {
			return nil, err
		}
		clauses, err := c.clauses(c.out.states[state].obligations)
		if err != nil {
			return nil, err
		}
		for labelIndex, root := range labels {
			if err := step(b); err != nil {
				return nil, err
			}
			var alternatives []*wordNFA
			for _, clause := range clauses {
				if err := step(b); err != nil {
					return nil, err
				}
				matches := true
				for _, node := range clause {
					if err := step(b); err != nil {
						return nil, err
					}
					pattern := k.definition.States[node].Node
					for range pattern.Selectors {
						if err := step(b); err != nil {
							return nil, err
						}
					}
					if !root.matches(pattern) {
						matches = false
						break
					}
				}
				if !matches {
					continue
				}
				if err := charge(b, uint64(len(clause))+1, 16); err != nil {
					return nil, err
				}
				parts := make([]*wordNFA, 0, len(clause)+1)
				for _, node := range clause {
					if err := step(b); err != nil {
						return nil, err
					}
					parts = append(parts, c.sequences[k.definition.States[node].Node.Sequence])
				}
				width, err := c.width(root)
				if err != nil {
					return nil, err
				}
				parts = append(parts, width)
				word, err := c.intersectWords(parts)
				if err != nil {
					return nil, err
				}
				if err := charge(b, 1, 16); err != nil {
					return nil, err
				}
				alternatives = append(alternatives, word)
			}
			if len(alternatives) != 0 {
				word, err := automatonUnionWords(alternatives, b)
				if err != nil {
					return nil, err
				}
				c.out.states[state].words[labelIndex] = word
			}
		}
	}
	return &c.out, nil
}

func (c *automatonBuilder) joint(obligations []StateID) (int, error) {
	ids, err := automatonCanonicalIDs(obligations, c.b)
	if err != nil {
		return 0, err
	}
	if err := charge(c.b, uint64(len(ids)), 16); err != nil {
		return 0, err
	}
	keyIDs := make([]int, len(ids))
	for i, id := range ids {
		if err := step(c.b); err != nil {
			return 0, err
		}
		keyIDs[i] = int(id)
	}
	key, err := keyGroups(c.b, keyIDs)
	if err != nil {
		return 0, err
	}
	if existing, found := c.joints[key]; found {
		return existing, nil
	}
	if err := charge(c.b, 1, 192); err != nil {
		return 0, err
	}
	if err := charge(c.b, uint64(len(c.labels)), 8); err != nil {
		return 0, err
	}
	id := len(c.out.states)
	c.out.states = append(c.out.states, automatonState{
		obligations: ids,
		words:       make([]*wordNFA, len(c.labels)),
	})
	c.joints[key] = id
	return id, nil
}

func automatonCanonicalIDs(input []StateID, b Budget) ([]StateID, error) {
	if err := charge(b, uint64(len(input)), 8); err != nil {
		return nil, err
	}
	result := make([]StateID, 0, len(input))
	for _, id := range input {
		if err := step(b); err != nil {
			return nil, err
		}
		at := len(result)
		for at > 0 && result[at-1] > id {
			if err := step(b); err != nil {
				return nil, err
			}
			at--
		}
		if at > 0 && result[at-1] == id {
			continue
		}
		result = append(result, 0)
		for i := len(result) - 1; i > at; i-- {
			if err := step(b); err != nil {
				return nil, err
			}
			result[i] = result[i-1]
		}
		result[at] = id
	}
	return result, nil
}

func automatonRootClauses(states []State, b Budget) ([]automatonClauses, error) {
	if err := charge(b, uint64(len(states)), 32); err != nil {
		return nil, err
	}
	result := make([]automatonClauses, len(states))
	color := make([]uint8, len(states))
	type frame struct {
		id   StateID
		next int
	}
	for initial := range states {
		if err := step(b); err != nil {
			return nil, err
		}
		if color[initial] == 2 {
			continue
		}
		if err := charge(b, 1, 32); err != nil {
			return nil, err
		}
		stack := []frame{{id: StateID(initial)}}
		color[initial] = 1
		for len(stack) != 0 {
			if err := step(b); err != nil {
				return nil, err
			}
			last := len(stack) - 1
			current := &stack[last]
			state := states[current.id]
			if state.Op == Or || state.Op == And || state.Op == Alias {
				if current.next < len(state.Args) {
					child := state.Args[current.next]
					current.next++
					if color[child] == 1 {
						return nil, invalid("unguarded state cycle")
					}
					if color[child] == 0 {
						if err := charge(b, 1, 32); err != nil {
							return nil, err
						}
						color[child] = 1
						stack = append(stack, frame{id: child})
					}
					continue
				}
			}
			var clauses automatonClauses
			switch state.Op {
			case Top:
				if err := charge(b, 1, 32); err != nil {
					return nil, err
				}
				clauses = automatonClauses{nil}
			case Bottom:
			case Node:
				if err := charge(b, 1, 40); err != nil {
					return nil, err
				}
				clauses = automatonClauses{{current.id}}
			case Alias:
				clauses = result[state.Args[0]]
			case Or:
				for _, child := range state.Args {
					if err := step(b); err != nil {
						return nil, err
					}
					if err := charge(b, uint64(len(result[child])), 48); err != nil {
						return nil, err
					}
					clauses = append(clauses, result[child]...)
				}
			case And:
				if err := charge(b, 1, 32); err != nil {
					return nil, err
				}
				clauses = automatonClauses{nil}
				for _, child := range state.Args {
					if err := step(b); err != nil {
						return nil, err
					}
					var err error
					clauses, err = automatonConjoinClauses(clauses, result[child], b)
					if err != nil {
						return nil, err
					}
				}
			default:
				return nil, invalid("unknown state operation")
			}
			result[current.id] = clauses
			color[current.id] = 2
			stack = stack[:last]
		}
	}
	return result, nil
}

func automatonConjoinClauses(left, right automatonClauses, b Budget) (automatonClauses, error) {
	var result automatonClauses
	for _, a := range left {
		if err := step(b); err != nil {
			return nil, err
		}
		for _, z := range right {
			if err := step(b); err != nil {
				return nil, err
			}
			if len(a) > int(^uint(0)>>1)-len(z) {
				return nil, exhausted("clause size overflow")
			}
			if err := charge(b, uint64(len(a)+len(z)), 8); err != nil {
				return nil, err
			}
			joined := make([]StateID, 0, len(a)+len(z))
			joined = append(joined, a...)
			joined = append(joined, z...)
			clause, err := automatonCanonicalIDs(joined, b)
			if err != nil {
				return nil, err
			}
			if err := charge(b, 1, 48); err != nil {
				return nil, err
			}
			result = append(result, clause)
		}
	}
	return result, nil
}

func (c *automatonBuilder) clauses(ids []StateID) (automatonClauses, error) {
	if err := charge(c.b, 1, 32); err != nil {
		return nil, err
	}
	result := automatonClauses{nil}
	for _, id := range ids {
		if err := step(c.b); err != nil {
			return nil, err
		}
		var err error
		result, err = automatonConjoinClauses(result, c.roots[id], c.b)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func automatonNewWord(size int, b Budget) (*wordNFA, error) {
	if size < 1 {
		return nil, invalid("empty NFA state storage")
	}
	if err := charge(b, uint64(size), 32); err != nil {
		return nil, err
	}
	if err := charge(b, 1, 64); err != nil {
		return nil, err
	}
	return &wordNFA{accept: make([]bool, size), edges: make([][]wordEdge, size)}, nil
}

func automatonAddEdge(nfa *wordNFA, from int, edge wordEdge, b Budget) error {
	if err := step(b); err != nil {
		return err
	}
	if err := charge(b, 1, 48); err != nil {
		return err
	}
	nfa.edges[from] = append(nfa.edges[from], edge)
	return nil
}

func automatonCopyWord(out, source *wordNFA, offset int, b Budget) error {
	for state, edges := range source.edges {
		if err := step(b); err != nil {
			return err
		}
		out.accept[offset+state] = source.accept[state]
		for _, edge := range edges {
			if err := automatonAddEdge(out, offset+state, wordEdge{to: offset + edge.to, symbol: edge.symbol}, b); err != nil {
				return err
			}
		}
	}
	return nil
}

func automatonWordsSize(parts []*wordNFA, initial int, b Budget) (int, error) {
	total := initial
	for _, part := range parts {
		if err := step(b); err != nil {
			return 0, err
		}
		if len(part.edges) > int(^uint(0)>>1)-total {
			return 0, exhausted("NFA size overflow")
		}
		total += len(part.edges)
	}
	return total, nil
}

func automatonUnionWords(parts []*wordNFA, b Budget) (*wordNFA, error) {
	if len(parts) == 1 {
		return parts[0], nil
	}
	size, err := automatonWordsSize(parts, 1, b)
	if err != nil {
		return nil, err
	}
	out, err := automatonNewWord(size, b)
	if err != nil {
		return nil, err
	}
	offset := 1
	for _, part := range parts {
		if err := step(b); err != nil {
			return nil, err
		}
		if err := automatonCopyWord(out, part, offset, b); err != nil {
			return nil, err
		}
		if err := automatonAddEdge(out, 0, wordEdge{to: offset + part.start, symbol: -1}, b); err != nil {
			return nil, err
		}
		offset += len(part.edges)
	}
	return out, nil
}

func automatonConcatWords(parts []*wordNFA, b Budget) (*wordNFA, error) {
	if len(parts) == 0 {
		out, err := automatonNewWord(1, b)
		if err != nil {
			return nil, err
		}
		out.accept[0] = true
		return out, nil
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	size, err := automatonWordsSize(parts, 0, b)
	if err != nil {
		return nil, err
	}
	out, err := automatonNewWord(size, b)
	if err != nil {
		return nil, err
	}
	out.start = parts[0].start
	offset := 0
	for i, part := range parts {
		if err := step(b); err != nil {
			return nil, err
		}
		if err := automatonCopyWord(out, part, offset, b); err != nil {
			return nil, err
		}
		if i+1 < len(parts) {
			nextStart := offset + len(part.edges) + parts[i+1].start
			for state, final := range part.accept {
				if err := step(b); err != nil {
					return nil, err
				}
				if final {
					out.accept[offset+state] = false
					if err := automatonAddEdge(out, offset+state, wordEdge{to: nextStart, symbol: -1}, b); err != nil {
						return nil, err
					}
				}
			}
		}
		offset += len(part.edges)
	}
	return out, nil
}

func automatonStarWord(part *wordNFA, b Budget) (*wordNFA, error) {
	if len(part.edges) == int(^uint(0)>>1) {
		return nil, exhausted("NFA size overflow")
	}
	out, err := automatonNewWord(len(part.edges)+1, b)
	if err != nil {
		return nil, err
	}
	out.accept[0] = true
	if err := automatonCopyWord(out, part, 1, b); err != nil {
		return nil, err
	}
	if err := automatonAddEdge(out, 0, wordEdge{to: part.start + 1, symbol: -1}, b); err != nil {
		return nil, err
	}
	for state, final := range part.accept {
		if err := step(b); err != nil {
			return nil, err
		}
		if final {
			out.accept[state+1] = false
			if err := automatonAddEdge(out, state+1, wordEdge{to: 0, symbol: -1}, b); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func (c *automatonBuilder) compileSequences() ([]*wordNFA, error) {
	return compileWords(c.k, func(id StateID) (int, error) {
		return c.joint([]StateID{id})
	}, c.b)
}

// compileWords compiles the sequence DAG independently of vertical
// determinization. Membership uses original StateIDs as symbols; the hedge
// compiler uses interned sets of simultaneous state obligations instead.
func compileWords(k *Kind, symbol func(StateID) (int, error), b Budget) ([]*wordNFA, error) {
	if err := requireBudget(b); err != nil {
		return nil, err
	}
	if k == nil || symbol == nil {
		return nil, invalid("missing sequence compiler input")
	}
	sequences := k.definition.Sequences
	if err := charge(b, uint64(len(sequences)), 16); err != nil {
		return nil, err
	}
	result := make([]*wordNFA, len(sequences))
	color := make([]uint8, len(sequences))
	type frame struct {
		id   SeqID
		next int
	}
	for initial := range sequences {
		if err := step(b); err != nil {
			return nil, err
		}
		if color[initial] == 2 {
			continue
		}
		if err := charge(b, 1, 32); err != nil {
			return nil, err
		}
		stack := []frame{{id: SeqID(initial)}}
		color[initial] = 1
		for len(stack) != 0 {
			if err := step(b); err != nil {
				return nil, err
			}
			last := len(stack) - 1
			current := &stack[last]
			sequence := sequences[current.id]
			if current.next < len(sequence.Args) {
				child := sequence.Args[current.next]
				current.next++
				if color[child] == 1 {
					return nil, invalid("cyclic sequence expression")
				}
				if color[child] == 0 {
					if err := charge(b, 1, 32); err != nil {
						return nil, err
					}
					color[child] = 1
					stack = append(stack, frame{id: child})
				}
				continue
			}
			if err := charge(b, uint64(len(sequence.Args)), 16); err != nil {
				return nil, err
			}
			parts := make([]*wordNFA, len(sequence.Args))
			for i, child := range sequence.Args {
				if err := step(b); err != nil {
					return nil, err
				}
				parts[i] = result[child]
			}
			var word *wordNFA
			var err error
			switch sequence.Op {
			case Never, Epsilon:
				word, err = automatonNewWord(1, b)
				if err == nil {
					word.accept[0] = sequence.Op == Epsilon
				}
			case Item:
				var child int
				child, err = symbol(sequence.Item)
				if err == nil {
					word, err = automatonNewWord(2, b)
				}
				if err == nil {
					word.accept[1] = true
					err = automatonAddEdge(word, 0, wordEdge{to: 1, symbol: child}, b)
				}
			case Concat:
				word, err = automatonConcatWords(parts, b)
			case Choice:
				word, err = automatonUnionWords(parts, b)
			case Star:
				word, err = automatonStarWord(parts[0], b)
			default:
				return nil, invalid("unknown sequence operation")
			}
			if err != nil {
				return nil, err
			}
			result[current.id] = word
			color[current.id] = 2
			stack = stack[:last]
		}
	}
	return result, nil
}

func compileWord(k *Kind, root SeqID, symbol func(StateID) (int, error), b Budget) (*wordNFA, error) {
	if k == nil || uint64(root) >= uint64(len(k.definition.Sequences)) {
		return nil, invalid("invalid sequence root")
	}
	words, err := compileWords(k, symbol, b)
	if err != nil {
		return nil, err
	}
	return words[root], nil
}

func (c *automatonBuilder) width(root label) (*wordNFA, error) {
	if root.maxChildren < -1 || (root.maxChildren >= 0 && uint64(root.maxChildren) < uint64(root.minChildren)) {
		return nil, invalid("invalid root width")
	}
	last := uint64(root.minChildren)
	if root.maxChildren >= 0 {
		last = uint64(root.maxChildren)
	}
	if last >= uint64(^uint(0)>>1) {
		return nil, exhausted("root width exceeds address space")
	}
	word, err := automatonNewWord(int(last)+1, c.b)
	if err != nil {
		return nil, err
	}
	top, err := c.joint(nil)
	if err != nil {
		return nil, err
	}
	for state := range word.edges {
		if err := step(c.b); err != nil {
			return nil, err
		}
		word.accept[state] = uint64(state) >= uint64(root.minChildren)
		if state+1 < len(word.edges) {
			if err := automatonAddEdge(word, state, wordEdge{to: state + 1, symbol: top}, c.b); err != nil {
				return nil, err
			}
		} else if root.maxChildren < 0 {
			if err := automatonAddEdge(word, state, wordEdge{to: state, symbol: top}, c.b); err != nil {
				return nil, err
			}
		}
	}
	return word, nil
}

// intersectWords advances epsilon edges independently and consuming edges in
// lockstep. Synchronized symbols describe one child, so their obligations are
// interned together, rather than interpreted as independent child choices.
func (c *automatonBuilder) intersectWords(parts []*wordNFA) (*wordNFA, error) {
	if len(parts) == 1 {
		return parts[0], nil
	}
	if len(parts) == 0 {
		return nil, invalid("missing horizontal intersection operand")
	}
	if err := charge(c.b, 1, 128); err != nil {
		return nil, err
	}
	out := &wordNFA{}
	vectors := make([][]int, 0)
	known := make(map[string]int)
	add := func(vector []int) (int, error) {
		key, err := keyGroups(c.b, vector)
		if err != nil {
			return 0, err
		}
		if id, found := known[key]; found {
			return id, nil
		}
		if err := charge(c.b, 1, 192); err != nil {
			return 0, err
		}
		if err := charge(c.b, uint64(len(vector)), 16); err != nil {
			return 0, err
		}
		stored := make([]int, len(vector))
		final := true
		for i, state := range vector {
			if err := step(c.b); err != nil {
				return 0, err
			}
			stored[i] = state
			final = final && parts[i].accept[state]
		}
		id := len(vectors)
		vectors = append(vectors, stored)
		out.accept = append(out.accept, final)
		out.edges = append(out.edges, nil)
		known[key] = id
		return id, nil
	}
	if err := charge(c.b, uint64(len(parts)), 16); err != nil {
		return nil, err
	}
	initial := make([]int, len(parts))
	for i, part := range parts {
		if err := step(c.b); err != nil {
			return nil, err
		}
		initial[i] = part.start
	}
	var err error
	out.start, err = add(initial)
	if err != nil {
		return nil, err
	}
	for state := 0; state < len(vectors); state++ {
		if err := step(c.b); err != nil {
			return nil, err
		}
		vector := vectors[state]
		if err := charge(c.b, uint64(len(parts)), 64); err != nil {
			return nil, err
		}
		consuming := make([][]wordEdge, len(parts))
		canConsume := true
		for component, part := range parts {
			if err := step(c.b); err != nil {
				return nil, err
			}
			for _, edge := range part.edges[vector[component]] {
				if err := step(c.b); err != nil {
					return nil, err
				}
				if edge.symbol == -1 {
					if err := charge(c.b, uint64(len(vector)), 16); err != nil {
						return nil, err
					}
					next := make([]int, len(vector))
					copy(next, vector)
					next[component] = edge.to
					to, err := add(next)
					if err != nil {
						return nil, err
					}
					if err := automatonAddEdge(out, state, wordEdge{to: to, symbol: -1}, c.b); err != nil {
						return nil, err
					}
				} else {
					if err := charge(c.b, 1, 48); err != nil {
						return nil, err
					}
					consuming[component] = append(consuming[component], edge)
				}
			}
			if len(consuming[component]) == 0 {
				canConsume = false
			}
		}
		if !canConsume {
			continue
		}
		if err := charge(c.b, uint64(len(parts)), 16); err != nil {
			return nil, err
		}
		positions := make([]int, len(parts))
		more := true
		for more {
			if err := step(c.b); err != nil {
				return nil, err
			}
			if err := charge(c.b, uint64(len(parts)), 16); err != nil {
				return nil, err
			}
			next := make([]int, len(parts))
			var obligations []StateID
			for component, at := range positions {
				if err := step(c.b); err != nil {
					return nil, err
				}
				edge := consuming[component][at]
				next[component] = edge.to
				child := c.out.states[edge.symbol].obligations
				if err := charge(c.b, uint64(len(child)), 8); err != nil {
					return nil, err
				}
				obligations = append(obligations, child...)
			}
			child, err := c.joint(obligations)
			if err != nil {
				return nil, err
			}
			to, err := add(next)
			if err != nil {
				return nil, err
			}
			if err := automatonAddEdge(out, state, wordEdge{to: to, symbol: child}, c.b); err != nil {
				return nil, err
			}
			for component := len(positions) - 1; component >= 0; component-- {
				if err := step(c.b); err != nil {
					return nil, err
				}
				positions[component]++
				if positions[component] < len(consuming[component]) {
					break
				}
				positions[component] = 0
				if component == 0 {
					more = false
				}
			}
		}
	}
	return out, nil
}

// liveStates computes greatest-fixed-point productivity. Unlike finite-tree
// productivity, a cycle with no leaf alternative can remain live: it denotes
// an infinite vertical tree. Horizontal acceptance is still finite-word reachability.
func liveStates(a *automaton, b Budget) ([]bool, error) {
	if err := requireBudget(b); err != nil {
		return nil, err
	}
	if a == nil {
		return nil, invalid("nil shape automaton")
	}
	if err := charge(b, uint64(len(a.states)), 1); err != nil {
		return nil, err
	}
	live := make([]bool, len(a.states))
	for i := range live {
		if err := step(b); err != nil {
			return nil, err
		}
		live[i] = true
	}
	changed := true
	for changed {
		if err := step(b); err != nil {
			return nil, err
		}
		changed = false
		for state, shape := range a.states {
			if err := step(b); err != nil {
				return nil, err
			}
			if !live[state] {
				continue
			}
			productive := false
			for _, word := range shape.words {
				if err := step(b); err != nil {
					return nil, err
				}
				accepted, err := automatonLiveWord(word, live, b)
				if err != nil {
					return nil, err
				}
				if accepted {
					productive = true
					break
				}
			}
			if !productive {
				live[state] = false
				changed = true
			}
		}
	}
	return live, nil
}

func automatonLiveWord(word *wordNFA, live []bool, b Budget) (bool, error) {
	if word == nil {
		return false, nil
	}
	if err := charge(b, uint64(len(word.edges)), 16); err != nil {
		return false, err
	}
	seen := make([]bool, len(word.edges))
	queue := make([]int, 0, len(word.edges))
	seen[word.start] = true
	queue = append(queue, word.start)
	for at := 0; at < len(queue); at++ {
		if err := step(b); err != nil {
			return false, err
		}
		state := queue[at]
		if word.accept[state] {
			return true, nil
		}
		for _, edge := range word.edges[state] {
			if err := step(b); err != nil {
				return false, err
			}
			if edge.symbol != -1 && !live[edge.symbol] {
				continue
			}
			if !seen[edge.to] {
				seen[edge.to] = true
				queue = append(queue, edge.to)
			}
		}
	}
	return false, nil
}
