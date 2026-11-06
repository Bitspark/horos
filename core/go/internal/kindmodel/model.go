// Package kindmodel implements positive structural kinds over finite-branching
// observation trees. Recursive equations have greatest-fixed-point semantics.
// The package has no knowledge of declaration names or privileged kind names.
package kindmodel

import (
	"errors"
	"fmt"
	"strconv"
)

// Budget belongs to the enclosing operation. The engine never resets it.
// Step receives semantic nesting depth where there is one, and zero for an
// iterative graph operation. Bytes charges retained and temporary allocation.
type Budget interface {
	Step(depth uint64) error
	Bytes(n uint64) error
}

type StateID uint32
type SeqID uint32
type ObsID uint32

type Op uint8

const (
	Top Op = iota
	Bottom
	Or
	And
	Alias
	Node
)

type SequenceOp uint8

const (
	Never SequenceOp = iota
	Epsilon
	Item
	Concat
	Choice
	Star
)

type Selector struct {
	Key   string
	Value string
}

// SelectorDomain is an approved selector vocabulary. Open permits additional
// equality values; inclusion partitions those into mentioned values and Other.
type SelectorDomain struct {
	Key      string
	Values   []string
	Open     bool
	Optional bool
}

type Form struct {
	Name        string
	Selectors   []SelectorDomain
	MinChildren uint32
	MaxChildren int64 // -1 permits every finite width at least MinChildren.
}

type Schema struct {
	Forms []Form
}

type NodePattern struct {
	Form      string
	Selectors []Selector
	Sequence  SeqID
}

type State struct {
	Op   Op
	Args []StateID
	Node NodePattern
}

type Sequence struct {
	Op   SequenceOp
	Item StateID
	Args []SeqID
}

// Definition is a positive simultaneous equation system. Every cycle must
// descend through a Node's observed child before returning to a state.
// Sequence expressions themselves form a DAG; Star means finite repetition.
type Definition struct {
	Root      StateID
	States    []State
	Sequences []Sequence
}

type ObservationNode struct {
	Form      string
	Selectors []Selector
	Children  []ObsID
}

// Observation is finite, but Children may contain backedges. Its unfolding can
// be infinite vertically. Every individual child sequence remains finite.
type Observation struct {
	Root  ObsID
	Nodes []ObservationNode
}

// Kind is immutable. Compile copies its input; all query caches are local.
type Kind struct {
	valid      bool
	schema     Schema
	definition Definition
	forms      map[string]int
}

var ErrInvalid = errors.New("invalid structural kind model")

// ErrExhausted reports an engine representation limit. Budget callback errors
// are propagated unchanged so callers retain their aggregate-limit detail.
var ErrExhausted = errors.New("structural kind resource limit exceeded")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

func exhausted(message string) error {
	return fmt.Errorf("%w: %s", ErrExhausted, message)
}

func requireBudget(b Budget) error {
	if b == nil {
		return invalid("an aggregate operation budget is required")
	}
	return nil
}

func requireKind(kind *Kind) error {
	if kind == nil || !kind.valid {
		return invalid("kind was not produced by Compile")
	}
	return nil
}

func step(b Budget) error {
	return b.Step(0)
}

func charge(b Budget, count, size uint64) error {
	if size != 0 && count > ^uint64(0)/size {
		return exhausted("allocation accounting overflow")
	}
	return b.Bytes(count * size)
}

func keyGroups(b Budget, groups ...[]int) (string, error) {
	var count uint64 = uint64(len(groups))
	for _, group := range groups {
		if err := step(b); err != nil {
			return "", err
		}
		if uint64(len(group)) > ^uint64(0)-count {
			return "", exhausted("state key size overflow")
		}
		count += uint64(len(group))
	}
	if err := charge(b, count, 64); err != nil {
		return "", err
	}
	if count > uint64(^uint(0)>>1)/24 {
		return "", exhausted("state key exceeds address space")
	}
	key := make([]byte, 0, int(count*24))
	for _, group := range groups {
		if err := step(b); err != nil {
			return "", err
		}
		key = append(key, '[')
		for _, value := range group {
			if err := step(b); err != nil {
				return "", err
			}
			key = strconv.AppendInt(key, int64(value), 10)
			key = append(key, ',')
		}
		key = append(key, ']')
	}
	return string(key), nil
}

// label is a finite equivalence class of schema-approved root metadata.
// Other and absent are separate from every concrete string value.
type labelValue struct {
	value   string
	other   bool
	present bool
}

type label struct {
	form        string
	values      map[string]labelValue
	minChildren uint32
	maxChildren int64
}

func (l label) matches(pattern NodePattern) bool {
	if l.form != pattern.Form {
		return false
	}
	for _, selected := range pattern.Selectors {
		value := l.values[selected.Key]
		if !value.present || value.other || value.value != selected.Value {
			return false
		}
	}
	return true
}

// automaton is a nondeterministic top-down hedge automaton. Infinite vertical
// runs are all accepting; its horizontal NFAs accept finite words only.
type automaton struct {
	root   int
	states []automatonState
}

type automatonState struct {
	obligations []StateID
	words       []*wordNFA // indexed by label, nil means no transition.
}

type wordNFA struct {
	start  int
	accept []bool
	edges  [][]wordEdge
}

type wordEdge struct {
	to     int
	symbol int // -1 is epsilon; otherwise an automaton state.
}
