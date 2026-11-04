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
