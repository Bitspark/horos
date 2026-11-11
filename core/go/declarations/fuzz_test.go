package declarations_test

// Go native fuzz targets for the declarations layer. Module values reach this
// package from outside (Bittype assets, kernel models), so every target feeds
// arbitrary ontos values to the public API and checks, for each input:
//
//	(a) no operation panics;
//	(b) every error is a typed *decl.Error with a documented code, a failed
//	    operation returns no usable partial Program, handle or output, and
//	    under smaller limits every outcome equals the default-limit outcome or
//	    is Exhausted (declarations.md §8: "Exhaustion establishes neither
//	    invalid source nor nonmembership");
//	(c) the same input gives the same Program, results and errors;
//	(d) every GroundDescriptor result passes horos.WF, equals Descriptor and
//	    is re-admitted unchanged through Proper;
//	(e) Apply honours saturation: missing, partial and extra argument lists
//	    and ill-sorted arguments fail with KindMismatch and a zero value, and a
//	    saturated application has the callee's result sort.
//
// The seed corpus is testdata/modules (module sets resolved by the existing
// tests, see its README) plus the bundled horos.kinds asset. Each seed is the
// ontos-codec-v1 encoding of Tuple(root, dependency...). Under plain `go test`
// the seeds and the regression inputs under testdata/fuzz run as ordinary
// tests. Run a target for a sustained period with, for example:
//
//	go test ./declarations -run '^$' -fuzz '^FuzzResolve$' -fuzztime 2h -parallel 16

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	decl "github.com/Bitspark/horos/core/go/declarations"
	codec "github.com/bitspark/ontos/codec/go"
	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

// knobs packs one fuzz input's limits and choices into a uint64 so the
// fuzzer mutates them alongside the module bytes. Bytes, low to high:
//
//	0  flags: bit 0 repairs import pins; bits 1-7 rotate argument choices
//	1  MaxDepth-1 (1..256)
//	2-3 MaxNodes/2 (1..100000)
//	4-5 MaxBytes/64 (1..16 MiB)
//	6  MaxModules-1 (1..256)
type knobs uint64

func (k knobs) repairPins() bool { return k&1 != 0 }
func (k knobs) choice() int      { return int(k>>1) & 0x7f }

// smallLimits never exceeds the default limits, so every outcome under them
// must equal the default-limit outcome or be Exhausted.
func (k knobs) smallLimits() decl.Limits {
	def := decl.DefaultLimits()
	return decl.Limits{
		MaxDepth:   min(1+int(k>>8&0xff), def.MaxDepth),
		MaxNodes:   min(1+int(k>>16&0xffff)*2, def.MaxNodes),
		MaxBytes:   min(1+int(k>>32&0xffff)*64, def.MaxBytes),
		MaxModules: min(1+int(k>>48&0xff), def.MaxModules),
	}
}

func packKnobs(flags uint8, depth uint8, nodes, bytes uint16, modules uint8) uint64 {
	return uint64(flags) | uint64(depth)<<8 | uint64(nodes)<<16 | uint64(bytes)<<32 | uint64(modules)<<48
}

// seedKnobs are tight and roomy limit configurations, with and without
// import-pin repair; seeds take them in turn to keep regression runs short.
var seedKnobs = []uint64{
	packKnobs(0, 0, 0, 0, 0),
	packKnobs(1, 127, 20000, 4096, 15),
	packKnobs(3, 255, 0xffff, 0xffff, 255),
}

const moduleSeedDir = "testdata/modules"

