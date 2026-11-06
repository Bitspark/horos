// Package golang supplies the Horos pagination representation composition.
package golang
import(
"context"
 g "github.com/Bitspark/bittype/contracts/generation/go"
 "github.com/Bitspark/bittype/contracts/transform/go"
 c "github.com/Bitspark/bittype/generation/syntax/go"
 source "github.com/Bitspark/bittype/generation/syntax/source/horos/go"
 target "github.com/Bitspark/bittype/generation/syntax/target/golang/go"
 h "github.com/Bitspark/bittype/syntax/horos/go"
 s "github.com/Bitspark/bittype/syntax/golang/go"
 edge "github.com/Bitspark/bittype/transforms/horos-to-go/go"
 "github.com/Bitspark/bittype/trees/ast/go"
 "github.com/Bitspark/bittype/trees/representation/go"
 mappings "github.com/Bitspark/horos/tooling/bittype/generation/syntax/golang/go"
 rules "github.com/Bitspark/horos/tooling/bittype/generation/syntax/go"
)
const RepresentationProfile=rules.RepresentationProfile
const EnforcementProfile=rules.EnforcementProfile
type Config struct {TreeID ast.TreeID;Profile string;Package string
// Configure selects exact additions and explicit overrides per invocation. It
// must retain stable policy; no runtime discovery or fallback is performed.
Configure func(*source.Source,*target.Backend[h.Language],*mappings.Setup)error
}
func New(from,to representation.Language,config Config)(transform.ASTTransformer,error){
if config.TreeID==""||(config.Profile!=RepresentationProfile&&config.Profile!=EnforcementProfile){return nil,transform.ErrInvalidTransformer}
mapper,err:=edge.New(from,to,func(ctx context.Context,r transform.Request[h.Language])(c.Composition[h.Language,s.Language],error){
zero:=c.Composition[h.Language,s.Language]{}
if config.Profile==EnforcementProfile{return zero,g.ErrUnsupported}
for _,input:=range r.Inputs(){info,err:=input.Inspect();if err!=nil{return zero,err};if info.ID==config.TreeID{return zero,g.ErrInvalid}}
src,err:=source.New(ctx,r);if err!=nil{return zero,err};limits:=g.DefaultLimits()
b,err:=target.New[h.Language](ctx,to,target.Config{TreeID:config.TreeID,Package:config.Package},limits);if err!=nil{return zero,err}
setup,err:=mappings.Bindings(src,b);if err!=nil{return zero,err}
if config.Configure!=nil{if err=config.Configure(src,b,&setup);err!=nil{return zero,err}}
bindings,err:=c.NewBindings(from,setup.Bindings,setup.Overrides,setup.Structural);if err!=nil{return zero,err}
return c.Composition[h.Language,s.Language]{Config:g.Config[h.Language,s.Language]{Source:src,Native:b,Target:to,Evidence:source.Contract().Evidence},Catalog:src,Bindings:bindings,Roots:src.Roots(),Helpers:setup.Helpers,Limits:limits},nil
})
if err!=nil{return nil,err};return rules.Diagnostics(mapper,from,to),nil
}

