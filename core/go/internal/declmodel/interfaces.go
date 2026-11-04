package declmodel

import (
	"slices"

	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

// Claim describes a proper result. Data refines Type, not the ground any type.
type Claim string

const (
	TypeClaim Claim = "type"
	DataClaim Claim = "data"
)

// Interface is immutable. Constructor inputs are nonempty; every result is proper.
type Interface struct {
	inputs []Interface
	result Claim
}

func (i Interface) IsConstructor() bool { return len(i.inputs) != 0 }
func (i Interface) Result() Claim       { return i.result }
func (i Interface) Inputs() []Interface {
	result := make([]Interface, len(i.inputs))
	for n, child := range i.inputs {
		result[n] = cloneInterface(child)
	}
	return result
}
func (i Interface) Kind() Kind {
	result := Kind{inputs: make([]Kind, len(i.inputs))}
	for n, child := range i.inputs {
		result.inputs[n] = child.Kind()
	}
	return result
}
func (i Interface) Equal(other Interface) bool {
	if i.result != other.result || len(i.inputs) != len(other.inputs) {
		return false
	}
	for n, child := range i.inputs {
		if !child.Equal(other.inputs[n]) {
			return false
		}
	}
	return true
}

// Kind is an erased, finite, n-ary shape. The zero shape is the proper Type kind.
type Kind struct{ inputs []Kind }

func (k Kind) IsConstructor() bool { return len(k.inputs) != 0 }
func (k Kind) Inputs() []Kind {
	result := make([]Kind, len(k.inputs))
	for i, child := range k.inputs {
		result[i] = Kind{inputs: child.Inputs()}
	}
	return result
}
func (k Kind) Equal(other Kind) bool {
	if len(k.inputs) != len(other.inputs) {
		return false
	}
	for i, child := range k.inputs {
		if !child.Equal(other.inputs[i]) {
			return false
		}
	}
	return true
}

func cloneInterface(i Interface) Interface { return Interface{inputs: i.Inputs(), result: i.result} }

func ProperInterface(claim Claim) (Interface, error) {
	if claim != TypeClaim && claim != DataClaim {
		return Interface{}, &Error{Code: Invalid, Subject: SubjectRequest, Message: "unknown proper claim"}
	}
	return Interface{result: claim}, nil
}

func ConstructorInterface(inputs []Interface, result Claim) (Interface, error) {
	if len(inputs) == 0 {
		return Interface{}, &Error{Code: Invalid, Subject: SubjectRequest, Message: "constructor inputs must be nonempty"}
	}
	i := Interface{inputs: inputs, result: result}
	if err := validateInterface(i, &budget{limits: DefaultLimits()}, 0); err != nil {
		return Interface{}, argumentFailure(err, DeclarationID{})
	}
	return cloneInterface(i), nil
}

func validateInterface(i Interface, work *budget, depth int) error {
	if err := work.step(depth); err != nil {
		return err
	}
	if i.result != TypeClaim && i.result != DataClaim {
		return failure(Invalid, "unknown interface result claim")
	}
	for _, child := range i.inputs {
		if err := validateInterface(child, work, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func readClaim(value core.Value) (Claim, error) {
	atom, ok := value.(core.Atom)
	if !ok {
		return "", failure(Invalid, "result claim must be an atom")
	}
	claim := Claim(string(atom.Bytes()))
	if claim != TypeClaim && claim != DataClaim {
		return "", failure(Invalid, "unknown result claim")
	}
	return claim, nil
}

func readInterface(value core.Value, grammar Grammar) (Interface, error) {
	if args, ok := grammar.form(value, "proper", 1); ok {
		claim, err := readClaim(args[0])
		return Interface{result: claim}, err
	}
	args, ok := grammar.form(value, "constructor", 2)
	if !ok {
		return Interface{}, failure(Invalid, "expected proper or constructor interface")
	}
	values, err := data.ReadList(args[0])
	if err != nil || len(values) == 0 {
		return Interface{}, failure(Invalid, "constructor inputs must be a nonempty canonical list")
	}
	claim, err := readClaim(args[1])
	if err != nil {
		return Interface{}, err
	}
	i := Interface{inputs: make([]Interface, len(values)), result: claim}
	for n, value := range values {
		child, err := readInterface(value, grammar)
		if err != nil {
			return Interface{}, err
		}
		i.inputs[n] = child
	}
	return i, nil
}

// Interface comparison is finite constructor substitutability, not descriptor subtyping.
func compatible(actual, expected Interface, work *budget, depth int) (bool, error) {
	if err := work.step(depth); err != nil {
		return false, err
	}
	if len(actual.inputs) != len(expected.inputs) {
		return false, nil
	}
	if expected.result == DataClaim && actual.result != DataClaim {
		return false, nil
	}
	for i := range actual.inputs {
		ok, err := compatible(expected.inputs[i], actual.inputs[i], work, depth+1)
		if err != nil || !ok {
			return false, err
		}
	}
	return true, nil
}

type SummaryKind string

const (
	SummaryTrue  SummaryKind = "true"
	SummaryFalse SummaryKind = "false"
	SummaryNeed  SummaryKind = "need"
	SummaryAnd   SummaryKind = "and"
	SummaryProbe SummaryKind = "probe"
)

type DataStatus string

const (
	DataProven      DataStatus = "proven"
	DataFailed      DataStatus = "failed"
	DataConditional DataStatus = "conditional"
)

// DataSummary is immutable symbolic evidence. Probe exposes checked syntax only;
// traversing the probe's own metadata again is not a summary-containment edge.
type DataSummary struct {
	kind      SummaryKind
	parameter ParameterID
	children  []*DataSummary
	probe     *Expression
}

func (s *DataSummary) Kind() SummaryKind              { return s.kind }
func (s *DataSummary) Parameter() (ParameterID, bool) { return s.parameter, s.kind == SummaryNeed }
func (s *DataSummary) Children() []*DataSummary {
	result := slices.Clone(s.children)
	for i, child := range result {
		result[i] = copySummary(child)
	}
	return result
}
func (s *DataSummary) Probe() *Expression { return copyExpression(s.probe) }
func copySummary(s *DataSummary) *DataSummary {
	if s == nil {
		return nil
	}
	copy := *s
	return &copy
}
