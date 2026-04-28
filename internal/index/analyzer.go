package index

import (
	"strings"
	"unicode"

	"github.com/kljensen/snowball"
)

var stopEN = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "and": {}, "or": {}, "not": {},
	"is": {}, "are": {}, "was": {}, "were": {}, "be": {}, "been": {},
	"have": {}, "has": {}, "had": {}, "do": {}, "does": {}, "did": {},
	"will": {}, "would": {}, "could": {}, "should": {}, "may": {},
	"to": {}, "of": {}, "in": {}, "for": {}, "on": {}, "with": {},
	"at": {}, "by": {}, "from": {}, "as": {}, "it": {}, "its": {},
	"this": {}, "that": {}, "i": {}, "you": {}, "he": {}, "she": {},
	"we": {}, "they": {}, "me": {}, "him": {}, "her": {}, "us": {},
	"them": {}, "my": {}, "your": {}, "his": {}, "our": {}, "their": {},
}

var stopRU = map[string]struct{}{
	"и": {}, "в": {}, "во": {}, "не": {}, "что": {}, "он": {},
	"на": {}, "я": {}, "с": {}, "как": {}, "а": {}, "то": {},
	"она": {}, "так": {}, "его": {}, "но": {}, "да": {}, "ты": {},
	"к": {}, "у": {}, "же": {}, "вы": {}, "за": {}, "бы": {},
	"по": {}, "только": {}, "ее": {}, "от": {}, "это": {}, "о": {},
	"из": {}, "ему": {}, "когда": {}, "или": {}, "ни": {}, "быть": {},
	"был": {}, "до": {}, "вас": {}, "уже": {}, "там": {}, "где": {},
	"для": {}, "мы": {}, "их": {}, "чем": {}, "была": {}, "без": {},
	"раз": {}, "тоже": {}, "под": {}, "будет": {}, "кто": {},
	"этот": {}, "при": {}, "об": {}, "про": {}, "всё": {}, "нас": {},
}

// Analyzer токенизирует, удаляет стоп-слова и стеммирует текст
type Analyzer struct {
	lang string // "english" | "russian"
}

func NewAnalyzer(lang string) *Analyzer {
	if lang == "" {
		lang = "english"
	}
	return &Analyzer{lang: lang}
}

func (a *Analyzer) Analyze(text string) []string {
	tokens := tokenize(text)
	out := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		tok = strings.ToLower(tok)
		if len(tok) < 2 {
			continue
		}
		if a.isStop(tok) {
			continue
		}
		stemmed, err := snowball.Stem(tok, a.lang, true)
		if err != nil || len(stemmed) == 0 {
			stemmed = tok
		}
		out = append(out, stemmed)
	}
	return out
}

func (a *Analyzer) isStop(w string) bool {
	if a.lang == "russian" {
		_, ok := stopRU[w]
		return ok
	}
	_, ok := stopEN[w]
	return ok
}

func tokenize(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}
