// Package declmodel implements shared immutable declaration and binding mechanics.
//
// It resolves supplied Ontos module values without IO, preserves open generic
// bindings, and specializes applications using a caller-owned fixed grammar.
// It supplies no source parser, native generator, value codec or runtime binding.
package declmodel

import (
	"encoding/hex"
	"fmt"
	"slices"

	core "github.com/bitspark/ontos/core/go"
)

// Digest addresses canonical codec-v1 module bytes, not semantic equivalence.
type Digest [32]byte

func (d Digest) String() string { return hex.EncodeToString(d[:]) }

// DeclarationID retains the defining module even when the body is expanded.
type DeclarationID struct {
	Module Digest
	Name   string
}

// ParameterID distinguishes same-spelled binders in different declarations.
type ParameterID struct {
	Declaration DeclarationID
	Index       int
}

type TypeParameter struct {
	Name      string
	Interface Interface
}

type Code string

const (
	Invalid            Code = "invalid"
	BoundViolation     Code = "bound-violation"
	KindMismatch       Code = "kind-mismatch"
	MissingDependency  Code = "missing-dependency"
	ImportMismatch     Code = "import-mismatch"
	UnknownImport      Code = "unknown-import"
	UnknownDeclaration Code = "unknown-declaration"
	InvalidOptions     Code = "invalid-options"
	Cycle              Code = "cycle"
	Exhausted          Code = "exhausted"
)

// Subject identifies what a failed operation establishes a result about.
// A rejected dependency context does not establish that the root source is
// invalid under a different, valid context. Exhaustion is about the bounded
// request, never a rejection of source or context.
type Subject string

const (
	SubjectSource        Subject = "source"
	SubjectContext       Subject = "context"
	SubjectConfiguration Subject = "configuration"
	SubjectRequest       Subject = "request"
)

// Error supplies stable classification and details without interpreting Message.
// Path is a tuple-index path within the declaration body. A dependency body
// failure keeps its defining Module/Declaration/Path and identifies that module
// in Dependency. When Declaration is empty and ImportAlias is present, Module
// identifies the importing module and Dependency is the required pin.
// ExpectedName and ActualName describe an import name mismatch. Target identifies
// a missing declaration, or the declaration requested by invalid specialization
// arguments. Details are absent when the operation could not establish them.
type Error struct {
	Code         Code
	Subject      Subject
	Module       Digest
	Declaration  string
	Path         []int
	ImportAlias  string
	Dependency   Digest
	ExpectedName string
	ActualName   string
	Target       DeclarationID
	CalleeParameter ParameterID
	Expected     Interface
	Message      string
}

func (e *Error) Error() string {
	return fmt.Sprintf("horos declarations %s: %s (%s %s %v)", e.Code, e.Message, e.Module, e.Declaration, e.Path)
}

func failure(code Code, message string) error {
	subject := SubjectSource
	switch code {
	case MissingDependency, ImportMismatch:
		subject = SubjectContext
	case InvalidOptions:
		subject = SubjectConfiguration
	case Exhausted:
		subject = SubjectRequest
	}
	return &Error{Code: code, Subject: subject, Message: message}
}

// Limits bound each operation, including literal data and expansion work.
// MaxDepth must be in [1,512]; all other limits must be positive.
type Limits struct {
	MaxDepth   int
	MaxNodes   int
	MaxBytes   int
	MaxModules int
}

func DefaultLimits() Limits {
	return Limits{MaxDepth: 256, MaxNodes: 100000, MaxBytes: 16 << 20, MaxModules: 256}
}

func (l Limits) valid() bool {
	return l.MaxDepth > 0 && l.MaxDepth <= 512 && l.MaxNodes > 0 && l.MaxBytes > 0 && l.MaxModules > 0
}

// ExpressionKind classifies syntax forms, independently of their shape kinds.
type ExpressionKind string

const (
	Descriptor  ExpressionKind = "descriptor"
	Parameter   ExpressionKind = "parameter"
	Reference   ExpressionKind = "reference"
	Application ExpressionKind = "application"
)

// Operand identifies a descriptor child without interpreting literal operands.
// Tuple indices include the constructor tag at index zero.
type Operand struct {
	path       []int
	expression *Expression
}

func (o Operand) Path() []int             { return slices.Clone(o.path) }
func (o Operand) Expression() *Expression { return copyExpression(o.expression) }

