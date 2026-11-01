// Command horos-conform evaluates descriptor judgments from ontos-codec-v1 hex.
package main

import (
	"encoding/hex"
	"fmt"
	"os"

	horos "github.com/Bitspark/horos/core/go"
	codec "github.com/bitspark/ontos/codec/go"
)

func run(args []string) (bool, error) {
	if len(args) != 2 && len(args) != 3 {
		return false, fmt.Errorf("usage: horos-conform wf <descriptor> | conforms <descriptor> <value>")
	}
	decode := func(s string) (horos.Value, error) {
		b, err := hex.DecodeString(s)
		if err != nil { return nil, err }
		return codec.Decode(b)
	}
	desc, err := decode(args[1])
	if err != nil { return false, err }
	if args[0] == "wf" && len(args) == 2 { return horos.WF(desc), nil }
	if args[0] == "conforms" && len(args) == 3 {
		value, err := decode(args[2])
		if err != nil { return false, err }
		return horos.Conforms(value, desc), nil
	}
	return false, fmt.Errorf("unknown judgment or wrong arity")
}

func main() {
	v, err := run(os.Args[1:])
	if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	fmt.Println(v)
}
