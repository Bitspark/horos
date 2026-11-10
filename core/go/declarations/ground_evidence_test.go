package declarations_test

// Ground-descriptor evidence for issue #36: declarations materialised by
// Program.GroundDescriptor are ordinary ground descriptors, so every ground
// checker must give them the same judgments. Expected descriptors and verdicts
// are authored from docs/spec/declarations.md §7 and docs/spec/horos.md, never
// read back from a checker. The Go checker is asserted here; with
// -ground-evidence=PATH the same cases are written for the TypeScript lane
// (conformance/ground.py, run by `bash scripts/check.sh conformance`).

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"math/big"
	"os"
	"testing"

	horos "github.com/Bitspark/horos/core/go"
	decl "github.com/Bitspark/horos/core/go/declarations"
	codec "github.com/bitspark/ontos/codec/go"
	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

var groundEvidence = flag.String("ground-evidence", "", "write materialised descriptors and authored judgments as JSON to this path")

type judgment struct {
	name    string
	value   core.Value
	verdict horos.Verdict
}

type evidenceCase struct {
	declaration string
	descriptor  core.Value // authored expectation for GroundDescriptor
	values      []judgment
}

func evidenceRecord(fields ...core.Value) core.Value {
	return h("record", append([]core.Value{a("closed")}, fields...)...)
}

func evidenceField(name, presence string, descriptor core.Value) core.Value {
	return h("field", text(name), a(presence), descriptor)
}

