package declarations

import (
	"crypto/sha256"
	"math/big"

	codec "github.com/bitspark/ontos/codec/go"
	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

var standardPrelude = buildPrelude()
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
	return preludeForm("import", preludeText("horos.kinds"), core.NewAtom(pin[:]))
}

func preludeForm(tag string, values ...core.Value) core.Value {
	items := make([]core.Value, len(values)+1)
	items[0] = core.NewAtom([]byte("horos/decl/" + tag))
	copy(items[1:], values)
	return core.NewTuple(items...)
}

func preludeText(value string) core.Value { return data.MustEncodeText(value) }
func preludeInteger(value int64) core.Value { return data.EncodeInt(big.NewInt(value)) }
func preludeList(values ...core.Value) core.Value { return data.EncodeList(values) }

func preludeMap(entries ...[2]core.Value) core.Value {
	value, err := data.EncodeMap(entries)
	if err != nil {
		panic("invalid ordinary Horos prelude map: " + err.Error())
	}
	return value
}

func preludeReference(name string) core.Value {
	return preludeForm("kind-local", preludeText(name))
}

func preludeApplication(name string, arguments ...core.Value) core.Value {
	return preludeForm("kind-apply", preludeReference(name), preludeList(arguments...))
}

func preludeNode(form string, selectors core.Value, sequence core.Value) core.Value {
	return preludeForm("node", preludeText(form), selectors, sequence)
}

func preludeKind(body core.Value, parameters ...string) core.Value {
	proper := preludeForm("sort-proper")
	inputs := make([]core.Value, len(parameters))
	for i, name := range parameters {
		inputs[i] = preludeForm("kind-parameter", preludeText(name), proper)
	}
	return preludeForm("kind", preludeList(inputs...), proper, body)
}

// buildPrelude authors canonical kind definitions. No category is recognized
// by name in membership or inclusion; even Data is an ordinary guarded union.
func buildPrelude() core.Value {
	proper := preludeForm("sort-proper")
	empty := preludeMap()
	epsilon := preludeForm("epsilon")
	parameter := func(index int64) core.Value {
		return preludeForm("kind-param", preludeInteger(index))
	}
	item := func(kind core.Value) core.Value { return preludeForm("item", kind) }
	many := func(kind core.Value) core.Value { return preludeForm("star", item(kind)) }
	union := func(kinds ...core.Value) core.Value {
		return preludeForm("or", proper, preludeList(kinds...))
	}
	leaf := func(form string) core.Value { return preludeNode(form, empty, epsilon) }
	selected := func(form, key, value string, sequence core.Value) core.Value {
		selectors := preludeMap([2]core.Value{preludeText(key), preludeText(value)})
		return preludeNode(form, selectors, sequence)
	}
	var entries [][2]core.Value
	define := func(name string, body core.Value, parameters ...string) {
		entries = append(entries, [2]core.Value{preludeText(name), preludeKind(body, parameters...)})
	}

	define("Type", preludeForm("top", proper))
	define("Any", leaf("any"))
	define("AtomFamily", union(
		leaf("atom"), leaf("atom-exact"), leaf("atom-len"), leaf("atom-in"),
	))
	define("Int", selected("scalar", "selector", "int", epsilon))
	define("Text", selected("scalar", "selector", "utf8-text", epsilon))
	define("Bool", selected("scalar", "selector", "bool", epsilon))
	define("Scalar", union(
		preludeReference("Int"), preludeReference("Text"), preludeReference("Bool"),
		leaf("text-len"), leaf("key-grammar"), leaf("text-non-blank"), leaf("lowercase-uuid"),
	))
	define("Exact", leaf("exact"))

	for _, family := range [][2]string{
		{"List", "list-of"}, {"Seq", "seq"}, {"Set", "set-of"},
		{"Not", "not"}, {"Wire", "wire"},
	} {
		define(family[0], preludeNode(family[1], empty, item(parameter(0))), "K")
	}
	for _, family := range [][2]string{
		{"Tuple", "tuple"}, {"Compound", "compound"},
		{"UnionOf", "union"}, {"AndOf", "and"},
	} {
		define(family[0], preludeNode(family[1], empty, many(parameter(0))), "K")
	}
	define("Map", preludeNode("map-of", empty,
		preludeForm("concat", preludeList(item(parameter(0)), item(parameter(1))))), "K", "V")
	define("Record", preludeNode("record", empty, many(parameter(0))), "K")
	define("ClosedRecord", selected("record", "mode", "closed", many(parameter(0))), "K")
	define("OpenRecord", selected("record", "mode", "open", many(parameter(0))), "K")

	recursive := preludeForm("recursive-var", preludeInteger(0), preludeInteger(0))
	dataBody := union(
		preludeReference("Any"), preludeReference("AtomFamily"),
		preludeReference("Scalar"), preludeReference("Exact"),
		preludeApplication("Tuple", recursive),
		preludeApplication("Compound", recursive),
		preludeApplication("Seq", recursive),
		preludeApplication("List", recursive),
		preludeApplication("Set", recursive),
		preludeApplication("Map", recursive, recursive),
		preludeApplication("ClosedRecord", recursive),
		preludeApplication("OpenRecord", recursive),
		preludeApplication("UnionOf", recursive),
		preludeApplication("AndOf", recursive),
		preludeApplication("Not", recursive),
	)
	define("Data", preludeForm("nu", preludeList(dataBody), preludeInteger(0)))

	return preludeForm("module", preludeInteger(4), preludeText("horos.kinds"),
		empty, preludeMap(entries...))
}
