package horos

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Replays the hand-authored location corpus (vectors/diagnose.json, ADR 0009) through
// DiagnoseWF and Diagnose, comparing the contract JSON form of each result.
func TestDiagnosticLocationsReplay(t *testing.T) {
	if checked := replayLocated(t, "diagnose.json", "wf"); checked < 100 {
		t.Errorf("expected at least 100 located judgments, got %d", checked)
	}
}

// Replays the diagnoses of the ref-scoping oracle (vectors/recursion-scope.json,
// ADR 0011), whose booleans TestVectorsReplay checks. Its wf diagnosis is diagnose_wf,
// since its wf is the boolean.
func TestRecursionScopeLocationsReplay(t *testing.T) {
	if checked := replayLocated(t, "recursion-scope.json", "diagnose_wf"); checked == 0 {
		t.Error("no located judgments")
	}
}

// replayLocated checks every diagnosis of a located corpus in testdata; wfKey names the
// case member that holds the expected DiagnoseWF result.
func replayLocated(t *testing.T, file, wfKey string) int {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatalf("read testdata/%s: %v", file, err)
	}
	var doc struct {
		Cases []struct {
			Name       string `json:"name"`
			Descriptor string `json:"descriptor"`
			Values     []struct {
				Note      string          `json:"note"`
				Value     string          `json:"value"`
				Diagnosis json.RawMessage `json:"diagnosis"`
			} `json:"values"`
		} `json:"cases"`
	}
	var wfs struct {
		Cases []map[string]json.RawMessage `json:"cases"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse testdata/%s: %v", file, err)
	}
	if err := json.Unmarshal(raw, &wfs); err != nil {
		t.Fatalf("parse testdata/%s: %v", file, err)
	}
	if len(doc.Cases) == 0 {
		t.Fatalf("testdata/%s has no cases", file)
	}
	same := func(label string, got Diagnosis, want json.RawMessage) {
		t.Helper()
		gotJSON, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("%s: marshal: %v", label, err)
		}
		var g, w any
		if err := json.Unmarshal(gotJSON, &g); err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if err := json.Unmarshal(want, &w); err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if !reflect.DeepEqual(g, w) {
			t.Errorf("%s:\n got  %s\n want %s", label, gotJSON, want)
		}
	}
	checked := 0
	for k, c := range doc.Cases {
		desc := decodeHex(t, c.Name, "descriptor", c.Descriptor)
		want, ok := wfs.Cases[k][wfKey]
		if !ok {
			t.Fatalf("%s: case %s has no %s", file, c.Name, wfKey)
		}
		same(c.Name+" wf", DiagnoseWF(desc), want)
		checked++
		for i, v := range c.Values {
			val := decodeHex(t, c.Name, "value", v.Value)
			same(fmt.Sprintf("%s / value[%d] %s", c.Name, i, v.Note), Diagnose(val, desc), v.Diagnosis)
			checked++
		}
	}
	return checked
}

// A conforming diagnosis carries no location; every other one does.
func TestLocationPresenceFollowsTheVerdict(t *testing.T) {
	if d := Diagnose(dAtom("x"), desc("any")); d.Location != nil {
		t.Errorf("conforms carries a location: %+v", d.Location)
	}
	d := Diagnose(dAtom("x"), dScalarInt())
	if d.Location == nil || d.Location.Reason != ReasonRefused || len(d.Location.Path) != 0 || len(d.Location.At) != 0 {
		t.Errorf("mismatch location: %+v", d.Location)
	}
	w := DiagnoseWF(desc("scalar", dAtom("float")))
	if w.Location == nil || w.Location.Reason != ReasonArgument || w.Location.Arg != 1 {
		t.Errorf("malformed location: %+v", w.Location)
	}
	e := Diagnose(dDeepCons(171), dConsList())
	if e.Location == nil || e.Location.Reason != ReasonLimit || len(e.Location.Path) != 171 {
		t.Errorf("exhausted location: %+v", e.Location)
	}
}
