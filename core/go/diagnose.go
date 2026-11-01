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
}

var (
	conformsD     = Diagnosis{Verdict: VerdictConforms}
	mismatchD     = Diagnosis{Verdict: VerdictMismatch}
	notWellFormed = Diagnosis{Verdict: VerdictNotWellFormed}
)

func exhaustedD(l Limit) Diagnosis { return Diagnosis{Verdict: VerdictExhausted, Limit: l} }

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

// ----- the Kleene folds (ADR 0008 D2/D3) -----

// kleeneNot swaps the completed verdicts and PRESERVES exhaustion. It never completes an
// incomplete subdecision — the rule this file exists for.
func kleeneNot(inner Diagnosis) Diagnosis {
	switch inner.Verdict {
	case VerdictConforms:
		return mismatchD
	case VerdictMismatch:
		return conformsD
	default:
		// NotWellFormed cannot arise below the entry wf gate; Exhausted is preserved.
		return inner
	}
}

// kleeneAny is disjunction: a decisive Conforms wins over an exhausted sibling.
func kleeneAny(outcomes []Diagnosis) Diagnosis {
	exhausted := LimitNone
	notWF := false
	for _, o := range outcomes {
		switch o.Verdict {
		case VerdictConforms:
			return conformsD // decisive — stop here
		case VerdictMismatch:
		case VerdictNotWellFormed:
			notWF = true
		case VerdictExhausted:
			if exhausted == LimitNone {
				exhausted = o.Limit
			}
		}
	}
	if exhausted != LimitNone {
		return exhaustedD(exhausted)
	}
	if notWF {
		return notWellFormed
	}
	return mismatchD
}

// kleeneAll is conjunction: a decisive Mismatch wins over an exhausted sibling.
func kleeneAll(outcomes []Diagnosis) Diagnosis {
	exhausted := LimitNone
	notWF := false
	for _, o := range outcomes {
		switch o.Verdict {
		case VerdictMismatch:
			return mismatchD // decisive — stop here
		case VerdictConforms:
		case VerdictNotWellFormed:
			notWF = true
		case VerdictExhausted:
			if exhausted == LimitNone {
				exhausted = o.Limit
			}
		}
	}
	if exhausted != LimitNone {
		return exhaustedD(exhausted)
	}
	if notWF {
		return notWellFormed
	}
	return conformsD
}

// ----- the public entry points -----

// DiagnoseWF diagnoses wf(T): Conforms when T is a well-formed descriptor, NotWellFormed when
// it is conclusively not, and Exhausted when the well-formedness traversal itself could not
// finish — the distinction horos-internal#211 asks for by name.
func DiagnoseWF(t Value) Diagnosis { return diagnoseWFIn(t, nil, nil, 0) }

// Diagnose diagnoses v : T. Mirrors Conforms: the wf(T) soundness gate runs first (spec §3
// MUST), so a descriptor that is not well-formed yields NotWellFormed and a descriptor whose
// well-formedness could not be decided yields Exhausted — neither is reported as a mismatch of
// the value.
func Diagnose(v, t Value) Diagnosis {
	switch w := DiagnoseWF(t); w.Verdict {
	case VerdictConforms:
		return diagnoseIn(v, t, nil, nil, 0)
	case VerdictExhausted:
		return w
	default:
		return notWellFormed
	}
}

// ----- the recursions: arm-for-arm mirrors of conformsIn / wfIn -----

type diagEnv struct {
	name []byte
	body Value
}

