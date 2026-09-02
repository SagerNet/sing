package domain

import (
	"encoding/binary"
	"math/bits"

	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/varbin"
)

// mod from https://github.com/openacid/succinct

type succinctSet struct {
	leaves, labelBitmap []uint64
	labels              []byte
	ranks, selects      []int32
	storage             any
}

type Mmap struct {
	Leaves      []uint64
	LabelBitmap []uint64
	Labels      []byte
	Ranks       []int32
	Selects     []int32
	Storage     any
}

func (ss *succinctSet) mmap() Mmap {
	return Mmap{
		Leaves:      ss.leaves,
		LabelBitmap: ss.labelBitmap,
		Labels:      ss.labels,
		Ranks:       ss.ranks,
		Selects:     ss.selects,
		Storage:     ss.storage,
	}
}

func newSuccinctSetFromMmap(data Mmap) (*succinctSet, error) {
	onesCount, zerosCount := countLabelBitmap(data.LabelBitmap)
	if onesCount != zerosCount+1 || len(data.Labels) != zerosCount {
		return nil, E.New("domain: malformed succinct set")
	}
	if len(data.Leaves) < (onesCount+63)>>6 {
		return nil, E.New("domain: malformed succinct set leaves")
	}
	if len(data.Ranks) != len(data.LabelBitmap)+1 {
		return nil, E.New("domain: malformed succinct set ranks")
	}
	rank := int32(0)
	for i, word := range data.LabelBitmap {
		if data.Ranks[i] != rank {
			return nil, E.New("domain: malformed succinct set ranks")
		}
		rank += int32(bits.OnesCount64(word))
	}
	if data.Ranks[len(data.LabelBitmap)] != rank {
		return nil, E.New("domain: malformed succinct set ranks")
	}
	if len(data.Selects) != (onesCount+31)>>5 {
		return nil, E.New("domain: malformed succinct set selects")
	}
	ith := -1
	for i := range len(data.LabelBitmap) << 6 {
		if data.LabelBitmap[i>>6]&(1<<uint(i&63)) == 0 {
			continue
		}
		ith++
		if ith&31 == 0 && data.Selects[ith>>5] != int32(i) {
			return nil, E.New("domain: malformed succinct set selects")
		}
	}
	return &succinctSet{
		leaves:      data.Leaves,
		labelBitmap: data.LabelBitmap,
		labels:      data.Labels,
		ranks:       data.Ranks,
		selects:     data.Selects,
		storage:     data.Storage,
	}, nil
}

func countLabelBitmap(labelBitmap []uint64) (onesCount int, zerosCount int) {
	lastOneIndex := -1
	for wordIndex, word := range labelBitmap {
		onesCount += bits.OnesCount64(word)
		if word != 0 {
			lastOneIndex = wordIndex<<6 | (63 - bits.LeadingZeros64(word))
		}
	}
	zerosCount = lastOneIndex + 1 - onesCount
	return
}

