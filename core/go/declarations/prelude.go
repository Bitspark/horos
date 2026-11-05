package declarations

import (
	"crypto/sha256"
	_ "embed"

	codec "github.com/bitspark/ontos/codec/go"
	core "github.com/bitspark/ontos/core/go"
)

//go:embed standard/horos.kinds.ontos
var standardPreludeEncoding []byte

var standardPrelude = decodePrelude()
var standardPreludePin = Digest(sha256.Sum256(codec.Encode(standardPrelude)))

// Prelude returns the ordinary canonical horos.kinds module. Its definitions
// use the same kind language as consumer modules. Source references still
// require an explicit ordinary import; resolver formation uses its own context.
func Prelude() core.Value { return standardPrelude }

// PreludePin is the module address used by ordinary declaration imports.
func PreludePin() Digest { return standardPreludePin }

// PreludeImport returns the canonical import value for the ordinary prelude.
// The importing module chooses its own local alias.
func PreludeImport() core.Value {
	pin := PreludePin()
	return declarationForm("import", declarationText("horos.kinds"), core.NewAtom(pin[:]))
}

// The standard module is authored in standard/horos.kinds.horos and lowered by
// Bittype's ordinary module encoder before packaging. Resolution admits this
// decoded value through the same module loader and kind calculus as user input.
func decodePrelude() core.Value {
	value, err := codec.Decode(standardPreludeEncoding)
	if err != nil {
		panic("decode bundled Horos prelude: " + err.Error())
	}
	return value
}
