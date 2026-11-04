// Package declmodel implements immutable revision-4 Horos declarations.
package declmodel

import (
	"encoding/hex"
	"fmt"
	"slices"

	core "github.com/bitspark/ontos/core/go"
)

type Digest [32]byte
func (d Digest) String() string { return hex.EncodeToString(d[:]) }
type DeclarationID struct { Module Digest; Name string }
type ParameterID struct { Declaration DeclarationID; Index int }

type Code string
const (
	Invalid Code = "invalid"
	BoundViolation Code = "bound-violation"
	KindMismatch Code = "kind-mismatch"
	UnprovedObligation Code = "unproved-obligation"
	MissingDependency Code = "missing-dependency"
	ImportMismatch Code = "import-mismatch"
	UnknownImport Code = "unknown-import"
	UnknownDeclaration Code = "unknown-declaration"
	InvalidOptions Code = "invalid-options"
	Cycle Code = "cycle"
	Exhausted Code = "exhausted"
)
type Subject string
const (
	SubjectSource Subject = "source"
	SubjectContext Subject = "context"
	SubjectConfiguration Subject = "configuration"
	SubjectRequest Subject = "request"
)
// Region identifies the canonical declaration component containing Path:
// body, result, parameter-kind, kind-body, result-sort, or kind-parameter-sort.
type Error struct {
	Code Code
	Subject Subject
	Module Digest
	Declaration string
	Region string
	ParameterIndex int
	Path []int
	ImportAlias string
	Dependency Digest
	ExpectedName string
	ActualName string
	Target DeclarationID
	CalleeParameter ParameterID
	Expected *Kind
	Message string
}
func (e *Error) Error() string { return fmt.Sprintf("horos declarations %s: %s (%s %s %s %v)", e.Code,e.Message,e.Module,e.Declaration,e.Region,e.Path) }
func failure(code Code, message string) error {
	subject := SubjectSource
	switch code { case MissingDependency,ImportMismatch: subject=SubjectContext; case InvalidOptions: subject=SubjectConfiguration; case Exhausted: subject=SubjectRequest }
	return &Error{Code:code,Subject:subject,Message:message}
}
type Limits struct { MaxDepth,MaxNodes,MaxBytes,MaxModules int }
func DefaultLimits() Limits { return Limits{MaxDepth:256,MaxNodes:100000,MaxBytes:16<<20,MaxModules:256} }
func (l Limits) valid() bool { return l.MaxDepth>0&&l.MaxDepth<=512&&l.MaxNodes>0&&l.MaxBytes>0&&l.MaxModules>0 }

type Verdict string
const ( Proven Verdict="proven"; Refuted Verdict="refuted"; Unproved Verdict="unproved" )

type TypeParameter struct { Name string; Kind *Kind }
type KindParameter struct { Name string; Sort Sort }
type Assumption struct { parameter ParameterID; kind *Kind }
func (a Assumption) Parameter() ParameterID { return a.parameter }
func (a Assumption) Kind() *Kind { return copyKind(a.kind) }
type Membership struct { kind *Kind; assumptions []Assumption }
func (m Membership) Kind() *Kind { return copyKind(m.kind) }
func (m Membership) Assumptions() []Assumption { return slices.Clone(m.assumptions) }
type DeclarationForm string
const ( TypeDefinition DeclarationForm="type"; KindDefinition DeclarationForm="kind" )
type ExpressionKind string
const ( Descriptor ExpressionKind="descriptor"; Parameter ExpressionKind="parameter"; Reference ExpressionKind="reference"; Application ExpressionKind="application" )
type Operand struct { path []int; expression *Expression }
func (o Operand) Path() []int { return slices.Clone(o.path) }
func (o Operand) Expression() *Expression { return copyExpression(o.expression) }

