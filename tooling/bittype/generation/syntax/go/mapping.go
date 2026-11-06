// Package generation supplies Horos-owned representation rules. It is an
// optional integration module; Horos core and Bittype do not depend on it.
package generation

import (
	"context"
	"fmt"
	"strings"
	g "github.com/Bitspark/bittype/contracts/generation/go"
	c "github.com/Bitspark/bittype/generation/syntax/go"
	source "github.com/Bitspark/bittype/generation/syntax/source/horos/go"
	m "github.com/Bitspark/bittype/generation/syntax/target/go"
	h "github.com/Bitspark/bittype/syntax/horos/go"
	"github.com/Bitspark/bittype/trees/ast/go"
)

const RepresentationProfile="pagination-representation/1"
const EnforcementProfile="pagination-enforcement/1"

type UseFunc[T any] func(context.Context,g.UseRequest[h.Language,T])(g.Fragment[T],error)
type rule[T any] struct { source *source.Source; native g.Native[h.Language,T]; space g.Space[g.AssociationRole[h.Language,T]]; records map[g.Token[h.Language,T]]g.Captured[h.Language,T]; build UseFunc[T] }
func newRule[T any](source *source.Source,native g.Native[h.Language,T],build UseFunc[T])*rule[T]{return &rule[T]{source:source,native:native,build:build,space:g.NewSpace[g.AssociationRole[h.Language,T]](),records:map[g.Token[h.Language,T]]g.Captured[h.Language,T]{}}}
func (r *rule[T]) issue(ctx context.Context,w g.Want[h.Language,T],payload g.Fragment[T])(g.Token[h.Language,T],error){
	if !w.Snapshot().Same(r.source.Snapshot()){return g.Token[h.Language,T]{},g.ErrScope};if len(r.records)>=w.Site().Limits().References{return g.Token[h.Language,T]{},g.ErrLimit}
	if err:=r.native.CheckProduct(ctx,w,payload);err!=nil{return g.Token[h.Language,T]{},err}
	token,_:=r.space.Ref(uint64(len(r.records)+1));r.records[token]=g.Captured[h.Language,T]{Binding:w.Binding(),Token:token,Payload:payload};return token,nil
}
func(r *rule[T]) Capture(ctx context.Context,w g.Want[h.Language,T],token g.Token[h.Language,T])(g.Captured[h.Language,T],error){if err:=ctx.Err();err!=nil{return g.Captured[h.Language,T]{},err};v,ok:=r.records[token];if !ok{return v,g.ErrScope};return v,nil}
func(r *rule[T]) Use(ctx context.Context,request g.UseRequest[h.Language,T])(g.Product[h.Language,T],error){payload,err:=r.build(ctx,request);if err!=nil{return g.Product[h.Language,T]{},err};token,err:=r.issue(ctx,request.Want(),payload);if err!=nil{return g.Product[h.Language,T]{},err};return request.Return(token)}

// Direct installs one explicit representation rule. Its callback owns the
// complete result relation, including its selected children. This representation
// profile carries no claim of native constraint enforcement or Logos proof.
func Direct[T any](id string,source *source.Source,native g.Native[h.Language,T],build UseFunc[T]) c.Mapping[h.Language,T] {
	return c.Mapping[h.Language,T]{ID:id,PluginID:"horos/"+id,Plan:func(d g.Declaration[h.Language])(g.Plan[h.Language,T],error){r:=newRule(source,native,build);return g.Plan[h.Language,T]{Declaration:d,User:r,Associations:r},nil}}
}
type bound[T any] struct{}
func(bound[T]) Use(_ context.Context,r g.UseRequest[h.Language,T])(g.Product[h.Language,T],error){return r.Bound()}
func Bound[T any]() c.Mapping[h.Language,T]{return c.Mapping[h.Language,T]{ID:"horos/binder/1",PluginID:"horos/binder",Plan:func(d g.Declaration[h.Language])(g.Plan[h.Language,T],error){return g.Plan[h.Language,T]{Declaration:d,User:bound[T]{}},nil}}}

type Native[T any] interface {
	g.Native[h.Language,T]
	Reserve(g.Site[h.Language,T],string,bool,ast.NodeRef)(g.Destination[T],bool,error)
	Header(g.Site[h.Language,T],g.Destination[T],[]string,[]ast.NodeRef)(g.Layout[T],error)
	Formal(g.Layout[T],int)(m.Symbol[T],error)
	Named(g.Callee[h.Language,T])(m.Symbol[T],error)
	Reference(m.Symbol[T],[]g.Fragment[T],ast.NodeRef)(g.Fragment[T],error)
	UsesFormal(g.Fragment[T],g.Layout[T],int) bool
}
type structural[T any] struct { *rule[T]; target Native[T]; declaration g.Declaration[h.Language] }

