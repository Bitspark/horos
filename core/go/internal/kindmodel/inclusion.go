package kindmodel

// Empty decides emptiness in the free shape universe described by the schema.
// A nonempty free shape need not be realizable by an admitted Horos declaration.
func Empty(kind *Kind, b Budget) (bool, error) {
	if err := requireBudget(b); err != nil {
		return false, err
	}
	if err := requireKind(kind); err != nil {
		return false, err
	}
	labels, err := labelsFor([]*Kind{kind}, b)
	if err != nil {
		return false, err
	}
	automaton, err := buildAutomaton(kind, labels, b)
	if err != nil {
		return false, err
	}
	live, err := liveStates(automaton, b)
	if err != nil {
		return false, err
	}
	return !live[automaton.root], nil
}

// Includes decides left <= right in the common free shape universe. A false
// answer is abstract non-inclusion, not actual Horos nonmembership. The caller
// must not publish it as an admitted descriptor counterexample. Any budget
// error leaves the query unanswered; no partial approximation is returned.
func Includes(left, right *Kind, b Budget) (bool, error) {
	if err := requireBudget(b); err != nil {
		return false, err
	}
	if err := requireKind(left); err != nil {
		return false, err
	}
	if err := requireKind(right); err != nil {
		return false, err
	}
	if err := step(b); err != nil {
		return false, err
	}
	if left == right {
		return true, nil
	}
	same, err := sameSchema(left, right, b)
	if err != nil {
		return false, err
	}
	if !same {
		return false, invalid("inclusion requires the same shape schema")
	}
	labels, err := labelsFor([]*Kind{left, right}, b)
	if err != nil {
		return false, err
	}
	a, err := buildAutomaton(left, labels, b)
	if err != nil {
		return false, err
	}
	alive, err := liveStates(a, b)
	if err != nil {
		return false, err
	}
	if !alive[a.root] {
		return true, nil
	}
	z, err := buildAutomaton(right, labels, b)
	if err != nil {
		return false, err
	}
	zlive, err := liveStates(z, b)
	if err != nil {
		return false, err
	}
	if !zlive[z.root] {
		return false, nil
	}
	return finiteCutInclusion(a, z, alive, zlive, len(labels), b)
}

type prefixSummary struct {
	left  int
	right []int
}

// Every summary describes one finite cut: one possible left state and the
// exact set of right states accepting that same cut. Cuts only occur at live
// vertical states. Intersections have already become simultaneous obligations
// in buildAutomaton, before any cut is introduced.
func finiteCutInclusion(left, right *automaton, leftLive, rightLive []bool, labelCount int, b Budget) (bool, error) {
	if err := charge(b, uint64(len(right.states)), 8); err != nil {
		return false, err
	}
	cutRight := make([]int, 0)
	for state, live := range rightLive {
		if err := step(b); err != nil {
			return false, err
		}
		if live {
			cutRight = append(cutRight, state)
		}
	}
	if err := charge(b, 1, 64); err != nil {
		return false, err
	}
	seen := make(map[string]bool)
	var summaries []prefixSummary
	add := func(state int, set []int) (bool, bool, error) {
		if err := step(b); err != nil {
			return false, false, err
		}
		if !leftLive[state] {
			return false, false, nil
		}
		if state == left.root {
			accepted := false
			for _, candidate := range set {
				if err := step(b); err != nil {
					return false, false, err
				}
				accepted = accepted || candidate == right.root
			}
			if !accepted {
				return false, true, nil
			}
		}
		key, err := keyGroups(b, []int{state}, set)
		if err != nil {
			return false, false, err
		}
		if seen[key] {
			return false, false, nil
		}
		if err := charge(b, 1, 128); err != nil {
			return false, false, err
		}
		seen[key] = true
		// Sets are operation-local immutable slices and can be shared by
		// several left alternatives of the same finite cut.
		summaries = append(summaries, prefixSummary{left: state, right: set})
		return true, false, nil
	}
	for state, live := range leftLive {
		if err := step(b); err != nil {
			return false, err
		}
		if live {
			_, counterexample, err := add(state, cutRight)
			if err != nil {
				return false, err
			}
			if counterexample {
				return false, nil
			}
		}
	}
	for {
		if err := step(b); err != nil {
			return false, err
		}
		before := len(summaries)
		// One round uses a fixed alphabet. New parent summaries become
		// available as child letters in the following round.
		alphabet := summaries[:before]
		for labelIndex := 0; labelIndex < labelCount; labelIndex++ {
			if err := step(b); err != nil {
				return false, err
			}
			counterexample, err := exploreFiniteWords(left, right, leftLive, rightLive, labelIndex, alphabet, add, b)
			if err != nil {
				return false, err
			}
			if counterexample {
				return false, nil
			}
		}
		if len(summaries) == before {
			return true, nil
		}
	}
}