func diagPushEnv(env []diagEnv, name []byte, body Value) []diagEnv {
	out := make([]diagEnv, len(env), len(env)+1)
	copy(out, env)
	return append(out, diagEnv{name: name, body: body})
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

func lookupEnv(env []diagEnv, name []byte) (Value, bool) {
	for i := len(env) - 1; i >= 0; i-- {
		if string(env[i].name) == string(name) {
			return env[i].body, true
		}
	}
	return nil, false
}

func diagnoseIn(v, t Value, env []diagEnv, pending [][]byte, depth int) Diagnosis {
	if depth > MaxRecursionDepth {
		return exhaustedD(LimitRecursionDepth)
	}
	ctor, args, ok := head(t)
	if !ok {
		return mismatchD // not a horos descriptor
	}

	// Leaves never recurse into a value position, so they are always decisive here.
	if isLeafCtor(ctor) {
		if leafClassifies(v, t) {
			return conformsD
		}
		return mismatchD
	}

	switch ctor {
	// Composition on the SAME v — pending threads through unchanged.
	case "union":
		outs := make([]Diagnosis, 0, len(args))
		for _, ct := range args {
			outs = append(outs, diagnoseIn(v, ct, env, pending, depth+1))
		}
		return kleeneAny(outs)
	case "and":
		outs := make([]Diagnosis, 0, len(args))
		for _, ct := range args {
			outs = append(outs, diagnoseIn(v, ct, env, pending, depth+1))
		}
		return kleeneAll(outs)
	case "not":
		if len(args) != 1 {
			return mismatchD
		}
		return kleeneNot(diagnoseIn(v, args[0], env, pending, depth+1))

	// Recursion — mirrors the recognizer's guard exactly.
	case "rec":
		if len(args) != 2 {
			return mismatchD
		}
		nb, okn := atomBytes(args[0])
		if !okn {
			return mismatchD
		}
		return diagnoseIn(v, args[1], diagPushEnv(env, nb, args[1]), diagPushName(pending, nb), depth+1)
	case "ref":
		if len(args) != 1 {
			return mismatchD
		}
		nb, okn := atomBytes(args[0])
		if !okn {
			return mismatchD
		}
		if containsName(pending, nb) {
			return mismatchD // unguarded — the recognizer's false
		}
		body, found := lookupEnv(env, nb)
		if !found {
			return mismatchD // unbound
		}
		return diagnoseIn(v, body, env, diagPushName(pending, nb), depth+1)
	}

	// Structural arms: the gate is decisive, and a descent into a CHILD VALUE clears pending.
	children, gateOK := structuralChildren(v, ctor, args)
	if !gateOK {
		return mismatchD
	}
	outs := make([]Diagnosis, 0, len(children))
	for _, c := range children {
		outs = append(outs, diagnoseIn(c[0], c[1], env, nil, depth+1))
	}
	return kleeneAll(outs)
}

// structuralChildren is the structural gate plus the ordered child sub-judgments, for the arms
// that descend into a child VALUE. Mirrors the recognizer's recursive arms; ok=false means the
// gate failed, so v cannot conform regardless of any child.
func structuralChildren(v Value, ctor string, args []Value) (children [][2]Value, ok bool) {
	switch ctor {
	case "tuple":
		tv, isTup := v.(core.Tuple)
		if !isTup {
			return nil, false
		}
		tup := tv.Items()
		if len(tup) != len(args) {
			return nil, false
		}
		for i, ct := range args {
			children = append(children, [2]Value{tup[i], ct})
		}
		return children, true
	case "seq":
		if len(args) != 1 {
			return nil, false
		}
		tv, isTup := v.(core.Tuple)
		if !isTup {
			return nil, false
		}
		for _, c := range tv.Items() {
			children = append(children, [2]Value{c, args[0]})
		}
		return children, true
	case "compound":
		if len(args) < 1 {
			return nil, false
		}
		lb, okl := atomBytes(args[0])
		if !okl {
			return nil, false
		}
		tv, isTup := v.(core.Tuple)
		if !isTup {
			return nil, false
		}
		tup := tv.Items()
		if len(tup) != len(args) {
			return nil, false
		}
		hb, okh := atomBytes(tup[0])
		if !okh || string(hb) != string(lb) {
			return nil, false
		}
		for i, ct := range args[1:] {
			children = append(children, [2]Value{tup[i+1], ct})
		}
		return children, true
	case "list-of", "set-of":
		if len(args) != 1 {
			return nil, false
		}
		var elems []Value
		var err error
		if ctor == "list-of" {
			elems, err = data.ReadList(v)
		} else {
			elems, err = data.ReadSet(v)
		}
		if err != nil {
			return nil, false
		}
		for _, e := range elems {
			children = append(children, [2]Value{e, args[0]})
		}
		return children, true
	case "map-of":
		if len(args) != 2 {
			return nil, false
		}
		entries, err := data.ReadMap(v)
		if err != nil {
			return nil, false
		}
		for _, kv := range entries {
			children = append(children, [2]Value{kv[0], args[0]})
			children = append(children, [2]Value{kv[1], args[1]})
		}
		return children, true
	case "record":
		if len(args) < 1 {
			return nil, false
		}
		return recordChildren(v, args[0], args[1:])
	}
	return nil, false
}

// recordChildren mirrors conformsRecord's gate and present-field worklist.
func recordChildren(v, mode Value, fields []Value) (children [][2]Value, ok bool) {
	mb, okm := atomBytes(mode)
	if !okm {
		return nil, false
	}
	var closed bool
	switch string(mb) {
	case "closed":
		closed = true
	case "open":
		closed = false
	default:
		return nil, false
	}
	entries, err := data.ReadMap(v)
	if err != nil {
		return nil, false
	}
	keyed := make(map[string]Value, len(entries))
	for _, kv := range entries {
		k, errK := data.ReadText(kv[0])
		if errK != nil {
			return nil, false
		}
		keyed[k] = kv[1]
	}
	declared := make(map[string]bool, len(fields))
	for _, f := range fields {
		name, required, ty, okf := readField(f)
		if !okf {
			return nil, false
		}
		declared[name] = true
		val, present := keyed[name]
		if !present {
			if required {
				return nil, false // a missing required field is decisive
			}
			continue
		}
		children = append(children, [2]Value{val, ty})
	}
	if closed {
		for k := range keyed {
			if !declared[k] {
				return nil, false // an extra key in a closed record is decisive
			}
		}
	}
	return children, true
}

func diagnoseWFIn(t Value, bound, pending [][]byte, depth int) Diagnosis {
	if depth > MaxRecursionDepth {
		return exhaustedD(LimitRecursionDepth)
	}
	ctor, args, ok := head(t)
	if !ok {
		return notWellFormed
	}
	if isLeafCtor(ctor) {
		if leafWellFormed(t) {
			return conformsD
		}
		return notWellFormed
	}
	wfAll := func(ts []Value, pending [][]byte) Diagnosis {
		outs := make([]Diagnosis, 0, len(ts))
		for _, ct := range ts {
			outs = append(outs, diagnoseWFIn(ct, bound, pending, depth+1))
		}
		return kleeneAll(outs)
	}
	switch ctor {
	case "tuple":
		return wfAll(args, nil)
	case "seq":
		if len(args) != 1 {
			return notWellFormed
		}
		return diagnoseWFIn(args[0], bound, nil, depth+1)
	case "compound":
		if len(args) < 1 {
			return notWellFormed
		}
		if _, okl := atomBytes(args[0]); !okl {
			return notWellFormed
		}
		return wfAll(args[1:], nil)
	case "list-of", "set-of":
		if len(args) != 1 {
			return notWellFormed
		}
		return diagnoseWFIn(args[0], bound, nil, depth+1)
	case "map-of":
		if len(args) != 2 {
			return notWellFormed
		}
		return kleeneAll([]Diagnosis{
			diagnoseWFIn(args[0], bound, nil, depth+1),
			diagnoseWFIn(args[1], bound, nil, depth+1),
		})
	case "union", "and":
		return wfAll(args, pending)
	case "not":
		if len(args) != 1 {
			return notWellFormed
		}
		return diagnoseWFIn(args[0], bound, pending, depth+1)
	}
	// Everything else (records, rec/ref, arity errors, unknown ctors) is decided by the frozen
	// WF for THIS node; only the recursive shapes above can hide an exhaustion.
	if WF(t) {
		return conformsD
	}
	return notWellFormed
}
