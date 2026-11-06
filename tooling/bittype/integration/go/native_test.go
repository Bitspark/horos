package integration_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"github.com/Bitspark/bittype/contracts/check/go"
	"github.com/Bitspark/bittype/contracts/format/go"
	"github.com/Bitspark/bittype/contracts/parse/go"
	"github.com/Bitspark/bittype/contracts/print/go"
	"github.com/Bitspark/bittype/trees/ast/go"
	"github.com/Bitspark/bittype/trees/cst/go"
)
type nativeProvider[L any] interface {
	parse.Provider[parse.Input,cst.CST[L]]
	check.Provider[L]
	format.Provider[ast.View[L],cst.CST[L]]
	print.Provider[cst.CST[L],string]
}
func formatted[L any](t *testing.T,p nativeProvider[L],tree ast.AST[L])string{t.Helper();c,err:=p.Format(context.Background(),tree);if err!=nil{t.Fatal(err)};text,err:=p.Print(context.Background(),c);if err!=nil{t.Fatal(err)};return text}
func command(t *testing.T,dir,name string,args ...string){t.Helper();cmd:=exec.Command(name,args...);cmd.Dir=dir;out,err:=cmd.CombinedOutput();t.Logf("%s %s\n%s",name,strings.Join(args," "),out);if err!=nil{t.Fatal(err)}}
