package index

import (
	"math"
	"sort"
	"sync"

	"inverse_index/internal/lsm"
	"inverse_index/internal/roaring"
)

const BlockSize = uint32(65536)

func blockKey(term string, blockID uint32) string {
	var buf [4]byte
	buf[0] = byte(blockID >> 24)
	buf[1] = byte(blockID >> 16)
	buf[2] = byte(blockID >> 8)
	buf[3] = byte(blockID)
	return term + string(buf[:])
}

func postingMergeFn(newer, older []byte) []byte {
	a, err := DecodePostingList(newer)
	if err != nil {
		return newer
	}
	b, err := DecodePostingList(older)
	if err != nil {
		return newer
	}
	return MergePostingLists(a, b).Encode()
}

type ScoredDoc struct {
	DocID uint32
	Score float64
}

type InvertedIndex struct {
	tree     *lsm.Tree
	analyzer *Analyzer

	mu        sync.Mutex
	allDocs   *roaring.Bitmap
	tf        map[uint32]map[string]int // tf[docID][stemmedTerm] = count
	df        map[string]int            // df[term] = число документов
	tiers     []map[string][]ScoredDoc
	tierSizes []int
}

func NewInvertedIndex(dir string, lang string) (*InvertedIndex, error) {
	tree, err := lsm.NewWithMerge(dir, postingMergeFn)
	if err != nil {
		return nil, err
	}
	sizes := []int{100, 1000, 10000}
	tiers := make([]map[string][]ScoredDoc, len(sizes))
	for i := range tiers {
		tiers[i] = make(map[string][]ScoredDoc)
	}
	return &InvertedIndex{
		tree:      tree,
		analyzer:  NewAnalyzer(lang),
		allDocs:   roaring.New(),
		tf:        make(map[uint32]map[string]int),
		df:        make(map[string]int),
		tiers:     tiers,
		tierSizes: sizes,
	}, nil
}

func (idx *InvertedIndex) Index(docID uint32, text string) {
	idx.mu.Lock()
	idx.allDocs.Add(docID)
	idx.mu.Unlock()

	tokensWithPos := idx.analyzer.AnalyzeWithPositions(text)

	blockID := docID / BlockSize
	relID := docID % BlockSize

	termPositions := make(map[string][]uint32)
	for _, tp := range tokensWithPos {
		termPositions[tp.Term] = append(termPositions[tp.Term], tp.Position)
	}

	idx.mu.Lock()
	if idx.tf[docID] == nil {
		idx.tf[docID] = make(map[string]int)
	}
	for term, positions := range termPositions {
		idx.tf[docID][term] = len(positions)
		idx.df[term]++
	}
	idx.mu.Unlock()

	for term, positions := range termPositions {
		pl := PostingList{relID: positions}
		idx.tree.Put(blockKey(term, blockID), string(pl.Encode()))
	}
}

func (idx *InvertedIndex) LookupPostings(term string) PostingList {
	terms := idx.analyzer.Analyze(term)
	if len(terms) == 0 {
		return PostingList{}
	}
	stemmed := terms[0]

	startKey := blockKey(stemmed, 0)
	endKey := blockKey(stemmed, ^uint32(0)) + "\xFF"
	pairs, err := idx.tree.RangeScan(startKey, endKey, 1<<24)
	if err != nil || len(pairs) == 0 {
		return PostingList{}
	}

	result := PostingList{}
	for _, kv := range pairs {
		k := kv[0]
		n := len(k)
		if n < 4 {
			continue
		}
		blockID := uint32(k[n-4])<<24 | uint32(k[n-3])<<16 |
			uint32(k[n-2])<<8 | uint32(k[n-1])
		base := blockID * BlockSize
		pl, err := DecodePostingList([]byte(kv[1]))
		if err != nil {
			continue
		}
		for relID, positions := range pl {
			result[base+relID] = append(result[base+relID], positions...)
		}
	}
	return result
}

// обратная совместимость
func (idx *InvertedIndex) Lookup(word string) *roaring.Bitmap {
	pl := idx.LookupPostings(word)
	bm := roaring.New()
	for docID := range pl {
		bm.Add(docID)
	}
	return bm
}

func (idx *InvertedIndex) AllDocs() *roaring.Bitmap {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return idx.allDocs.Clone()
}

func (idx *InvertedIndex) Query(expr Expr) (*roaring.Bitmap, error) {
	return evalExpr(idx, expr)
}

// tfidf: TF = 1+log(count), IDF = log(N/df)
func (idx *InvertedIndex) tfidf(docID uint32, stemmedTerm string) float64 {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	n := float64(len(idx.tf))
	if n == 0 {
		return 0
	}
	count := idx.tf[docID][stemmedTerm]
	if count == 0 {
		return 0
	}
	df := float64(idx.df[stemmedTerm])
	if df == 0 {
		return 0
	}
	return (1 + math.Log(float64(count))) * math.Log(n/df)
}

