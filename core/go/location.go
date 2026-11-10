// Diagnostic locations (ADR 0009, docs/spec/horos.md#diagnostic-locations).

package horos

import "encoding/json"

// StepKind names how a path step selects a sub-value.
type StepKind string

const (
	// StepItem selects item Index of a bare tuple; a compound's first child is item 1.
	StepItem StepKind = "item"
	// StepElem selects element Index of a canonical list or set.
	StepElem StepKind = "elem"
	// StepKey selects the key of entry Index of a canonical map.
	StepKey StepKind = "key"
	// StepValue selects the value of entry Index of a canonical map.
	StepValue StepKind = "value"
	// StepField selects the value of the record field Name.
	StepField StepKind = "field"
)

// Step is one step of a value path. Index is 0-based in canonical order; Name is set
// only for StepField.
type Step struct {
	Kind  StepKind
	Index int
	Name  string
}

// Reason says why a location was reported.
type Reason string

const (
	// ReasonRefused: the descriptor at At does not accept the value at Path.
	ReasonRefused Reason = "refused"
	// ReasonMissingField: the record at At requires Field, which the value lacks.
	ReasonMissingField Reason = "missing-field"
	// ReasonExtraField: the closed record at At does not declare the value's Field.
	ReasonExtraField Reason = "extra-field"
	// ReasonNoAlternative: no alternative of the union at At descended into the value.
	ReasonNoAlternative Reason = "no-alternative"
	// ReasonComplement: the inner descriptor of the not at At accepted the value.
	ReasonComplement Reason = "complement"
	// ReasonLimit: the declared limit was reached entering the descriptor at At.
	ReasonLimit Reason = "limit"
	// ReasonNotDescriptor: the value at At is not a tuple headed by a horos constructor atom.
	ReasonNotDescriptor Reason = "not-descriptor"
	// ReasonUnknownConstructor: the constructor at At is not a descriptor constructor.
	ReasonUnknownConstructor Reason = "unknown-constructor"
	// ReasonArity: the descriptor at At has the wrong number of arguments.
	ReasonArity Reason = "arity"
	// ReasonArgument: item Arg of the descriptor at At has the wrong form.
	ReasonArgument Reason = "argument"
	// ReasonDuplicateField: the field form at item Arg repeats an earlier field name.
	ReasonDuplicateField Reason = "duplicate-field"
	// ReasonUnboundRef: the ref at At names no enclosing rec.
	ReasonUnboundRef Reason = "unbound-ref"
	// ReasonUnguardedRef: the ref at At reaches its rec without structural descent.
	ReasonUnguardedRef Reason = "unguarded-ref"
)

// Location names where a check failed: the path into the value, the position of the
// descriptor at fault as tuple-item indices from the root descriptor, and the reason.
// Field is meaningful for ReasonMissingField and ReasonExtraField; Arg is the item index
// of the faulty argument when it is positive.
type Location struct {
	Path   []Step
	At     []int
	Reason Reason
	Field  string
	Arg    int
}

// MarshalJSON renders the contract's JSON form of a step.
func (s Step) MarshalJSON() ([]byte, error) {
	if s.Kind == StepField {
		return json.Marshal(struct {
			Kind StepKind `json:"kind"`
			Name string   `json:"name"`
		}{s.Kind, s.Name})
	}
	return json.Marshal(struct {
		Kind  StepKind `json:"kind"`
		Index int      `json:"index"`
	}{s.Kind, s.Index})
}

// MarshalJSON renders the contract's JSON form of a location.
func (l Location) MarshalJSON() ([]byte, error) {
	out := struct {
		Path   []Step  `json:"path"`
		At     []int   `json:"at"`
		Reason Reason  `json:"reason"`
		Field  *string `json:"field,omitempty"`
		Arg    *int    `json:"arg,omitempty"`
	}{Path: l.Path, At: l.At, Reason: l.Reason}
	if out.Path == nil {
		out.Path = []Step{}
	}
	if out.At == nil {
		out.At = []int{}
	}
	if l.Reason == ReasonMissingField || l.Reason == ReasonExtraField {
		out.Field = &l.Field
	}
	if l.Arg > 0 {
		out.Arg = &l.Arg
	}
	return json.Marshal(out)
}

// MarshalJSON renders the contract's JSON form of a diagnosis.
func (d Diagnosis) MarshalJSON() ([]byte, error) {
	out := struct {
		Verdict  Verdict   `json:"verdict"`
		Limit    Limit     `json:"limit,omitempty"`
		Location *Location `json:"location,omitempty"`
	}{d.Verdict, d.Limit, d.Location}
	return json.Marshal(out)
}

// ----- lazily materialized trails -----

// pathNode is one step of the value path, linked to its parent; nil is the root.
type pathNode struct {
	parent *pathNode
	step   Step
	n      int
}

func (p *pathNode) len() int {
	if p == nil {
		return 0
	}
	return p.n
}

func (p *pathNode) push(s Step) *pathNode { return &pathNode{parent: p, step: s, n: p.len() + 1} }

func (p *pathNode) steps() []Step {
	out := make([]Step, p.len())
	for q := p; q != nil; q = q.parent {
		out[q.n-1] = q.step
	}
	return out
}

// posNode is one tuple-item index of a descriptor position; nil is the root descriptor.
type posNode struct {
	parent *posNode
	index  int
	n      int
}

func (p *posNode) len() int {
	if p == nil {
		return 0
	}
	return p.n
}

func (p *posNode) push(indices ...int) *posNode {
	for _, i := range indices {
		p = &posNode{parent: p, index: i, n: p.len() + 1}
	}
	return p
}

func (p *posNode) indices() []int {
	out := make([]int, p.len())
	for q := p; q != nil; q = q.parent {
		out[q.n-1] = q.index
	}
	return out
}

// fault is a location whose path and position are not yet materialized.
type fault struct {
	path   *pathNode
	at     *posNode
	reason Reason
	field  string
	arg    int
}

func (f *fault) location() *Location {
	return &Location{Path: f.path.steps(), At: f.at.indices(), Reason: f.reason, Field: f.field, Arg: f.arg}
}
