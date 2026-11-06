package consumer_test

import (
	"context"
	analyze "github.com/Bitspark/bittype/analyze/files/horos/go"
	"github.com/Bitspark/bittype/contracts/analysis/go"
	"github.com/Bitspark/bittype/contracts/check/go"
	"github.com/Bitspark/bittype/contracts/parse/go"
	"github.com/Bitspark/bittype/contracts/resolution/go"
	"github.com/Bitspark/bittype/contracts/semantic/go"
	"github.com/Bitspark/bittype/contracts/transform/go"
	"github.com/Bitspark/bittype/engine/go"
	hp "github.com/Bitspark/bittype/provider/horos/go"
	tp "github.com/Bitspark/bittype/provider/typescript/go"
	resolve "github.com/Bitspark/bittype/resolve/files/horos/go"
	h "github.com/Bitspark/bittype/syntax/horos/go"
	ts "github.com/Bitspark/bittype/syntax/typescript/go"
	"github.com/Bitspark/bittype/trees/ast/go"
	"github.com/Bitspark/bittype/trees/representation/go"
	generate "github.com/Bitspark/horos/tooling/bittype/transforms/horos-to-typescript/go"
	"strings"
	"testing"
)

func TestInstalledSemanticGeneration(t *testing.T) {
	ctx := context.Background()
	source, err := (hp.Provider{}).Parse(context.Background(), parse.Input{SourceID: "input.horos", TreeID: "input", Text: "module installed; type Page<T> = { items: list<T>; next?: text; }; type People = Page<text>;"})
	if err != nil {
		t.Fatal(err)
	}
	from, err := representation.NewLanguage[h.Language](h.Language{}, h.Profile)
	if err != nil {
		t.Fatal(err)
	}
	to, err := representation.NewLanguage[ts.Language](ts.Language{}, "native-types-v1")
	if err != nil {
		t.Fatal(err)
	}
	e := engine.New()
	for _, d := range []representation.Language{from, to} {
		if err = e.RegisterLanguage(d); err != nil {
			t.Fatal(err)
		}
	}
	hc, err := check.NewChecker[h.Language](from, hp.Provider{})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := check.NewChecker[ts.Language](to, tp.Provider{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []check.Checker{hc, tc} {
		if err = e.RegisterChecker(c); err != nil {
			t.Fatal(err)
		}
	}
	resolver, err := resolution.NewResolver[h.Language](from, resolve.Contract(), hp.Provider{})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.RegisterResolver(resolver); err != nil {
		t.Fatal(err)
	}
	analyzer, err := analysis.NewAnalyzer[h.Language](from, analyze.Contract(), hp.Provider{})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.RegisterAnalyzer(analyzer); err != nil {
		t.Fatal(err)
	}
	mapper, err := generate.New(from, to, generate.Config{TreeID: "output", Profile: generate.RepresentationProfile})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.RegisterTransformer(mapper); err != nil {
		t.Fatal(err)
	}
	packed, err := representation.Pack(from, source)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := e.Capture(ctx, semantic.Input{Entry: ast.NodeRef{Tree: source.Abstract.ID, Node: source.Abstract.Root}, Trees: []representation.AST{packed}})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := e.Resolve(ctx, snapshot, semantic.Options{})
	if err != nil {
		t.Fatal(err)
	}
	analyzed, err := e.Analyze(ctx, analysis.Input{Snapshot: snapshot, Resolution: &resolved})
	if err != nil {
		t.Fatal(err)
	}
	output, err := e.TransformTo(ctx, transform.Input{Source: snapshot, Evidence: []semantic.Result{analyzed}}, to.ID())
	if err != nil {
		t.Fatal(err)
	}
	typed, err := representation.Unpack[ts.Language](to, output.EntryAST())
	if err != nil {
		t.Fatal(err)
	}
	concrete, err := (tp.Provider{}).Format(context.Background(), typed)
	if err != nil {
		t.Fatal(err)
	}
	text, err := (tp.Provider{}).Print(context.Background(), concrete)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "export interface Page<T>") || !strings.Contains(text, "next?: string;") || !strings.Contains(text, "export type People = Page<string>;") {
		t.Fatal(text)
	}
}
