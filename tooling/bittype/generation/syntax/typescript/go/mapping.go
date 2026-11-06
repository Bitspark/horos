package generation

import (
	"context"
	g "github.com/Bitspark/bittype/contracts/generation/go"
	c "github.com/Bitspark/bittype/generation/syntax/go"
	source "github.com/Bitspark/bittype/generation/syntax/source/horos/go"
	m "github.com/Bitspark/bittype/generation/syntax/target/go"
	target "github.com/Bitspark/bittype/generation/syntax/target/typescript/go"
	h "github.com/Bitspark/bittype/syntax/horos/go"
	s "github.com/Bitspark/bittype/syntax/typescript/go"
	rules "github.com/Bitspark/horos/tooling/bittype/generation/syntax/go"
	"strconv"
)

func textMapping(src *source.Source, b *target.Backend[h.Language]) c.Mapping[h.Language, s.Language] {
	return rules.Direct("text/typescript/1", src, b, func(_ context.Context, r g.UseRequest[h.Language, s.Language]) (g.Fragment[s.Language], error) {
		view, err := src.Read(r.Want().Use().Use)
		if err != nil {
			return g.Fragment[s.Language]{}, err
		}
		return b.Node(m.Node(s.KindKeywordType, m.Attributes("name", "string"), view.Origin))
	})
}
func boolMapping(src *source.Source, b *target.Backend[h.Language]) c.Mapping[h.Language, s.Language] {
	return rules.Direct("bool/typescript/1", src, b, func(_ context.Context, r g.UseRequest[h.Language, s.Language]) (g.Fragment[s.Language], error) {
		view, err := src.Read(r.Want().Use().Use)
		if err != nil {
			return g.Fragment[s.Language]{}, err
		}
		return b.Node(m.Node(s.KindKeywordType, m.Attributes("name", "boolean"), view.Origin))
	})
}
func listMapping(src *source.Source, b *target.Backend[h.Language]) c.Mapping[h.Language, s.Language] {
	return rules.Direct("list/typescript/1", src, b, func(_ context.Context, r g.UseRequest[h.Language, s.Language]) (g.Fragment[s.Language], error) {
		view, err := src.Read(r.Want().Use().Use)
		if err != nil {
			return g.Fragment[s.Language]{}, err
		}
		operand, err := src.Operand(r.Want().Use().Use, 1)
		if err != nil {
			return g.Fragment[s.Language]{}, err
		}
		child, err := r.Child(operand)
		if err != nil {
			return g.Fragment[s.Language]{}, err
		}
		return b.Node(m.Node(s.KindArrayType, nil, view.Origin), m.Child[s.Language]{Role: s.RoleElement, Fragment: child.Payload()})
	})
}
func recordMapping(src *source.Source, b *target.Backend[h.Language]) c.Mapping[h.Language, s.Language] {
	return rules.Direct("record/typescript/1", src, b, func(_ context.Context, r g.UseRequest[h.Language, s.Language]) (g.Fragment[s.Language], error) {
		use := r.Want().Use()
		if !src.IsBody(use.Environment, use.Use) {
			return g.Fragment[s.Language]{}, src.Unsupported(use.Use, "nested records require a named declaration in this profile")
		}
		view, err := src.Read(use.Use)
		if err != nil {
			return g.Fragment[s.Language]{}, err
		}
		fields, err := src.Fields(use.Use)
		if err != nil {
			return g.Fragment[s.Language]{}, err
		}
		var children []m.Child[s.Language]
		names := map[string]bool{}
		for _, field := range fields {

			if names[field.Name] {
				return g.Fragment[s.Language]{}, src.Unsupported(use.Use, "native field name collision")
			}
			names[field.Name] = true
			child, err := r.Child(field.Use)
			if err != nil {
				return g.Fragment[s.Language]{}, err
			}
			ty := child.Payload()
			member, err := b.Node(m.Node(s.KindProperty, m.Attributes("name", field.Name, "optional", strconv.FormatBool(field.Optional)), field.Origin), m.Child[s.Language]{Role: s.RoleType, Fragment: ty})
			if err != nil {
				return member, err
			}
			children = append(children, m.Child[s.Language]{Role: s.RoleMember, Fragment: member})
		}
		return b.Node(m.Node(s.KindObjectType, nil, view.Origin), children...)
	})
}
