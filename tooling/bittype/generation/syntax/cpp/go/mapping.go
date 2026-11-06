package generation
import("context"

g "github.com/Bitspark/bittype/contracts/generation/go"
c "github.com/Bitspark/bittype/generation/syntax/go"
source "github.com/Bitspark/bittype/generation/syntax/source/horos/go"
m "github.com/Bitspark/bittype/generation/syntax/target/go"
target "github.com/Bitspark/bittype/generation/syntax/target/cpp/go"
h "github.com/Bitspark/bittype/syntax/horos/go"
s "github.com/Bitspark/bittype/syntax/cpp/go"
rules "github.com/Bitspark/horos/tooling/bittype/generation/syntax/go")
func textMapping(src *source.Source,b *target.Backend[h.Language]) c.Mapping[h.Language,s.Language]{
return rules.Direct("text/cpp/1",src,b,func(_ context.Context,r g.UseRequest[h.Language,s.Language])(g.Fragment[s.Language],error){
view,err:=src.Read(r.Want().Use().Use);if err!=nil{return g.Fragment[s.Language]{},err};if err=b.Include("string");err!=nil{return g.Fragment[s.Language]{},err};return b.Path([]string{"std","string"},nil,view.Origin,true)
})}
func boolMapping(src *source.Source,b *target.Backend[h.Language]) c.Mapping[h.Language,s.Language]{
return rules.Direct("bool/cpp/1",src,b,func(_ context.Context,r g.UseRequest[h.Language,s.Language])(g.Fragment[s.Language],error){
view,err:=src.Read(r.Want().Use().Use);if err!=nil{return g.Fragment[s.Language]{},err};return b.Node(m.Node(s.KindBuiltinType,m.Attributes("name","bool"),view.Origin))
})}
func listMapping(src *source.Source,b *target.Backend[h.Language]) c.Mapping[h.Language,s.Language]{
return rules.Direct("list/cpp/1",src,b,func(_ context.Context,r g.UseRequest[h.Language,s.Language])(g.Fragment[s.Language],error){
view,err:=src.Read(r.Want().Use().Use);if err!=nil{return g.Fragment[s.Language]{},err};operand,err:=src.Operand(r.Want().Use().Use,1);if err!=nil{return g.Fragment[s.Language]{},err};child,err:=r.Child(operand);if err!=nil{return g.Fragment[s.Language]{},err};if err=b.Include("vector");err!=nil{return g.Fragment[s.Language]{},err};return b.Path([]string{"std","vector"},[]g.Fragment[s.Language]{child.Payload()},view.Origin,true)
})}
func recordMapping(src *source.Source,b *target.Backend[h.Language]) c.Mapping[h.Language,s.Language]{
return rules.Direct("record/cpp/1",src,b,func(_ context.Context,r g.UseRequest[h.Language,s.Language])(g.Fragment[s.Language],error){
use:=r.Want().Use();if !src.IsBody(use.Environment,use.Use){return g.Fragment[s.Language]{},src.Unsupported(use.Use,"nested records require a named declaration in this profile")}
view,err:=src.Read(use.Use);if err!=nil{return g.Fragment[s.Language]{},err};fields,err:=src.Fields(use.Use);if err!=nil{return g.Fragment[s.Language]{},err}
var children []m.Child[s.Language];names:=map[string]bool{}
for _,field:=range fields{

if names[field.Name]{return g.Fragment[s.Language]{},src.Unsupported(use.Use,"native field name collision")};names[field.Name]=true
child,err:=r.Child(field.Use);if err!=nil{return g.Fragment[s.Language]{},err};ty:=child.Payload();if field.Optional{if err=b.Include("optional");err!=nil{return ty,err};ty,err=b.Path([]string{"std","optional"},[]g.Fragment[s.Language]{ty},field.Origin,true);if err!=nil{return ty,err}}
member,err:=b.Node(m.Node(s.KindField,m.Attributes("name",field.Name),field.Origin),m.Child[s.Language]{Role:s.RoleType,Fragment:ty});if err!=nil{return member,err};children=append(children,m.Child[s.Language]{Role:s.RoleMember,Fragment:member})
}
return b.Node(m.Node(s.KindRecord,nil,view.Origin),children...)
})}

