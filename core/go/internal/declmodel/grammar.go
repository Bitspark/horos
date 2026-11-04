package declmodel

import (
	"github.com/Bitspark/horos/core/go/internal/kindmodel"
	core "github.com/bitspark/ontos/core/go"
)

// Grammar is the fixed revision-4 primitive signature. Operand contracts are
// ordinary kind expressions interpreted in the explicit bundled prelude module.
type Grammar struct {
	DeclarationPrefix string
	DescriptorPrefix string
	Any func() core.Value
	Positions func(string,[]core.Value)([][]int,error)
	WF func(core.Value) error
	GroundWF func(core.Value) error
	Metadata func(string,[]core.Value)([]kindmodel.Selector,error)
	Schema kindmodel.Schema
	Prelude core.Value
	OperandKind func(string,[]core.Value) core.Value
	GroundKind core.Value
}
func Failure(code Code,message string) error { return failure(code,message) }
func Head(value core.Value,prefix string)(string,[]core.Value,bool){return head(value,prefix)}
func (p *Program) Limits() Limits{return p.limits}
