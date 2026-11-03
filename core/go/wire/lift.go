package wire

import (
	"errors"
	"slices"

	ground "github.com/Bitspark/horos/core/go/declarations"
	model "github.com/Bitspark/horos/core/go/internal/declmodel"
	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

const LiftingRevision = 1

type LiftOptions struct {
	Limits           Limits
	ModuleNames      map[ground.Digest]string
	DeclarationNames map[ground.DeclarationID]string
}

func DefaultLiftOptions() LiftOptions { return LiftOptions{Limits: DefaultLimits()} }

type DeclarationMapping struct {
	Source ground.DeclarationID
	Target Reference
}
type ParameterMapping struct {
	Source ground.ParameterID
	Target Reference
	Index  int
}
type Origin struct {
	Source     ground.DeclarationID
	SourcePath []int
	Target     Reference
	TargetPath []int
	Rule       string
}
type DerivedModule struct {
	source ground.Digest
	pin    Digest
	value  core.Value
}

func (m DerivedModule) Source() ground.Digest { return m.source }
func (m DerivedModule) Pin() Digest           { return m.pin }
func (m DerivedModule) Value() core.Value     { return m.value }

// Derivation owns the complete generated module set and its source/slot/path
// correspondence. Absorbed source dependencies remain in Sources. Roots can be
// in different source modules; callers explicitly choose their output entry.
type Derivation struct {
	sourceRoot   ground.Digest
	sources      []ground.Digest
	modules      []DerivedModule
	roots        []Reference
	declarations []DeclarationMapping
	parameters   []ParameterMapping
	origins      []Origin
}

func (d *Derivation) SourceRoot() ground.Digest          { return d.sourceRoot }
func (d *Derivation) Sources() []ground.Digest           { return slices.Clone(d.sources) }
func (d *Derivation) Modules() []DerivedModule           { return slices.Clone(d.modules) }
func (d *Derivation) Roots() []Reference                 { return slices.Clone(d.roots) }
func (d *Derivation) Declarations() []DeclarationMapping { return slices.Clone(d.declarations) }
func (d *Derivation) Parameters() []ParameterMapping     { return slices.Clone(d.parameters) }
func (d *Derivation) Origins() []Origin {
	result := slices.Clone(d.origins)
	for i := range result {
		result[i].SourcePath = slices.Clone(result[i].SourcePath)
		result[i].TargetPath = slices.Clone(result[i].TargetPath)
	}
	return result
}

// Program checks a generated entry's full import closure. It does not infer an
// entry from the presence of Wire or merge independent modules into one module.
func (d *Derivation) Program(entry Digest, limits Limits) (*Program, error) {
	if d == nil {
		return nil, model.Failure(InvalidOptions, "missing derivation")
	}
	var root core.Value
	var dependencies []core.Value
	for _, m := range d.modules {
		if m.pin == entry {
			root = m.value
		} else {
			dependencies = append(dependencies, m.value)
		}
	}
	if root == nil {
		return nil, model.Failure(InvalidOptions, "entry is not a derived module")
	}
	return ResolveWithLimits(root, dependencies, limits)
}

// Embed is the explicitly selected closed-embedding operation. The checked
// source program, declaration and ordered arguments remain caller-held origins;
// a failed Lift never invokes this operation as a fallback.
func Embed(source *ground.Program, id ground.DeclarationID, arguments []core.Value, limits Limits) (core.Value, error) {
	checked, err := readSource(source, limits)
	if err != nil {
		return nil, err
	}
	value, err := checked.Specialize(id, arguments)
	if err != nil {
		return nil, err
	}
	result := tagged("data", value)
	if err := WF(result, limits); err != nil {
		return nil, err
	}
	return result, nil
}

func readSource(source *ground.Program, limits Limits) (*ground.Program, error) {
	if source == nil {
		return nil, model.Failure(InvalidOptions, "missing ground program")
	}
	root, ok := source.Module(source.Root())
	if !ok {
		return nil, model.Failure(InvalidOptions, "unresolved ground program")
	}
	var dependencies []core.Value
	for _, m := range source.Modules() {
		if m.Pin() != source.Root() {
			dependencies = append(dependencies, m.Value())
		}
	}
	// The entire original transitive closure is checked before any output pruning.
	return ground.ResolveWithLimits(root.Value(), dependencies, limits)
}

type liftState struct {
	source           *ground.Program
	options          LiftOptions
	work             *model.Work
	closed           *model.Work
	bodies           map[ground.DeclarationID]core.Value
	active           map[ground.DeclarationID]bool
	imports          map[ground.Digest]map[string]ground.Digest
	names            map[ground.Digest]string
	declarationNames map[ground.DeclarationID]string
	origins          []Origin
	metadataBytes    int
}

// Lift implements the ordered partial mapping in docs/spec/wire-types.md.
// It preserves applications before considering closed embedding. Every failure
// returns a nil derivation, including unsupported forms and budget exhaustion.
func Lift(source *ground.Program, roots []ground.DeclarationID, options LiftOptions) (*Derivation, error) {
	work, err := model.NewWork(options.Limits)
	if err != nil {
		return nil, err
	}
	checked, err := readSource(source, options.Limits)
	if err != nil {
		return nil, err
	}
	if len(roots) == 0 || len(roots) > options.Limits.MaxNodes {
		return nil, model.Failure(InvalidOptions, "expected bounded selected roots")
	}
	closed, _ := model.NewWork(options.Limits)
	s := &liftState{source: checked, options: options, work: work, closed: closed,
		bodies: make(map[ground.DeclarationID]core.Value), active: make(map[ground.DeclarationID]bool),
		imports: make(map[ground.Digest]map[string]ground.Digest), names: make(map[ground.Digest]string), declarationNames: make(map[ground.DeclarationID]string)}
	// Capture configuration by constructing owned maps. Unknown configuration
	// entries are errors, even if their output would otherwise be pruned.
	configBytes := 0
	for pin, name := range options.ModuleNames {
		if _, ok := checked.Module(pin); !ok || !model.ModuleName(name) {
			return nil, model.Failure(InvalidOptions, "invalid module naming choice")
		}
		if len(name) > options.Limits.MaxBytes-configBytes {
			return nil, model.Failure(Exhausted, "naming byte budget exceeded")
		}
		configBytes += len(name)
		s.names[pin] = name
	}
	for id, name := range options.DeclarationNames {
		if _, ok := checked.Declaration(id); !ok || !model.Identifier(name) {
			return nil, model.Failure(InvalidOptions, "invalid declaration naming choice")
		}
		if len(name) > options.Limits.MaxBytes-configBytes {
			return nil, model.Failure(Exhausted, "naming byte budget exceeded")
		}
		configBytes += len(name)
		s.declarationNames[id] = name
	}
	seen := make(map[ground.DeclarationID]bool)
	for _, id := range roots {
		if seen[id] {
			return nil, model.Failure(InvalidOptions, "duplicate selected root")
		}
		seen[id] = true
		if _, err := s.declaration(id, 0); err != nil {
			return nil, err
		}
	}
	return s.finish(slices.Clone(roots))
}

func (s *liftState) located(err error, id ground.DeclarationID, path []int) error {
	var detail *Error
	if errors.As(err, &detail) && detail.Declaration == "" {
		copy := *detail
		copy.Module = id.Module
		copy.Declaration = id.Name
		copy.Path = slices.Clone(path)
		return &copy
	}
	return err
}
func (s *liftState) declaration(id ground.DeclarationID, depth int) (core.Value, error) {
	if err := s.work.Step(depth); err != nil {
		return nil, s.located(err, id, nil)
	}
	if body, ok := s.bodies[id]; ok {
		return body, nil
	}
	decl, ok := s.source.Declaration(id)
	if !ok {
		return nil, &Error{Code: UnknownDeclaration, Subject: SubjectRequest, Target: id, Message: "selected declaration is not in source"}
	}
	if s.active[id] {
		return nil, s.located(model.Failure(Cycle, "cyclic derived application"), id, nil)
	}
	s.active[id] = true
	body, err := s.expression(decl.Expression(), id, nil, nil, depth+1)
	if err != nil {
		return nil, err
	}
	delete(s.active, id)
	s.bodies[id] = body
	return body, nil
}
func (s *liftState) name(id ground.DeclarationID) string {
	if name, ok := s.declarationNames[id]; ok {
		return name
	}
	return id.Name
}
func (s *liftState) origin(id ground.DeclarationID, sourcePath, targetPath []int, rule string) error {
	cost := 32 + len(id.Name) + len(rule) + 8*(len(sourcePath)+len(targetPath))
	if cost > s.options.Limits.MaxBytes-s.metadataBytes {
		return s.located(model.Failure(Exhausted, "provenance byte budget exceeded"), id, sourcePath)
	}
	s.metadataBytes += cost
	s.origins = append(s.origins, Origin{Source: id, SourcePath: slices.Clone(sourcePath), TargetPath: slices.Clone(targetPath), Rule: rule})
	return nil
}
func (s *liftState) expression(e *ground.Expression, owner ground.DeclarationID, path, targetPath []int, depth int) (core.Value, error) {
	if err := s.work.Step(depth); err != nil {
		return nil, s.located(err, owner, path)
	}
	switch e.Kind() {
	case ground.Parameter:
		binder, _ := e.Parameter()
		if binder.Declaration != owner {
			return nil, s.located(invalid("foreign lexical binder"), owner, path)
		}
		if err := s.origin(owner, path, targetPath, "parameter"); err != nil {
			return nil, err
		}
		return tagged("param", e.Value().(core.Tuple).At(1)), nil
	case ground.Application:
		target, _ := e.Target()
		if _, err := s.declaration(target, depth+1); err != nil {
			return nil, err
		}
		var reference core.Value
		_, application, _ := model.Head(e.Value(), "horos/decl/")
		scope, ref, _ := model.Head(application[0], "horos/decl/")
		if scope == "local" {
			reference = tagged("local", data.MustEncodeText(s.name(target)))
		} else {
			alias, _ := data.ReadText(ref[0])
			if s.imports[owner.Module] == nil {
				s.imports[owner.Module] = make(map[string]ground.Digest)
			}
			s.imports[owner.Module][alias] = target.Module
			reference = tagged("imported", ref[0], data.MustEncodeText(s.name(target)))
		}
		arguments := e.Arguments()
		values := make([]core.Value, len(arguments))
		for i, argument := range arguments {
			v, err := s.expression(argument, owner, append(slices.Clone(path), 2, i+1), append(slices.Clone(targetPath), 2, i+1), depth+1)
			if err != nil {
				return nil, err
			}
			values[i] = v
		}
		if err := s.origin(owner, path, targetPath, "application"); err != nil {
			return nil, err
		}
		return tagged("apply", reference, data.EncodeList(values)), nil
	}
	constructor := e.Constructor()
	operands := e.Operands()
	if constructor == "union" && len(operands) == 1 {
		if err := s.origin(owner, path, targetPath, "singleton-union"); err != nil {
			return nil, err
		}
		return s.expression(operands[0].Expression(), owner, append(slices.Clone(path), 1), targetPath, depth+1)
	}
	structural := constructor == "tuple" || constructor == "compound" || constructor == "list-of" || constructor == "union" && len(operands) >= 2
	if constructor == "record" {
		structural = atom(e.Value().(core.Tuple).At(1), "closed")
	}
	if structural {
		parts := e.Value().(core.Tuple).Items()
		parts[0] = core.NewAtom([]byte("horos/wire/" + constructor))
		for _, operand := range operands {
			position := operand.Path()
			v, err := s.expression(operand.Expression(), owner, append(slices.Clone(path), position...), append(slices.Clone(targetPath), position...), depth+1)
			if err != nil {
				return nil, err
			}
			if len(position) == 1 {
				parts[position[0]] = v
			} else {
				field := parts[position[0]].(core.Tuple).Items()
				field[0] = core.NewAtom([]byte("horos/wire/field"))
				field[position[1]] = v
				parts[position[0]] = core.NewTuple(field...)
			}
		}
		if err := s.origin(owner, path, targetPath, "structural"); err != nil {
			return nil, err
		}
		return core.NewTuple(parts...), nil
	}
	closed, err := s.source.Close(e)
	if err != nil {
		var detail *Error
		if errors.As(err, &detail) && detail.Code == Exhausted {
			return nil, s.located(err, owner, path)
		}
		return nil, &Error{Code: UnsupportedLift, Subject: SubjectRequest, Module: owner.Module, Declaration: owner.Name, Path: slices.Clone(path), Message: "open or independently unclosed form has no template lift"}
	}
	// Count each closed output occurrence across the request before retaining it.
	if err := s.closed.Value(closed); err != nil {
		return nil, s.located(err, owner, path)
	}
	if err := s.origin(owner, path, targetPath, "data"); err != nil {
		return nil, err
	}
	return tagged("data", closed), nil
}

func (s *liftState) finish(roots []ground.DeclarationID) (*Derivation, error) {
	result := &Derivation{sourceRoot: s.source.Root()}
	retained := make(map[ground.Digest][]*ground.Declaration)
	for _, m := range s.source.Modules() {
		result.sources = append(result.sources, m.Pin())
		for _, d := range m.Declarations() {
			if _, ok := s.bodies[d.ID()]; ok {
				retained[m.Pin()] = append(retained[m.Pin()], d)
			}
		}
		if _, ok := s.names[m.Pin()]; !ok {
			s.names[m.Pin()] = m.Name() + ".wire"
		}
	}
	names := make(map[string]bool)
	for pin, decls := range retained {
		if names[s.names[pin]] {
			return nil, model.Failure(InvalidOptions, "derived module name collision")
		}
		names[s.names[pin]] = true
		used := make(map[string]bool)
		for _, d := range decls {
			name := s.name(d.ID())
			if used[name] {
				return nil, model.Failure(InvalidOptions, "derived declaration name collision")
			}
			used[name] = true
		}
	}
	values := make(map[ground.Digest]DerivedModule)
	output, _ := model.NewWork(s.options.Limits)
	var build func(ground.Digest, int) error
	build = func(pin ground.Digest, depth int) error {
		if err := s.work.Step(depth); err != nil {
			return err
		}
		if _, ok := values[pin]; ok {
			return nil
		}
		original, _ := s.source.Module(pin)
		var imports [][2]core.Value
		for _, imp := range original.Imports() {
			dep, used := s.imports[pin][imp.Alias()]
			if !used {
				continue
			}
			if err := build(dep, depth+1); err != nil {
				return err
			}
			derived := values[dep]
			imports = append(imports, [2]core.Value{data.MustEncodeText(imp.Alias()), tagged("import", data.MustEncodeText(s.names[dep]), core.NewAtom(derived.pin[:]))})
		}
		var declarations [][2]core.Value
		for _, d := range retained[pin] {
			parameters := d.Parameters()
			params := make([]core.Value, len(parameters))
			for i, name := range parameters {
				params[i] = data.MustEncodeText(name)
			}
			declarations = append(declarations, [2]core.Value{data.MustEncodeText(s.name(d.ID())), tagged("type", data.EncodeList(params), s.bodies[d.ID()])})
		}
		imps, err := data.EncodeMap(imports)
		if err != nil {
			return err
		}
		decls, err := data.EncodeMap(declarations)
		if err != nil {
			return err
		}
		// Reuse the source's exact revision integer spelling (revision 1).
		v := tagged("module", original.Value().(core.Tuple).At(1), data.MustEncodeText(s.names[pin]), imps, decls)
		if err := output.Value(v); err != nil {
			return err
		}
		p, err := PinWithLimits(v, s.options.Limits)
		if err != nil {
			return err
		}
		values[pin] = DerivedModule{source: pin, pin: p, value: v}
		result.modules = append(result.modules, values[pin])
		return nil
	}
	for _, m := range s.source.Modules() {
		if _, ok := retained[m.Pin()]; ok {
			if err := build(m.Pin(), 0); err != nil {
				return nil, err
			}
		}
	}
	refs := make(map[ground.DeclarationID]Reference)
	for _, m := range s.source.Modules() {
		for _, decl := range retained[m.Pin()] {
			ref := Reference{Profile: Profile, Revision: Revision, Declaration: DeclarationID{Module: values[m.Pin()].pin, Name: s.name(decl.ID())}}
			refs[decl.ID()] = ref
			result.declarations = append(result.declarations, DeclarationMapping{Source: decl.ID(), Target: ref})
			for i := range decl.Parameters() {
				result.parameters = append(result.parameters, ParameterMapping{Source: ground.ParameterID{Declaration: decl.ID(), Index: i}, Target: ref, Index: i})
			}
		}
	}
	for _, root := range roots {
		result.roots = append(result.roots, refs[root])
	}
	for _, origin := range s.origins {
		origin.Target = refs[origin.Source]
		result.origins = append(result.origins, origin)
	}
	// Every generated entry must resolve under the same requested bounds before
	// a Derivation is usable, including disconnected selected roots.
	validation, _ := model.NewWork(s.options.Limits)
	checkedEntries := make(map[Digest]bool)
	for _, root := range result.roots {
		if checkedEntries[root.Declaration.Module] {
			continue
		}
		checkedEntries[root.Declaration.Module] = true
		// Each entry validation reads the supplied bundle. Charge repeated work
		// across independent roots rather than multiplying an unbounded budget.
		for _, m := range result.modules {
			if err := validation.Value(m.value); err != nil {
				return nil, err
			}
		}
		if _, err := result.Program(root.Declaration.Module, s.options.Limits); err != nil {
			return nil, err
		}
	}
	return result, nil
}
