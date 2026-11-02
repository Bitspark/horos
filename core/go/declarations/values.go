package declarations

import (
	"crypto/sha256"
	"encoding/binary"
	"strings"

	horos "github.com/Bitspark/horos/core/go"
	codec "github.com/bitspark/ontos/codec/go"
	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

type budget struct {
	limits Limits
	nodes  int
	bytes  int
}

func (b *budget) step(depth int) error {
	if depth > b.limits.MaxDepth || b.nodes >= b.limits.MaxNodes {
		return failure(Exhausted, "depth or node-work limit reached")
	}
	b.nodes++
	return nil
}

// Check all value nodes before the codec or recursive recognizers see them.
// Tuple.At avoids copying a wide tuple merely to check its resource limit.
func (b *budget) value(value core.Value, depth int) error {
	if err := b.step(depth); err != nil {
		return err
	}
	var header [binary.MaxVarintLen64]byte
	switch v := value.(type) {
	case core.Atom:
		size := 1 + binary.PutUvarint(header[:], uint64(v.Len()))
		if v.Len() > b.limits.MaxBytes-b.bytes || size > b.limits.MaxBytes-b.bytes-v.Len() {
			return failure(Exhausted, "encoded-byte limit reached")
		}
		b.bytes += size + v.Len()
	case core.Tuple:
		size := 1 + binary.PutUvarint(header[:], uint64(v.Len()))
		if size > b.limits.MaxBytes-b.bytes || v.Len() > b.limits.MaxNodes-b.nodes {
			return failure(Exhausted, "encoded-byte or node-work limit reached")
		}
		b.bytes += size
		for i := 0; i < v.Len(); i++ {
			if err := b.value(v.At(i), depth+1); err != nil {
				return err
			}
		}
	default:
		return failure(Invalid, "expected an Ontos Atom or Tuple value")
	}
	return nil
}

// Pin computes the exact module-byte digest after bounded value admission.
// It does not claim that the value is a well-formed declaration module.
func Pin(value core.Value) (Digest, error) {
	return PinWithLimits(value, DefaultLimits())
}

func PinWithLimits(value core.Value, limits Limits) (Digest, error) {
	if !limits.valid() {
		return Digest{}, failure(InvalidOptions, "invalid checking limits")
	}
	if err := (&budget{limits: limits}).value(value, 0); err != nil {
		return Digest{}, err
	}
	return sha256.Sum256(codec.Encode(value)), nil
}

func head(value core.Value, prefix string) (string, []core.Value, bool) {
	tuple, ok := value.(core.Tuple)
	if !ok || tuple.Len() == 0 {
		return "", nil, false
	}
	label, ok := tuple.At(0).(core.Atom)
	if !ok {
		return "", nil, false
	}
	name := string(label.Bytes())
	if !strings.HasPrefix(name, prefix) {
		return "", nil, false
	}
	return strings.TrimPrefix(name, prefix), tuple.Items()[1:], true
}

func form(value core.Value, name string, arity int) ([]core.Value, bool) {
	tag, args, ok := head(value, "horos/decl/")
	return args, ok && tag == name && len(args) == arity
}

func identifier(name string) bool {
	if name == "" {
		return false
	}
	for i, c := range []byte(name) {
		if c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

func moduleName(name string) bool {
	first := true
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '.' {
			if first {
				return false
			}
			first = true
			continue
		}
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || !first && c >= '0' && c <= '9') {
			return false
		}
		first = false
	}
	return !first
}

func index(value core.Value) (int, bool) {
	n, err := data.ReadInt(value)
	if err != nil || !n.IsInt64() || n.Sign() < 0 || uint64(n.Int64()) > uint64(^uint(0)>>1) {
		return 0, false
	}
	return int(n.Int64()), true
}

func digest(value core.Value) (Digest, bool) {
	atom, ok := value.(core.Atom)
	if !ok || atom.Len() != 32 {
		return Digest{}, false
	}
	var pin Digest
	copy(pin[:], atom.Bytes())
	return pin, true
}

func descriptorWF(value core.Value) error {
	result := horos.DiagnoseWF(value)
	switch result.Verdict {
	case horos.VerdictConforms:
		return nil
	case horos.VerdictExhausted:
		return failure(Exhausted, "Horos descriptor checking limit reached")
	default:
		return failure(Invalid, "malformed or unguarded Horos descriptor")
	}
}

func descriptorAny() core.Value {
	return core.NewTuple(core.NewAtom([]byte("horos/any")))
}

func at(value core.Value, path []int) core.Value {
	for _, i := range path {
		tuple, ok := value.(core.Tuple)
		if !ok {
			return nil
		}
		value = tuple.At(i)
	}
	return value
}

// Paths are checked descriptor positions, never arbitrary searches in literals.
func replaceOperands(value core.Value, operands []Operand, children []core.Value) core.Value {
	if len(operands) == 0 {
		return value
	}
	items := value.(core.Tuple).Items()
	for i, operand := range operands {
		path := operand.path
		if len(path) == 1 {
			items[path[0]] = children[i]
		} else {
			// Record fields are the only two-level descriptor positions.
			field := items[path[0]].(core.Tuple).Items()
			field[path[1]] = children[i]
			items[path[0]] = core.NewTuple(field...)
		}
	}
	return core.NewTuple(items...)
}
