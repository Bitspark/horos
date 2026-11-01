package horos

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	codec "github.com/bitspark/ontos/codec/go"
)

// vectorDoc mirrors vectors/conformance.json: the shared, hand-authored oracle the
// three cores (go/rs/ts) replay byte-for-byte. descriptor and value are
// ontos-codec-v1 lowercase hex; wf/conforms are the expected judgments authored
// from docs/spec/horos.md.
type vectorDoc struct {
	Version string       `json:"version"`
	Note    string       `json:"note"`
	Cases   []vectorCase `json:"cases"`
}

type vectorCase struct {
	Name       string        `json:"name"`
	Descriptor string        `json:"descriptor"`
	WF         bool          `json:"wf"`
	Values     []vectorValue `json:"values"`
}

type vectorValue struct {
	Value    string `json:"value"`
	Conforms bool   `json:"conforms"`
	Note     string `json:"note"`
}

// decodeHex decodes lowercase hex into bytes, then ontos-codec-v1 into a Value.
// This is the exact replay path the Rust core runs (vectors.rs::vectors_replay):
// hex -> bytes -> codec.Decode -> WF/Conforms.
func decodeHex(t *testing.T, name, what, h string) Value {
	t.Helper()
	raw, err := hex.DecodeString(h)
	if err != nil {
		t.Fatalf("%s: %s hex decode failed: %v", name, what, err)
	}
	v, err := codec.Decode(raw)
	if err != nil {
		t.Fatalf("%s: %s codec.Decode failed: %v", name, what, err)
	}
	return v
}

// TestVectorsReplay replays the committed vectors/conformance.json — decode each
// descriptor/value from ontos-codec-v1 hex and re-check WF/Conforms. This proves
// Go == the oracle == Rust (the three cores agree on one oracle, byte-for-byte).
func TestVectorsReplay(t *testing.T) {
	// vectors/conformance.json sits at <repo>/vectors/, i.e. ../../vectors from
	// core/go (mirroring the Rust core's CARGO_MANIFEST_DIR/../../vectors).
	path := filepath.Join("testdata", "conformance.json")
	txt, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (run the Rust core with HOROS_EMIT_VECTORS=1 first)", path, err)
	}
	var doc vectorDoc
	if err := json.Unmarshal(txt, &doc); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	if len(doc.Cases) == 0 {
		t.Fatalf("%s: no cases", path)
	}
	for _, c := range doc.Cases {
		desc := decodeHex(t, c.Name, "descriptor", c.Descriptor)
		if got := WF(desc); got != c.WF {
			t.Errorf("WF(%s) = %v, want %v", c.Name, got, c.WF)
		}
		for _, v := range c.Values {
			val := decodeHex(t, c.Name, "value", v.Value)
			if got := Conforms(val, desc); got != v.Conforms {
				t.Errorf("Conforms(%s / %s) = %v, want %v", c.Name, v.Note, got, v.Conforms)
			}
		}
	}
}
