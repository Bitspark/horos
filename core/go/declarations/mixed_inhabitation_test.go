package declarations_test

// Form checks for vectors/mixed-inhabitation.json (ADR 0012). Horos ships no
// checker for mixed inhabitation; a binding backend replays the corpus. These
// tests establish only that the corpus is about what it claims to be about:
// every descriptor is a materialized Descriptor, every value is a well-formed
// mixed value tree, and where a descriptor has no wire, every authored verdict
// is the existing ground judgment (decision 2), which needs no mixed checker.

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	horos "github.com/Bitspark/horos/core/go"
	decl "github.com/Bitspark/horos/core/go/declarations"
	codec "github.com/bitspark/ontos/codec/go"
	core "github.com/bitspark/ontos/core/go"
)

type mixedCorpus struct {
	Format string `json:"format"`
	Cases  []struct {
		Name       string `json:"name"`
		Descriptor string `json:"descriptor"`
		Values     []struct {
			Name    string          `json:"name"`
			Value   json.RawMessage `json:"value"`
			Verdict string          `json:"verdict"`
		} `json:"values"`
	} `json:"cases"`
}

// mixedTree reads the value-tree notation. It returns the ground value when the
// tree holds no wire leaf, and ok=false when it holds one.
func mixedTree(t *testing.T, raw json.RawMessage) (core.Value, bool) {
	t.Helper()
	var node map[string]json.RawMessage
	if err := json.Unmarshal(raw, &node); err != nil || len(node) != 1 {
		t.Fatalf("value %s: want exactly one of atom, tuple or wire", raw)
	}
	if body, ok := node["atom"]; ok {
		var s string
		if err := json.Unmarshal(body, &s); err != nil {
			t.Fatalf("atom %s: %v", body, err)
		}
		b, err := hex.DecodeString(s)
		if err != nil || hex.EncodeToString(b) != s {
			t.Fatalf("atom %q: want lowercase hex", s)
		}
		return core.NewAtom(b), true
	}
	if body, ok := node["wire"]; ok {
		var name string
		if err := json.Unmarshal(body, &name); err != nil || name == "" {
			t.Fatalf("wire %s: want a nonempty name", body)
		}
		return nil, false
	}
	body, ok := node["tuple"]
	if !ok {
		t.Fatalf("value %s: unknown leaf", raw)
	}
	var items []json.RawMessage
	if err := json.Unmarshal(body, &items); err != nil {
		t.Fatalf("tuple %s: %v", body, err)
	}
	values := make([]core.Value, 0, len(items))
	ground := true
	for _, item := range items {
		v, ok := mixedTree(t, item)
		ground = ground && ok
		values = append(values, v)
	}
	if !ground {
		return nil, false
	}
	return core.NewTuple(values...), true
}

func TestMixedInhabitationCorpusForm(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "testdata", "mixed-inhabitation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus mixedCorpus
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if corpus.Format != "horos-mixed-inhabitation-v1" || len(corpus.Cases) == 0 {
		t.Fatalf("unexpected corpus %q with %d cases", corpus.Format, len(corpus.Cases))
	}
	p := resolve(t, module("example.mixed", preludeImports()), decl.Prelude())
	verdicts := map[string]horos.Verdict{
		"conforms": horos.VerdictConforms, "mismatch": horos.VerdictMismatch, "exhausted": horos.VerdictExhausted,
	}
	mixed, groundOnly := 0, 0
	for _, c := range corpus.Cases {
		bytes, err := hex.DecodeString(c.Descriptor)
		if err != nil {
			t.Fatalf("%s: descriptor hex: %v", c.Name, err)
		}
		descriptor, err := codec.Decode(bytes)
		if err != nil {
			t.Fatalf("%s: descriptor: %v", c.Name, err)
		}
		// The descriptor is a closed proper type that Descriptor materializes unchanged.
		value, err := p.Proper(descriptor)
		if err != nil {
			t.Fatalf("%s: not a closed proper type: %v", c.Name, err)
		}
		materialized, err := p.Descriptor(value)
		if err != nil {
			t.Fatalf("%s: Descriptor: %v", c.Name, err)
		}
		if !core.Equal(materialized, descriptor) {
			t.Fatalf("%s: Descriptor changed the expression", c.Name)
		}
		groundDescriptor, groundErr := p.GroundDescriptor(value)
		var detail *decl.Error
		if groundErr != nil && (!errors.As(groundErr, &detail) || detail.Code != decl.BoundViolation) {
			t.Fatalf("%s: GroundDescriptor: %v", c.Name, groundErr)
		}
		if groundErr != nil {
			mixed++
		} else {
			groundOnly++
		}
		for _, v := range c.Values {
			want, ok := verdicts[v.Verdict]
			if !ok {
				t.Fatalf("%s / %s: unknown verdict %q", c.Name, v.Name, v.Verdict)
			}
			ground, wireFree := mixedTree(t, v.Value)
			if groundErr != nil {
				continue // A descriptor with wire: only a backend judges these values.
			}
			// Decision 2: without wire, inhabitation is ground conformance of the recovered value.
			if !wireFree {
				if want != horos.VerdictMismatch {
					t.Errorf("%s / %s: a capability under a ground descriptor must be %s, corpus says %s", c.Name, v.Name, horos.VerdictMismatch, want)
				}
				continue
			}
			if got := horos.Diagnose(ground, groundDescriptor).Verdict; got != want {
				t.Errorf("%s / %s: ground judgment %s, corpus says %s", c.Name, v.Name, got, want)
			}
		}
	}
	if mixed == 0 || groundOnly == 0 {
		t.Fatalf("want cases with and without wire, got %d and %d", mixed, groundOnly)
	}
}

// The specification lists the positions where wire can occur in a materialized
// Descriptor. Every other type position takes a Data operand and refuses it.
func TestMixedInhabitationWirePositions(t *testing.T) {
	p := resolve(t, module("example.mixed", preludeImports()), decl.Prelude())
	w := h("wire", h("scalar", a("int")))
	admitted := map[string]core.Value{
		"root":          w,
		"tuple":         h("tuple", w),
		"compound":      h("compound", a("label"), w),
		"list-of":       h("list-of", w),
		"closed record": h("record", a("closed"), h("field", text("f"), a("req"), w)),
		"union":         h("union", w, h("scalar", a("int"))),
		"wire message":  h("wire", h("tuple", w)),
	}
	refused := map[string]core.Value{
		"open record":  h("record", a("open"), h("field", text("f"), a("req"), w)),
		"seq":          h("seq", w),
		"set-of":       h("set-of", w),
		"map-of key":   h("map-of", w, h("scalar", a("int"))),
		"map-of value": h("map-of", h("scalar", a("utf8-text")), w),
		"and":          h("and", w, w),
		"not":          h("not", w),
		"rec":          h("rec", a("X"), h("union", w, h("tuple", h("ref", a("X"))))),
	}
	for name, expr := range admitted {
		v, err := p.Proper(expr)
		if err != nil {
			t.Errorf("%s: wire refused: %v", name, err)
			continue
		}
		if out, err := p.Descriptor(v); err != nil || !core.Equal(out, expr) {
			t.Errorf("%s: Descriptor %v", name, err)
		}
	}
	for name, expr := range refused {
		_, err := p.Proper(expr)
		var detail *decl.Error
		if !errors.As(err, &detail) || detail.Code != decl.BoundViolation {
			t.Errorf("%s: want a bound violation, got %v", name, err)
		}
	}
}
