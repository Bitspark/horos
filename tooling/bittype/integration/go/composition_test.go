package integration_test

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"
 g "github.com/Bitspark/bittype/contracts/generation/go"
 "github.com/Bitspark/bittype/contracts/transform/go"
 c "github.com/Bitspark/bittype/generation/syntax/go"
 source "github.com/Bitspark/bittype/generation/syntax/source/horos/go"
 m "github.com/Bitspark/bittype/generation/syntax/target/go"
 target "github.com/Bitspark/bittype/generation/syntax/target/typescript/go"
 tp "github.com/Bitspark/bittype/provider/typescript/go"
 h "github.com/Bitspark/bittype/syntax/horos/go"
 ts "github.com/Bitspark/bittype/syntax/typescript/go"
 "github.com/Bitspark/bittype/trees/representation/go"
 "github.com/Bitspark/bittype/trees/ast/go"
 decl "github.com/Bitspark/horos/core/go/declarations"
 rules "github.com/Bitspark/horos/tooling/bittype/generation/syntax/go"
 mappings "github.com/Bitspark/horos/tooling/bittype/generation/syntax/typescript/go"
 tg "github.com/Bitspark/horos/tooling/bittype/transforms/horos-to-typescript/go"
 core "github.com/bitspark/ontos/core/go"
 data "github.com/bitspark/ontos/data/go"
)

