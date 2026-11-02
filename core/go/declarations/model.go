// Package declarations implements revision 1 of the Horos declaration model.
//
// It resolves supplied Ontos module values without IO, preserves open generic
// bindings, and specializes concrete applications to existing Horos descriptors.
// It supplies no source parser, native generator, value codec or runtime binding.
package declarations

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

type Code string

const (
	Invalid    Code = "invalid"
	Resolution Code = "resolution"
	Cycle      Code = "cycle"
	Exhausted  Code = "exhausted"
)

// Error distinguishes malformed/unresolved input from an incomplete attempt.
// Path is a tuple-index path within the declaration body (empty for module errors).
type Error struct {
	Code        Code
	Module      Digest
	Declaration string
	Path        []int
	Message     string
}

func (e *Error) Error() string {
	return fmt.Sprintf("horos declarations %s: %s (%s %s %v)", e.Code, e.Message, e.Module, e.Declaration, e.Path)
}

func failure(code Code, message string) error { return &Error{Code: code, Message: message} }

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

// Kind classifies declaration expressions, never behavioral library contracts.
type Kind string

const (
	Descriptor  Kind = "descriptor"
	Parameter   Kind = "parameter"
	Application Kind = "application"
)

// Operand identifies a descriptor child without interpreting literal operands.
// Tuple indices include the constructor tag at index zero.
type Operand struct {
	path       []int
	expression *Expression
}

func (o Operand) Path() []int             { return slices.Clone(o.path) }
func (o Operand) Expression() *Expression { return o.expression }

// Expression is immutable. Values are immutable Ontos values; slice accessors
// return independent storage. Application arguments keep their caller bindings.
type Expression struct {
	kind        Kind
	value       core.Value
	constructor string
	operands    []Operand
	parameter   ParameterID
	target      DeclarationID
	arguments   []*Expression
}

func (e *Expression) Kind() Kind               { return e.kind }
func (e *Expression) Value() core.Value        { return e.value }
func (e *Expression) Constructor() string      { return e.constructor }
func (e *Expression) Operands() []Operand      { return slices.Clone(e.operands) }
func (e *Expression) Arguments() []*Expression { return slices.Clone(e.arguments) }
func (e *Expression) Parameter() (ParameterID, bool) {
	return e.parameter, e.kind == Parameter
}
func (e *Expression) Target() (DeclarationID, bool) {
	return e.target, e.kind == Application
}

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
	parameters []string
	body       core.Value
	expression *Expression
}

func (d *Declaration) ID() DeclarationID       { return d.id }
func (d *Declaration) Parameters() []string    { return slices.Clone(d.parameters) }
func (d *Declaration) Expression() *Expression { return d.expression }

type Module struct {
	pin          Digest
	name         string
	value        core.Value
	imports      []Import
	declarations []*Declaration
}

func (m *Module) Pin() Digest                  { return m.pin }
func (m *Module) Name() string                 { return m.name }
func (m *Module) Value() core.Value            { return m.value }
func (m *Module) Imports() []Import            { return slices.Clone(m.imports) }
func (m *Module) Declarations() []*Declaration { return slices.Clone(m.declarations) }

// Program is a checked, immutable transitive module closure, safe for concurrent
// reading and specialization. Its zero value is not a resolved program.
type Program struct {
	root         Digest
	modules      map[Digest]*Module
	order        []*Module
	declarations map[DeclarationID]*Declaration
	limits       Limits
}

func (p *Program) Root() Digest       { return p.root }
func (p *Program) Modules() []*Module { return slices.Clone(p.order) }
func (p *Program) Module(pin Digest) (*Module, bool) {
	m, ok := p.modules[pin]
	return m, ok
}
func (p *Program) Declaration(id DeclarationID) (*Declaration, bool) {
	d, ok := p.declarations[id]
	return d, ok
}
