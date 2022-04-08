// Package trieset provides several succinct data types.
// https://github.com/openacid/succinct
package trieset

import (
	"github.com/openacid/low/bitmap"
)

// Set is a succinct, sorted and static string set impl with compacted trie as
// storage. The space cost is about half lower than the original data.
//
// Implementation
//
// It stores sorted strings in a compacted trie(AKA prefix tree).
// A trie node has at most 256 outgoing labels.
// A label is just a single byte.
// E.g., [ab, abc, abcd, axy, buv] is represented with a trie like the following:
// (Numbers are node id)
//
//   ^ -a-> 1 -b-> 3 $
//     |      |      `c-> 6 $
//     |      |             `d-> 9 $
//     |      `x-> 4 -y-> 7 $
//     `b-> 2 -u-> 5 -v-> 8 $
//
// Internally it uses a packed []byte and a bitmap with `len([]byte)` bits to
// describe the outgoing labels of a node,:
//   ^: ab  00
//   1: bx  00
//   2: u   0
//   3: c   0
//   4: y   0
//   5: v   0
//   6: d   0
//   7: ø
//   8: ø
//   9: ø
//
// In storage it packs labels together and bitmaps joined with separator `1`:
//   labels(ignore space): "ab bx u c y v d"
//   label bitmap:          0010010101010101111
//
// In this way every node has a `0` pointing to it(except the root node)
// and has a corresponding `1` for it:
//                                  .-----.
//                          .--.    | .---|-.
//                          |.-|--. | | .-|-|.
//                          || ↓  ↓ | | | ↓ ↓↓
//   labels(ignore space):  ab bx u c y v d øøø
//   label bitmap:          0010010101010101111
//   node-id:               0  1  2 3 4 5 6 789
//                             || | ↑ ↑ ↑ |   ↑
//                             || `-|-|-' `---'
//                             |`---|-'
//                             `----'
// To walk from a parent node along a label to a child node, count the number of
// `0` upto the bit the label position, then find where the the corresponding
// `1` is:
//   childNodeId = select1(rank0(i))
// In our impl, it is:
//   nodeId = countZeros(ss.labelBitmap, ss.ranks, bmIdx+1)
//   bmIdx = selectIthOne(ss.labelBitmap, ss.ranks, ss.selects, nodeId-1) + 1
//
// Finally leaf nodes are indicated by another bitmap `leaves`, in which a `1`
// at i-th bit indicates the i-th node is a leaf:
//   leaves: 0001001111
type Set struct {
	leaves, labelBitmap []uint64
	labels              []byte
	ranks, selects      []int32
}

// NewSet creates a new *Set struct, from a slice of sorted strings.
func NewSet(keys []string) *Set {
	ss := &Set{}
	lIdx := 0

	type qElt struct{ s, e, col int }

	queue := []qElt{{0, len(keys), 0}}

	for i := 0; i < len(queue); i++ {
		elt := queue[i]

		if elt.col == len(keys[elt.s]) {
			// a leaf node
			elt.s++
			setBit(&ss.leaves, i, 1)
		}

		for j := elt.s; j < elt.e; {

			frm := j

			for ; j < elt.e && keys[j][elt.col] == keys[frm][elt.col]; j++ {
			}

			queue = append(queue, qElt{frm, j, elt.col + 1})
			ss.labels = append(ss.labels, keys[frm][elt.col])
			setBit(&ss.labelBitmap, lIdx, 0)
			lIdx++
		}

		setBit(&ss.labelBitmap, lIdx, 1)
		lIdx++
	}

	ss.init()
	return ss
}

// Has query for a key and return whether it presents in the Set.
func (ss *Set) Has(key string) bool {
	nodeId, bmIdx := 0, 0

	for i := 0; i < len(key); i++ {
		c := key[i]
		for ; ; bmIdx++ {
			if getBit(ss.labelBitmap, bmIdx) != 0 {
				// no more labels in this node
				return false
			}

			if ss.labels[bmIdx-nodeId] == c {
				break
			}
		}

		// go to next level
		nodeId = countZeros(ss.labelBitmap, ss.ranks, bmIdx+1)
		bmIdx = selectIthOne(ss.labelBitmap, ss.ranks, ss.selects, nodeId-1) + 1
	}

	return getBit(ss.leaves, nodeId) != 0
}

func setBit(bm *[]uint64, i int, v int) {
	for i>>6 >= len(*bm) {
		*bm = append(*bm, 0)
	}
	(*bm)[i>>6] |= uint64(v) << uint(i&63)
}

func getBit(bm []uint64, i int) uint64 {
	return bm[i>>6] & (1 << uint(i&63))
}

// init builds pre-calculated cache to speed up rank() and select()
func (ss *Set) init() {
	ss.selects, ss.ranks = bitmap.IndexSelect32R64(ss.labelBitmap)
}

// countZeros counts the number of "0" in a bitmap before the i-th bit(excluding
// the i-th bit) on behalf of rank index.
// E.g.:
//   countZeros("010010", 4) == 3
//   //          012345
func countZeros(bm []uint64, ranks []int32, i int) int {
	a, _ := bitmap.Rank64(bm, ranks, int32(i))
	return i - int(a)
}

// selectIthOne returns the index of the i-th "1" in a bitmap, on behalf of rank
// and select indexes.
// E.g.:
//   selectIthOne("010010", 1) == 4
//   //            012345
func selectIthOne(bm []uint64, ranks, selects []int32, i int) int {
	a, _ := bitmap.Select32R64(bm, selects, ranks, int32(i))
	return int(a)
}
