// Diagnostic checking outcomes (horos-internal#211, docs/architecture/decisions/0008-diagnostic-outcomes-kleene.md).
//
// A byte-for-byte port of core/rs/src/diagnose.rs. An ADDITIVE companion to the frozen
// boolean surface: WF and Conforms are unchanged, vectors/conformance.json is unchanged, and
// nothing here reinterprets what today's false means.
//
// What it adds is the distinction the boolean cannot carry: a completed structural MISMATCH, a
// MALFORMED descriptor, and a computation that reached the declared MaxRecursionDepth bound are
// three different pieces of evidence, and the boolean renders all three as false.
//
// The composition rules are the content (ADR 0008 D2/D3), not the extra enum:
//
//   - Complement PRESERVES exhaustion: not(Exhausted) = Exhausted, never Conforms. The frozen
//     recognizer computes !conformsIn(…), so an inner descent that trips the cap yields false
//     and not reports an unqualified true — a completed positive out of a computation that
//     never finished. That is the defect this file exists to name.
//   - union/and are KLEENE: a DECISIVE alternative beats an exhausted sibling, so the evaluator
//     short-circuits on a decisive result but NEVER on an Exhausted. The answer is a function of
//     the SET of sub-outcomes, never of traversal order.

package horos

import (
	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

// Limit names the declared bound an Exhausted reached, so a consumer can say WHICH limit
// stopped the attempt rather than "something ran out".
type Limit string

const (
	// LimitRecursionDepth is the MaxRecursionDepth descent cap (spec §3, horos-internal#37/horos-internal#71).
	LimitRecursionDepth Limit = "recursion_depth"
	// LimitNone is the zero value, carried by every completed outcome.
	LimitNone Limit = ""
)

// Value reports the effective value of a limit in this build; 0 when there is none.
func (l Limit) Value() int {
	if l == LimitRecursionDepth {
		return MaxRecursionDepth
	}
	return 0
}

// Verdict is the completed/incomplete classification of a checking attempt.
type Verdict string

const (
	// VerdictConforms is completed: v exhibits the form T under Profile.
	VerdictConforms Verdict = "conforms"
	// VerdictMismatch is completed: v does not.
	VerdictMismatch Verdict = "mismatch"
	// VerdictNotWellFormed is completed: T is not a well-formed descriptor.
	VerdictNotWellFormed Verdict = "not_well_formed"
	// VerdictExhausted is INCOMPLETE: a declared bound was reached; no claim either way. It is
	// a statement about THIS bounded attempt and is never a refutation.
	VerdictExhausted Verdict = "exhausted"
)

// Diagnosis is a checking outcome that distinguishes a completed decision from a reached bound.
type Diagnosis struct {
	Verdict Verdict
	// Limit is set iff Verdict is VerdictExhausted.
	Limit Limit
	// Location names where the check failed (ADR 0009). It is nil iff Verdict is
	// VerdictConforms. It never changes the verdict.
	Location *Location
}

// IsComplete reports whether this outcome is a completed decision.
func (d Diagnosis) IsComplete() bool { return d.Verdict != VerdictExhausted }

// CompletedBool reports the legacy boolean this outcome corresponds to WHERE ONE IS DETERMINED
// (ADR 0008 D4): (true, true) for Conforms, (false, true) for Mismatch/NotWellFormed, and
// ok=false for Exhausted — the boolean is unconstrained there, and collapsing the outcome into
// it is exactly what horos-internal#211 asks not to do.
func (d Diagnosis) CompletedBool() (value bool, ok bool) {
	switch d.Verdict {
	case VerdictConforms:
		return true, true
	case VerdictMismatch, VerdictNotWellFormed:
		return false, true
	default:
		return false, false
	}
}

// Profile is the checking profile a Diagnosis is stated under. A completed negative is
// completed UNDER THIS PROFILE, never absolutely — so the profile travels with the verdict.
type Profile struct {
	MaxRecursionDepth int
	AtomLenBoundMin   string // decimal, i128 range (horos-internal#18/horos-internal#57)
	AtomLenBoundMax   string
}

// CheckingProfile reports the profile this build decides under.
func CheckingProfile() Profile {
	return Profile{
		MaxRecursionDepth: MaxRecursionDepth,
		AtomLenBoundMin:   atomLenBoundMin.String(),
		AtomLenBoundMax:   atomLenBoundMax.String(),
	}
}

// ----- internal outcomes: a verdict with its unmaterialized location -----

type outcome struct {
	verdict Verdict
	limit   Limit
	f       *fault
}

var conformsO = outcome{verdict: VerdictConforms}

func failAt(verdict Verdict, path *pathNode, at *posNode, reason Reason) outcome {
	return outcome{verdict: verdict, f: &fault{path: path, at: at, reason: reason}}
}

func exhaustedAt(path *pathNode, at *posNode) outcome {
	return outcome{verdict: VerdictExhausted, limit: LimitRecursionDepth, f: &fault{path: path, at: at, reason: ReasonLimit}}
}

func (o outcome) diagnosis() Diagnosis {
	d := Diagnosis{Verdict: o.verdict, Limit: o.limit}
	if o.f != nil {
		d.Location = o.f.location()
	}
	return d
}

// ----- the Kleene folds (ADR 0008 D2/D3), each keeping the first deciding location -----

// allOf is conjunction: the first mismatch is decisive; otherwise the first exhaustion,
// then the first malformed outcome. Order of evaluation never changes the verdict.
type allOf struct {
	exhausted, notWF *outcome
}

// add records o and reports whether it decides the conjunction (a mismatch).
func (a *allOf) add(o outcome) bool {
	switch o.verdict {
	case VerdictMismatch:
		return true
	case VerdictExhausted:
		if a.exhausted == nil {
			a.exhausted = &o
		}
	case VerdictNotWellFormed:
		if a.notWF == nil {
			a.notWF = &o
		}
	}
	return false
}

func (a *allOf) result() outcome {
	if a.exhausted != nil {
		return *a.exhausted
	}
	if a.notWF != nil {
		return *a.notWF
	}
	return conformsO
}

// ----- the public entry points -----

// DiagnoseWF diagnoses wf(T): Conforms when T is a well-formed descriptor, NotWellFormed when
// it is conclusively not, and Exhausted when the well-formedness traversal itself could not
// finish — the distinction horos-internal#211 asks for by name. A non-conforming result names
// the malformed descriptor's position and, where one is at fault, its argument (ADR 0009).
func DiagnoseWF(t Value) Diagnosis { return diagnoseWFIn(t, nil, nil, 0, nil).diagnosis() }

// Diagnose diagnoses v : T. Mirrors Conforms: the wf(T) soundness gate runs first (spec §3
// MUST), so a descriptor that is not well-formed yields NotWellFormed and a descriptor whose
// well-formedness could not be decided yields Exhausted — neither is reported as a mismatch of
// the value. A non-conforming result names where the check failed (ADR 0009).
func Diagnose(v, t Value) Diagnosis {
	if w := diagnoseWFIn(t, nil, nil, 0, nil); w.verdict != VerdictConforms {
		return w.diagnosis()
	}
	return diagnoseIn(v, t, nil, nil, 0, nil, nil).diagnosis()
}

// ----- the recursions: arm-for-arm mirrors of conformsIn / wfIn -----

type diagEnv struct {
	name []byte
	body Value
	at   *posNode
}

func diagPushEnv(env []diagEnv, b diagEnv) []diagEnv {
	out := make([]diagEnv, len(env), len(env)+1)
	copy(out, env)
	return append(out, b)
}

func diagPushName(names [][]byte, name []byte) [][]byte {
	out := make([][]byte, len(names), len(names)+1)
	copy(out, names)
	return append(out, name)
}

func containsName(names [][]byte, name []byte) bool {
	for _, n := range names {
		if string(n) == string(name) {
			return true
		}
	}
	return false
}

func lookupEnv(env []diagEnv, name []byte) (diagEnv, bool) {
	for i := len(env) - 1; i >= 0; i-- {
		if string(env[i].name) == string(name) {
			return env[i], true
		}
	}
	return diagEnv{}, false
}

func diagnoseIn(v, t Value, env []diagEnv, pending [][]byte, depth int, path *pathNode, at *posNode) outcome {
	if depth > MaxRecursionDepth {
		return exhaustedAt(path, at)
	}
	refused := func() outcome { return failAt(VerdictMismatch, path, at, ReasonRefused) }
	ctor, args, ok := head(t)
	if !ok {
		return refused() // not a horos descriptor; unreachable below the wf gate
	}

	// Leaves never recurse into a value position, so they are always decisive here.
	if isLeafCtor(ctor) {
		if leafClassifies(v, t) {
			return conformsO
		}
		return refused()
	}

	switch ctor {
	// Composition on the SAME v — pending threads through unchanged.
	case "union":
		// A conforming alternative is decisive; otherwise the first exhaustion. A mismatch
		// reports the first alternative that descended furthest, else the union itself.
		var exhausted *outcome
		var deepest *fault
		for i, ct := range args {
			o := diagnoseIn(v, ct, env, pending, depth+1, path, at.push(i+1))
			switch o.verdict {
			case VerdictConforms:
				return conformsO
			case VerdictExhausted:
				if exhausted == nil {
					exhausted = &o
				}
			case VerdictMismatch:
				if deepest == nil || o.f.path.len() > deepest.path.len() {
					deepest = o.f
				}
			}
		}
		if exhausted != nil {
			return *exhausted
		}
		if deepest == nil || deepest.path.len() <= path.len() {
			return failAt(VerdictMismatch, path, at, ReasonNoAlternative)
		}
		return outcome{verdict: VerdictMismatch, f: deepest}
	case "and":
		var all allOf
		for i, ct := range args {
			if o := diagnoseIn(v, ct, env, pending, depth+1, path, at.push(i+1)); all.add(o) {
				return o
			}
		}
		return all.result()
	case "not":
		if len(args) != 1 {
			return refused()
		}
		switch o := diagnoseIn(v, args[0], env, pending, depth+1, path, at.push(1)); o.verdict {
		case VerdictConforms:
			return failAt(VerdictMismatch, path, at, ReasonComplement)
		case VerdictMismatch:
			return conformsO
		default:
			// NotWellFormed cannot arise below the entry wf gate; Exhausted is preserved.
			return o
		}

	// Recursion — mirrors the recognizer's guard exactly. A ref continues at its rec body's
	// position, so a location is always a node of the written descriptor.
	case "rec":
		if len(args) != 2 {
			return refused()
		}
		nb, okn := atomBytes(args[0])
		if !okn {
			return refused()
		}
		bodyAt := at.push(2)
		return diagnoseIn(v, args[1], diagPushEnv(env, diagEnv{name: nb, body: args[1], at: bodyAt}), diagPushName(pending, nb), depth+1, path, bodyAt)
	case "ref":
		if len(args) != 1 {
			return refused()
		}
		nb, okn := atomBytes(args[0])
		if !okn || containsName(pending, nb) {
			return refused() // unguarded — the recognizer's false
		}
		b, found := lookupEnv(env, nb)
		if !found {
			return refused() // unbound
		}
		return diagnoseIn(v, b.body, env, diagPushName(pending, nb), depth+1, path, b.at)
	}

	// Structural arms: the gate is decisive, and a descent into a CHILD VALUE clears pending.
	children, gate := structuralChildren(v, ctor, args)
	if gate != nil {
		f := &fault{path: path, at: at, reason: gate.reason, field: gate.field}
		if gate.key >= 0 {
			f.path = path.push(Step{Kind: StepKey, Index: gate.key})
		}
		return outcome{verdict: VerdictMismatch, f: f}
	}
	var all allOf
	for _, c := range children {
		if o := diagnoseIn(c.v, c.t, env, nil, depth+1, path.push(c.step), at.push(c.at...)); all.add(o) {
			return o
		}
	}
	return all.result()
}

// child is one sub-judgment of a structural arm: the child value, its descriptor, the path
// step to the value and the item indices from the arm's descriptor to the child descriptor.
type child struct {
	v, t Value
	step Step
	at   []int
}

// gateFailure is a structural arm's own refusal. key is the entry index of a record key that
// is not text, or -1.
type gateFailure struct {
	reason Reason
	field  string
	key    int
}

var gateRefused = &gateFailure{reason: ReasonRefused, key: -1}

// structuralChildren is the structural gate plus the ordered child sub-judgments, for the arms
// that descend into a child VALUE. Mirrors the recognizer's recursive arms; a non-nil
// gateFailure means v cannot conform regardless of any child.
func structuralChildren(v Value, ctor string, args []Value) ([]child, *gateFailure) {
	var children []child
	switch ctor {
	case "tuple":
		tv, isTup := v.(core.Tuple)
		if !isTup || len(tv.Items()) != len(args) {
			return nil, gateRefused
		}
		for i, ct := range args {
			children = append(children, child{tv.Items()[i], ct, Step{Kind: StepItem, Index: i}, []int{i + 1}})
		}
		return children, nil
	case "seq":
		tv, isTup := v.(core.Tuple)
		if len(args) != 1 || !isTup {
			return nil, gateRefused
		}
		for i, c := range tv.Items() {
			children = append(children, child{c, args[0], Step{Kind: StepItem, Index: i}, []int{1}})
		}
		return children, nil
	case "compound":
		if len(args) < 1 {
			return nil, gateRefused
		}
		lb, okl := atomBytes(args[0])
		tv, isTup := v.(core.Tuple)
		if !okl || !isTup || len(tv.Items()) != len(args) {
			return nil, gateRefused
		}
		tup := tv.Items()
		hb, okh := atomBytes(tup[0])
		if !okh || string(hb) != string(lb) {
			return nil, gateRefused
		}
		for j := 1; j < len(tup); j++ {
			children = append(children, child{tup[j], args[j], Step{Kind: StepItem, Index: j}, []int{j + 1}})
		}
		return children, nil
	case "list-of", "set-of":
		if len(args) != 1 {
			return nil, gateRefused
		}
		var elems []Value
		var err error
		if ctor == "list-of" {
			elems, err = data.ReadList(v)
		} else {
			elems, err = data.ReadSet(v)
		}
		if err != nil {
			return nil, gateRefused
		}
		for i, e := range elems {
			children = append(children, child{e, args[0], Step{Kind: StepElem, Index: i}, []int{1}})
		}
		return children, nil
	case "map-of":
		if len(args) != 2 {
			return nil, gateRefused
		}
		entries, err := data.ReadMap(v)
		if err != nil {
			return nil, gateRefused
		}
		for i, kv := range entries {
			children = append(children,
				child{kv[0], args[0], Step{Kind: StepKey, Index: i}, []int{1}},
				child{kv[1], args[1], Step{Kind: StepValue, Index: i}, []int{2}})
		}
		return children, nil
	case "record":
		if len(args) < 1 {
			return nil, gateRefused
		}
		return recordChildren(v, args[0], args[1:])
	}
	return nil, gateRefused
}

// recordChildren mirrors conformsRecord's gate and present-field worklist, checking in the
// contract's order: a canonical map, text keys, missing required fields in descriptor order,
// extra keys of a closed record in canonical order, then the present declared fields.
func recordChildren(v, mode Value, fields []Value) ([]child, *gateFailure) {
	mb, okm := atomBytes(mode)
	if !okm || (string(mb) != "closed" && string(mb) != "open") {
		return nil, gateRefused
	}
	closed := string(mb) == "closed"
	entries, err := data.ReadMap(v)
	if err != nil {
		return nil, gateRefused
	}
	keys := make([]string, len(entries))
	keyed := make(map[string]Value, len(entries))
	for i, kv := range entries {
		k, errK := data.ReadText(kv[0])
		if errK != nil {
			return nil, &gateFailure{reason: ReasonRefused, key: i}
		}
		keys[i] = k
		keyed[k] = kv[1]
	}
	type declaredField struct {
		name string
		ty   Value
		item int
	}
	declared := make([]declaredField, 0, len(fields))
	names := make(map[string]bool, len(fields))
	for i, f := range fields {
		name, required, ty, okf := readField(f)
		if !okf {
			return nil, gateRefused
		}
		names[name] = true
		if _, present := keyed[name]; !present && required {
			return nil, &gateFailure{reason: ReasonMissingField, field: name, key: -1}
		}
		declared = append(declared, declaredField{name, ty, i + 2})
	}
	if closed {
		for _, k := range keys {
			if !names[k] {
				return nil, &gateFailure{reason: ReasonExtraField, field: k, key: -1}
			}
		}
	}
	var children []child
	for _, f := range declared {
		if val, present := keyed[f.name]; present {
			children = append(children, child{val, f.ty, Step{Kind: StepField, Name: f.name}, []int{f.item, 3}})
		}
	}
	return children, nil
}

// ----- well-formedness: own form first, then child descriptors in item order -----

func isDescriptorCtor(ctor string) bool {
	switch ctor {
	case "tuple", "seq", "compound", "list-of", "set-of", "map-of", "record", "union", "and", "not", "rec", "ref":
		return true
	}
	return isLeafCtor(ctor)
}

// leafFault names the first fault of a leaf descriptor that leafWellFormed rejects: the
// arity, then each argument in item order against the arguments before it.
func leafFault(ctor string, args []Value) (Reason, int) {
	arity := map[string]int{"any": 0, "atom": 0, "key-grammar": 0, "text-non-blank": 0, "lowercase-uuid": 0,
		"atom-exact": 1, "atom-in": 1, "scalar": 1, "exact": 1, "atom-len": 2, "text-len": 2}[ctor]
	if len(args) != arity {
		return ReasonArity, 0
	}
	switch ctor {
	case "atom-exact":
		if _, ok := args[0].(core.Atom); !ok {
			return ReasonArgument, 1
		}
	case "atom-in":
		elems, err := data.ReadSet(args[0])
		if err != nil {
			return ReasonArgument, 1
		}
		for _, e := range elems {
			if _, ok := e.(core.Atom); !ok {
				return ReasonArgument, 1
			}
		}
	case "scalar":
		k, ok := atomBytes(args[0])
		if !ok || (string(k) != "int" && string(k) != "utf8-text" && string(k) != "bool") {
			return ReasonArgument, 1
		}
	case "atom-len", "text-len":
		lo, err := data.ReadInt(args[0])
		if err != nil || !atomLenBoundInRange(lo) || lo.Sign() < 0 {
			return ReasonArgument, 1
		}
		hi, err := data.ReadInt(args[1])
		if err != nil || !atomLenBoundInRange(hi) || lo.Cmp(hi) > 0 {
			return ReasonArgument, 2
		}
	}
	// Unreachable while leafFault mirrors leafWellFormed; name the descriptor itself.
	return ReasonArgument, 0
}

func diagnoseWFIn(t Value, bound, pending [][]byte, depth int, at *posNode) outcome {
	if depth > MaxRecursionDepth {
		return exhaustedAt(nil, at)
	}
	malformed := func(reason Reason, arg int) outcome {
		return outcome{verdict: VerdictNotWellFormed, f: &fault{at: at, reason: reason, arg: arg}}
	}
	ctor, args, ok := head(t)
	if !ok {
		return malformed(ReasonNotDescriptor, 0)
	}
	if !isDescriptorCtor(ctor) {
		return malformed(ReasonUnknownConstructor, 0)
	}
	if isLeafCtor(ctor) {
		if leafWellFormed(t) {
			return conformsO
		}
		return malformed(leafFault(ctor, args))
	}
	// children are judged in item order once the own form holds; exhaustion among them takes
	// precedence over a malformed sibling (ADR 0008).
	wfChildren := func(first int, ts []Value, pending [][]byte) outcome {
		var all allOf
		for i, ct := range ts {
			all.add(diagnoseWFIn(ct, bound, pending, depth+1, at.push(first+i)))
		}
		return all.result()
	}
	switch ctor {
	case "tuple":
		return wfChildren(1, args, nil)
	case "seq", "list-of", "set-of":
		if len(args) != 1 {
			return malformed(ReasonArity, 0)
		}
		return wfChildren(1, args, nil)
	case "compound":
		if len(args) < 1 {
			return malformed(ReasonArity, 0)
		}
		if _, okl := atomBytes(args[0]); !okl {
			return malformed(ReasonArgument, 1)
		}
		return wfChildren(2, args[1:], nil)
	case "map-of":
		if len(args) != 2 {
			return malformed(ReasonArity, 0)
		}
		return wfChildren(1, args, nil)
	case "union", "and":
		return wfChildren(1, args, pending)
	case "not":
		if len(args) != 1 {
			return malformed(ReasonArity, 0)
		}
		return wfChildren(1, args, pending)
	case "rec":
		if len(args) != 2 {
			return malformed(ReasonArity, 0)
		}
		name, ok := atomBytes(args[0])
		if !ok {
			return malformed(ReasonArgument, 1)
		}
		return diagnoseWFIn(args[1], diagPushName(bound, name), diagPushName(pending, name), depth+1, at.push(2))
	case "ref":
		if len(args) != 1 {
			return malformed(ReasonArity, 0)
		}
		name, ok := atomBytes(args[0])
		switch {
		case !ok:
			return malformed(ReasonArgument, 1)
		case !containsName(bound, name):
			return malformed(ReasonUnboundRef, 1)
		case containsName(pending, name):
			return malformed(ReasonUnguardedRef, 1)
		}
		return conformsO
	default: // "record"
		if len(args) < 1 {
			return malformed(ReasonArity, 0)
		}
		mode, ok := atomBytes(args[0])
		if !ok || (string(mode) != "open" && string(mode) != "closed") {
			return malformed(ReasonArgument, 1)
		}
		names := make(map[string]bool)
		types := make([]Value, 0, len(args)-1)
		for i, field := range args[1:] {
			name, _, ty, ok := readField(field)
			if !ok {
				return malformed(ReasonArgument, i+2)
			}
			if names[name] {
				return malformed(ReasonDuplicateField, i+2)
			}
			names[name] = true
			types = append(types, ty)
		}
		var all allOf
		for i, ty := range types {
			all.add(diagnoseWFIn(ty, bound, nil, depth+1, at.push(i+2, 3)))
		}
		return all.result()
	}
}
