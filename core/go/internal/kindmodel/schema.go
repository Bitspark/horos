package kindmodel

func sameSchema(left, right *Kind, b Budget) (bool, error) {
	if len(left.schema.Forms) != len(right.schema.Forms) {
		return false, nil
	}
	for _, form := range left.schema.Forms {
		if err := step(b); err != nil {
			return false, err
		}
		index, found := right.forms[form.Name]
		if !found {
			return false, nil
		}
		other := right.schema.Forms[index]
		if form.MinChildren != other.MinChildren || form.MaxChildren != other.MaxChildren || len(form.Selectors) != len(other.Selectors) {
			return false, nil
		}
		for _, domain := range form.Selectors {
			matched := false
			for _, candidate := range other.Selectors {
				if err := step(b); err != nil {
					return false, err
				}
				if domain.Key != candidate.Key {
					continue
				}
				matched = true
				if domain.Open != candidate.Open || domain.Optional != candidate.Optional || len(domain.Values) != len(candidate.Values) {
					return false, nil
				}
				for _, value := range domain.Values {
					present := false
					for _, otherValue := range candidate.Values {
						if err := step(b); err != nil {
							return false, err
						}
						present = present || value == otherValue
					}
					if !present {
						return false, nil
					}
				}
				break
			}
			if !matched {
				return false, nil
			}
		}
	}
	return true, nil
}

// labelsFor constructs the exact finite quotient observable by these kinds.
// Closed-domain values that no pattern distinguishes share one Other class.
// Open domains additionally retain an Other class for unmentioned values.
func labelsFor(kinds []*Kind, b Budget) ([]label, error) {
	if len(kinds) == 0 || kinds[0] == nil {
		return nil, invalid("missing schema for selector partition")
	}
	var result []label
	for _, form := range kinds[0].schema.Forms {
		if err := step(b); err != nil {
			return nil, err
		}
		if err := charge(b, 1, 96); err != nil {
			return nil, err
		}
		partial := []label{{form: form.Name, minChildren: form.MinChildren, maxChildren: form.MaxChildren}}
		for _, domain := range form.Selectors {
			if err := charge(b, 1, 64); err != nil {
				return nil, err
			}
			mentioned := make(map[string]bool)
			var values []string
			for _, kind := range kinds {
				for _, state := range kind.definition.States {
					if err := step(b); err != nil {
						return nil, err
					}
					if state.Op != Node || state.Node.Form != form.Name {
						continue
					}
					for _, selected := range state.Node.Selectors {
						if err := step(b); err != nil {
							return nil, err
						}
						if selected.Key == domain.Key && !mentioned[selected.Value] {
							if err := charge(b, 1, 96); err != nil {
								return nil, err
							}
							mentioned[selected.Value] = true
							values = append(values, selected.Value)
						}
					}
				}
			}
			// Preserve declaration encounter order. It is deterministic and avoids
			// sorting predicates or exposing map iteration as semantic order.
			if err := charge(b, uint64(len(values))+2, 32); err != nil {
				return nil, err
			}
			options := make([]labelValue, 0, len(values)+2)
			for _, value := range values {
				options = append(options, labelValue{value: value, present: true})
			}
			other := domain.Open
			for _, value := range domain.Values {
				if err := step(b); err != nil {
					return nil, err
				}
				other = other || !mentioned[value]
			}
			if other {
				options = append(options, labelValue{present: true, other: true})
			}
			if domain.Optional {
				options = append(options, labelValue{})
			}
			var next []label
			for _, previous := range partial {
				for _, option := range options {
					if err := step(b); err != nil {
						return nil, err
					}
					if err := charge(b, uint64(len(previous.values))+2, 96); err != nil {
						return nil, err
					}
					copyLabel := previous
					copyLabel.values = make(map[string]labelValue, len(previous.values)+1)
					for key, value := range previous.values {
						if err := step(b); err != nil {
							return nil, err
						}
						copyLabel.values[key] = value
					}
					copyLabel.values[domain.Key] = option
					next = append(next, copyLabel)
				}
			}
			partial = next
		}
		if err := charge(b, uint64(len(partial)), 64); err != nil {
			return nil, err
		}
		result = append(result, partial...)
	}
	return result, nil
}

func copyObservation(k *Kind, source Observation, b Budget) (Observation, error) {
	if len(source.Nodes) == 0 || uint64(source.Root) >= uint64(len(source.Nodes)) {
		return Observation{}, invalid("missing or out-of-range observation root")
	}
	if uint64(len(source.Nodes)) > uint64(^ObsID(0)) {
		return Observation{}, invalid("observation exceeds its identifier space")
	}
	if err := charge(b, uint64(len(source.Nodes)), 72); err != nil {
		return Observation{}, err
	}
	result := Observation{Root: source.Root, Nodes: make([]ObservationNode, len(source.Nodes))}
	for i, node := range source.Nodes {
		if err := step(b); err != nil {
			return Observation{}, err
		}
		index, found := k.forms[node.Form]
		if !found {
			return Observation{}, invalid("unknown observation form %q", node.Form)
		}
		form := k.schema.Forms[index]
		width := uint64(len(node.Children))
		if width < uint64(form.MinChildren) || (form.MaxChildren >= 0 && width > uint64(form.MaxChildren)) {
			return Observation{}, invalid("invalid observation width for form %q", node.Form)
		}
		if err := admitSelectors(form, node.Selectors, true, b); err != nil {
			return Observation{}, err
		}
		for _, child := range node.Children {
			if err := step(b); err != nil {
				return Observation{}, err
			}
			if uint64(child) >= uint64(len(source.Nodes)) {
				return Observation{}, invalid("out-of-range observation child at node %d", i)
			}
		}
		if err := charge(b, uint64(len(node.Selectors)), 32); err != nil {
			return Observation{}, err
		}
		if err := charge(b, uint64(len(node.Children)), 4); err != nil {
			return Observation{}, err
		}
		result.Nodes[i] = node
		result.Nodes[i].Selectors = append([]Selector(nil), node.Selectors...)
		result.Nodes[i].Children = append([]ObsID(nil), node.Children...)
	}
	return result, nil
}

func observationMatches(node ObservationNode, pattern NodePattern, b Budget) (bool, error) {
	if node.Form != pattern.Form {
		return false, nil
	}
	for _, selected := range pattern.Selectors {
		found := false
		for _, actual := range node.Selectors {
			if err := step(b); err != nil {
				return false, err
			}
			if actual.Key == selected.Key && actual.Value == selected.Value {
				found = true
				break
			}
		}
		if !found {
			return false, nil
		}
	}
	return true, nil
}
