package generation

import (
	"context"

	g "github.com/Bitspark/bittype/contracts/generation/go"
	c "github.com/Bitspark/bittype/generation/syntax/go"
	source "github.com/Bitspark/bittype/generation/syntax/source/horos/go"
	m "github.com/Bitspark/bittype/generation/syntax/target/go"
	target "github.com/Bitspark/bittype/generation/syntax/target/golang/go"
	s "github.com/Bitspark/bittype/syntax/golang/go"
	h "github.com/Bitspark/bittype/syntax/horos/go"
	rules "github.com/Bitspark/horos/tooling/bittype/generation/syntax/go"
)

func textMapping(src *source.Source, b *target.Backend[h.Language]) c.Mapping[h.Language, s.Language] {
	return rules.Direct("text/golang/1", src, b, func(_ context.Context, r g.UseRequest[h.Language, s.Language]) (g.Fragment[s.Language], error) {
		view, err := src.Read(r.Want().Use().Use)
		if err != nil {
			return g.Fragment[s.Language]{}, err
		}
		return b.Node(m.Node(s.KindNamedType, m.Attributes("name", "string"), view.Origin))
	})
}
func boolMapping(src *source.Source, b *target.Backend[h.Language]) c.Mapping[h.Language, s.Language] {
	return rules.Direct("bool/golang/1", src, b, func(_ context.Context, r g.UseRequest[h.Language, s.Language]) (g.Fragment[s.Language], error) {
		view, err := src.Read(r.Want().Use().Use)
		if err != nil {
			return g.Fragment[s.Language]{}, err
		}
		return b.Node(m.Node(s.KindNamedType, m.Attributes("name", "bool"), view.Origin))
	})
}
func listMapping(src *source.Source, b *target.Backend[h.Language]) c.Mapping[h.Language, s.Language] {
	return rules.Direct("list/golang/1", src, b, func(_ context.Context, r g.UseRequest[h.Language, s.Language]) (g.Fragment[s.Language], error) {
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
		return b.Node(m.Node(s.KindSliceType, nil, view.Origin), m.Child[s.Language]{Role: s.RoleElement, Fragment: child.Payload()})
	})
}
func recordMapping(src *source.Source, b *target.Backend[h.Language]) c.Mapping[h.Language, s.Language] {
	return rules.Direct("record/golang/1", src, b, func(_ context.Context, r g.UseRequest[h.Language, s.Language]) (g.Fragment[s.Language], error) {
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
			field.Name, err = rules.Exported(field.Name)
			if err != nil {
				return g.Fragment[s.Language]{}, err
			}
			if names[field.Name] {
				return g.Fragment[s.Language]{}, src.Unsupported(use.Use, "native field name collision")
			}
			names[field.Name] = true
			child, err := r.Child(field.Use)
			if err != nil {
				return g.Fragment[s.Language]{}, err
			}
			ty := child.Payload()
			if field.Optional {
				callee, err := r.RequireHelper("horos/optional")
				if err != nil {
					return ty, err
				}
				symbol, err := b.Named(callee)
				if err != nil {
					return ty, err
				}
				ty, err = b.Reference(symbol, []g.Fragment[s.Language]{ty}, field.Origin)
				if err != nil {
					return ty, err
				}
			}
			member, err := b.Node(m.Node(s.KindField, m.Attributes("name", field.Name), field.Origin), m.Child[s.Language]{Role: s.RoleType, Fragment: ty})
			if err != nil {
				return member, err
			}
			children = append(children, m.Child[s.Language]{Role: s.RoleField, Fragment: member})
		}
		return b.Node(m.Node(s.KindStructType, nil, view.Origin), children...)
	})
}
