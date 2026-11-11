package declarations_test

// Structural mutation for FuzzResolveEdited and import-pin repair for every
// module-set target. Both keep valid ontos values, so the resolver rather
// than the codec sees the fuzzer's variations.

import (
	"crypto/sha256"
	"slices"

	decl "github.com/Bitspark/horos/core/go/declarations"
	codec "github.com/bitspark/ontos/codec/go"
	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

const (
	maxEdits      = 24
	maxEditNodes  = 1 << 15
	maxEditDepth  = 300
	maxDictionary = 512
)

// Vocabulary of revision-4 declarations (declarations.md §2) and of the
// descriptor forms the grammar knows, so replacements produce plausible tags.
var declarationTags = []string{
	"module", "import", "sort-proper", "sort-arrow", "kind-local", "kind-imported",
	"top", "bottom", "kind-param", "kind-apply", "or", "and", "node", "nu",
	"recursive-var", "arrow", "epsilon", "item", "concat", "alternative", "star",
	"kind-parameter", "parameter", "kind", "type", "param", "local", "imported", "apply",
}
var descriptorTags = []string{
	"any", "atom", "atom-exact", "atom-len", "atom-in", "exact", "int-min", "int-max",
	"text-len", "key-grammar", "text-non-blank", "lowercase-uuid", "scalar", "wire",
	"seq", "list-of", "set-of", "not", "map-of", "tuple", "compound", "union", "and",
	"record", "field", "rec", "ref",
}
var vocabularyWords = []string{
	"int", "utf8-text", "bool", "open", "closed", "req", "opt", "x", "y",
	"k", "Type", "Data", "Scalar", "List", "Record", "Exact", "T", "selector", "mode",
}

func staticDictionary() []core.Value {
	var dict []core.Value
	for _, tag := range declarationTags {
		dict = append(dict, a("horos/decl/"+tag))
	}
	for _, tag := range descriptorTags {
		dict = append(dict, a("horos/"+tag))
	}
	for _, word := range vocabularyWords {
		dict = append(dict, a(word), text(word))
	}
	proper := properSort()
	dict = append(dict,
		proper, d("sort-arrow", list(proper), proper),
		d("top", proper), d("bottom", proper), d("epsilon"), d("item", d("top", proper)),
		d("star", d("item", d("kind-param", integer(0)))), d("kind-param", integer(0)),
		d("recursive-var", integer(0), integer(0)), d("param", integer(0)), d("param", integer(1)),
		kType(), kData(), kPrelude("List", kData()), kArrow(kType(), kType()),
		d("nu", list(kOr(d("node", text("list-of"), mapping(), d("item", d("recursive-var", integer(0), integer(0)))), kType())), integer(0)),
		h("any"), h("scalar", a("int")), h("scalar", a("utf8-text")), h("list-of", h("any")),
		h("rec", a("x"), h("list-of", h("ref", a("x")))), h("ref", a("x")), h("wire", h("any")),
		list(), mapping(), integer(0), integer(1), integer(-1), integer(4),
		decl.PreludeImport(), tparam("T", kType()),
	)
	return dict
}

var baseDictionary = staticDictionary()

// editor reads an edit script. Every byte string is a valid script; reading
// past its end stops editing.
type editor struct {
	script []byte
	at     int
}

func (e *editor) next() (int, bool) {
	if e.at >= len(e.script) {
		return 0, false
	}
	e.at++
	return int(e.script[e.at-1]), true
}

func (e *editor) wide() (int, bool) {
	hi, ok := e.next()
	if !ok {
		return 0, false
	}
	lo, ok := e.next()
	return hi<<8 | lo, ok
}

// paths lists every node's path in preorder, or fails for oversized trees.
func paths(root core.Value) ([][]int, bool) {
	var out [][]int
	var walk func(core.Value, []int) bool
	walk = func(v core.Value, path []int) bool {
		if len(out) >= maxEditNodes || len(path) > maxEditDepth {
			return false
		}
		out = append(out, path)
		if t, ok := v.(core.Tuple); ok {
			for i := 0; i < t.Len(); i++ {
				if !walk(t.At(i), append(slices.Clone(path), i)) {
					return false
				}
			}
		}
		return true
	}
	return out, walk(root, nil)
}

func nodeAt(root core.Value, path []int) core.Value {
	for _, i := range path {
		root = root.(core.Tuple).At(i)
	}
	return root
}

func replaceAt(root core.Value, path []int, value core.Value) core.Value {
	if len(path) == 0 {
		return value
	}
	t := root.(core.Tuple)
	items := t.Items()
	items[path[0]] = replaceAt(items[path[0]], path[1:], value)
	return core.NewTuple(items...)
}

func atomsOf(root core.Value, dict []core.Value) []core.Value {
	seen := map[string]bool{}
	var walk func(core.Value)
	walk = func(v core.Value) {
		if len(dict) >= maxDictionary {
			return
		}
		switch x := v.(type) {
		case core.Atom:
			if key := string(x.Bytes()); !seen[key] {
				seen[key] = true
				dict = append(dict, x)
			}
		case core.Tuple:
			for i := 0; i < x.Len(); i++ {
				walk(x.At(i))
			}
		}
	}
	walk(root)
	return dict
}

// applyEdits interprets script against root. Each edit names its nodes by
// preorder index modulo the current tree size.
func applyEdits(root core.Value, script []byte) (core.Value, bool) {
	dict := atomsOf(root, slices.Clone(baseDictionary))
	e := &editor{script: script}
	for range maxEdits {
		code, ok := e.next()
		if !ok {
			break
		}
		all, ok := paths(root)
		if !ok {
			return nil, false
		}
		target, ok := e.wide()
		if !ok {
			break
		}
		path := all[target%len(all)]
		node := nodeAt(root, path)
		tuple, isTuple := node.(core.Tuple)
		switch code % 9 {
		case 0: // transplant another subtree here
			source, ok := e.wide()
			if !ok {
				return root, true
			}
			root = replaceAt(root, path, nodeAt(root, all[source%len(all)]))
		case 1: // delete a child
			child, ok := e.next()
			if !ok || !isTuple || tuple.Len() == 0 {
				continue
			}
			items := tuple.Items()
			i := child % len(items)
			root = replaceAt(root, path, core.NewTuple(slices.Delete(items, i, i+1)...))
		case 2: // duplicate a child in place
			child, ok := e.next()
			if !ok || !isTuple || tuple.Len() == 0 {
				continue
			}
			items := tuple.Items()
			i := child % len(items)
			root = replaceAt(root, path, core.NewTuple(slices.Insert(items, i, items[i])...))
		case 3: // swap two children
			first, ok1 := e.next()
			second, ok2 := e.next()
			if !ok1 || !ok2 || !isTuple || tuple.Len() < 2 {
				continue
			}
			items := tuple.Items()
			i, j := first%len(items), second%len(items)
			items[i], items[j] = items[j], items[i]
			root = replaceAt(root, path, core.NewTuple(items...))
		case 4: // replace with dictionary vocabulary
			word, ok := e.wide()
			if !ok {
				return root, true
			}
			root = replaceAt(root, path, dict[word%len(dict)])
		case 5: // replace with a small integer
			n, ok := e.next()
			if !ok {
				return root, true
			}
			root = replaceAt(root, path, integer(int64(int8(n))))
		case 6: // wrap in a tagged tuple or a list
			word, ok := e.wide()
			if !ok {
				return root, true
			}
			if word&1 == 0 {
				root = replaceAt(root, path, list(node))
			} else {
				root = replaceAt(root, path, core.NewTuple(dict[(word>>1)%len(dict)], node))
			}
		case 7: // insert a dictionary item as a new child
			word, ok := e.wide()
			if !ok || !isTuple {
				continue
			}
			items := tuple.Items()
			i := word % (len(items) + 1)
			root = replaceAt(root, path, core.NewTuple(slices.Insert(items, i, dict[(word>>4)%len(dict)])...))
		case 8: // hoist a child over its parent
			child, ok := e.next()
			if !ok || !isTuple || tuple.Len() == 0 {
				continue
			}
			root = replaceAt(root, path, tuple.At(child%tuple.Len()))
		}
	}
	if _, ok := paths(root); !ok {
		return nil, false
	}
	return root, true
}

// repairPins rewrites each import whose expected module name is supplied (or
// is the bundled prelude's) to that module's current pin, until a fixpoint.
// Mutated dependencies change their pins; without repair the fuzzer would
// mostly reach MissingDependency.
func repairPins(modules []core.Value) []core.Value {
	out := slices.Clone(modules)
	prelude := decl.PreludePin()
	for range len(out) + 1 {
		pins := make([][32]byte, len(out))
		named := map[string]int{}
		for i, m := range out {
			pins[i] = sha256.Sum256(codec.Encode(m))
			if name, ok := moduleNameOf(m); ok {
				if _, dup := named[name]; !dup {
					named[name] = i
				}
			}
		}
		changed := false
		for i, m := range out {
			repaired, ok := rewriteImports(m, func(name string) ([]byte, bool) {
				if j, ok := named[name]; ok && j != i {
					return pins[j][:], true
				}
				if name == "horos.kinds" {
					return prelude[:], true
				}
				return nil, false
			})
			if ok {
				out[i] = repaired
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return out
}

func moduleNameOf(m core.Value) (string, bool) {
	t, ok := m.(core.Tuple)
	if !ok || t.Len() != 5 || !core.Equal(t.At(0), a("horos/decl/module")) {
		return "", false
	}
	name, err := data.ReadText(t.At(2))
	return name, err == nil
}

// rewriteImports replaces the pin of every import form inside a module's
// imports operand. It reports whether anything changed.
func rewriteImports(m core.Value, pinFor func(string) ([]byte, bool)) (core.Value, bool) {
	t, ok := m.(core.Tuple)
	if !ok || t.Len() != 5 || !core.Equal(t.At(0), a("horos/decl/module")) {
		return m, false
	}
	changed := false
	var walk func(core.Value, int) core.Value
	walk = func(v core.Value, depth int) core.Value {
		x, ok := v.(core.Tuple)
		if !ok || depth > 16 {
			return v
		}
		if x.Len() == 3 && core.Equal(x.At(0), a("horos/decl/import")) {
			name, err := data.ReadText(x.At(1))
			if err != nil {
				return v
			}
			pin, ok := pinFor(name)
			if old, isAtom := x.At(2).(core.Atom); !ok || (isAtom && slices.Equal(old.Bytes(), pin)) {
				return v
			}
			changed = true
			return core.NewTuple(x.At(0), x.At(1), core.NewAtom(pin))
		}
		items := x.Items()
		for i := range items {
			items[i] = walk(items[i], depth+1)
		}
		return core.NewTuple(items...)
	}
	items := t.Items()
	items[3] = walk(items[3], 0)
	if !changed {
		return m, false
	}
	return core.NewTuple(items...), true
}
