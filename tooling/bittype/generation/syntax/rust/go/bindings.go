package generation
import(
g "github.com/Bitspark/bittype/contracts/generation/go"
c "github.com/Bitspark/bittype/generation/syntax/go"
source "github.com/Bitspark/bittype/generation/syntax/source/horos/go"
target "github.com/Bitspark/bittype/generation/syntax/target/rust/go"
h "github.com/Bitspark/bittype/syntax/horos/go"
s "github.com/Bitspark/bittype/syntax/rust/go"
rules "github.com/Bitspark/horos/tooling/bittype/generation/syntax/go"
)
type Setup struct{
Bindings []c.Binding[h.Language,s.Language]
Overrides []c.Override[h.Language,s.Language]
Structural *c.Mapping[h.Language,s.Language]
Helpers []g.Plan[h.Language,s.Language]
}
func Bindings(src *source.Source,b *target.Backend[h.Language])(Setup,error){
out:=Setup{};structural:=rules.Structural[s.Language](src,b);out.Structural=&structural
for _,entry:=range []struct{head,discriminator string;mapping c.Mapping[h.Language,s.Language]}{
{"scalar","utf8-text",textMapping(src,b)},{"scalar","bool",boolMapping(src,b)},{"list-of","",listMapping(src,b)},{"record","closed",recordMapping(src,b)},
}{key,err:=source.Builtin(src.Snapshot().Language(),entry.head,entry.discriminator);if err!=nil{return Setup{},err};out.Bindings=append(out.Bindings,c.Binding[h.Language,s.Language]{Selector:key,Mapping:entry.mapping})}
bound,err:=source.Bound(src.Snapshot().Language());if err!=nil{return Setup{},err};out.Bindings=append(out.Bindings,c.Binding[h.Language,s.Language]{Selector:bound,Mapping:rules.Bound[s.Language]()})

return out,nil
}

