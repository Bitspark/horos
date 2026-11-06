package generation
import("context"

g "github.com/Bitspark/bittype/contracts/generation/go"
c "github.com/Bitspark/bittype/generation/syntax/go"
source "github.com/Bitspark/bittype/generation/syntax/source/horos/go"
m "github.com/Bitspark/bittype/generation/syntax/target/go"
target "github.com/Bitspark/bittype/generation/syntax/target/rust/go"
h "github.com/Bitspark/bittype/syntax/horos/go"
s "github.com/Bitspark/bittype/syntax/rust/go"
rules "github.com/Bitspark/horos/tooling/bittype/generation/syntax/go")
func textMapping(src *source.Source,b *target.Backend[h.Language]) c.Mapping[h.Language,s.Language]{
return rules.Direct("text/rust/1",src,b,func(_ context.Context,r g.UseRequest[h.Language,s.Language])(g.Fragment[s.Language],error){
view,err:=src.Read(r.Want().Use().Use);if err!=nil{return g.Fragment[s.Language]{},err};return b.Path([]string{"std","string","String"},nil,view.Origin,true)
})}
func boolMapping(src *source.Source,b *target.Backend[h.Language]) c.Mapping[h.Language,s.Language]{
return rules.Direct("bool/rust/1",src,b,func(_ context.Context,r g.UseRequest[h.Language,s.Language])(g.Fragment[s.Language],error){
view,err:=src.Read(r.Want().Use().Use);if err!=nil{return g.Fragment[s.Language]{},err};return b.Node(m.Node(s.KindPrimitiveType,m.Attributes("name","bool"),view.Origin))
})}
func listMapping(src *source.Source,b *target.Backend[h.Language]) c.Mapping[h.Language,s.Language]{
return rules.Direct("list/rust/1",src,b,func(_ context.Context,r g.UseRequest[h.Language,s.Language])(g.Fragment[s.Language],error){
view,err:=src.Read(r.Want().Use().Use);if err!=nil{return g.Fragment[s.Language]{},err};operand,err:=src.Operand(r.Want().Use().Use,1);if err!=nil{return g.Fragment[s.Language]{},err};child,err:=r.Child(operand);if err!=nil{return g.Fragment[s.Language]{},err};return b.Path([]string{"std","vec","Vec"},[]g.Fragment[s.Language]{child.Payload()},view.Origin,true)
})}
func recordMapping(src *source.Source,b *target.Backend[h.Language]) c.Mapping[h.Language,s.Language]{
return rules.Direct("record/rust/1",src,b,func(_ context.Context,r g.UseRequest[h.Language,s.Language])(g.Fragment[s.Language],error){
use:=r.Want().Use();if !src.IsBody(use.Environment,use.Use){return g.Fragment[s.Language]{},src.Unsupported(use.Use,"nested records require a named declaration in this profile")}
view,err:=src.Read(use.Use);if err!=nil{return g.Fragment[s.Language]{},err};fields,err:=src.Fields(use.Use);if err!=nil{return g.Fragment[s.Language]{},err}
var children []m.Child[s.Language];names:=map[string]bool{}
for _,field:=range fields{

if names[field.Name]{return g.Fragment[s.Language]{},src.Unsupported(use.Use,"native field name collision")};names[field.Name]=true
child,err:=r.Child(field.Use);if err!=nil{return g.Fragment[s.Language]{},err};ty:=child.Payload();if field.Optional{ty,err=b.Path([]string{"std","option","Option"},[]g.Fragment[s.Language]{ty},field.Origin,true);if err!=nil{return ty,err}}
member,err:=b.Node(m.Node(s.KindField,m.Attributes("name",field.Name,"visibility","public"),field.Origin),m.Child[s.Language]{Role:s.RoleType,Fragment:ty});if err!=nil{return member,err};children=append(children,m.Child[s.Language]{Role:s.RoleField,Fragment:member})
}
return b.Node(m.Node(s.KindStruct,nil,view.Origin),children...)
})}