func Structural[T any](source *source.Source,native Native[T]) c.Mapping[h.Language,T] {
	return c.Mapping[h.Language,T]{ID:"horos/structural/1",PluginID:"horos/structural",Plan:func(d g.Declaration[h.Language])(g.Plan[h.Language,T],error){
		m:=&structural[T]{rule:newRule[T](source,native,nil),target:native,declaration:d}
		return g.Plan[h.Language,T]{Declaration:d,User:m,Publisher:m,Definer:m,Convention:m,Associations:m.rule,Acquire:m.acquire},nil
	}}
}
func(m *structural[T]) acquire(ctx context.Context,site g.Site[h.Language,T])(g.Destination[T],bool,error){
	d,err:=m.source.Declaration(m.declaration);if err!=nil{return g.Destination[T]{},false,err};name,err:=Exported(d.Name);if err!=nil{return g.Destination[T]{},false,m.source.Unsupported(d.Body,err.Error())};if !d.Root{name="M"+d.ModulePin[:16]+"_"+name}
	view,err:=m.source.Read(d.Body);if err!=nil{return g.Destination[T]{},false,err};return m.target.Reserve(site,name,view.Constructor=="record",d.Origin)
}
func(m *structural[T]) Publish(_ context.Context,r g.PublishRequest[h.Language,T])(g.Publication[h.Language,T],error){c,err:=r.Acquire();if err!=nil{return g.Publication[h.Language,T]{},err};return r.Expose(c)}
func(m *structural[T]) Introduce(ctx context.Context,r g.IntroduceRequest[h.Language,T])(g.Introduction[h.Language,T],error){
	var names []string;var origins []ast.NodeRef
	for _,f:=range r.Open().Formals{p,err:=m.source.Parameter(f.Binder);if err!=nil{return g.Introduction[h.Language,T]{},err};name,err:=Exported(p.Name);if err!=nil{return g.Introduction[h.Language,T]{},err};names=append(names,name);origins=append(origins,p.Origin)}
	l,err:=m.target.Header(r.Site(),r.Destination(),names,origins);if err!=nil{return g.Introduction[h.Language,T]{},err};formals,err:=r.FreezeLayout(l);if err!=nil{return g.Introduction[h.Language,T]{},err}
	var bindings []g.FormalBinding[h.Language,T]
	for i,f:=range r.Open().Formals{w,err:=formals.WantFor(f.Binder);if err!=nil{return g.Introduction[h.Language,T]{},err};symbol,err:=m.target.Formal(l,i);if err!=nil{return g.Introduction[h.Language,T]{},err};payload,err:=m.target.Reference(symbol,nil,origins[i]);if err!=nil{return g.Introduction[h.Language,T]{},err};token,err:=m.issue(ctx,w,payload);if err!=nil{return g.Introduction[h.Language,T]{},err};bindings=append(bindings,g.FormalBinding[h.Language,T]{Binder:f.Binder,Token:token})}
	return formals.Finish(bindings)
}
func(m *structural[T]) Use(_ context.Context,r g.UseRequest[h.Language,T])(g.Product[h.Language,T],error){return r.Apply()}
func(m *structural[T]) Apply(ctx context.Context,r g.ApplyRequest[h.Language,T])(g.Product[h.Language,T],error){
	var args []g.Fragment[T]
	for _,f:=range r.Callee().Open().Formals{p,err:=r.Represent(f.Binder);if err!=nil{return g.Product[h.Language,T]{},err};args=append(args,p.Payload())}
	symbol,err:=m.target.Named(r.Callee());if err!=nil{return g.Product[h.Language,T]{},err};view,err:=m.source.Read(r.Want().Use().Use);if err!=nil{return g.Product[h.Language,T]{},err};payload,err:=m.target.Reference(symbol,args,view.Origin);if err!=nil{return g.Product[h.Language,T]{},err};token,err:=m.issue(ctx,r.Want(),payload);if err!=nil{return g.Product[h.Language,T]{},err};return r.Return(token)
}
func(m *structural[T]) Define(_ context.Context,r g.DefineRequest[h.Language,T])(g.Definition[h.Language,T],error){
	p,err:=r.Use(r.Open().Body);if err!=nil{return g.Definition[h.Language,T]{},err}
	for i:=range r.Open().Formals{if !m.target.UsesFormal(p.Payload(),r.Callee().Layout(),i){return g.Definition[h.Language,T]{},m.source.Unsupported(r.Open().Body,"unused native generic parameters require an explicit representation")}}
	return r.CompleteBody(p.Payload())
}
func Exported(name string)(string,error){
	if name==""||name=="_"||strings.Contains(name,"__"){return "",fmt.Errorf("unsupported identifier %q",name)}
	for i,b:=range []byte(name){if !(b>='a'&&b<='z'||b>='A'&&b<='Z'||b=='_'||i>0&&b>='0'&&b<='9'){return "",fmt.Errorf("unsupported identifier %q",name)}}
	if name[0]=='_'{name="Type"+name}else if name[0]>='a'&&name[0]<='z'{name=string(name[0]-32)+name[1:]};if name=="Self"{name+="_"};return name,nil
}