// moduleSeeds returns encoded module sets: the harvested test modules, the
// bundled prelude as a root, and a prelude-importing module built here.
func moduleSeeds(tb testing.TB) [][]byte {
	tb.Helper()
	entries, err := os.ReadDir(moduleSeedDir)
	if err != nil {
		tb.Fatal(err)
	}
	var seeds [][]byte
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ontos") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(moduleSeedDir, entry.Name()))
		if err != nil {
			tb.Fatal(err)
		}
		if _, err := codec.Decode(b); err != nil {
			tb.Fatalf("%s: seed does not decode: %v", entry.Name(), err)
		}
		seeds = append(seeds, b)
	}
	if len(seeds) == 0 {
		tb.Fatalf("no module seeds in %s", moduleSeedDir)
	}
	seeds = append(seeds,
		codec.Encode(core.NewTuple(decl.Prelude())),
		codec.Encode(core.NewTuple(shapeModule())),
		codec.Encode(core.NewTuple(shapeModule(), decl.Prelude())))
	return seeds
}

// decodeModuleSet reads Tuple(root, dependency...). Any other decodable value
// is a lone root. Bytes that do not decode are not module input at all.
func decodeModuleSet(b []byte) ([]core.Value, bool) {
	v, err := codec.Decode(b)
	if err != nil {
		return nil, false
	}
	if t, ok := v.(core.Tuple); ok && t.Len() > 0 {
		return t.Items(), true
	}
	return []core.Value{v}, true
}

// FuzzResolve mutates the codec encoding of a module set. Inputs that do not
// decode are skipped, so coverage measures the resolver, not the codec.
func FuzzResolve(f *testing.F) {
	for i, seed := range moduleSeeds(f) {
		f.Add(seed, seedKnobs[i%len(seedKnobs)])
	}
	f.Fuzz(func(t *testing.T, encoded []byte, k uint64) {
		modules, ok := decodeModuleSet(encoded)
		if !ok {
			return
		}
		checkModuleSet(t, modules, knobs(k))
	})
}

// FuzzResolveEdited applies a structural edit script to a decoded module set:
// subtree transplants, insertions, deletions and grammar-vocabulary
// replacements. Every edit keeps a valid ontos value, so byte-level mutation
// of the script reaches shapes the length-prefixed codec rarely survives.
func FuzzResolveEdited(f *testing.F) {
	scripts := [][]byte{nil, {0, 0, 1, 0, 2}, {4, 0, 7, 0, 9}, {2, 0, 0, 0}}
	for i, seed := range moduleSeeds(f) {
		f.Add(seed, scripts[i%len(scripts)], seedKnobs[(i+1)%len(seedKnobs)]|1)
	}
	f.Fuzz(func(t *testing.T, encoded []byte, script []byte, k uint64) {
		modules, ok := decodeModuleSet(encoded)
		if !ok {
			return
		}
		edited, ok := applyEdits(core.NewTuple(modules...), script)
		if !ok {
			return
		}
		if tuple, isTuple := edited.(core.Tuple); isTuple && tuple.Len() > 0 {
			modules = tuple.Items()
		} else {
			modules = []core.Value{edited}
		}
		checkModuleSet(t, modules, knobs(k))
	})
}

// checkModuleSet resolves one input three times: twice under default limits
// (determinism) and once under the knob's smaller limits (monotonicity), and
// drives the query surface of every published Program.
func checkModuleSet(t *testing.T, modules []core.Value, k knobs) {
	if k.repairPins() {
		modules = repairPins(modules)
	}
	root, deps := modules[0], slices.Clone(modules[1:])
	first := newSession(t, "default", root, deps, decl.DefaultLimits())
	first.explore(k)
	second := newSession(t, "repeat", root, deps, decl.DefaultLimits())
	second.explore(k)
	first.sameAs(second)
	small := newSession(t, "small", root, deps, k.smallLimits())
	small.replay(first)
}

