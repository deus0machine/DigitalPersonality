package utterance

import (
	"context"
	"math"
	"sort"
	"strings"
	"unicode"
)

// BM25Scorer implements Scorer using the Okapi BM25 ranking function.
// All computation is in-memory; no external dependencies.
//
// Known limitations (acceptable for lexical baseline):
//   - no Russian morphology / stemming — "существует" ≠ "существование"
//   - no synonym expansion
//   - exact token matching only
type BM25Scorer struct {
	K1 float64 // term saturation (default 1.2)
	B  float64 // length normalisation (default 0.75)
}

// NewBM25Scorer returns a BM25Scorer with standard parameters.
func NewBM25Scorer() *BM25Scorer {
	return &BM25Scorer{K1: 1.2, B: 0.75}
}

// Score ranks utterances against query using BM25. Returns results sorted by
// score descending; utterances with score == 0 are omitted.
func (s *BM25Scorer) Score(_ context.Context, query string, utterances []Utterance) ([]SearchResult, error) {
	queryTerms := tokenize(query)
	if len(queryTerms) == 0 || len(utterances) == 0 {
		return nil, nil
	}

	corpus := buildCorpus(utterances)
	if corpus.avgdl == 0 {
		return nil, nil
	}

	var results []SearchResult
	for i, u := range utterances {
		sc := s.scoreDoc(corpus, corpus.docs[i], queryTerms)
		if sc > 0 {
			results = append(results, SearchResult{Utterance: u, Score: sc})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results, nil
}

// ── internals ────────────────────────────────────────────────────────────────

type bm25Corpus struct {
	N      int            // total document count
	df     map[string]int // document frequency per term
	avgdl  float64        // average document length in tokens
	docs   [][]string     // tokenised form of each utterance (same index as input)
}

func buildCorpus(utterances []Utterance) bm25Corpus {
	c := bm25Corpus{
		N:    len(utterances),
		df:   make(map[string]int),
		docs: make([][]string, len(utterances)),
	}

	totalLen := 0
	for i, u := range utterances {
		tokens := tokenize(u.Text)
		c.docs[i] = tokens
		totalLen += len(tokens)

		seen := make(map[string]struct{}, len(tokens))
		for _, t := range tokens {
			if _, ok := seen[t]; !ok {
				seen[t] = struct{}{}
				c.df[t]++
			}
		}
	}

	if len(utterances) > 0 {
		c.avgdl = float64(totalLen) / float64(len(utterances))
	}
	return c
}

func (s *BM25Scorer) scoreDoc(c bm25Corpus, docTokens []string, queryTerms []string) float64 {
	dl := float64(len(docTokens))
	if dl == 0 {
		return 0
	}

	tf := make(map[string]int, len(docTokens))
	for _, t := range docTokens {
		tf[t]++
	}

	var score float64
	for _, qt := range queryTerms {
		f := float64(tf[qt])
		if f == 0 {
			continue
		}
		df := float64(c.df[qt])
		N := float64(c.N)
		idf := math.Log((N-df+0.5)/(df+0.5) + 1)
		score += idf * (f * (s.K1 + 1)) / (f + s.K1*(1-s.B+s.B*dl/c.avgdl))
	}
	return score
}

// tokenize lowercases s and replaces all non-letter, non-digit runes with spaces.
func tokenize(s string) []string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, s)
	return strings.Fields(s)
}
