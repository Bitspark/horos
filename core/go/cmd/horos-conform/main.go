// Command horos-conform evaluates descriptor judgments from ontos-codec-v1 hex.
package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	horos "github.com/Bitspark/horos/core/go"
	codec "github.com/bitspark/ontos/codec/go"
)

const usage = "usage: horos-conform wf <descriptor> | conforms <descriptor> <value> | diagnose-wf <descriptor> | diagnose <descriptor> <value>"

// run prints one judgment: a boolean for wf/conforms, the contract JSON for diagnose-wf/diagnose.
func run(args []string) (string, error) {
	if len(args) != 2 && len(args) != 3 {
		return "", fmt.Errorf(usage)
	}
	decode := func(s string) (horos.Value, error) {
		b, err := hex.DecodeString(s)
		if err != nil {
			return nil, err
		}
		return codec.Decode(b)
	}
	desc, err := decode(args[1])
	if err != nil {
		return "", err
	}
	located := func(d horos.Diagnosis) (string, error) {
		out, err := json.Marshal(d)
		return string(out), err
	}
	switch {
	case args[0] == "wf" && len(args) == 2:
		return fmt.Sprint(horos.WF(desc)), nil
	case args[0] == "diagnose-wf" && len(args) == 2:
		return located(horos.DiagnoseWF(desc))
	case (args[0] == "conforms" || args[0] == "diagnose") && len(args) == 3:
		value, err := decode(args[2])
		if err != nil {
			return "", err
		}
		if args[0] == "diagnose" {
			return located(horos.Diagnose(value, desc))
		}
		return fmt.Sprint(horos.Conforms(value, desc)), nil
	}
	return "", fmt.Errorf("unknown judgment or wrong arity")
}

func main() {
	v, err := run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(v)
}