func newSuccinctSet(keys []string) *succinctSet {
	ss := &succinctSet{}
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

func (ss *succinctSet) keys() []string {
	var result []string
	var currentKey []byte
	type frame struct {
		nodeId, bmIdx int
	}
	if getBit(ss.leaves, 0) != 0 {
		result = append(result, "")
	}
	stack := []frame{{0, 0}}
	for len(stack) > 0 {
		top := &stack[len(stack)-1]
		if getBit(ss.labelBitmap, top.bmIdx) != 0 {
			stack = stack[:len(stack)-1]
			if len(stack) > 0 {
				currentKey = currentKey[:len(currentKey)-1]
				stack[len(stack)-1].bmIdx++
			}
			continue
		}
		currentKey = append(currentKey, ss.labels[top.bmIdx-top.nodeId])
		nextNodeId := countZeros(ss.labelBitmap, ss.ranks, top.bmIdx+1)
		nextBmIdx := selectIthOne(ss.labelBitmap, ss.ranks, ss.selects, nextNodeId-1) + 1
		if getBit(ss.leaves, nextNodeId) != 0 {
			result = append(result, string(currentKey))
		}
		stack = append(stack, frame{nextNodeId, nextBmIdx})
	}
	return result
}

func readSuccinctSet(reader varbin.Reader) (*succinctSet, error) {
	_, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	leaves, err := varbin.ReadSlice[uint64](reader, binary.BigEndian)
	if err != nil {
		return nil, err
	}
	labelBitmap, err := varbin.ReadSlice[uint64](reader, binary.BigEndian)
	if err != nil {
		return nil, err
	}
	labels, err := varbin.ReadSlice[byte](reader, binary.BigEndian)
	if err != nil {
		return nil, err
	}
	onesCount, zerosCount := countLabelBitmap(labelBitmap)
	if onesCount != zerosCount+1 || len(labels) != zerosCount {
		return nil, E.New("domain: malformed succinct set")
	}
	leavesWordCount := (onesCount + 63) >> 6
	if len(leaves) < leavesWordCount {
		leaves = append(leaves, make([]uint64, leavesWordCount-len(leaves))...)
	}
	set := &succinctSet{
		leaves:      leaves,
		labelBitmap: labelBitmap,
		labels:      labels,
	}
	set.init()
	return set, nil
}

func (ss *succinctSet) Write(writer varbin.Writer) error {
	err := writer.WriteByte(0)
	if err != nil {
		return err
	}
	err = writeUint64Slice(writer, ss.leaves)
	if err != nil {
		return err
	}
	err = writeUint64Slice(writer, ss.labelBitmap)
	if err != nil {
		return err
	}
	return writeByteSlice(writer, ss.labels)
}

func writeUint64Slice(writer varbin.Writer, value []uint64) error {
	_, err := varbin.WriteUvarint(writer, uint64(len(value)))
	if err != nil {
		return err
	}
	if len(value) == 0 {
		return nil
	}
	return binary.Write(writer, binary.BigEndian, value)
}

func writeByteSlice(writer varbin.Writer, value []byte) error {
	_, err := varbin.WriteUvarint(writer, uint64(len(value)))
	if err != nil {
		return err
	}
	if len(value) == 0 {
		return nil
	}
	_, err = writer.Write(value)
	return err
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

func (ss *succinctSet) init() {
	ss.selects, ss.ranks = indexSelect32R64(ss.labelBitmap)
}

func countZeros(bm []uint64, ranks []int32, i int) int {
	a, _ := rank64(bm, ranks, int32(i))
	return i - int(a)
}

func selectIthOne(bm []uint64, ranks, selects []int32, i int) int {
	a, _ := select32R64(bm, selects, ranks, int32(i))
	return int(a)
}

func rank64(words []uint64, rindex []int32, i int32) (int32, int32) {
	wordI := i >> 6
	j := uint32(i & 63)
	n := rindex[wordI]
	w := words[wordI]
	c1 := n + int32(bits.OnesCount64(w&mask[j]))
	return c1, int32(w>>uint(j)) & 1
}

func indexRank64(words []uint64, opts ...bool) []int32 {
	trailing := false
	if len(opts) > 0 {
		trailing = opts[0]
	}
	l := len(words)
	if trailing {
		l++
	}
	idx := make([]int32, l)
	n := int32(0)
	for i := range len(words) {
		idx[i] = n
		n += int32(bits.OnesCount64(words[i]))
	}
	if trailing {
		idx[len(words)] = n
	}
	return idx
}

func select32R64(words []uint64, selectIndex, rankIndex []int32, i int32) (int32, int32) {
	a := int32(0)
	l := int32(len(words))
	wordI := selectIndex[i>>5] >> 6
	for ; rankIndex[wordI+1] <= i; wordI++ {
	}
	w := words[wordI]
	ww := w
	base := wordI << 6
	findIth := int(i - rankIndex[wordI])
	offset := int32(0)
	ones := bits.OnesCount32(uint32(ww))
	if ones <= findIth {
		findIth -= ones
		offset |= 32
		ww >>= 32
	}
	ones = bits.OnesCount16(uint16(ww))
	if ones <= findIth {
		findIth -= ones
		offset |= 16
		ww >>= 16
	}
	ones = bits.OnesCount8(uint8(ww))
	if ones <= findIth {
		a = int32(select8Lookup[(ww>>5)&0x7f8|uint64(findIth-ones)]) + offset + 8
	} else {
		a = int32(select8Lookup[(ww&0xff)<<3|uint64(findIth)]) + offset
	}
	a += base
	w &= rMaskUpto[a&63]
	if w != 0 {
		return a, base + int32(bits.TrailingZeros64(w))
	}
	wordI++
	for ; wordI < l; wordI++ {
		w = words[wordI]
		if w != 0 {
			return a, wordI<<6 + int32(bits.TrailingZeros64(w))
		}
	}
	return a, l << 6
}

func indexSelect32R64(words []uint64) ([]int32, []int32) {
	l := len(words) << 6
	sidx := make([]int32, 0, len(words))

	ith := -1
	for i := range l {
		if words[i>>6]&(1<<uint(i&63)) != 0 {
			ith++
			if ith&31 == 0 {
				sidx = append(sidx, int32(i))
			}
		}
	}

	// clone to reduce cap to len
	sidx = append(sidx[:0:0], sidx...)
	return sidx, indexRank64(words, true)
}

func init() {
	initMasks()
	initSelectLookup()
}

var (
	mask      [65]uint64
	rMaskUpto [64]uint64
)

func initMasks() {
	for i := range 65 {
		mask[i] = (1 << uint(i)) - 1
	}

	var maskUpto [64]uint64
	for i := range 64 {
		maskUpto[i] = (1 << uint(i+1)) - 1
		rMaskUpto[i] = ^maskUpto[i]
	}
}

var select8Lookup [256 * 8]uint8

func initSelectLookup() {
	for i := range 256 {
		w := uint8(i)
		for j := range 8 {
			// x-th 1 in w
			// if x-th 1 is not found, it is 8
			x := bits.TrailingZeros8(w)
			w &= w - 1

			select8Lookup[i*8+j] = uint8(x)
		}
	}
}