type Expression struct {
	path []int
	kind ExpressionKind
	value core.Value
	constructor string
	operands []Operand
	parameter ParameterID
	target DeclarationID
	callee *Expression
	arguments []*Expression
	binding *Expression
	sort Sort
	contract *Kind
	semantic *kindTerm
	memberships []Membership
}
func (e *Expression) Kind() ExpressionKind { return e.kind }
func (e *Expression) Value() core.Value { return e.value }
func (e *Expression) Constructor() string { return e.constructor }
func (e *Expression) Operands() []Operand { return slices.Clone(e.operands) }
func (e *Expression) Arguments() []*Expression { r:=make([]*Expression,len(e.arguments));for i,x:=range e.arguments { r[i]=copyExpression(x) };return r }
func (e *Expression) Parameter() (ParameterID,bool) { return e.parameter,e.kind==Parameter }
func (e *Expression) Target() (DeclarationID,bool) { return e.target,e.kind==Reference }
func (e *Expression) Callee() *Expression { return copyExpression(e.callee) }
func (e *Expression) Sort() Sort { return cloneSort(e.sort) }
func (e *Expression) Contract() *Kind { return copyKind(e.contract) }
func (e *Expression) Path() []int { return slices.Clone(e.path) }
func (e *Expression) Memberships() []Membership { return slices.Clone(e.memberships) }

type Import struct { alias,name string; pin Digest }
func (i Import) Alias() string { return i.alias }
func (i Import) Name() string { return i.name }
func (i Import) Pin() Digest { return i.pin }
type Declaration struct {
	id DeclarationID
	value core.Value
	form DeclarationForm
	parameters []TypeParameter
	kindParameters []KindParameter
	result *Kind
	contract *Kind
	kindBody *Kind
	sort Sort
	body core.Value
	expression *Expression
	parameterValues []core.Value
	resultValue core.Value
	semantic *kindTerm
	memberships []Membership
}
func (d *Declaration) ID() DeclarationID { return d.id }
func (d *Declaration) Form() DeclarationForm { return d.form }
func (d *Declaration) Parameters() []TypeParameter { r:=slices.Clone(d.parameters);for i:=range r { r[i].Kind=copyKind(r[i].Kind) };return r }
func (d *Declaration) KindParameters() []KindParameter { r:=slices.Clone(d.kindParameters);for i:=range r { r[i].Sort=cloneSort(r[i].Sort) };return r }
func (d *Declaration) Result() *Kind { return copyKind(d.result) }
func (d *Declaration) Contract() *Kind { return copyKind(d.contract) }
func (d *Declaration) KindBody() *Kind { return copyKind(d.kindBody) }
func (d *Declaration) Sort() Sort { return cloneSort(d.sort) }
func (d *Declaration) Expression() *Expression { return copyExpression(d.expression) }
func (d *Declaration) Value() core.Value { return d.value }
func (d *Declaration) Memberships() []Membership { return slices.Clone(d.memberships) }

type Module struct { pin Digest;name string;value core.Value;imports []Import;importByName map[string]Import;declarations []*Declaration }
func (m *Module) Pin() Digest { return m.pin }
func (m *Module) Name() string { return m.name }
func (m *Module) Value() core.Value { return m.value }
func (m *Module) Imports() []Import { return slices.Clone(m.imports) }
func (m *Module) Declarations() []*Declaration { r:=make([]*Declaration,len(m.declarations));for i,d:=range m.declarations { r[i]=copyDeclaration(d) };return r }

// Program owns immutable checked definitions and sealed query values. Queries
// allocate their own aggregate budget and caches; they never mutate the Program.
type Program struct {
	grammar Grammar
	root Digest
	prelude Digest
	modules map[Digest]*Module
	order []*Module
	declarations map[DeclarationID]*Declaration
	declarationOrder []*Declaration
	limits Limits
	ground *kindTerm
}
func (p *Program) Root() Digest { return p.root }
func (p *Program) Modules() []*Module { r:=make([]*Module,len(p.order));for i,m:=range p.order { r[i]=copyModule(m) };return r }
func (p *Program) Module(pin Digest) (*Module,bool) { m,ok:=p.modules[pin];return copyModule(m),ok }
func (p *Program) Declaration(id DeclarationID) (*Declaration,bool) { d,ok:=p.declarations[id];return copyDeclaration(d),ok }
func copyModule(m *Module) *Module { if m==nil{return nil};r:=*m;return &r }
func copyDeclaration(d *Declaration) *Declaration { if d==nil{return nil};r:=*d;return &r }
func copyExpression(e *Expression) *Expression { if e==nil{return nil};r:=*e;return &r }
