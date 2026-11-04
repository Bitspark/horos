package kindmodel

// Member decides membership of a closed finite observation graph, including
// its potentially infinite regular unfolding. It solves all positive Boolean
// obligations together at their greatest fixed point. An encountered backedge
// alone never establishes membership.
func Member(kind *Kind, source Observation, b Budget) (bool, error) {
	if err := requireBudget(b); err != nil {
		return false, err
	}
	if err := requireKind(kind); err != nil {
		return false, err
	}
	observation, err := copyObservation(kind, source, b)
	if err != nil {
		return false, err
	}
	words, err := compileWords(kind, func(state StateID) (int, error) {
		return int(state), nil
	}, b)
	if err != nil {
		return false, err
	}
	if err := charge(b, uint64(len(words)), 24); err != nil {
		return false, err
	}
	starts := make([][]int, len(words))
	for i, word := range words {
		initial, err := wordInitial(word, b)
		if err != nil {
			return false, err
		}
		starts[i] = initial
	}
	stateCount := len(kind.definition.States)
	maximumInt := int(^uint(0) >> 1)
	if len(observation.Nodes) > maximumInt/stateCount {
		return false, exhausted("membership obligation count overflow")
	}
	count := len(observation.Nodes) * stateCount
	if err := charge(b, uint64(count), 1); err != nil {
		return false, err
	}
	values := make([]bool, count)
	for i := range values {
		if err := step(b); err != nil {
			return false, err
		}
		values[i] = true
	}
	for {
		changed := false
		for nodeID, node := range observation.Nodes {
			for stateID, state := range kind.definition.States {
				if err := step(b); err != nil {
					return false, err
				}
				index := nodeID*stateCount + stateID
				if !values[index] {
					continue
				}
				accepted := false
				switch state.Op {
				case Top:
					accepted = true
				case Bottom:
				case Alias:
					accepted = values[nodeID*stateCount+int(state.Args[0])]
				case Or:
					for _, argument := range state.Args {
						if err := step(b); err != nil {
							return false, err
						}
						if values[nodeID*stateCount+int(argument)] {
							accepted = true
							break
						}
					}
				case And:
					accepted = true
					for _, argument := range state.Args {
						if err := step(b); err != nil {
							return false, err
						}
						if !values[nodeID*stateCount+int(argument)] {
							accepted = false
							break
						}
					}
				case Node:
					matches, err := observationMatches(node, state.Node, b)
					if err != nil {
						return false, err
					}
					if matches {
						accepted, err = memberSequence(words[state.Node.Sequence], starts[state.Node.Sequence], node.Children, values, stateCount, b)
						if err != nil {
							return false, err
						}
					}
				}
				if !accepted {
					values[index] = false
					changed = true
				}
			}
		}
		if !changed {
			return values[int(observation.Root)*stateCount+int(kind.definition.Root)], nil
		}
	}
}

func memberSequence(nfa *wordNFA, initial []int, children []ObsID, values []bool, stateCount int, b Budget) (bool, error) {
	current := initial
	for _, child := range children {
		if err := step(b); err != nil {
			return false, err
		}
		if len(current) == 0 {
			return false, nil
		}
		if err := charge(b, uint64(len(nfa.edges)), 16); err != nil {
			return false, err
		}
		selected := make([]bool, len(nfa.edges))
		for _, state := range current {
			if err := step(b); err != nil {
				return false, err
			}
			for _, edge := range nfa.edges[state] {
				if err := step(b); err != nil {
					return false, err
				}
				if edge.symbol >= 0 && values[int(child)*stateCount+edge.symbol] {
					selected[edge.to] = true
				}
			}
		}
		seeds := make([]int, 0)
		for state, present := range selected {
			if err := step(b); err != nil {
				return false, err
			}
			if present {
				seeds = append(seeds, state)
			}
		}
		var err error
		current, err = wordClosure(nfa, seeds, b)
		if err != nil {
			return false, err
		}
	}
	return wordAccepts(nfa, current, b)
}
