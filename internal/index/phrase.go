package index

type queryWord struct {
	stemmed   string
	gapBefore uint32
}

func (idx *InvertedIndex) PhraseSearch(phrase ...string) []uint32 {
	if len(phrase) == 0 {
		return nil
	}
	var words []queryWord
	var gapAccum uint32 = 0
	for _, w := range phrase {
		terms := idx.analyzer.Analyze(w)
		if len(terms) == 0 {
			gapAccum++
			continue
		}
		words = append(words, queryWord{stemmed: terms[0], gapBefore: 1 + gapAccum})
		gapAccum = 0
	}
	if len(words) == 0 {
		return nil
	}

	postings := make([]PostingList, len(words))
	for i, w := range words {
		postings[i] = idx.LookupPostings(w.stemmed)
	}

	candidates := make(map[uint32]struct{})
	for docID := range postings[0] {
		candidates[docID] = struct{}{}
	}
	for i := 1; i < len(postings); i++ {
		for docID := range candidates {
			if _, ok := postings[i][docID]; !ok {
				delete(candidates, docID)
			}
		}
	}

	var result []uint32
	for docID := range candidates {
		if matchPhrase(docID, postings, words) {
			result = append(result, docID)
		}
	}
	return result
}

func matchPhrase(docID uint32, postings []PostingList, words []queryWord) bool {
	anchors := postings[0][docID]
	for step := 1; step < len(postings); step++ {
		anchors = findWithGap(anchors, postings[step][docID], words[step].gapBefore)
		if len(anchors) == 0 {
			return false
		}
	}
	return true
}

// findWithGap: next[j] == prev[i] + gap -> возвращает next[j] (якорь двигается вперёд).
func findWithGap(prev, next []uint32, gap uint32) []uint32 {
	var result []uint32
	i, j := 0, 0
	for i < len(prev) && j < len(next) {
		want := prev[i] + gap
		switch {
		case next[j] == want:
			result = append(result, next[j])
			i++
			j++
		case next[j] < want:
			j++
		default:
			i++
		}
	}
	return result
}
