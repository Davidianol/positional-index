package lsm

import (
	"fmt"
	"inverse_index/internal/rbtree"
	"inverse_index/internal/sstable"
	"log"
	"os"
	"sync"
	"time"
)

const (
	MemTableSize = 1024 * 1024
	LLimit       = 10
	Tomb         = "__TOMBSTONE__"
)

// MergeFn объединяет newer и older значения одного ключа при компакции
type MergeFn func(newer, older []byte) []byte

type Tree struct {
	sync.RWMutex
	memTable *rbtree.RBTree
	memSize  int
	levels   [][]*sstable.SSTable
	dir      string
	MergeFn  MergeFn
}

func New(dir string) (*Tree, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return &Tree{
		memTable: rbtree.New(),
		levels:   make([][]*sstable.SSTable, 0),
		dir:      dir,
	}, nil
}

// NewWithMerge создаёт дерево с пользовательской функцией слияния.
func NewWithMerge(dir string, fn MergeFn) (*Tree, error) {
	t, err := New(dir)
	if err != nil {
		return nil, err
	}
	t.MergeFn = fn
	return t, nil
}

func (t *Tree) Put(key, value string) {
	t.Lock()
	defer t.Unlock()

	if t.MergeFn != nil {
		if old, exists := t.memTable.Get(key); exists && old != Tomb && value != Tomb {
			merged := t.MergeFn([]byte(value), []byte(old))
			value = string(merged)
		}
	}

	t.memTable.Put(key, value)

	if t.memTable.Size() > MemTableSize {
		t.flush()
	}
}

func (t *Tree) Get(key string) (string, bool) {
	t.RLock()
	defer t.RUnlock()
	if v, ok := t.memTable.Get(key); ok {
		if v == Tomb {
			return "", false
		}
		return v, true
	}
	for _, level := range t.levels {
		for _, sst := range level {
			if val, found := sst.Get(key); found {
				if val == Tomb {
					return "", false
				}
				return val, true
			}
		}
	}
	return "", false
}

func (t *Tree) Delete(key string) { t.Put(key, Tomb) }

func (t *Tree) flush() {
	if t.memTable.Count() == 0 {
		return
	}

	filename := fmt.Sprintf("%s/L0_%d.sst", t.dir, time.Now().UnixNano())
	writer, err := sstable.NewWriter(filename)
	if err != nil {
		log.Printf("flush: NewWriter: %v", err)
		return
	}

	var writeErr error
	t.memTable.InOrder(func(k, v string) {
		if writeErr == nil {
			writeErr = writer.Append(k, v)
		}
	})
	if writeErr != nil {
		log.Printf("flush: write: %v", writeErr)
		return
	}

	sst, err := writer.Close()
	if err != nil {
		log.Printf("flush: close: %v", err)
		return
	}

	if len(t.levels) == 0 {
		t.levels = append(t.levels, []*sstable.SSTable{})
	}
	t.levels[0] = append([]*sstable.SSTable{sst}, t.levels[0]...)
	t.memTable = rbtree.New()

	t.checkCompaction(0)
}

func (t *Tree) checkCompaction(levelIdx int) {
	if levelIdx >= len(t.levels) {
		return
	}
	if len(t.levels[levelIdx]) >= LLimit {
		if err := t.runCompaction(levelIdx); err != nil {
			log.Printf("compaction error level %d: %v", levelIdx, err)
			return
		}
		t.checkCompaction(levelIdx + 1)
	}
}
