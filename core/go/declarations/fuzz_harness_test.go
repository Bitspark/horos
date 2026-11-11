package declarations_test

// The fuzz session: resolve one module set, plan a bounded sequence of
// queries over the published Program, check each result's invariants, and
// keep a rendered record of every outcome so that two sessions can be
// compared (determinism) and a plan can be replayed under smaller limits.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	horos "github.com/Bitspark/horos/core/go"
	decl "github.com/Bitspark/horos/core/go/declarations"
	codec "github.com/bitspark/ontos/codec/go"
	core "github.com/bitspark/ontos/core/go"
)

// Bounds that keep one fuzz execution in the low milliseconds.
const (
	maxSessionValues = 24
	maxSessionKinds  = 6
	maxApplyRounds   = 2
	// Ordered pairs of distinct kinds compared by Includes.
	maxInclusionPairs = 24
	// Query items per FuzzQueries input and unions/intersections among them.
	maxQueryItems   = 6
	maxCombinations = 3
	// Exhausted operations after which a session stops materializing.
	maxSessionExhausted = 4
)

type opKind uint8

const (
	opValue opKind = iota
	opProper
	opCompile
	opDescriptor
	opGround
	opApply
	opSatisfies
	opIncludes
)

var opNames = [...]string{"Value", "Proper", "CompileKind", "Descriptor", "GroundDescriptor", "Apply", "Satisfies", "Includes"}

// expectation is a property one operation must establish in addition to the
// invariants every operation keeps.
type expectation uint8

const (
	expectNothing expectation = iota
	// expectKindMismatch: unsaturated, oversaturated, ill-sorted or proper
	// callee applications fail with KindMismatch (README "Checked kinds").
	expectKindMismatch
	// expectProven: the verdict is Proven unless the operation is exhausted.
	expectProven
	// expectClosedVerdict: closed proper membership is exact (kinds.md §6),
	// so the verdict is Proven or Refuted.
	expectClosedVerdict
	// expectSuccess: the operation succeeds unless exhausted.
	expectSuccess
)

type op struct {
	kind   opKind
	out    string // label of the produced handle, if any
	in     []string
	id     decl.DeclarationID
	scope  decl.Digest
	value  core.Value
	expect expectation
}

func (o op) String() string {
	var b strings.Builder
	b.WriteString(opNames[o.kind])
	if o.out != "" {
		fmt.Fprintf(&b, " out=%s", o.out)
	}
	if len(o.in) > 0 {
		fmt.Fprintf(&b, " in=%v", o.in)
	}
	if o.id.Name != "" {
		fmt.Fprintf(&b, " id=%x:%s", o.id.Module[:4], o.id.Name)
	}
	if o.value != nil {
		fmt.Fprintf(&b, " value=%s", digestValue(o.value))
	}
	if o.expect != expectNothing {
		fmt.Fprintf(&b, " expect=%d", o.expect)
	}
	return b.String()
}

// record is one rendered outcome. code is empty on success.
type record struct {
	text string
	code decl.Code
}

const skipped decl.Code = "fuzz-skipped"

type session struct {
	t         *testing.T
	name      string
	root      core.Value
	limits    decl.Limits
	program   *decl.Program
	outcome   string // rendered Resolve outcome
	code      decl.Code
	values    map[string]decl.TypeValue
	order     []string // value labels in creation order
	kinds     map[string]decl.KindHandle
	korder    []string
	results   map[string]core.Value // materialized descriptors by op index key
	plan      []op
	records   []record
	applied   int
	exhausted int
}

func emptySession(t *testing.T, name string) *session {
	return &session{t: t, name: name, limits: decl.DefaultLimits(), values: map[string]decl.TypeValue{}, kinds: map[string]decl.KindHandle{}, results: map[string]core.Value{}}
}

// programSession queries an already resolved Program.
func programSession(t *testing.T, name string, p *decl.Program) *session {
	s := emptySession(t, name)
	s.program = p
	s.limits = p.Limits()
	return s
}

