package declarations_test

import (
	"testing"

	decl "github.com/Bitspark/horos/core/go/declarations"
	core "github.com/bitspark/ontos/core/go"
)

func TestCloseRequiresOwnedIndependentlyClosedExpression(t *testing.T) {
	root := module("close", mapping(), entry("Box", definition(h("tuple",param(0),h("scalar",a("int"))),"T")))
	p := resolve(t,root)
	d, _ := p.Declaration(id(p,"Box"))
	operands:=d.Expression().Operands()
	v,err:=p.Close(operands[1].Expression())
	if err!=nil || !core.Equal(v,h("scalar",a("int"))) { t.Fatal("closed subtree",err) }
	if v,err:=p.Close(operands[0].Expression()); v!=nil || err==nil { t.Fatal("free parameter closed") }
	foreign:=resolve(t,root)
	if v,err:=foreign.Close(operands[1].Expression()); v!=nil || err==nil { t.Fatal("foreign expression admitted") }
	var zero decl.Program
	if v,err:=zero.Close(nil); v!=nil || err==nil { t.Fatal("zero model admitted") }
}
