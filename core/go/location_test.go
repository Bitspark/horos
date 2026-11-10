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
	raw, err := os.ReadFile(filepath.Join("testdata", "diagnose.json"))
	if err != nil {
		t.Fatalf("read testdata/diagnose.json: %v", err)
	}
	var doc struct {
		Cases []struct {
			Name       string          `json:"name"`
			Descriptor string          `json:"descriptor"`
			WF         json.RawMessage `json:"wf"`
			Values     []struct {
				Note      string          `json:"note"`
				Value     string          `json:"value"`
				Diagnosis json.RawMessage `json:"diagnosis"`
			} `json:"values"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse testdata/diagnose.json: %v", err)
	}
	if len(doc.Cases) == 0 {
		t.Fatal("testdata/diagnose.json has no cases")
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
	for _, c := range doc.Cases {
		desc := decodeHex(t, c.Name, "descriptor", c.Descriptor)
		same(c.Name+" wf", DiagnoseWF(desc), c.WF)
		checked++
		for i, v := range c.Values {
			val := decodeHex(t, c.Name, "value", v.Value)
			same(fmt.Sprintf("%s / value[%d] %s", c.Name, i, v.Note), Diagnose(val, desc), v.Diagnosis)
			checked++
		}
	}
	if checked < 100 {
		t.Errorf("expected at least 100 located judgments, got %d", checked)
	}
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
