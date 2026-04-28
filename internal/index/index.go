package index

import (
	"sync"

	"inverse_index/internal/lsm"
	"inverse_index/internal/roaring"
)

const BlockSize = uint32(65536) // 2^16
func blockKey(term string, blockID uint32) string {
	var buf [4]byte
	buf[0] = byte(blockID >> 24)
	buf[1] = byte(blockID >> 16)
	buf[2] = byte(blockID >> 8)
	buf[3] = byte(blockID)
	return term + string(buf[:])
}

// bitmapMergeFn - MergeFn для LSM: OR двух сериализованных Bitmap
func bitmapMergeFn(newer, older []byte) []byte {
	bmA, err := roaring.Decode(newer)
	if err != nil {
		return newer
	}
	bmB, err := roaring.Decode(older)
	if err != nil {
		return newer
	}
	return roaring.Or(bmA, bmB).Encode()
}

type InvertedIndex struct {
	tree     *lsm.Tree
	analyzer *Analyzer

	mu      sync.Mutex
	allDocs *roaring.Bitmap // NOT-запросы
}

func NewInvertedIndex(dir string, lang string) (*InvertedIndex, error) {
	tree, err := lsm.NewWithMerge(dir, bitmapMergeFn)
	if err != nil {
		return nil, err
	}
	return &InvertedIndex{
		tree:     tree,
		analyzer: NewAnalyzer(lang),
		allDocs:  roaring.New(),
	}, nil
}

// Index добавляет документ в индекс
func (idx *InvertedIndex) Index(docID uint32, text string) {
	idx.mu.Lock()
	idx.allDocs.Add(docID)
	idx.mu.Unlock()

	terms := idx.analyzer.Analyze(text)
	seen := make(map[string]struct{}, len(terms))

	blockID := docID / BlockSize
	relID := docID % BlockSize // относительная позиция внутри блока

	for _, term := range terms {
		if _, dup := seen[term]; dup {
			continue
		}
		seen[term] = struct{}{}

		bm := roaring.New()
		bm.Add(relID)
		idx.tree.Put(blockKey(term, blockID), string(bm.Encode()))
	}
}

func (idx *InvertedIndex) Lookup(word string) *roaring.Bitmap {
	terms := idx.analyzer.Analyze(word)
	if len(terms) == 0 {
		return roaring.New()
	}
	stemmed := terms[0]
	result := roaring.New()

	startKey := blockKey(stemmed, 0)
	endKey := blockKey(stemmed, ^uint32(0)) + "\xFF"

	pairs, err := idx.tree.RangeScan(startKey, endKey, 1<<24)
	if err != nil || len(pairs) == 0 {
		return result
	}

	for _, kv := range pairs {
		k := kv[0]

		n := len(k)
		if n < 4 {
			continue
		}
		blockID := uint32(k[n-4])<<24 | uint32(k[n-3])<<16 |
			uint32(k[n-2])<<8 | uint32(k[n-1])
		base := blockID * BlockSize

		bm, err := roaring.Decode([]byte(kv[1]))
		if err != nil {
			continue
		}
		for _, rel := range bm.ToArray() {
			result.Add(base + rel)
		}
	}
	return result
}

// AllDocs возвращает universe (все docID)
func (idx *InvertedIndex) AllDocs() *roaring.Bitmap {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return idx.allDocs.Clone()
}

func (idx *InvertedIndex) Query(expr Expr) (*roaring.Bitmap, error) {
	return evalExpr(idx, expr)
}