func newSession(t *testing.T, name string, root core.Value, deps []core.Value, limits decl.Limits) *session {
	s := emptySession(t, name)
	s.root = root
	s.limits = limits
	p, err := decl.ResolveWithLimits(root, deps, limits)
	if err != nil {
		if p != nil {
			t.Fatalf("%s: Resolve returned a Program with error %v", name, err)
		}
		s.outcome, s.code = s.failure("Resolve", err)
		return s
	}
	if p == nil {
		t.Fatalf("%s: Resolve returned neither Program nor error", name)
	}
	if p.Limits() != limits {
		t.Fatalf("%s: Program retains limits %+v, requested %+v", name, p.Limits(), limits)
	}
	if want, err := decl.PinWithLimits(root, limits); err != nil || p.Root() != want {
		t.Fatalf("%s: Program root %x is not the root pin %x (%v)", name, p.Root(), want, err)
	}
	s.program = p
	s.outcome = renderProgram(p)
	return s
}

var knownCodes = map[decl.Code]bool{
	decl.Invalid: true, decl.BoundViolation: true, decl.KindMismatch: true,
	decl.UnprovedObligation: true, decl.MissingDependency: true, decl.ImportMismatch: true,
	decl.UnknownImport: true, decl.UnknownDeclaration: true, decl.Cycle: true, decl.Exhausted: true,
}
var knownSubjects = map[decl.Subject]bool{
	decl.SubjectSource: true, decl.SubjectContext: true, decl.SubjectConfiguration: true, decl.SubjectRequest: true,
}

// failure checks that err is a documented typed error and renders it. Valid
// limits never produce InvalidOptions.
func (s *session) failure(what string, err error) (string, decl.Code) {
	s.t.Helper()
	detail, ok := err.(*decl.Error)
	if !ok || detail == nil {
		s.t.Fatalf("%s: %s returned an untyped error %T: %v", s.name, what, err, err)
	}
	if !knownCodes[detail.Code] || !knownSubjects[detail.Subject] {
		s.t.Fatalf("%s: %s returned an undocumented code or subject: %+v", s.name, what, detail)
	}
	return renderError(detail), detail.Code
}

func (s *session) addValue(label string, v decl.TypeValue) {
	if _, ok := s.values[label]; !ok {
		s.order = append(s.order, label)
	}
	s.values[label] = v
}

func (s *session) addKind(label string, k decl.KindHandle) {
	if _, ok := s.kinds[label]; !ok {
		s.korder = append(s.korder, label)
	}
	s.kinds[label] = k
}

// do appends o to the plan, executes it and records its outcome.
func (s *session) do(o op) record {
	s.plan = append(s.plan, o)
	r := s.exec(o, len(s.plan)-1)
	s.records = append(s.records, r)
	if r.code == decl.Exhausted {
		s.exhausted++
	}
	return r
}

func (s *session) exec(o op, index int) record {
	s.t.Helper()
	for _, label := range o.in {
		_, value := s.values[label]
		_, kind := s.kinds[label]
		if !value && !kind {
			return record{text: "skipped", code: skipped}
		}
	}
	p := s.program
	fail := func(format string, args ...any) {
		s.t.Helper()
		s.t.Fatalf("%s: op %d %s: %s", s.name, index, o, fmt.Sprintf(format, args...))
	}
	var r record
	switch o.kind {
	case opValue, opProper, opApply:
		var v decl.TypeValue
		var err error
		switch o.kind {
		case opValue:
			v, err = p.Value(o.id)
		case opProper:
			v, err = p.Proper(o.value)
		default:
			args := make([]decl.TypeValue, len(o.in)-1)
			for i, label := range o.in[1:] {
				args[i] = s.values[label]
			}
			v, err = p.Apply(s.values[o.in[0]], args)
		}
		if err != nil {
			if v.Expression() != nil || v.Contract() != nil || !v.Sort().IsProper() {
				fail("failed with a usable partial value: %v", err)
			}
			r.text, r.code = s.failure(opNames[o.kind], err)
			break
		}
		if o.kind == opApply {
			callee := s.values[o.in[0]].Sort()
			want, ok := callee.Result()
			if !ok || !sameSort(v.Sort(), want) {
				fail("result sort %s differs from the callee's result sort %s", renderSort(v.Sort()), renderSort(callee))
			}
		}
		if o.out != "" {
			s.addValue(o.out, v)
		}
		r.text = "value " + renderTypeValue(v)
	case opCompile:
		k, err := p.CompileKind(o.scope, o.value)
		if err != nil {
			if k.Expression() != nil {
				fail("failed with a usable partial kind: %v", err)
			}
			r.text, r.code = s.failure("CompileKind", err)
			break
		}
		s.addKind(o.out, k)
		r.text = "kind " + renderSort(k.Sort()) + " " + renderKind(k.Expression())
	case opDescriptor, opGround:
		v := s.values[o.in[0]]
		var result core.Value
		var err error
		if o.kind == opGround {
			result, err = p.GroundDescriptor(v)
		} else {
			result, err = p.Descriptor(v)
		}
		if (err == nil) == (result == nil) {
			fail("result %v and error %v disagree", result != nil, err)
		}
		if err != nil {
			r.text, r.code = s.failure(opNames[o.kind], err)
			if !v.Sort().IsProper() && r.code != decl.KindMismatch {
				fail("a constructor value failed materialization with %s, not KindMismatch", r.code)
			}
			break
		}
		if !v.Sort().IsProper() {
			fail("a constructor value was materialized")
		}
		if o.kind == opGround && !horos.WF(result) {
			fail("GroundDescriptor returned a descriptor that fails horos.WF: %s", result)
		}
		s.results[fmt.Sprint(index)] = result
		r.text = "descriptor " + digestValue(result)
	case opSatisfies, opIncludes:
		var verdict decl.Verdict
		var err error
		if o.kind == opSatisfies {
			verdict, err = p.Satisfies(s.values[o.in[0]], s.kinds[o.in[1]])
		} else {
			verdict, err = p.Includes(s.kinds[o.in[0]], s.kinds[o.in[1]])
		}
		if err != nil {
			r.text, r.code = s.failure(opNames[o.kind], err)
			break
		}
		if verdict != decl.Proven && verdict != decl.Refuted && verdict != decl.Unproved {
			fail("undocumented verdict %q", verdict)
		}
		r.text = "verdict " + string(verdict)
	}
	s.expectation(o, r, fail)
	return r
}