func externalTS(b *target.Backend[h.Language], name string, argument g.Fragment[ts.Language]) (g.Fragment[ts.Language],error) {
 node,err:=b.Node(m.Node(ts.KindTypeReference,m.Attributes("name",name),ast.NodeRef{}));if err!=nil{return node,err}
 return b.Node(m.Node(ts.KindTypeApplication,nil,ast.NodeRef{}),m.Child[ts.Language]{Role:ts.RoleConstructor,Fragment:node},m.Child[ts.Language]{Role:ts.RoleArgument,Fragment:argument})
}
func readonlyList(src *source.Source,b *target.Backend[h.Language]) c.Mapping[h.Language,ts.Language] {
 return rules.Direct("consumer/readonly-list/1",src,b,func(_ context.Context,r g.UseRequest[h.Language,ts.Language])(g.Fragment[ts.Language],error){
  operand,err:=src.Operand(r.Want().Use().Use,1);if err!=nil{return g.Fragment[ts.Language]{},err}
  child,err:=r.Child(operand);if err!=nil{return g.Fragment[ts.Language]{},err}
  return externalTS(b,"ReadonlyArray",child.Payload())
 })
}
func TestListOverrideComposesThroughGenericPage(t *testing.T) {
 e,from,input:=generationInput(t,paginationModule())
 tree:=generated(t,e,from,input,tp.Provider{},func(f,to representation.Language)(transform.ASTTransformer,error){
  return tg.New(f,to,tg.Config{TreeID:"override",Profile:tg.RepresentationProfile,Configure:func(src *source.Source,b *target.Backend[h.Language],setup *mappings.Setup)error{
   key,err:=source.Builtin(f,"list-of","");if err!=nil{return err}
   setup.Overrides=append(setup.Overrides,c.Override[h.Language,ts.Language]{Binding:c.Binding[h.Language,ts.Language]{Selector:key,Mapping:readonlyList(src,b)},ExpectedID:"list/typescript/1"})
   return nil
  }})
 })
 output:=formatted(t,tp.Provider{},tree)
 if !strings.Contains(output,"items: ReadonlyArray<T>")||!strings.Contains(output,"Page<Page<Person>>")||strings.Contains(output,"T[]"){t.Fatal(output)}
 checkGenerationLinks(t,tree)
 compileTSComposition(t,output,"const page: NestedPage = {items: [{items: [{name: 'Ada', active: true}]}]};\nvoid page;\n")
}
func TestSelectedMappingFailureNeverFallsBack(t *testing.T) {
 _,from,input:=generationInput(t,paginationModule())
 sentinel:=errors.New("selected mapping failed")
 called:=0
 mapper,err:=tg.New(from,mustLanguage[ts.Language](t,ts.Language{}),tg.Config{TreeID:"failed",Profile:tg.RepresentationProfile,Configure:func(src *source.Source,b *target.Backend[h.Language],setup *mappings.Setup)error{
  key,_:=source.Builtin(from,"list-of","")
  replacement:=rules.Direct("consumer/failure/1",src,b,func(context.Context,g.UseRequest[h.Language,ts.Language])(g.Fragment[ts.Language],error){called++;return g.Fragment[ts.Language]{},sentinel})
  setup.Overrides=append(setup.Overrides,c.Override[h.Language,ts.Language]{Binding:c.Binding[h.Language,ts.Language]{Selector:key,Mapping:replacement},ExpectedID:"list/typescript/1"})
  return nil
 }})
 if err!=nil{t.Fatal(err)}
 out,err:=mapper.Transform(context.Background(),input)
 if !errors.Is(err,sentinel)||out.LanguageID()!=""||called!=1{t.Fatalf("mapping retried or partial output: %v (%d calls)",err,called)}
}
func TestLibraryOwnedCellAvoidsBodyExpansion(t *testing.T) {
 // The body needs Wire, deliberately absent from the builtin mapping table.
 library:=genModule("cells",genMap(),genEntry("Cell",genType(genTuple("horos/wire",genParam(0)),"T")))
 pin,err:=decl.Pin(library);if err!=nil{t.Fatal(err)}
 imported:=genTuple("horos/decl/import",data.MustEncodeText("cells"),core.NewAtom(pin[:]))
 root:=genModule("consumer",genMap(genEntry("cells",imported)),genEntry("Page",genType(genPage(),"T")),genEntry("Cells",genType(genLocal("Page",genImported("cells","Cell",genScalar("utf8-text"))))))
 e,from,input:=generationInput(t,root,library)
 // Without the exact library-owned binding, structural expansion must fail.
 plain,err:=tsMapper(from,mustLanguage[ts.Language](t,ts.Language{}));if err!=nil{t.Fatal(err)}
 if out,err:=plain.Transform(context.Background(),input);err==nil||out.LanguageID()!=""{t.Fatal("missing Cell mapping accepted")}
 calls:=0
 tree:=generated(t,e,from,input,tp.Provider{},func(f,to representation.Language)(transform.ASTTransformer,error){
  return tg.New(f,to,tg.Config{TreeID:"cells",Profile:tg.RepresentationProfile,Configure:func(src *source.Source,b *target.Backend[h.Language],setup *mappings.Setup)error{
   key,err:=source.Named(f,pin.String(),"Cell");if err!=nil{return err}
   mapping:=rules.Direct("cells/typescript/1",src,b,func(_ context.Context,r g.UseRequest[h.Language,ts.Language])(g.Fragment[ts.Language],error){
    calls++
    use:=r.Want().Use();if len(use.Actuals)!=1{return g.Fragment[ts.Language]{},g.ErrInvalid}
    child,err:=r.Child(use.Actuals[0].Use);if err!=nil{return g.Fragment[ts.Language]{},err}
    return externalTS(b,"LibraryCell",child.Payload())
   })
   setup.Bindings=append(setup.Bindings,c.Binding[h.Language,ts.Language]{Selector:key,Mapping:mapping})
   return nil
  }})
 })
 output:=formatted(t,tp.Provider{},tree)
 if calls!=1||!strings.Contains(output,"Page<LibraryCell<string>>")||strings.Contains(output,"_Cell"){t.Fatal(output,calls)}
 // External names have no invented local declaration target.
 for _,node:=range tree.Nodes{if nodeAttribute(node,ts.AttributeName)=="LibraryCell"&&len(node.References)!=0{t.Fatal("invented local target")}}
 compileTSComposition(t,output,"interface LibraryCell<T> { value: T; }\nconst cells: Cells = {items:[{value:'text'}]};\nvoid cells;\n")
}
func compileTSComposition(t *testing.T,source,consumer string) {
 t.Helper();if os.Getenv("BITTYPE_NATIVE")!="1"{return}
 dir:=t.TempDir();if err:=os.WriteFile(filepath.Join(dir,"types.ts"),[]byte(source+"\n"+consumer),0600);err!=nil{t.Fatal(err)}
 command(t,dir,"tsc","--strict","--noEmit","--exactOptionalPropertyTypes","types.ts")
}
