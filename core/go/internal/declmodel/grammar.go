// Package declmodel contains profile-neutral module, binding and expansion
// mechanics. Concrete profiles provide their closed grammar; there is no
// profile registry, prefix switch or behavioral contract catalogue here.
package declmodel

import core "github.com/bitspark/ontos/core/go"

type Grammar struct {
	DeclarationPrefix string
	DescriptorPrefix  string
	Any               func() core.Value
	Positions         func(string, []core.Value) ([][]int, error)
	WF                func(core.Value) error
}

func Failure(code Code, message string) error                           { return failure(code, message) }
func Head(value core.Value, prefix string) (string, []core.Value, bool) { return head(value, prefix) }
func AdmitValue(value core.Value, limits Limits) error {
	if !limits.valid() {
		return failure(InvalidOptions, "invalid checking limits")
	}
	return (&budget{limits: limits}).value(value, 0)
}

func (p *Program) Limits() Limits { return p.limits }

// Work is request-local accounting shared by declaration transformations.
// It has no grammar or registry knowledge.
type Work struct{ budget budget }

func NewWork(limits Limits) (*Work, error) {
	if !limits.valid() {
		return nil, failure(InvalidOptions, "invalid checking limits")
	}
	return &Work{budget: budget{limits: limits}}, nil
}
func (w *Work) Step(depth int) error         { return w.budget.step(depth) }
func (w *Work) Value(value core.Value) error { return w.budget.value(value, 0) }
func Identifier(name string) bool            { return identifier(name) }
func ModuleName(name string) bool            { return moduleName(name) }