func (s *session) expectation(o op, r record, fail func(string, ...any)) {
	switch o.expect {
	case expectKindMismatch:
		if r.code == decl.Exhausted {
			return
		}
		if r.code != decl.KindMismatch {
			fail("want KindMismatch, got %s", r.text)
		}
		if !strings.Contains(r.text, " subject=request ") {
			fail("want a request-subject KindMismatch, got %s", r.text)
		}
	case expectProven:
		if r.code != decl.Exhausted && r.text != "verdict proven" {
			fail("want Proven, got %s", r.text)
		}
	case expectClosedVerdict:
		if r.code != decl.Exhausted && r.text != "verdict proven" && r.text != "verdict refuted" {
			fail("closed proper membership must be exact, got %s", r.text)
		}
	case expectSuccess:
		if r.code != "" && r.code != decl.Exhausted {
			fail("want success, got %s", r.text)
		}
	}
}

// explore plans and runs the queries for a default-limit session. The plan
// depends only on earlier outcomes, so a deterministic resolver plans the
// same queries in every session.
func (s *session) explore(k knobs) {
	p := s.program
	if p == nil {
		return
	}
	prelude := decl.PreludePin()
	s.do(op{kind: opCompile, out: "k:Type", scope: prelude, value: kindLocalValue("Type"), expect: expectSuccess})
	s.do(op{kind: opCompile, out: "k:Data", scope: prelude, value: kindLocalValue("Data"), expect: expectSuccess})
	modules := p.Modules()
	ordered := make([]*decl.Module, 0, len(modules))
	for _, m := range modules {
		if m.Pin() == p.Root() {
			ordered = append([]*decl.Module{m}, ordered...)
		} else if m.Pin() != prelude {
			ordered = append(ordered, m)
		}
	}
	for mi, m := range ordered {
		for _, d := range m.Declarations() {
			if d.Form() == decl.KindDefinition && len(d.KindParameters()) == 0 && len(s.korder) < maxSessionKinds {
				s.do(op{kind: opCompile, out: fmt.Sprintf("k:%d:%s", mi, d.ID().Name), scope: m.Pin(), value: kindLocalValue(d.ID().Name)})
			}
		}
	}
	values := 0
	for mi, m := range ordered {
		for _, d := range m.Declarations() {
			if values >= maxSessionValues {
				break
			}
			if d.Form() == decl.TypeDefinition {
				values++
				s.do(op{kind: opValue, out: fmt.Sprintf("v:%d:%s", mi, d.ID().Name), id: d.ID()})
			} else if mi == 0 {
				// A kind definition is not a type value.
				s.do(op{kind: opValue, id: d.ID(), expect: expectKindMismatch})
			}
		}
	}
	for i, descriptor := range primitiveDescriptors() {
		s.do(op{kind: opProper, out: fmt.Sprintf("p:%d", i), value: descriptor, expect: expectSuccess})
	}
	examined := 0
	for round := 0; round <= maxApplyRounds; round++ {
		fresh := s.order[examined:]
		examined = len(s.order)
		for _, label := range fresh {
			s.examine(label)
		}
		if round < maxApplyRounds {
			for _, label := range fresh {
				s.applications(label, k)
			}
		}
	}
	s.inclusions()
}

