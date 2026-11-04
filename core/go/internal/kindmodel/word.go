package kindmodel

func wordInitial(nfa *wordNFA, b Budget) ([]int, error) {
	if nfa == nil {
		return nil, nil
	}
	return wordClosure(nfa, []int{nfa.start}, b)
}

func wordClosure(nfa *wordNFA, seeds []int, b Budget) ([]int, error) {
	if nfa == nil || len(seeds) == 0 {
		return nil, nil
	}
	if err := charge(b, uint64(len(nfa.edges)), 24); err != nil {
		return nil, err
	}
	seen := make([]bool, len(nfa.edges))
	queue := make([]int, 0, len(nfa.edges))
	for _, seed := range seeds {
		if err := step(b); err != nil {
			return nil, err
		}
		if !seen[seed] {
			seen[seed] = true
			queue = append(queue, seed)
		}
	}
	for at := 0; at < len(queue); at++ {
		if err := step(b); err != nil {
			return nil, err
		}
		for _, edge := range nfa.edges[queue[at]] {
			if err := step(b); err != nil {
				return nil, err
			}
			if edge.symbol == -1 && !seen[edge.to] {
				seen[edge.to] = true
				queue = append(queue, edge.to)
			}
		}
	}
	result := make([]int, 0, len(queue))
	for state, present := range seen {
		if err := step(b); err != nil {
			return nil, err
		}
		if present {
			result = append(result, state)
		}
	}
	return result, nil
}

// wordAdvance consumes exactly one child, then takes epsilon closure. Symbols
// describe alternative possible states for that same child, not extra children.
func wordAdvance(nfa *wordNFA, from, symbols []int, b Budget) ([]int, error) {
	if nfa == nil || len(from) == 0 || len(symbols) == 0 {
		return nil, nil
	}
	if err := charge(b, uint64(len(symbols)), 64); err != nil {
		return nil, err
	}
	allowed := make(map[int]bool, len(symbols))
	for _, symbol := range symbols {
		if err := step(b); err != nil {
			return nil, err
		}
		allowed[symbol] = true
	}
	if err := charge(b, uint64(len(nfa.edges)), 16); err != nil {
		return nil, err
	}
	seen := make([]bool, len(nfa.edges))
	for _, state := range from {
		if err := step(b); err != nil {
			return nil, err
		}
		for _, edge := range nfa.edges[state] {
			if err := step(b); err != nil {
				return nil, err
			}
			if edge.symbol >= 0 && allowed[edge.symbol] {
				seen[edge.to] = true
			}
		}
	}
	seeds := make([]int, 0)
	for state, present := range seen {
		if err := step(b); err != nil {
			return nil, err
		}
		if present {
			seeds = append(seeds, state)
		}
	}
	return wordClosure(nfa, seeds, b)
}

func wordAccepts(nfa *wordNFA, states []int, b Budget) (bool, error) {
	if nfa == nil {
		return false, nil
	}
	for _, state := range states {
		if err := step(b); err != nil {
			return false, err
		}
		if nfa.accept[state] {
			return true, nil
		}
	}
	return false, nil
}
