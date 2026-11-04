package declmodel

import (
	"crypto/sha256"
	"slices"

	codec "github.com/bitspark/ontos/codec/go"
	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

func Resolve(root core.Value,dependencies []core.Value,limits Limits,grammar Grammar)(*Program,error){
	if !limits.valid(){return nil,failure(InvalidOptions,"invalid limits")}
	if len(dependencies)>=limits.MaxModules{return nil,failure(Exhausted,"supplied module limit reached")}
	input:=&budget{limits:limits};work:=&budget{limits:limits};pool:=map[Digest]core.Value{}
	all:=append([]core.Value{root},dependencies...);all=append(all,grammar.Prelude)
	var rootPin,preludePin Digest
	for i,v:=range all{if err:=input.value(v,0);err!=nil{if detail,ok:=err.(*Error);ok&&detail.Code==Invalid&&i>0{copy:=*detail;copy.Subject=SubjectContext;return nil,&copy};return nil,err};pin:=sha256.Sum256(codec.Encode(v));if old,ok:=pool[pin];ok&&!core.Equal(old,v){return nil,failure(Invalid,"distinct module values have the same identity")};pool[pin]=v;if i==0{rootPin=pin};if i==len(all)-1{preludePin=pin}}
	p:=&Program{grammar:grammar,root:rootPin,prelude:preludePin,limits:limits,modules:map[Digest]*Module{},declarations:map[DeclarationID]*Declaration{}}
	state:=map[Digest]uint8{};var load func(Digest,int)error
	load=func(pin Digest,depth int)error{
		if err:=work.step(depth);err!=nil{return err};if state[pin]==1{return sourceFailure(&Error{Code:Cycle,Subject:SubjectSource,Module:pin,Message:"cyclic module imports"},p.root)};if state[pin]==2{return nil};v,ok:=pool[pin];if !ok{return &Error{Code:MissingDependency,Subject:SubjectContext,Dependency:pin,Message:"missing pinned module"}}
		if len(p.modules)>=limits.MaxModules{return failure(Exhausted,"module closure limit reached")};m,err:=readModule(pin,v,grammar,work);if err!=nil{return sourceFailure(locate(err,DeclarationID{Module:pin},nil),rootPin)};p.modules[pin]=m;state[pin]=1
		for _,i:=range m.imports{if err:=load(i.pin,depth+1);err!=nil{if e,ok:=err.(*Error);ok&&e.Subject==SubjectContext&&e.ImportAlias==""&&e.Declaration==""{v:=*e;v.Module=pin;v.ImportAlias=i.alias;v.Dependency=i.pin;v.ExpectedName=i.name;return &v};return err};if p.modules[i.pin].name!=i.name{return &Error{Code:ImportMismatch,Subject:SubjectContext,Module:pin,ImportAlias:i.alias,Dependency:i.pin,ExpectedName:i.name,ActualName:p.modules[i.pin].name,Message:"import module name differs from pinned module"}}}
		state[pin]=2;return nil
	}
	if err:=load(preludePin,0);err!=nil{return nil,err};if err:=load(rootPin,0);err!=nil{return nil,err}
	for _,m:=range p.modules{p.order=append(p.order,m);for _,d:=range m.declarations{p.declarations[d.id]=d}}
	slices.SortFunc(p.order,func(a,b *Module)int{return slices.Compare(a.pin[:],b.pin[:])})
	r:=&expressionReader{program:p,budget:work,edges:map[DeclarationID][]DeclarationID{}}
	for _,m:=range p.order{for _,d:=range m.declarations{
		if d.form==KindDefinition{k,err:=r.readKind(d.body,d,nil,nil,0);if err!=nil{return nil,sourceFailure(regionError(err,d.id,"kind-body",0,nil),p.root)};if !sameSort(k.sort,d.sort){return nil,sourceFailure(regionError(failure(KindMismatch,"kind body sort differs from declared sort"),d.id,"kind-body",0,nil),p.root)};d.kindBody=k;continue}
		for i,v:=range d.parameterValues{k,err:=r.readKind(v,d,nil,nil,0);if err!=nil{return nil,sourceFailure(regionError(err,d.id,"parameter-kind",i,nil),p.root)};d.parameters[i].Kind=k}
		k,err:=r.readKind(d.resultValue,d,nil,nil,0);if err!=nil{return nil,sourceFailure(regionError(err,d.id,"result",0,nil),p.root)};d.result=k;d.contract=k;d.sort=k.sort
		if len(d.parameters)>0{inputs:=make([]*Kind,len(d.parameters));for i,x:=range d.parameters{inputs[i]=x.Kind};d.contract=arrowKind(inputs,k);d.sort=d.contract.sort}
	}}
	for _,m:=range p.order{for _,d:=range m.declarations{if d.form!=TypeDefinition{continue};e,skeleton,err:=r.read(d.body,d,nil,0);if err==nil&&!sameSort(e.sort,d.result.sort){err=failure(KindMismatch,"body sort differs from result kind")};if err==nil&&e.sort.IsProper(){err=grammar.WF(skeleton)};if err==nil{err=bindGround(e,work)};if err!=nil{return nil,sourceFailure(regionError(err,d.id,"body",0,nil),p.root)};d.expression=e}}
	if err:=r.acyclic();err!=nil{return nil,err}
	c:=newChecker(p,work)
	for _,d:=range p.declarationOrder{
		if d.form==KindDefinition{args:=make([]*kindTerm,len(d.kindParameters));for i,x:=range d.kindParameters{args[i]=topTerm(x.Sort)};t,err:=c.instantiate(d.kindBody,args,nil,0);if err==nil{err=c.admitKind(t,0)};if err!=nil{return nil,sourceFailure(regionError(err,d.id,"kind-body",0,nil),p.root)};continue}
		t,err:=c.instantiate(d.contract,nil,nil,0);if err==nil{err=c.admitKind(t,0)};if err!=nil{return nil,sourceFailure(regionError(err,d.id,"result",0,nil),p.root)};d.semantic=t
	}
	preludeOwner:=&Declaration{id:DeclarationID{Module:p.prelude}};g,err:=r.readKind(grammar.GroundKind,preludeOwner,nil,nil,0);if err!=nil{return nil,err};p.ground,err=c.instantiate(g,nil,nil,0);if err!=nil{return nil,err};if err=c.admitKind(p.ground,0);err!=nil{return nil,err}
	if err:=c.checkDeclarations();err!=nil{return nil,err};return p,nil
}

func locate(err error,id DeclarationID,path []int)error{if e,ok:=err.(*Error);ok&&e.Declaration==""{r:=*e;r.Module=id.Module;r.Declaration=id.Name;r.Path=append(slices.Clone(path),e.Path...);return &r};return err}
func regionError(err error,id DeclarationID,region string,index int,path []int)error{err=locate(err,id,path);if e,ok:=err.(*Error);ok&&e.Region==""{r:=*e;r.Region=region;r.ParameterIndex=index;return &r};return err}
func sourceFailure(err error,root Digest)error{if e,ok:=err.(*Error);ok&&e.Subject==SubjectSource&&e.Module!=root{r:=*e;r.Subject=SubjectContext;r.Dependency=e.Module;return &r};return err}

func readModule(pin Digest,value core.Value,g Grammar,work *budget)(*Module,error){
	a,ok:=g.form(value,"module",4);if !ok{return nil,failure(Invalid,"expected revision-4 module")};rev,ok:=index(a[0]);name,err:=data.ReadText(a[1]);if !ok||rev!=4||err!=nil||!moduleName(name){return nil,failure(Invalid,"invalid module revision or name")}
	imports,err:=data.ReadMap(a[2]);if err!=nil{return nil,failure(Invalid,"imports must be a canonical map")};decls,err:=data.ReadMap(a[3]);if err!=nil{return nil,failure(Invalid,"declarations must be a canonical map")};m:=&Module{pin:pin,name:name,value:value,importByName:map[string]Import{}}
	for _,entry:=range imports{alias,e:=data.ReadText(entry[0]);v,ok:=g.form(entry[1],"import",2);if e!=nil||!identifier(alias)||!ok{return nil,failure(Invalid,"invalid import")};n,e:=data.ReadText(v[0]);p,ok:=digest(v[1]);if e!=nil||!moduleName(n)||!ok{return nil,failure(Invalid,"invalid import name or identity")};i:=Import{alias:alias,name:n,pin:p};m.imports=append(m.imports,i);m.importByName[alias]=i}
	for _,entry:=range decls{
		if err:=work.step(0);err!=nil{return nil,err};name,e:=data.ReadText(entry[0]);tag,v,ok:=head(entry[1],g.DeclarationPrefix);if e!=nil||!identifier(name)||!ok||len(v)!=3||(tag!="type"&&tag!="kind"){return nil,failure(Invalid,"invalid declaration")};xs,e:=data.ReadList(v[0]);if e!=nil{return nil,failure(Invalid,"declaration parameters must be a canonical list")};d:=&Declaration{id:DeclarationID{Module:pin,Name:name},value:entry[1],form:DeclarationForm(tag),body:v[2],resultValue:v[1]};names:=map[string]bool{}
		if d.form==KindDefinition{s,e:=readSort(v[1],g,work,0);if e!=nil{return nil,regionError(e,d.id,"result-sort",0,nil)};d.sort=s}
		for _,x:=range xs{expected:="parameter";if d.form==KindDefinition{expected="kind-parameter"};parts,ok:=g.form(x,expected,2);if !ok{return nil,failure(Invalid,"invalid declaration parameter")};n,e:=data.ReadText(parts[0]);if e!=nil||!identifier(n)||names[n]{return nil,failure(Invalid,"invalid or duplicate parameter name")};names[n]=true;if d.form==KindDefinition{s,e:=readSort(parts[1],g,work,0);if e!=nil{return nil,regionError(e,d.id,"kind-parameter-sort",len(d.kindParameters),nil)};d.kindParameters=append(d.kindParameters,KindParameter{Name:n,Sort:s})}else{d.parameters=append(d.parameters,TypeParameter{Name:n});d.parameterValues=append(d.parameterValues,parts[1])}}
		m.declarations=append(m.declarations,d)
	};return m,nil
}

type expressionReader struct{program *Program;budget *budget;edges map[DeclarationID][]DeclarationID}
func(r *expressionReader)targetCategory(value core.Value,owner DeclarationID,category DeclarationForm,kind bool)(DeclarationID,error){
	g:=r.program.grammar;local,imported:="local","imported";if kind{local="kind-local";imported="kind-imported"};pin:=owner.Module;alias:="";name:=""
	if a,ok:=g.form(value,local,1);ok{var e error;name,e=data.ReadText(a[0]);if e!=nil||!identifier(name){return DeclarationID{},failure(Invalid,"invalid local reference")}}else if a,ok:=g.form(value,imported,2);ok{var e error;alias,e=data.ReadText(a[0]);name,_=data.ReadText(a[1]);if e!=nil||!identifier(alias)||!identifier(name){return DeclarationID{},failure(Invalid,"invalid imported reference")};m:=r.program.modules[owner.Module];i,ok:=m.importByName[alias];if !ok{return DeclarationID{},&Error{Code:UnknownImport,Subject:SubjectSource,ImportAlias:alias,Message:"unknown import alias"}};pin=i.pin}else{return DeclarationID{},failure(Invalid,"expected a named reference")}
	id:=DeclarationID{Module:pin,Name:name};d,ok:=r.program.declarations[id];if !ok{return DeclarationID{},&Error{Code:UnknownDeclaration,Subject:SubjectSource,ImportAlias:alias,Target:id,Message:"unknown declaration"}};if d.form!=category{return DeclarationID{},&Error{Code:KindMismatch,Subject:SubjectSource,Target:id,Message:"declaration category mismatch"}};return id,nil
}
func(r *expressionReader)read(value core.Value,owner *Declaration,path []int,depth int)(*Expression,core.Value,error){e,s,err:=r.readIn(value,owner,path,depth);if err!=nil{return nil,nil,locate(err,owner.id,path)};e.path=slices.Clone(path);return e,s,nil}
func(r *expressionReader)readIn(value core.Value,owner *Declaration,path []int,depth int)(*Expression,core.Value,error){
	if err:=r.budget.step(depth);err!=nil{return nil,nil,err};g:=r.program.grammar
	if a,ok:=g.form(value,"param",1);ok{slot,valid:=index(a[0]);if !valid||slot>=len(owner.parameters){return nil,nil,failure(Invalid,"unbound type parameter")};k:=owner.parameters[slot].Kind;e:=&Expression{kind:Parameter,value:value,parameter:ParameterID{Declaration:owner.id,Index:slot},sort:k.sort,contract:k};if k.sort.IsProper(){return e,g.Any(),nil};return e,nil,nil}
	_,local:=g.form(value,"local",1);_,imported:=g.form(value,"imported",2);if local||imported{id,err:=r.targetCategory(value,owner.id,TypeDefinition,false);if err!=nil{return nil,nil,err};d:=r.program.declarations[id];r.edges[owner.id]=append(r.edges[owner.id],id);e:=&Expression{kind:Reference,value:value,target:id,sort:d.sort,contract:d.contract};if d.sort.IsProper(){return e,g.Any(),nil};return e,nil,nil}
	if a,ok:=g.form(value,"apply",2);ok{
		callee,_,err:=r.read(a[0],owner,append(slices.Clone(path),1),depth+1);if err!=nil{return nil,nil,err};if callee.sort.IsProper(){return nil,nil,locate(applicationFailure(KindMismatch,callee,nil,"application callee is not a constructor"),owner.id,append(slices.Clone(path),1))}
		xs,err:=data.ReadList(a[1]);if err!=nil||len(xs)==0||len(xs)!=len(callee.sort.inputs){return nil,nil,applicationFailure(Invalid,callee,nil,"application must exactly saturate its callee")};e:=&Expression{kind:Application,value:value,callee:callee,sort:*callee.sort.result,contract:topKind(*callee.sort.result)}
		for i,x:=range xs{childPath:=append(slices.Clone(path),2,i+1);child,skeleton,err:=r.read(x,owner,childPath,depth+1);if err==nil&&!sameSort(child.sort,callee.sort.inputs[i]){err=applicationFailure(KindMismatch,callee,nil,"application argument sort mismatch")};if err==nil&&child.sort.IsProper(){err=g.WF(skeleton)};if err!=nil{return nil,nil,locate(err,owner.id,childPath)};e.arguments=append(e.arguments,child)}
		if e.sort.IsProper(){return e,g.Any(),nil};return e,nil,nil
	}
	tag,args,ok:=head(value,g.DescriptorPrefix);if !ok{return nil,nil,failure(Invalid,"expected a type expression")};positions,err:=g.Positions(tag,args);if err!=nil{return nil,nil,err};e:=&Expression{kind:Descriptor,value:value,constructor:tag,contract:topKind(Sort{})};var children []core.Value
	for _,position:=range positions{childPath:=append(slices.Clone(path),position...);x,v,err:=r.read(at(value,position),owner,childPath,depth+1);if err!=nil{return nil,nil,err};if !x.sort.IsProper(){return nil,nil,locate(failure(KindMismatch,"primitive operands must be proper types"),owner.id,childPath)};e.operands=append(e.operands,Operand{path:position,expression:x});children=append(children,v)}
	return e,replaceOperands(value,e.operands,children),nil
}
func(r *expressionReader)acyclic()error{state:=map[DeclarationID]uint8{};var visit func(DeclarationID,int)error;visit=func(id DeclarationID,depth int)error{if err:=r.budget.step(depth);err!=nil{return err};if state[id]==1{return locate(failure(Cycle,"cyclic named declaration references"),id,nil)};if state[id]==2{return nil};state[id]=1;for _,target:=range r.edges[id]{if err:=visit(target,depth+1);err!=nil{return err}};state[id]=2;r.program.declarationOrder=append(r.program.declarationOrder,r.program.declarations[id]);return nil};for _,m:=range r.program.order{for _,d:=range m.declarations{if err:=visit(d.id,0);err!=nil{return sourceFailure(err,r.program.root)}}};return nil}
func applicationFailure(code Code,callee *Expression,expected *Kind,message string)error{e:=&Error{Code:code,Subject:SubjectSource,Expected:copyKind(expected),Message:message};if callee!=nil{if callee.kind==Reference{e.Target=callee.target};if callee.kind==Parameter{e.CalleeParameter=callee.parameter}};return e}
