package lsm

import (
	"bytes"
	"container/heap"
	"fmt"
	"sort"

	"inverse_index/internal/sstable"
)

var tombBytes = []byte(Tomb)

type scanItem struct {
	key  []byte
	val  []byte
	prio int
	iter *sstable.Iterator
	end  []byte
}

type scanHeap []*scanItem

func (h scanHeap) Len() int { return len(h) }
func (h scanHeap) Less(i, j int) bool {
	if c := bytes.Compare(h[i].key, h[j].key); c != 0 {
		return c < 0
	}
	return h[i].prio < h[j].prio // меньше = новее
}
func (h scanHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *scanHeap) Push(x any)   { *h = append(*h, x.(*scanItem)) }
func (h *scanHeap) Pop() any {
	old := *h
	n := len(old)
	it := old[n-1]
	*h = old[:n-1]
	return it
}

func clone(b []byte) []byte { return append([]byte(nil), b...) }
func (t *Tree) RangeScan(start, end string, limit int) ([][2]string, error) {
	if end != "" && start >= end {
		return nil, fmt.Errorf("bad range: start >= end")
	}
	if limit <= 0 {
		limit = 100
	}

	startB := []byte(start)
	var endB []byte
	if end != "" {
		endB = []byte(end)
	}

	t.RLock()
	memPairs := make([][2]string, 0)
	t.memTable.InOrder(func(k, v string) {
		if k >= start && (end == "" || k < end) && v != Tomb {
			memPairs = append(memPairs, [2]string{k, v})
		}
	})
	levels := make([][]*sstable.SSTable, len(t.levels))
	for i := range t.levels {
		levels[i] = append([]*sstable.SSTable(nil), t.levels[i]...)
	}
	t.RUnlock()

	h := &scanHeap{}
	heap.Init(h)

	closeHeap := func() {
		for h.Len() > 0 {
			_ = heap.Pop(h).(*scanItem).iter.Close()
		}
	}

	advanceAndPush := func(item *scanItem) error {
		for item.iter.NextKey() {
			k := item.iter.Key
			if len(item.end) > 0 && bytes.Compare(k, item.end) >= 0 {
				_ = item.iter.Close()
				return nil
			}
			vb, ok := item.iter.ReadValue()
			if !ok {
				err := item.iter.Error()
				_ = item.iter.Close()
				if err == nil {
					err = fmt.Errorf("ReadValue failed")
				}
				return err
			}
			item.key = clone(k)
			item.val = clone(vb)
			heap.Push(h, item)
			return nil
		}
		if err := item.iter.Error(); err != nil {
			_ = item.iter.Close()
			return err
		}
		_ = item.iter.Close()
		return nil
	}

	pushSST := func(levelIdx, fileIdx int, sst *sstable.SSTable) error {
		if len(endB) > 0 && sst.MinKey >= end {
			return nil
		}
		if sst.MaxKey < start {
			return nil
		}
		idxPos := sort.Search(len(sst.Index), func(i int) bool {
			return sst.Index[i].Key > start
		})
		startIdx := idxPos - 1
		if startIdx < 0 {
			startIdx = 0
		}
		it, err := sst.NewIteratorAt(sst.Index[startIdx].Offset)
		if err != nil {
			return err
		}
		for it.NextKey() {
			k := it.Key
			if bytes.Compare(k, startB) < 0 {
				if !it.SkipValue() {
					err = it.Error()
					_ = it.Close()
					if err == nil {
						err = fmt.Errorf("SkipValue failed")
					}
					return err
				}
				continue
			}
			if len(endB) > 0 && bytes.Compare(k, endB) >= 0 {
				_ = it.Close()
				return nil
			}
			vb, ok := it.ReadValue()
			if !ok {
				err = it.Error()
				_ = it.Close()
				if err == nil {
					err = fmt.Errorf("ReadValue failed")
				}
				return err
			}
			heap.Push(h, &scanItem{
				key:  clone(k),
				val:  clone(vb),
				prio: levelIdx*1_000_000 + fileIdx,
				iter: it,
				end:  endB,
			})
			return nil
		}
		if err = it.Error(); err != nil {
			_ = it.Close()
			return err
		}
		_ = it.Close()
		return nil
	}

	for lvl := 0; lvl < len(levels); lvl++ {
		for fi := 0; fi < len(levels[lvl]); fi++ {
			if err := pushSST(lvl, fi, levels[lvl][fi]); err != nil {
				closeHeap()
				return nil, err
			}
		}
	}

	sstOut := make([][2]string, 0, 256)
	var sstLastKey []byte
	for h.Len() > 0 {
		top := heap.Pop(h).(*scanItem)
		if len(top.end) > 0 && bytes.Compare(top.key, top.end) >= 0 {
			_ = top.iter.Close()
			break
		}
		if !bytes.Equal(top.key, sstLastKey) {
			if !bytes.Equal(top.val, tombBytes) {
				sstOut = append(sstOut, [2]string{string(top.key), string(top.val)})
			}
			sstLastKey = append(sstLastKey[:0], top.key...)
		}
		if err := advanceAndPush(top); err != nil {
			closeHeap()
			return nil, err
		}
	}
	closeHeap()

	out := make([][2]string, 0, min(limit, 256))
	mi, si := 0, 0
	var lastKey string

	emit := func(k, v string) bool {
		if k == lastKey {
			return true // уже есть более новая версия
		}
		lastKey = k
		out = append(out, [2]string{k, v})
		return len(out) < limit
	}

	for mi < len(memPairs) && si < len(sstOut) && len(out) < limit {
		mk, sk := memPairs[mi][0], sstOut[si][0]
		if mk <= sk {
			if !emit(memPairs[mi][0], memPairs[mi][1]) {
				return out, nil
			}
			if mk == sk {
				si++ // SSTable-версия устарела — пропускаем
			}
			mi++
		} else {
			if !emit(sstOut[si][0], sstOut[si][1]) {
				return out, nil
			}
			si++
		}
	}
	for ; mi < len(memPairs) && len(out) < limit; mi++ {
		if !emit(memPairs[mi][0], memPairs[mi][1]) {
			return out, nil
		}
	}
	for ; si < len(sstOut) && len(out) < limit; si++ {
		if !emit(sstOut[si][0], sstOut[si][1]) {
			return out, nil
		}
	}
	return out, nil
}