func textMap(t *testing.T, entries ...[2]core.Value) core.Value {
	t.Helper()
	value, err := data.EncodeMap(entries)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

// countTree builds a chain of depth nested CountTree nodes. Each level costs the
// ground checker three descents (record field, list element, ref unfolding),
// so depth 100 completes and depth 200 passes MaxRecursionDepth (512).
func countTree(t *testing.T, depth int, leaf core.Value) core.Value {
	t.Helper()
	node := textMap(t, entry("value", leaf), entry("children", list()))
	for i := 1; i < depth; i++ {
		node = textMap(t, entry("value", integer(int64(i))), entry("children", list(node)))
	}
	return node
}

func groundEvidenceModule() core.Value {
	kData := d("kind-imported", text("k"), text("Data"))
	data0 := func(body core.Value) core.Value { return d("type", list(), kData, body) }
	return module("evidence.ground", mapping(entry("k", decl.PreludeImport())),
		entry("Person", data0(evidenceRecord(
			evidenceField("name", "req", h("scalar", a("utf8-text"))),
			evidenceField("age", "opt", h("scalar", a("int")))))),
		entry("Status", data0(h("union", h("exact", text("active")), h("exact", text("suspended"))))),
		entry("Blob", data0(h("atom"))),
		entry("Count", data0(h("scalar", a("int")))),
		entry("Tree", d("type", list(d("parameter", text("T"), kData)), kData,
			h("rec", a("tree"), evidenceRecord(
				evidenceField("value", "req", param(0)),
				evidenceField("children", "req", h("list-of", h("ref", a("tree")))))))),
		entry("CountTree", data0(local("Tree", local("Count")))),
		entry("Account", data0(evidenceRecord(
			evidenceField("owner", "req", local("Person")),
			evidenceField("status", "req", local("Status")),
			evidenceField("avatar", "opt", local("Blob")),
			evidenceField("balance", "req", local("Count"))))))
}

func groundEvidenceCases(t *testing.T) []evidenceCase {
	text_ := h("scalar", a("utf8-text"))
	count := h("scalar", a("int"))
	person := evidenceRecord(evidenceField("name", "req", text_), evidenceField("age", "opt", count))
	status := h("union", h("exact", text("active")), h("exact", text("suspended")))
	blob := h("atom")
	countTreeDescriptor := h("rec", a("tree"), evidenceRecord(
		evidenceField("value", "req", count),
		evidenceField("children", "req", h("list-of", h("ref", a("tree"))))))
	ada := textMap(t, entry("name", text("Ada")))
	big200 := data.EncodeInt(new(big.Int).Lsh(big.NewInt(1), 200))
	const (
		conforms  = horos.VerdictConforms
		mismatch  = horos.VerdictMismatch
		exhausted = horos.VerdictExhausted
	)
	return []evidenceCase{
		{"Person", person, []judgment{
			{"optional field absent", ada, conforms},
			{"optional field present", textMap(t, entry("name", text("Ada")), entry("age", integer(36))), conforms},
			{"optional field present as null", textMap(t, entry("name", text("Ada")), entry("age", data.EncodeNull())), mismatch},
			{"optional field with wrong scalar", textMap(t, entry("name", text("Ada")), entry("age", text("36"))), mismatch},
			{"required field absent", textMap(t, entry("age", integer(36))), mismatch},
			{"closed record extra field", textMap(t, entry("name", text("Ada")), entry("email", text("ada@example.org"))), mismatch},
			{"list instead of record", list(text("Ada")), mismatch},
		}},
		{"Status", status, []judgment{
			{"first literal", text("active"), conforms},
			{"second literal", text("suspended"), conforms},
			{"other text", text("archived"), mismatch},
			{"raw atom with literal bytes", a("active"), mismatch},
		}},
		{"Blob", blob, []judgment{
			{"binary bytes", core.NewAtom([]byte{0x00, 0xff, 0x10}), conforms},
			{"empty atom", core.NewAtom(nil), conforms},
			{"text embedding", text("bytes"), mismatch},
		}},
		{"Count", count, []judgment{
			{"zero", integer(0), conforms},
			{"negative", integer(-7), conforms},
			{"beyond 128 bits", big200, conforms},
			{"text digits", text("7"), mismatch},
			{"bool", data.EncodeBool(true), mismatch},
		}},
		{"CountTree", countTreeDescriptor, []judgment{
			{"leaf", countTree(t, 1, integer(1)), conforms},
			{"nested within the depth limit", countTree(t, 100, integer(1)), conforms},
			{"nested child with text value", countTree(t, 3, text("one")), mismatch},
			{"missing children", textMap(t, entry("value", integer(1))), mismatch},
			{"nesting beyond the depth limit", countTree(t, 200, integer(1)), exhausted},
			{"exhausting children beside a mismatching value", textMap(t,
				entry("value", text("root")), entry("children", list(countTree(t, 200, integer(1))))), mismatch},
		}},
		{"Account", evidenceRecord(
			evidenceField("owner", "req", person),
			evidenceField("status", "req", status),
			evidenceField("avatar", "opt", blob),
			evidenceField("balance", "req", count)), []judgment{
			{"complete", textMap(t, entry("owner", ada), entry("status", text("active")),
				entry("avatar", core.NewAtom([]byte{0x89, 0x50})), entry("balance", integer(-3))), conforms},
			{"optional avatar absent", textMap(t, entry("owner", ada), entry("status", text("suspended")), entry("balance", integer(0))), conforms},
			{"status outside the literals", textMap(t, entry("owner", ada), entry("status", text("closed")), entry("balance", integer(0))), mismatch},
			{"owner missing its required name", textMap(t, entry("owner", textMap(t)), entry("status", text("active")), entry("balance", integer(0))), mismatch},
		}},
	}
}

// expectedBool is the boolean API's documented answer: true only for a completed
// conforms; an over-limit descent is classified false (docs/spec/horos.md).
func expectedBool(verdict horos.Verdict) bool { return verdict == horos.VerdictConforms }

func TestGroundDescriptorEvidence(t *testing.T) {
	root := groundEvidenceModule()
	p := resolve(t, root, decl.Prelude())
	type outValue struct {
		Name     string `json:"name"`
		Value    string `json:"value"`
		Verdict  string `json:"verdict"`
		Conforms bool   `json:"conforms"`
		Limit    string `json:"limit,omitempty"`
	}
	type outCase struct {
		Declaration string     `json:"declaration"`
		Descriptor  string     `json:"descriptor"`
		Values      []outValue `json:"values"`
	}
	var out []outCase
	for _, c := range groundEvidenceCases(t) {
		value, err := p.Value(id(p, c.declaration))
		if err != nil {
			t.Fatalf("%s: value: %v", c.declaration, err)
		}
		descriptor, err := p.GroundDescriptor(value)
		if err != nil {
			t.Fatalf("%s: ground descriptor: %v", c.declaration, err)
		}
		if !core.Equal(descriptor, c.descriptor) {
			t.Fatalf("%s: materialised %x, want %x", c.declaration, codec.Encode(descriptor), codec.Encode(c.descriptor))
		}
		if !horos.WF(descriptor) || horos.DiagnoseWF(descriptor).Verdict != horos.VerdictConforms {
			t.Fatalf("%s: materialised descriptor is not well formed", c.declaration)
		}
		oc := outCase{Declaration: c.declaration, Descriptor: hex.EncodeToString(codec.Encode(descriptor))}
		for _, j := range c.values {
			diagnosis := horos.Diagnose(j.value, descriptor)
			if diagnosis.Verdict != j.verdict {
				t.Errorf("%s / %s: Go diagnose %s, want %s", c.declaration, j.name, diagnosis.Verdict, j.verdict)
			}
			if (diagnosis.Verdict == horos.VerdictExhausted) != (diagnosis.Limit == horos.LimitRecursionDepth) {
				t.Errorf("%s / %s: Go diagnose limit %q with verdict %s", c.declaration, j.name, diagnosis.Limit, diagnosis.Verdict)
			}
			if got := horos.Conforms(j.value, descriptor); got != expectedBool(j.verdict) {
				t.Errorf("%s / %s: Go conforms %v, want %v", c.declaration, j.name, got, expectedBool(j.verdict))
			}
			encoded := codec.Encode(j.value)
			if decoded, err := codec.Decode(encoded); err != nil || !core.Equal(decoded, j.value) {
				t.Fatalf("%s / %s: value does not round-trip through the codec: %v", c.declaration, j.name, err)
			}
			oc.Values = append(oc.Values, outValue{Name: j.name, Value: hex.EncodeToString(encoded),
				Verdict: string(j.verdict), Conforms: expectedBool(j.verdict), Limit: string(diagnosis.Limit)})
		}
		out = append(out, oc)
	}

	// The constructor is materialised only once applied: the declaration and an
	// explicit application of Tree to Count yield the same ground descriptor.
	tree, err := p.Value(id(p, "Tree"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.GroundDescriptor(tree); err == nil {
		t.Fatal("an unapplied constructor materialised as a ground descriptor")
	}
	countValue, err := p.Value(id(p, "Count"))
	if err != nil {
		t.Fatal(err)
	}
	applied, err := p.Apply(tree, []decl.TypeValue{countValue})
	if err != nil {
		t.Fatal(err)
	}
	appliedDescriptor, err := p.GroundDescriptor(applied)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(codec.Encode(appliedDescriptor)); got != out[4].Descriptor || out[4].Declaration != "CountTree" {
		t.Fatalf("Tree<Count> materialised %s, want the CountTree descriptor", got)
	}

	if t.Failed() || *groundEvidence == "" {
		return
	}
	encoded, err := json.MarshalIndent(map[string]any{"cases": out}, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(*groundEvidence, append(encoded, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