// examine materializes one value and checks its ordinary memberships.
func (s *session) examine(label string) {
	v := s.values[label]
	closed := strings.HasPrefix(label, "p:") || strings.HasPrefix(label, "q:") || strings.HasSuffix(label, "~")
	if s.exhausted >= maxSessionExhausted {
		// Exhausting materializations run to the limit; a few per input
		// suffice and keep executions fast.
		return
	}
	if !v.Sort().IsProper() {
		s.do(op{kind: opDescriptor, in: []string{label}})
		s.do(op{kind: opGround, in: []string{label}})
		return
	}
	described := s.do(op{kind: opDescriptor, in: []string{label}})
	grounded := s.do(op{kind: opGround, in: []string{label}})
	groundIndex := len(s.plan) - 1
	s.do(op{kind: opSatisfies, in: []string{label, "k:Type"}, expect: expectProven})
	expect := expectNothing
	if closed {
		expect = expectClosedVerdict
	}
	data := s.do(op{kind: opSatisfies, in: []string{label, "k:Data"}, expect: expect})
	for _, kind := range s.korder {
		if kind == "k:Type" || kind == "k:Data" || !s.kinds[kind].Sort().IsProper() {
			continue
		}
		s.do(op{kind: opSatisfies, in: []string{label, kind}, expect: expect})
	}
	if grounded.code != "" {
		return
	}
	// GroundDescriptor establishes Data membership and returns the ordinary
	// descriptor, the same one Descriptor materializes.
	if described.code == "" && described.text != grounded.text {
		s.t.Fatalf("%s: %s: Descriptor %s differs from GroundDescriptor %s", s.name, label, described.text, grounded.text)
	}
	if described.code != "" && described.code != decl.Exhausted {
		s.t.Fatalf("%s: %s: GroundDescriptor succeeded but Descriptor failed: %s", s.name, label, described.text)
	}
	if data.code == "" && data.text != "verdict proven" {
		s.t.Fatalf("%s: %s: GroundDescriptor succeeded but Satisfies(Data) is %s", s.name, label, data.text)
	}
	if strings.HasSuffix(label, "~") {
		if prior := strings.TrimSuffix(label, "~"); s.groundText(prior) != "" && s.groundText(prior) != grounded.text {
			s.t.Fatalf("%s: %s: re-admitted ground descriptor %s differs from %s", s.name, label, grounded.text, s.groundText(prior))
		}
		return
	}
	// The ground descriptor is a closed primitive descriptor: Proper admits it
	// and GroundDescriptor reproduces it.
	result := s.results[fmt.Sprint(groundIndex)]
	s.do(op{kind: opProper, out: label + "~", value: result, expect: expectSuccess})
}

func (s *session) groundText(label string) string {
	for i, o := range s.plan {
		if o.kind == opGround && len(o.in) == 1 && o.in[0] == label && s.records[i].code == "" {
			return s.records[i].text
		}
	}
	return ""
}

// applications probes saturation and applies a constructor to sort-matching
// arguments chosen by the knob.
func (s *session) applications(label string, k knobs) {
	callee := s.values[label]
	if callee.Sort().IsProper() {
		if label == s.order[0] {
			s.do(op{kind: opApply, in: []string{label, label}, expect: expectKindMismatch})
		}
		return
	}
	inputs := callee.Sort().Inputs()
	n := len(inputs)
	filler := s.order[0]
	repeat := func(count int) []string {
		in := []string{label}
		for range count {
			in = append(in, filler)
		}
		return in
	}
	s.do(op{kind: opApply, in: []string{label}, expect: expectKindMismatch})
	s.do(op{kind: opApply, in: repeat(n + 1), expect: expectKindMismatch})
	if n > 1 {
		s.do(op{kind: opApply, in: repeat(n - 1), expect: expectKindMismatch})
	}
	// An argument of the wrong sort in the first slot.
	for _, candidate := range s.order {
		if !sameSort(s.values[candidate].Sort(), inputs[0]) {
			in := repeat(n)
			in[1] = candidate
			s.do(op{kind: opApply, in: in, expect: expectKindMismatch})
			break
		}
	}
	args := []string{label}
	for i, input := range inputs {
		var candidates []string
		for _, candidate := range s.order {
			if sameSort(s.values[candidate].Sort(), input) {
				candidates = append(candidates, candidate)
			}
		}
		if len(candidates) == 0 {
			return
		}
		args = append(args, candidates[(k.choice()+i)%len(candidates)])
	}
	s.applied++
	s.do(op{kind: opApply, out: fmt.Sprintf("a:%d", s.applied), in: args})
}

