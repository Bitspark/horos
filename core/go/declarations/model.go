// Package declarations implements the unchanged ground declaration revision 1.
// Binding mechanics are shared privately with optional profiles; this entry
// point always uses the ground grammar and never admits an optional envelope.
package declarations

import (
	model "github.com/Bitspark/horos/core/go/internal/declmodel"
	core "github.com/bitspark/ontos/core/go"
)

// These public facts retain one implementation and their established field
// layout. Program remains a profile-specific handle with no configurable grammar.
type Digest = model.Digest
type DeclarationID = model.DeclarationID
type ParameterID = model.ParameterID
type Code = model.Code
type Subject = model.Subject
type Error = model.Error
type Limits = model.Limits
type Kind = model.Kind
type Operand = model.Operand
type Expression = model.Expression
type Import = model.Import
type Declaration = model.Declaration
type Module = model.Module

const (
	Invalid              = model.Invalid
	MissingDependency    = model.MissingDependency
	ImportMismatch       = model.ImportMismatch
	UnknownImport        = model.UnknownImport
	UnknownDeclaration   = model.UnknownDeclaration
	InvalidOptions       = model.InvalidOptions
	Cycle                = model.Cycle
	Exhausted            = model.Exhausted
	SubjectSource        = model.SubjectSource
	SubjectContext       = model.SubjectContext
	SubjectConfiguration = model.SubjectConfiguration
	SubjectRequest       = model.SubjectRequest
	Descriptor           = model.Descriptor
	Parameter            = model.Parameter
	Application          = model.Application
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
func (p *Program) Specialize(id DeclarationID, arguments []core.Value) (core.Value, error) {
	var inner *model.Program
	if p != nil {
		inner = p.inner
	}
	return inner.Specialize(id, arguments)
}
