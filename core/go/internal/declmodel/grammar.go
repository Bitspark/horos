// Package declmodel contains private Horos module, binding and expansion
// mechanics. Declarations supplies the unified grammar; no profile registry
// or behavioral contract catalogue is exposed.
package declmodel

import core "github.com/bitspark/ontos/core/go"

type Grammar struct {
	DeclarationPrefix string
	DescriptorPrefix  string
	Any               func() core.Value
	Positions         func(string, []core.Value) ([][]int, error)
	WF                func(core.Value) error
	Rule              func(string, []core.Value) DomainRule
	Data              func(core.Value) (bool, error)
}

// DomainRule belongs to the Horos expression grammar. Behavioral library names
// never participate in these rules.
type DomainRule struct {
	RequiresData bool
	Capability   bool
}

func Failure(code Code, message string) error                           { return failure(code, message) }
func Head(value core.Value, prefix string) (string, []core.Value, bool) { return head(value, prefix) }
func (p *Program) Limits() Limits                                       { return p.limits }