// inclusions checks reflexivity, proper top and the soundness of inclusion
// against closed membership: a Proven inclusion k1 ⊆ k2 and a Proven
// membership in k1 never meet a Refuted membership in k2.
func (s *session) inclusions() {
	proper := []string{}
	for _, kind := range s.korder {
		s.do(op{kind: opIncludes, in: []string{kind, kind}, expect: expectProven})
		if s.kinds[kind].Sort().IsProper() {
			proper = append(proper, kind)
			s.do(op{kind: opIncludes, in: []string{kind, "k:Type"}, expect: expectProven})
		}
	}
	pairs := 0
	for _, a := range proper {
		for _, b := range proper {
			if a != b && pairs < maxInclusionPairs {
				pairs++
				s.do(op{kind: opIncludes, in: []string{a, b}})
			}
		}
	}
	verdicts := s.verdicts(opSatisfies)
	for pair, verdict := range s.verdicts(opIncludes) {
		if verdict != "verdict proven" {
			continue
		}
		for _, value := range s.order {
			if verdicts[[2]string{value, pair[0]}] == "verdict proven" && verdicts[[2]string{value, pair[1]}] == "verdict refuted" {
				s.t.Fatalf("%s: %s ⊆ %s is Proven and %s is in %s, yet its membership in %s is Refuted", s.name, pair[0], pair[1], value, pair[0], pair[1])
			}
		}
	}
}

// verdicts maps the inputs of every successful Satisfies or Includes to its
// rendered verdict.
func (s *session) verdicts(kind opKind) map[[2]string]string {
	verdicts := map[[2]string]string{}
	for i, o := range s.plan {
		if o.kind == kind && s.records[i].code == "" {
			verdicts[[2]string{o.in[0], o.in[1]}] = s.records[i].text
		}
	}
	return verdicts
}

