package lsm

import (
	"bytes"
	"container/heap"
	"fmt"
	"os"
	"time"

	"inverse_index/internal/sstable"
)

type mergeItem struct {
	key []byte
	val []byte

	fileIdx int
	iter    *sstable.Iterator
}

type mergeHeap []*mergeItem

func (h mergeHeap) Len() int { return len(h) }
func (h mergeHeap) Less(i, j int) bool {
	if c := bytes.Compare(h[i].key, h[j].key); c != 0 {
		return c < 0
	}
	return h[i].fileIdx < h[j].fileIdx
}
func (h mergeHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *mergeHeap) Push(x interface{}) { *h = append(*h, x.(*mergeItem)) }
func (h *mergeHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

func (t *Tree) runCompaction(levelIdx int) error {
	current := t.levels[levelIdx]
	nextLevelIdx := levelIdx + 1

	h := &mergeHeap{}
	heap.Init(h)

	closeAll := func() {
		for h.Len() > 0 {
			heap.Pop(h).(*mergeItem).iter.Close()
		}
	}

	for i, sst := range current {
		it, err := sst.NewIterator()
		if err != nil {
			return err
		}
		if it.NextKey() {
			vb, ok := it.ReadValue()
			if !ok {
				it.Close()
				return it.Error()
			}
			heap.Push(h, &mergeItem{
				key:     append([]byte(nil), it.Key...),
				val:     append([]byte(nil), vb...),
				fileIdx: i,
				iter:    it,
			})
		} else {
			it.Close()
		}
	}

	newFilename := fmt.Sprintf("%s/L%d_%d.sst", t.dir, nextLevelIdx, time.Now().UnixNano())
	writer, err := sstable.NewWriter(newFilename)
	if err != nil {
		closeAll()
		return err
	}

	// pendingKey/Val - буфер текущего ключа
	// Пишим в SSTable только при смене ключа, чтобы успеть вызвать MergeFn
	var pendingKey, pendingVal []byte
	hasPending := false

	flushPending := func() error {
		if !hasPending || bytes.Equal(pendingVal, tombBytes) {
			return nil
		}
		return writer.AppendBytes(pendingKey, pendingVal)
	}

	advance := func(item *mergeItem) error {
		if item.iter.NextKey() {
			vb, ok := item.iter.ReadValue()
			if !ok {
				item.iter.Close()
				return item.iter.Error()
			}
			item.key = append(item.key[:0], item.iter.Key...)
			item.val = append(item.val[:0], vb...)
			heap.Push(h, item)
		} else {
			if e := item.iter.Error(); e != nil {
				item.iter.Close()
				return e
			}
			item.iter.Close()
		}
		return nil
	}

	for h.Len() > 0 {
		top := heap.Pop(h).(*mergeItem)

		if hasPending && bytes.Equal(top.key, pendingKey) {
			// Дубликат ключа
			if t.MergeFn != nil &&
				!bytes.Equal(pendingVal, tombBytes) &&
				!bytes.Equal(top.val, tombBytes) {
				pendingVal = t.MergeFn(pendingVal, top.val)
			}
			// pendingVal (новее) побеждает
		} else {
			if err = flushPending(); err != nil {
				top.iter.Close()
				closeAll()
				return err
			}
			pendingKey = append([]byte(nil), top.key...)
			pendingVal = append([]byte(nil), top.val...)
			hasPending = true
		}

		if err = advance(top); err != nil {
			closeAll()
			return err
		}
	}

	if err = flushPending(); err != nil {
		return err
	}

	newSST, err := writer.Close()
	if err != nil {
		return err
	}

	for _, sst := range current {
		sst.Close()
		os.Remove(sst.File.Name())
	}

	t.levels[levelIdx] = nil
	if nextLevelIdx >= len(t.levels) {
		t.levels = append(t.levels, []*sstable.SSTable{})
	}
	t.levels[nextLevelIdx] = append([]*sstable.SSTable{newSST}, t.levels[nextLevelIdx]...)
	return nil
}