// Expression is immutable. Values are immutable Ontos values; slice accessors
// return independent storage. Application arguments keep their caller bindings.
type Expression struct {
	path        []int
	kind        ExpressionKind
	value       core.Value
	constructor string
	operands    []Operand
	parameter   ParameterID
	target      DeclarationID
	callee      *Expression
	arguments   []*Expression
	interface_  Interface
	summary     *DataSummary
	dataStatus  DataStatus
}

func (e *Expression) Kind() ExpressionKind { return e.kind }
func (e *Expression) Value() core.Value   { return e.value }
func (e *Expression) Constructor() string { return e.constructor }
func (e *Expression) Operands() []Operand { return slices.Clone(e.operands) }
func (e *Expression) Arguments() []*Expression {
	result := make([]*Expression, len(e.arguments))
	for i, child := range e.arguments {
		result[i] = copyExpression(child)
	}
	return result
}
func (e *Expression) Parameter() (ParameterID, bool) {
	return e.parameter, e.kind == Parameter
}
func (e *Expression) Target() (DeclarationID, bool) {
	return e.target, e.kind == Reference
}
func (e *Expression) Callee() *Expression { return copyExpression(e.callee) }
func (e *Expression) Interface() Interface { return cloneInterface(e.interface_) }
func (e *Expression) KindShape() Kind { return e.interface_.Kind() }
func (e *Expression) DataSummary() *DataSummary { return copySummary(e.summary) }
func (e *Expression) DataStatus() DataStatus { return e.dataStatus }
func (e *Expression) Path() []int { return slices.Clone(e.path) }

type Import struct {
	alias string
	name  string
	pin   Digest
}

func (i Import) Alias() string { return i.alias }
func (i Import) Name() string  { return i.name }
func (i Import) Pin() Digest   { return i.pin }

type Declaration struct {
	id         DeclarationID
	parameters []TypeParameter
	result     Claim
	interface_ Interface
	body       core.Value
	expression *Expression
}

func (d *Declaration) ID() DeclarationID           { return d.id }
func (d *Declaration) Parameters() []TypeParameter {
	result := slices.Clone(d.parameters)
	for i := range result { result[i].Interface = cloneInterface(result[i].Interface) }
	return result
}
func (d *Declaration) Expression() *Expression     { return copyExpression(d.expression) }
func (d *Declaration) Result() Claim { return d.result }
func (d *Declaration) Interface() Interface {
	return cloneInterface(d.interface_)
}
func (d *Declaration) Kind() Kind { return d.interface_.Kind() }
func (d *Declaration) DataSummary() *DataSummary { return copySummary(d.expression.summary) }
func (d *Declaration) DataStatus() DataStatus { return d.expression.dataStatus }

type Module struct {
	pin          Digest
	name         string
	value        core.Value
	imports      []Import
	importByName map[string]Import
	declarations []*Declaration
}

func (m *Module) Pin() Digest       { return m.pin }
func (m *Module) Name() string      { return m.name }
func (m *Module) Value() core.Value { return m.value }
func (m *Module) Imports() []Import { return slices.Clone(m.imports) }
func (m *Module) Declarations() []*Declaration {
	result := make([]*Declaration, len(m.declarations))
	for i, declaration := range m.declarations {
		result[i] = copyDeclaration(declaration)
	}
	return result
}

// Program is a checked, immutable transitive module closure, safe for concurrent
// reading and specialization. Its zero value is not a resolved program.
type Program struct {
	grammar      Grammar
	root         Digest
	modules      map[Digest]*Module
	order        []*Module
	declarations map[DeclarationID]*Declaration
	declarationOrder []*Declaration
	limits       Limits
}

func (p *Program) Root() Digest { return p.root }
func (p *Program) Modules() []*Module {
	result := make([]*Module, len(p.order))
	for i, module := range p.order {
		result[i] = copyModule(module)
	}
	return result
}
func (p *Program) Module(pin Digest) (*Module, bool) {
	m, ok := p.modules[pin]
	return copyModule(m), ok
}
func (p *Program) Declaration(id DeclarationID) (*Declaration, bool) {
	d, ok := p.declarations[id]
	return copyDeclaration(d), ok
}

// Exported structs can be overwritten even though their fields are private.
// Each accessor returns a separate handle; descendants are copied on access.
func copyModule(m *Module) *Module {
	if m == nil {
		return nil
	}
	copy := *m
	return &copy
}

func copyDeclaration(d *Declaration) *Declaration {
	if d == nil {
		return nil
	}
	copy := *d
	return &copy
}

func copyExpression(e *Expression) *Expression {
	if e == nil {
		return nil
	}
	copy := *e
	return &copy
}