// RankTFIDF ранжирует документы из bitmap по сумме TF-IDF
func (idx *InvertedIndex) RankTFIDF(bm *roaring.Bitmap, queryTerms []string) []ScoredDoc {
	stemmed := idx.stemAll(queryTerms)
	docs := bm.ToArray()
	results := make([]ScoredDoc, 0, len(docs))
	for _, docID := range docs {
		var score float64
		for _, term := range stemmed {
			score += idx.tfidf(docID, term)
		}
		if score > 0 {
			results = append(results, ScoredDoc{docID, score})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	return results
}

// RankVSM ранжирует по косинусному сходству в TF-IDF пространстве
func (idx *InvertedIndex) RankVSM(bm *roaring.Bitmap, queryTerms []string) []ScoredDoc {
	stemmed := idx.stemAll(queryTerms)
	idx.mu.Lock()
	n := float64(len(idx.tf))
	idx.mu.Unlock()
	if n == 0 {
		return nil
	}

	queryVec := make(map[string]float64)
	for _, term := range stemmed {
		idx.mu.Lock()
		df := float64(idx.df[term])
		idx.mu.Unlock()
		if df > 0 {
			queryVec[term] += math.Log(n / df)
		}
	}
	var queryNorm float64
	for _, v := range queryVec {
		queryNorm += v * v
	}
	queryNorm = math.Sqrt(queryNorm)
	if queryNorm == 0 {
		return nil
	}

	docs := bm.ToArray()
	results := make([]ScoredDoc, 0, len(docs))
	for _, docID := range docs {
		var dot, docNorm float64
		for term, qv := range queryVec {
			dv := idx.tfidf(docID, term)
			dot += qv * dv
			docNorm += dv * dv
		}
		docNorm = math.Sqrt(docNorm)
		if docNorm == 0 {
			continue
		}
		results = append(results, ScoredDoc{docID, dot / (queryNorm * docNorm)})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	return results
}

func (idx *InvertedIndex) BuildTieredIndex() {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	for term := range idx.df {
		n := float64(len(idx.tf))
		df := float64(idx.df[term])
		if df == 0 || n == 0 {
			continue
		}
		idf := math.Log(n / df)

		type entry struct {
			docID uint32
			score float64
		}
		var entries []entry
		for docID, termMap := range idx.tf {
			count := termMap[term]
			if count == 0 {
				continue
			}
			tf := 1 + math.Log(float64(count))
			entries = append(entries, entry{docID, tf * idf})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].score > entries[j].score
		})

		// заполняем каждый тир (нарастающим срезом)
		for t, size := range idx.tierSizes {
			end := size
			if end > len(entries) {
				end = len(entries)
			}
			champ := make([]ScoredDoc, end)
			for i := 0; i < end; i++ {
				champ[i] = ScoredDoc{entries[i].docID, entries[i].score}
			}
			idx.tiers[t][term] = champ
		}
	}
}

// overshoot -- множитель: ищем кандидатов >= K * overshoot перед скорингом
func (idx *InvertedIndex) TieredSearch(k int, overshoot int, queryTerms ...string) []ScoredDoc {
	stemmed := idx.stemAll(queryTerms)
	if overshoot <= 0 {
		overshoot = 5
	}

	for tierIdx := range idx.tiers {
		candidates := make(map[uint32]struct{})
		idx.mu.Lock()
		for _, term := range stemmed {
			for _, sd := range idx.tiers[tierIdx][term] {
				candidates[sd.DocID] = struct{}{}
			}
		}
		idx.mu.Unlock()

		// условие остановки: кандидатов достаточно
		if len(candidates) >= k*overshoot {
			return idx.scoreAndSort(candidates, stemmed, k)
		}
	}

	// все тиры исчерпаны -- скорим то, что есть
	candidates := make(map[uint32]struct{})
	idx.mu.Lock()
	lastTier := idx.tiers[len(idx.tiers)-1]
	for _, term := range stemmed {
		for _, sd := range lastTier[term] {
			candidates[sd.DocID] = struct{}{}
		}
	}
	idx.mu.Unlock()
	return idx.scoreAndSort(candidates, stemmed, k)
}

func (idx *InvertedIndex) scoreAndSort(candidates map[uint32]struct{}, stemmed []string, k int) []ScoredDoc {
	results := make([]ScoredDoc, 0, len(candidates))
	for docID := range candidates {
		var score float64
		for _, term := range stemmed {
			score += idx.tfidf(docID, term)
		}
		if score > 0 {
			results = append(results, ScoredDoc{docID, score})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if k > 0 && len(results) > k {
		results = results[:k]
	}
	return results
}

func (idx *InvertedIndex) stemAll(terms []string) []string {
	out := make([]string, 0, len(terms))
	for _, t := range terms {
		out = append(out, idx.analyzer.Analyze(t)...)
	}
	return out
}
