package generation

import (
	"context"
	g "github.com/Bitspark/bittype/contracts/generation/go"
	m "github.com/Bitspark/bittype/generation/syntax/target/go"
	target "github.com/Bitspark/bittype/generation/syntax/target/golang/go"
	s "github.com/Bitspark/bittype/syntax/golang/go"
	h "github.com/Bitspark/bittype/syntax/horos/go"
	"github.com/Bitspark/bittype/trees/ast/go"
)

type helperDefiner func(context.Context, g.DefineRequest[h.Language, s.Language]) (g.Definition[h.Language, s.Language], error)

func (f helperDefiner) Define(ctx context.Context, r g.DefineRequest[h.Language, s.Language]) (g.Definition[h.Language, s.Language], error) {
	return f(ctx, r)
}

// The optional representation is a native-only helper owned by this mapping.
// Its native T is explicit; no source declaration or source binder is fabricated.
func optionalHelper(b *target.Backend[h.Language]) g.Plan[h.Language, s.Language] {
	return g.Plan[h.Language, s.Language]{Key: "horos/optional", Acquire: func(_ context.Context, site g.Site[h.Language, s.Language]) (g.Destination[s.Language], bool, error) {
		return b.Reserve(site, "BittypeOptional", true, ast.NodeRef{})
	},
		Header: func(_ context.Context, site g.Site[h.Language, s.Language], d g.Destination[s.Language]) (g.Layout[s.Language], error) {
			return b.Header(site, d, []string{"T"}, []ast.NodeRef{{}})
		},
		Definer: helperDefiner(func(_ context.Context, r g.DefineRequest[h.Language, s.Language]) (g.Definition[h.Language, s.Language], error) {
			symbol, err := b.Formal(r.Callee().Layout(), 0)
			if err != nil {
				return g.Definition[h.Language, s.Language]{}, err
			}
			value, err := b.Reference(symbol, nil, ast.NodeRef{})
			if err != nil {
				return g.Definition[h.Language, s.Language]{}, err
			}
			boolean, err := b.Node(m.Node(s.KindNamedType, m.Attributes("name", "bool"), ast.NodeRef{}))
			if err != nil {
				return g.Definition[h.Language, s.Language]{}, err
			}
			var fields []m.Child[s.Language]
			for _, field := range []struct {
				name string
				ty   g.Fragment[s.Language]
			}{{"Present", boolean}, {"Value", value}} {
				node, err := b.Node(m.Node(s.KindField, m.Attributes("name", field.name), ast.NodeRef{}), m.Child[s.Language]{Role: s.RoleType, Fragment: field.ty})
				if err != nil {
					return g.Definition[h.Language, s.Language]{}, err
				}
				fields = append(fields, m.Child[s.Language]{Role: s.RoleField, Fragment: node})
			}
			body, err := b.Node(m.Node(s.KindStructType, nil, ast.NodeRef{}), fields...)
			if err != nil {
				return g.Definition[h.Language, s.Language]{}, err
			}
			return r.CompleteBody(body)
		}),
	}
}