// exploreQueries admits each item as a closed proper value and compiles it
// as a kind, then checks the exactness of closed membership against unions
// and intersections of the compiled kinds (kinds.md §6).
func (s *session) exploreQueries(items []core.Value, k knobs) {
	p := s.program
	prelude := decl.PreludePin()
	s.do(op{kind: opCompile, out: "k:Type", scope: prelude, value: kindLocalValue("Type"), expect: expectSuccess})
	s.do(op{kind: opCompile, out: "k:Data", scope: prelude, value: kindLocalValue("Data"), expect: expectSuccess})
	root, _ := p.Module(p.Root())
	for _, d := range root.Declarations() {
		if d.Form() == decl.KindDefinition && len(d.KindParameters()) == 0 {
			s.do(op{kind: opCompile, out: "k:0:" + d.ID().Name, scope: p.Root(), value: kindLocalValue(d.ID().Name)})
		}
	}
	scopes := make([]decl.Digest, len(items))
	for i, item := range items {
		scopes[i] = p.Root()
		if k>>(8+i)&1 == 1 {
			scopes[i] = prelude
		}
		s.do(op{kind: opProper, out: fmt.Sprintf("q:%d", i), value: item})
		s.do(op{kind: opCompile, out: fmt.Sprintf("kq:%d", i), scope: scopes[i], value: item})
	}
	type combination struct{ union, intersection, left, right string }
	var combinations []combination
	for i := range items {
		for j := i + 1; j < len(items) && len(combinations) < maxCombinations; j++ {
			left, right := fmt.Sprintf("kq:%d", i), fmt.Sprintf("kq:%d", j)
			kl, okl := s.kinds[left]
			kr, okr := s.kinds[right]
			if !okl || !okr || !kl.Sort().IsProper() || !kr.Sort().IsProper() || scopes[i] != scopes[j] {
				continue
			}
			c := combination{fmt.Sprintf("kor:%d:%d", i, j), fmt.Sprintf("kand:%d:%d", i, j), left, right}
			pair := list(items[i], items[j])
			s.do(op{kind: opCompile, out: c.union, scope: scopes[i], value: d("or", properSort(), pair), expect: expectSuccess})
			s.do(op{kind: opCompile, out: c.intersection, scope: scopes[i], value: d("and", properSort(), pair), expect: expectSuccess})
			for _, member := range []string{left, right} {
				s.do(op{kind: opIncludes, in: []string{member, c.union}, expect: expectProven})
				s.do(op{kind: opIncludes, in: []string{c.intersection, member}, expect: expectProven})
			}
			combinations = append(combinations, c)
		}
	}
	for i, descriptor := range primitiveDescriptors()[:3] {
		s.do(op{kind: opProper, out: fmt.Sprintf("p:%d", i), value: descriptor, expect: expectSuccess})
	}
	for _, label := range s.order {
		s.examine(label)
	}
	verdicts := s.verdicts(opSatisfies)
	for _, c := range combinations {
		for _, value := range s.order {
			if !strings.HasPrefix(value, "q:") && !strings.HasPrefix(value, "p:") {
				continue
			}
			l, okl := verdicts[[2]string{value, c.left}]
			r, okr := verdicts[[2]string{value, c.right}]
			u, oku := verdicts[[2]string{value, c.union}]
			n, okn := verdicts[[2]string{value, c.intersection}]
			proven := func(v string) bool { return v == "verdict proven" }
			if okl && okr && oku && proven(u) != (proven(l) || proven(r)) {
				s.t.Fatalf("%s: closed %s: %s in %s, %s in %s, yet %s in their union %s", s.name, value, l, c.left, r, c.right, u, c.union)
			}
			if okl && okr && okn && proven(n) != (proven(l) && proven(r)) {
				s.t.Fatalf("%s: closed %s: %s in %s, %s in %s, yet %s in their intersection %s", s.name, value, l, c.left, r, c.right, n, c.intersection)
			}
		}
	}
	s.inclusions()
}

// sameAs compares two independently resolved default-limit sessions.
func (s *session) sameAs(other *session) {
	if s.outcome != other.outcome {
		s.t.Fatalf("nondeterministic Resolve:\n%s\n---\n%s", s.outcome, other.outcome)
	}
	if len(s.plan) != len(other.plan) {
		s.t.Fatalf("nondeterministic query plan: %d versus %d operations", len(s.plan), len(other.plan))
	}
	for i := range s.plan {
		if s.plan[i].String() != other.plan[i].String() || s.records[i] != other.records[i] {
			s.t.Fatalf("nondeterministic operation %d:\n%s => %+v\n%s => %+v", i, s.plan[i], s.records[i], other.plan[i], other.records[i])
		}
	}
}

// replay runs the default session's plan under this session's smaller
// limits. Each outcome must be Exhausted or equal to the default outcome.
func (s *session) replay(def *session) {
	if s.code == decl.Exhausted {
		return
	}
	if s.outcome != def.outcome {
		s.t.Fatalf("smaller limits %+v changed the Resolve outcome without exhaustion:\n%s\n---\ndefault:\n%s", s.limits, s.outcome, def.outcome)
	}
	if s.program == nil {
		return
	}
	for i, o := range def.plan {
		r := s.exec(o, i)
		if r.code == decl.Exhausted || r.code == skipped {
			continue
		}
		if r != def.records[i] {
			s.t.Fatalf("smaller limits %+v changed operation %d %s without exhaustion:\nsmall:   %+v\ndefault: %+v", s.limits, i, o, r, def.records[i])
		}
	}
}

func primitiveDescriptors() []core.Value {
	return []core.Value{
		h("scalar", a("int")),
		h("scalar", a("utf8-text")),
		h("any"),
		h("wire", h("scalar", a("bool"))),
		h("list-of", h("scalar", a("int"))),
		person(),
		h("rec", a("x"), h("list-of", h("ref", a("x")))),
	}
}

func kindLocalValue(name string) core.Value { return d("kind-local", text(name)) }

func sameSort(x, y decl.Sort) bool { return renderSort(x) == renderSort(y) }

// Rendering. Values are named by their encoding's digest and length, which
// identifies them exactly for comparison without copying large descriptors.

func digestValue(v core.Value) string {
	if v == nil {
		return "nil"
	}
	b := codec.Encode(v)
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%s/%d", hex.EncodeToString(sum[:10]), len(b))
}