// A profile contains the current epsilon-closed NFA states for every possible
// parent state. It determinizes horizontal words only. It never constructs an
// infinite sibling word or treats a horizontal cycle as an accepted word.
func exploreFiniteWords(left, right *automaton, leftLive, rightLive []bool, labelIndex int, alphabet []prefixSummary, add func(int, []int) (bool, bool, error), b Budget) (bool, error) {
	width := len(left.states) + len(right.states)
	if width < len(left.states) {
		return false, exhausted("horizontal profile size overflow")
	}
	if err := charge(b, uint64(width), 40); err != nil {
		return false, err
	}
	nfas := make([]*wordNFA, width)
	initial := make([][]int, width)
	for i, state := range left.states {
		if err := step(b); err != nil {
			return false, err
		}
		if leftLive[i] {
			nfas[i] = state.words[labelIndex]
		}
	}
	for i, state := range right.states {
		if err := step(b); err != nil {
			return false, err
		}
		if rightLive[i] {
			nfas[len(left.states)+i] = state.words[labelIndex]
		}
	}
	leftPossible := false
	for i, nfa := range nfas {
		states, err := wordInitial(nfa, b)
		if err != nil {
			return false, err
		}
		initial[i] = states
		if i < len(left.states) && len(states) != 0 {
			leftPossible = true
		}
	}
	if !leftPossible {
		return false, nil
	}
	initialKey, err := keyGroups(b, initial...)
	if err != nil {
		return false, err
	}
	if err := charge(b, 1, 160); err != nil {
		return false, err
	}
	seen := map[string]bool{initialKey: true}
	queue := [][][]int{initial}
	for position := 0; position < len(queue); position++ {
		if err := step(b); err != nil {
			return false, err
		}
		profile := queue[position]
		if err := charge(b, uint64(len(right.states)), 8); err != nil {
			return false, err
		}
		parentRight := make([]int, 0)
		for i := range right.states {
			accepted, err := wordAccepts(nfas[len(left.states)+i], profile[len(left.states)+i], b)
			if err != nil {
				return false, err
			}
			if accepted {
				parentRight = append(parentRight, i)
			}
		}
		for i := range left.states {
			accepted, err := wordAccepts(nfas[i], profile[i], b)
			if err != nil {
				return false, err
			}
			if accepted {
				_, counterexample, err := add(i, parentRight)
				if err != nil {
					return false, err
				}
				if counterexample {
					return true, nil
				}
			}
		}
		for _, letter := range alphabet {
			if err := step(b); err != nil {
				return false, err
			}
			if err := charge(b, uint64(width), 24); err != nil {
				return false, err
			}
			next := make([][]int, width)
			possible := false
			for i := range left.states {
				states, err := wordAdvance(nfas[i], profile[i], []int{letter.left}, b)
				if err != nil {
					return false, err
				}
				next[i] = states
				possible = possible || len(states) != 0
			}
			if !possible {
				continue
			}
			for i := range right.states {
				states, err := wordAdvance(nfas[len(left.states)+i], profile[len(left.states)+i], letter.right, b)
				if err != nil {
					return false, err
				}
				next[len(left.states)+i] = states
			}
			key, err := keyGroups(b, next...)
			if err != nil {
				return false, err
			}
			if !seen[key] {
				if err := charge(b, 1, 128); err != nil {
					return false, err
				}
				seen[key] = true
				queue = append(queue, next)
			}
		}
	}
	return false, nil
}
