package horos

import core "github.com/bitspark/ontos/core/go"

// Exact literals are data, not descriptor descent. Preserve structural identity
// without recursively consuming the native stack for arbitrarily nested literals.
func exactEqual(left, right Value) bool {
	work := [][2]Value{{left, right}}
	for len(work) > 0 {
		pair := work[len(work)-1]
		work = work[:len(work)-1]
		switch a := pair[0].(type) {
		case core.Atom:
			b, ok := pair[1].(core.Atom)
			if !ok || !bytesEqual(a.Bytes(), b.Bytes()) {
				return false
			}
		case core.Tuple:
			b, ok := pair[1].(core.Tuple)
			if !ok {
				return false
			}
			aa, bb := a.Items(), b.Items()
			if len(aa) != len(bb) {
				return false
			}
			for i := range aa {
				work = append(work, [2]Value{aa[i], bb[i]})
			}
		default:
			return false
		}
	}
	return true
}
