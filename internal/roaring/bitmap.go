package roaring

type Bitmap struct {
	keys []uint16
	cons []*container
}

func New() *Bitmap { return &Bitmap{} }

func (bm *Bitmap) findIdx(high uint16) (pos int, found bool) {
	lo, hi := 0, len(bm.keys)
	for lo < hi {
		mid := (lo + hi) / 2
		if bm.keys[mid] < high {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < len(bm.keys) && bm.keys[lo] == high
}

func (bm *Bitmap) Add(x uint32) {
	high, low := uint16(x>>16), uint16(x&0xFFFF)
	pos, found := bm.findIdx(high)
	if found {
		bm.cons[pos].add(low)
		return
	}
	bm.keys = append(bm.keys, 0)
	bm.cons = append(bm.cons, nil)
	copy(bm.keys[pos+1:], bm.keys[pos:])
	copy(bm.cons[pos+1:], bm.cons[pos:])
	bm.keys[pos] = high
	bm.cons[pos] = newArrayContainer(low)
}

func (bm *Bitmap) Contains(x uint32) bool {
	high, low := uint16(x>>16), uint16(x&0xFFFF)
	pos, found := bm.findIdx(high)
	if !found {
		return false
	}
	return bm.cons[pos].contains(low)
}

func (bm *Bitmap) GetCardinality() uint64 {
	var n uint64
	for _, c := range bm.cons {
		n += uint64(c.count)
	}
	return n
}

func (bm *Bitmap) ToArray() []uint32 {
	out := make([]uint32, 0, bm.GetCardinality())
	for i, key := range bm.keys {
		base := uint32(key) << 16
		var lows []uint16
		c := bm.cons[i]
		if c.kind == kindArray {
			lows = c.arr
		} else {
			lows = bitsToValues(c.bits)
		}
		for _, low := range lows {
			out = append(out, base|uint32(low))
		}
	}
	return out
}

func (bm *Bitmap) Clone() *Bitmap {
	nb := &Bitmap{
		keys: make([]uint16, len(bm.keys)),
		cons: make([]*container, len(bm.cons)),
	}
	copy(nb.keys, bm.keys)
	for i, c := range bm.cons {
		nb.cons[i] = c.clone()
	}
	return nb
}

func Or(a, b *Bitmap) *Bitmap {
	res := &Bitmap{}
	ai, bi := 0, 0
	for ai < len(a.keys) && bi < len(b.keys) {
		ak, bk := a.keys[ai], b.keys[bi]
		switch {
		case ak < bk:
			res.keys = append(res.keys, ak)
			res.cons = append(res.cons, a.cons[ai].clone())
			ai++
		case ak > bk:
			res.keys = append(res.keys, bk)
			res.cons = append(res.cons, b.cons[bi].clone())
			bi++
		default:
			res.keys = append(res.keys, ak)
			res.cons = append(res.cons, orContainers(a.cons[ai], b.cons[bi]))
			ai++
			bi++
		}
	}
	for ; ai < len(a.keys); ai++ {
		res.keys = append(res.keys, a.keys[ai])
		res.cons = append(res.cons, a.cons[ai].clone())
	}
	for ; bi < len(b.keys); bi++ {
		res.keys = append(res.keys, b.keys[bi])
		res.cons = append(res.cons, b.cons[bi].clone())
	}
	return res
}

func And(a, b *Bitmap) *Bitmap {
	res := &Bitmap{}
	ai, bi := 0, 0
	for ai < len(a.keys) && bi < len(b.keys) {
		ak, bk := a.keys[ai], b.keys[bi]
		switch {
		case ak < bk:
			ai++
		case ak > bk:
			bi++
		default:
			c := andContainers(a.cons[ai], b.cons[bi])
			if c.count > 0 {
				res.keys = append(res.keys, ak)
				res.cons = append(res.cons, c)
			}
			ai++
			bi++
		}
	}
	return res
}

func AndNot(a, b *Bitmap) *Bitmap {
	res := &Bitmap{}
	bi := 0
	for ai := 0; ai < len(a.keys); ai++ {
		ak := a.keys[ai]
		for bi < len(b.keys) && b.keys[bi] < ak {
			bi++
		}
		if bi >= len(b.keys) || b.keys[bi] != ak {
			res.keys = append(res.keys, ak)
			res.cons = append(res.cons, a.cons[ai].clone())
		} else {
			c := andNotContainers(a.cons[ai], b.cons[bi])
			if c.count > 0 {
				res.keys = append(res.keys, ak)
				res.cons = append(res.cons, c)
			}
		}
	}
	return res
}
