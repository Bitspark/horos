// Package declarations implements Horos declaration revision 4 with general
// kinds. Retired revisions and profiles are not accepted.
package declarations

import (
	model "github.com/Bitspark/horos/core/go/internal/declmodel"
	core "github.com/bitspark/ontos/core/go"
)

// Public facts share the private implementation. Program exposes only the
// unified grammar, never a configurable or compatibility profile.
type Digest = model.Digest
type DeclarationID = model.DeclarationID
type ParameterID = model.ParameterID
type TypeParameter = model.TypeParameter
type KindParameter = model.KindParameter
type Assumption = model.Assumption
type Membership = model.Membership
type Code = model.Code
type Subject = model.Subject
type Error = model.Error
type Limits = model.Limits
type Sort = model.Sort
type Kind = model.Kind
type KindForm = model.KindForm
type KindOperand = model.KindOperand
type Selector = model.Selector
type Sequence = model.Sequence
type SequenceForm = model.SequenceForm
type ExpressionKind = model.ExpressionKind
type Operand = model.Operand
type Expression = model.Expression
type DeclarationForm = model.DeclarationForm
type Declaration = model.Declaration
type Import = model.Import
type Module = model.Module
type KindHandle = model.KindHandle
type TypeValue = model.TypeValue
type Verdict = model.Verdict

const (
	Invalid                = model.Invalid
	BoundViolation         = model.BoundViolation
	KindMismatch           = model.KindMismatch
	UnprovedObligation     = model.UnprovedObligation
	MissingDependency      = model.MissingDependency
	ImportMismatch         = model.ImportMismatch
	UnknownImport          = model.UnknownImport
	UnknownDeclaration     = model.UnknownDeclaration
	InvalidOptions         = model.InvalidOptions
	Cycle                  = model.Cycle
	Exhausted              = model.Exhausted
	SubjectSource          = model.SubjectSource
	SubjectContext         = model.SubjectContext
	SubjectConfiguration   = model.SubjectConfiguration
	SubjectRequest         = model.SubjectRequest
	Descriptor             = model.Descriptor
	Parameter              = model.Parameter
	Reference              = model.Reference
	Application            = model.Application
	TypeDefinition         = model.TypeDefinition
	KindDefinition         = model.KindDefinition
	KindTop                = model.KindTop
	KindBottom             = model.KindBottom
	KindParameterReference = model.KindParameterReference
	KindReference          = model.KindReference
	KindApplication        = model.KindApplication
	KindUnion              = model.KindUnion
	KindIntersection       = model.KindIntersection
	KindNode               = model.KindNode
	KindRecursive          = model.KindRecursive
	KindVariable           = model.KindVariable
	KindArrow              = model.KindArrow
	SequenceEpsilon        = model.SequenceEpsilon
	SequenceItem           = model.SequenceItem
	SequenceConcat         = model.SequenceConcat
	SequenceAlternative    = model.SequenceAlternative
	SequenceStar           = model.SequenceStar
	Proven                 = model.Proven
	Refuted                = model.Refuted
	Unproved               = model.Unproved
)

type Program struct{ inner *model.Program }

func DefaultLimits() Limits                { return model.DefaultLimits() }
func Pin(value core.Value) (Digest, error) { return model.Pin(value) }
func PinWithLimits(value core.Value, limits Limits) (Digest, error) {
	return model.PinWithLimits(value, limits)
}
func Resolve(root core.Value, dependencies []core.Value) (*Program, error) {
	return ResolveWithLimits(root, dependencies, DefaultLimits())
}
func ResolveWithLimits(root core.Value, dependencies []core.Value, limits Limits) (*Program, error) {
	p, err := model.Resolve(root, dependencies, limits, grammar())
	if err != nil {
		return nil, err
	}
	return &Program{inner: p}, nil
}
func (p *Program) Root() Digest {
	if p == nil || p.inner == nil {
		return Digest{}
	}
	return p.inner.Root()
}
func (p *Program) Limits() Limits {
	if p == nil || p.inner == nil {
		return Limits{}
	}
	return p.inner.Limits()
}
func (p *Program) Modules() []*Module {
	if p == nil || p.inner == nil {
		return nil
	}
	return p.inner.Modules()
}
func (p *Program) Module(pin Digest) (*Module, bool) {
	if p == nil || p.inner == nil {
		return nil, false
	}
	return p.inner.Module(pin)
}
func (p *Program) Declaration(id DeclarationID) (*Declaration, bool) {
	if p == nil || p.inner == nil {
		return nil, false
	}
	return p.inner.Declaration(id)
}
func (p *Program) CompileKind(module Digest, expression core.Value) (KindHandle, error) {
	var inner *model.Program
	if p != nil {
		inner = p.inner
	}
	return inner.CompileKind(module, expression)
}
func (p *Program) Value(id DeclarationID) (TypeValue, error) {
	var inner *model.Program
	if p != nil {
		inner = p.inner
	}
	return inner.Value(id)
}
func (p *Program) Proper(value core.Value) (TypeValue, error) {
	var inner *model.Program
	if p != nil {
		inner = p.inner
	}
	return inner.Proper(value)
}
func (p *Program) Apply(callee TypeValue, arguments []TypeValue) (TypeValue, error) {
	var inner *model.Program
	if p != nil {
		inner = p.inner
	}
	return inner.Apply(callee, arguments)
}
func (p *Program) Satisfies(value TypeValue, kind KindHandle) (Verdict, error) {
	var inner *model.Program
	if p != nil {
		inner = p.inner
	}
	return inner.Satisfies(value, kind)
}
func (p *Program) Includes(actual KindHandle, expected KindHandle) (Verdict, error) {
	var inner *model.Program
	if p != nil {
		inner = p.inner
	}
	return inner.Includes(actual, expected)
}
func (p *Program) Descriptor(value TypeValue) (core.Value, error) {
	var inner *model.Program
	if p != nil {
		inner = p.inner
	}
	return inner.Descriptor(value)
}
func (p *Program) GroundDescriptor(value TypeValue) (core.Value, error) {
	var inner *model.Program
	if p != nil {
		inner = p.inner
	}
	return inner.GroundDescriptor(value)
}
