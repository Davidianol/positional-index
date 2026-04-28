package roaring

import "math/bits"

const (
	maxArrayLen = 4096
	bitmapWords = 1024
	kindArray   = uint8(0)
	kindBitmap  = uint8(1)
)

type container struct {
	kind  uint8
	arr   []uint16
	bits  []uint64
	count int
}

func newArrayContainer(val uint16) *container {
	return &container{kind: kindArray, arr: []uint16{val}, count: 1}
}

func (c *container) add(val uint16) {
	if c.kind == kindArray {
		c.addToArray(val)
	} else {
		c.addToBitmap(val)
	}
}

func (c *container) addToArray(val uint16) {
	lo, hi := 0, len(c.arr)
	for lo < hi {
		mid := (lo + hi) / 2
		if c.arr[mid] < val {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(c.arr) && c.arr[lo] == val {
		return
	}
	c.arr = append(c.arr, 0)
	copy(c.arr[lo+1:], c.arr[lo:])
	c.arr[lo] = val
	c.count++
	if len(c.arr) > maxArrayLen {
		c.promoteToBitmap()
	}
}

func (c *container) addToBitmap(val uint16) {
	w, b := val/64, val%64
	if c.bits[w]&(1<<b) == 0 {
		c.bits[w] |= 1 << b
		c.count++
	}
}

func (c *container) contains(val uint16) bool {
	if c.kind == kindArray {
		lo, hi := 0, len(c.arr)
		for lo < hi {
			mid := (lo + hi) / 2
			if c.arr[mid] < val {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		return lo < len(c.arr) && c.arr[lo] == val
	}
	w, b := val/64, val%64
	return c.bits[w]&(1<<b) != 0
}

func (c *container) promoteToBitmap() {
	bts := make([]uint64, bitmapWords)
	for _, v := range c.arr {
		bts[v/64] |= 1 << (v % 64)
	}
	c.bits = bts
	c.arr = nil
	c.kind = kindBitmap
}

func (c *container) toBitsSlice() []uint64 {
	if c.kind == kindBitmap {
		return c.bits
	}
	tmp := make([]uint64, bitmapWords)
	for _, v := range c.arr {
		tmp[v/64] |= 1 << (v % 64)
	}
	return tmp
}

func bitsToValues(bts []uint64) []uint16 {
	out := make([]uint16, 0, 64)
	for i, w := range bts {
		for w != 0 {
			bit := bits.TrailingZeros64(w)
			out = append(out, uint16(i*64+bit))
			w &= w - 1 // сбросить младший бит
		}
	}
	return out
}

func fromBits(bts []uint64, cnt int) *container {
	if cnt <= maxArrayLen {
		return &container{
			kind:  kindArray,
			arr:   bitsToValues(bts),
			count: cnt,
		}
	}
	cp := make([]uint64, bitmapWords)
	copy(cp, bts)
	return &container{kind: kindBitmap, bits: cp, count: cnt}
}

func (c *container) clone() *container {
	nc := &container{kind: c.kind, count: c.count}
	if c.kind == kindArray {
		nc.arr = make([]uint16, len(c.arr))
		copy(nc.arr, c.arr)
	} else {
		nc.bits = make([]uint64, bitmapWords)
		copy(nc.bits, c.bits)
	}
	return nc
}
func orContainers(a, b *container) *container {
	aBits := a.toBitsSlice()
	bBits := b.toBitsSlice()
	res := make([]uint64, bitmapWords)
	cnt := 0
	for i := range res {
		res[i] = aBits[i] | bBits[i]
		cnt += bits.OnesCount64(res[i])
	}
	return fromBits(res, cnt)
}

func andContainers(a, b *container) *container {
	if a.kind == kindArray && b.kind == kindArray {
		return andArrays(a.arr, b.arr)
	}
	aBits := a.toBitsSlice()
	bBits := b.toBitsSlice()
	res := make([]uint64, bitmapWords)
	cnt := 0
	for i := range res {
		res[i] = aBits[i] & bBits[i]
		cnt += bits.OnesCount64(res[i])
	}
	return fromBits(res, cnt)
}

func andNotContainers(a, b *container) *container {
	if a.kind == kindArray && b.kind == kindArray {
		return andNotArrays(a.arr, b.arr)
	}
	aBits := a.toBitsSlice()
	bBits := b.toBitsSlice()
	res := make([]uint64, bitmapWords)
	cnt := 0
	for i := range res {
		res[i] = aBits[i] &^ bBits[i]
		cnt += bits.OnesCount64(res[i])
	}
	return fromBits(res, cnt)
}

func andArrays(a, b []uint16) *container {
	c := &container{kind: kindArray}
	ai, bi := 0, 0
	for ai < len(a) && bi < len(b) {
		switch {
		case a[ai] == b[bi]:
			c.arr = append(c.arr, a[ai])
			c.count++
			ai++
			bi++
		case a[ai] < b[bi]:
			ai++
		default:
			bi++
		}
	}
	return c
}

func andNotArrays(a, b []uint16) *container {
	c := &container{kind: kindArray}
	bi := 0
	for _, av := range a {
		for bi < len(b) && b[bi] < av {
			bi++
		}
		if bi >= len(b) || b[bi] != av {
			c.arr = append(c.arr, av)
			c.count++
		}
	}
	return c
}
