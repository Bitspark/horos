// Package wire implements the optional horos/wire revision-1 static profile.
// Its descriptions are ground data. Loading or specializing them performs no
// capability recognition, interaction, hydration or runtime value validation.
package wire

import (
	model "github.com/Bitspark/horos/core/go/internal/declmodel"
	core "github.com/bitspark/ontos/core/go"
)

const Profile = "horos/wire"
const Revision = 1

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
	Invalid = model.Invalid
	MissingDependency = model.MissingDependency
	ImportMismatch = model.ImportMismatch
	UnknownImport = model.UnknownImport
	UnknownDeclaration = model.UnknownDeclaration
	InvalidOptions = model.InvalidOptions
	Cycle = model.Cycle
	Exhausted = model.Exhausted
	UnsupportedLift Code = "unsupported-lift"
	SubjectSource = model.SubjectSource
	SubjectContext = model.SubjectContext
	SubjectConfiguration = model.SubjectConfiguration
	SubjectRequest = model.SubjectRequest
	Descriptor = model.Descriptor
	Parameter = model.Parameter
	Application = model.Application
)

// Reference qualifies document identity separately from semantic compatibility.
// No existing ground DeclarationID field or document-byte encoding is changed.
type Reference struct {
	Profile string
	Revision int
	Declaration DeclarationID
}

type Program struct { inner *model.Program }

func DefaultLimits() Limits { return model.DefaultLimits() }
func Pin(value core.Value) (Digest, error) { return model.Pin(value) }
func PinWithLimits(value core.Value, limits Limits) (Digest, error) { return model.PinWithLimits(value, limits) }
func Resolve(root core.Value, dependencies []core.Value) (*Program, error) {
	return ResolveWithLimits(root, dependencies, DefaultLimits())
}
func ResolveWithLimits(root core.Value, dependencies []core.Value, limits Limits) (*Program, error) {
	p, err := model.Resolve(root, dependencies, limits, grammar())
	if err != nil { return nil, err }
	return &Program{inner: p}, nil
}
func (p *Program) Root() Digest { if p == nil || p.inner == nil { return Digest{} }; return p.inner.Root() }
func (p *Program) Limits() Limits { if p == nil || p.inner == nil { return Limits{} }; return p.inner.Limits() }
func (p *Program) Modules() []*Module { if p == nil || p.inner == nil { return nil }; return p.inner.Modules() }
func (p *Program) Module(pin Digest) (*Module, bool) { if p == nil || p.inner == nil { return nil, false }; return p.inner.Module(pin) }
func (p *Program) Declaration(id DeclarationID) (*Declaration, bool) { if p == nil || p.inner == nil { return nil, false }; return p.inner.Declaration(id) }
func (p *Program) Reference(id DeclarationID) (Reference, bool) {
	if _, ok := p.Declaration(id); !ok { return Reference{}, false }
	return Reference{Profile: Profile, Revision: Revision, Declaration: id}, true
}
func (p *Program) Specialize(id DeclarationID, arguments []core.Value) (core.Value, error) {
	var inner *model.Program; if p != nil { inner = p.inner }
	return inner.Specialize(id, arguments)
}

// WF checks a closed optional expression. It does not resolve applications or
// interpret a wire-free expression as a ground descriptor.
func WF(value core.Value, limits Limits) error {
	if err := model.AdmitValue(value, limits); err != nil { return err }
	return wellFormed(value)
}