// queryPrograms resolves the fixed query module once: under default limits,
// and under the smallest node and depth limits that still publish it, so that
// replayed queries meet their limits often.
var queryPrograms = sync.OnceValues(func() (*decl.Program, []*decl.Program) {
	root := shapeModule()
	p, err := decl.Resolve(root, nil)
	if err != nil {
		panic(err)
	}
	smallest := func(set func(*decl.Limits, int), low, high int) *decl.Program {
		var found *decl.Program
		for low <= high {
			mid := (low + high) / 2
			limits := decl.DefaultLimits()
			set(&limits, mid)
			if q, err := decl.ResolveWithLimits(root, nil, limits); err == nil {
				found, high = q, mid-1
			} else {
				low = mid + 1
			}
		}
		return found
	}
	nodes := smallest(func(l *decl.Limits, n int) { l.MaxNodes = n }, 1, decl.DefaultLimits().MaxNodes)
	depth := smallest(func(l *decl.Limits, n int) { l.MaxDepth = n }, 1, decl.DefaultLimits().MaxDepth)
	return p, []*decl.Program{nodes, depth}
})

// querySeeds collects the distinct declaration bodies and kinds of the module
// seeds, three items per seed, with the bundled prelude's in its scope.
func querySeeds(tb testing.TB) []struct {
	encoded []byte
	knobs   uint64
} {
	var seeds []struct {
		encoded []byte
		knobs   uint64
	}
	add := func(items []core.Value, preludeScope bool) {
		for start := 0; start < len(items); start += 3 {
			group := items[start:min(start+3, len(items))]
			var k uint64
			if preludeScope {
				k = 0x7 << 8
			}
			seeds = append(seeds, struct {
				encoded []byte
				knobs   uint64
			}{codec.Encode(core.NewTuple(group...)), k})
		}
	}
	seen, seenItems := map[string]bool{}, map[string]bool{}
	for _, encoded := range moduleSeeds(tb) {
		modules, _ := decodeModuleSet(encoded)
		for _, m := range modules {
			t, ok := m.(core.Tuple)
			if !ok || t.Len() != 5 {
				continue
			}
			key := string(codec.Encode(m))
			if seen[key] {
				continue
			}
			seen[key] = true
			entries, err := data.ReadMap(t.At(4))
			if err != nil {
				continue
			}
			var items []core.Value
			for _, entry := range entries {
				declaration, ok := entry[1].(core.Tuple)
				if !ok || declaration.Len() != 4 {
					continue
				}
				for _, item := range []core.Value{declaration.At(3), declaration.At(2)} {
					if key := string(codec.Encode(item)); !seenItems[key] {
						seenItems[key] = true
						items = append(items, item)
					}
				}
			}
			add(items, core.Equal(m, decl.Prelude()))
		}
	}
	return seeds
}

// FuzzQueries feeds arbitrary values to the query surface of a fixed Program:
// each item is admitted with Proper and compiled with CompileKind (in the
// root's or the prelude's scope), then memberships, unions, intersections
// and inclusions among the results are checked against each other.
func FuzzQueries(f *testing.F) {
	for _, seed := range querySeeds(f) {
		f.Add(seed.encoded, seed.knobs)
	}
	f.Fuzz(func(t *testing.T, encoded []byte, k uint64) {
		v, err := codec.Decode(encoded)
		if err != nil {
			return
		}
		items := []core.Value{v}
		if tuple, ok := v.(core.Tuple); ok && tuple.Len() > 0 && !isTagged(tuple) {
			items = tuple.Items()
		}
		if len(items) > maxQueryItems {
			items = items[:maxQueryItems]
		}
		p, small := queryPrograms()
		first := programSession(t, "default", p)
		first.exploreQueries(items, knobs(k))
		second := programSession(t, "repeat", p)
		second.exploreQueries(items, knobs(k))
		first.sameAs(second)
		for _, q := range small {
			if q != nil {
				programSession(t, "small", q).replay(first)
			}
		}
	})
}

// isTagged reports whether a tuple is itself a declaration or descriptor
// form rather than a list of query items.
func isTagged(t core.Tuple) bool {
	label, ok := t.At(0).(core.Atom)
	return ok && strings.HasPrefix(string(label.Bytes()), "horos/")
}