func renderSort(x decl.Sort) string { return digestValue(x.Value()) }

func renderKind(k *decl.Kind) string {
	if k == nil {
		return "nil"
	}
	return string(k.Form()) + ":" + digestValue(k.Value()) + ":" + renderSort(k.Sort())
}

func renderTypeValue(v decl.TypeValue) string {
	expression := "nil"
	if e := v.Expression(); e != nil {
		expression = string(e.Kind()) + ":" + digestValue(e.Value())
	}
	return fmt.Sprintf("sort=%s contract=%s expression=%s", renderSort(v.Sort()), renderKind(v.Contract()), expression)
}

func renderError(e *decl.Error) string {
	return fmt.Sprintf("error code=%s subject=%s module=%x declaration=%q region=%q parameter=%d path=%v alias=%q dependency=%x expected=%q actual=%q target=%x:%q callee=%x:%q:%d kind=%s message=%q",
		e.Code, e.Subject, e.Module[:], e.Declaration, e.Region, e.ParameterIndex, e.Path, e.ImportAlias, e.Dependency[:], e.ExpectedName, e.ActualName,
		e.Target.Module[:], e.Target.Name, e.CalleeParameter.Declaration.Module[:], e.CalleeParameter.Declaration.Name, e.CalleeParameter.Index, renderKind(e.Expected), e.Message)
}

func renderMemberships(b *strings.Builder, indent string, memberships []decl.Membership) {
	for _, m := range memberships {
		fmt.Fprintf(b, "%smember %s", indent, renderKind(m.Kind()))
		for _, assumption := range m.Assumptions() {
			id := assumption.Parameter()
			fmt.Fprintf(b, " [%x:%s:%d %s]", id.Declaration.Module[:4], id.Declaration.Name, id.Index, renderKind(assumption.Kind()))
		}
		b.WriteString("\n")
	}
}

func renderExpression(b *strings.Builder, indent string, e *decl.Expression, depth int) {
	if e == nil {
		fmt.Fprintf(b, "%snil\n", indent)
		return
	}
	fmt.Fprintf(b, "%s%s %s path=%v sort=%s contract=%s", indent, e.Kind(), digestValue(e.Value()), e.Path(), renderSort(e.Sort()), renderKind(e.Contract()))
	if target, ok := e.Target(); ok {
		fmt.Fprintf(b, " target=%x:%s", target.Module[:4], target.Name)
	}
	if parameter, ok := e.Parameter(); ok {
		fmt.Fprintf(b, " parameter=%d", parameter.Index)
	}
	b.WriteString("\n")
	renderMemberships(b, indent+"  ", e.Memberships())
	if depth >= 4 {
		return
	}
	if callee := e.Callee(); callee != nil {
		renderExpression(b, indent+"  ", callee, depth+1)
	}
	for _, argument := range e.Arguments() {
		renderExpression(b, indent+"  ", argument, depth+1)
	}
	for _, operand := range e.Operands() {
		renderExpression(b, indent+"  ", operand.Expression(), depth+1)
	}
}

func renderProgram(p *decl.Program) string {
	var b strings.Builder
	fmt.Fprintf(&b, "root %x\n", p.Root())
	for _, m := range p.Modules() {
		fmt.Fprintf(&b, "module %x %q %s\n", m.Pin(), m.Name(), digestValue(m.Value()))
		for _, i := range m.Imports() {
			fmt.Fprintf(&b, " import %q %q %x\n", i.Alias(), i.Name(), i.Pin())
		}
		for _, d := range m.Declarations() {
			fmt.Fprintf(&b, " %s %q %s sort=%s contract=%s result=%s body=%s\n", d.Form(), d.ID().Name, digestValue(d.Value()), renderSort(d.Sort()), renderKind(d.Contract()), renderKind(d.Result()), renderKind(d.KindBody()))
			for _, x := range d.Parameters() {
				fmt.Fprintf(&b, "  parameter %q %s\n", x.Name, renderKind(x.Kind))
			}
			for _, x := range d.KindParameters() {
				fmt.Fprintf(&b, "  kind-parameter %q %s\n", x.Name, renderSort(x.Sort))
			}
			renderMemberships(&b, "  ", d.Memberships())
			if e := d.Expression(); e != nil {
				renderExpression(&b, "  ", e, 0)
			}
		}
	}
	return b.String()
}
