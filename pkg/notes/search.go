package notes

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// Field weights for BM25 scoring.
// A match in the title is worth more than a match buried in a comment.
const (
	weightTitle   = 3.0
	weightBody    = 1.0
	weightTags    = 2.0
	weightComment = 0.5
)

// BM25 tuning parameters.
const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// SearchOptions extends FindOptions with full-text search parameters.
type SearchOptions struct {
	FindOptions
	Limit int // default 20
}

// SearchResult is a scored search hit.
type SearchResult struct {
	ID    string  `json:"id"`
	Score float64 `json:"score"`
	State *State  `json:"state"`
}

// TextIndex holds precomputed BM25 data for full-text search.
// Built during Index.Build(), queried via Search().
type TextIndex struct {
	docIDs   []string         // parallel arrays — one entry per State
	docLens  []float64        // weighted token count per doc
	docTerms []map[string]float64 // weighted term frequencies per doc
	states   []*State         // parallel — for returning results

	df       map[string]int   // document frequency: how many docs contain term
	avgDL    float64          // average document length
	docCount int
}

// build constructs the text index from all states.
func (ti *TextIndex) build(states map[string]*State) {
	n := len(states)
	ti.docIDs = make([]string, 0, n)
	ti.docLens = make([]float64, 0, n)
	ti.docTerms = make([]map[string]float64, 0, n)
	ti.states = make([]*State, 0, n)
	ti.df = make(map[string]int)
	ti.docCount = n

	if n == 0 {
		return
	}

	// Index each state as a weighted document
	for id, state := range states {
		tf := make(map[string]float64)
		var docLen float64

		// Title — highest weight
		titleTokens := tokenize(state.Title)
		for _, tok := range titleTokens {
			tf[tok] += weightTitle
			docLen += weightTitle
		}

		// Body
		bodyTokens := tokenize(state.Body)
		for _, tok := range bodyTokens {
			tf[tok] += weightBody
			docLen += weightBody
		}

		// Tags
		for _, tag := range state.Tags {
			tagTokens := tokenize(tag)
			for _, tok := range tagTokens {
				tf[tok] += weightTags
				docLen += weightTags
			}
		}

		// Comments
		for _, c := range state.Comments {
			commentTokens := tokenize(c.Body)
			for _, tok := range commentTokens {
				tf[tok] += weightComment
				docLen += weightComment
			}
		}

		// Document frequency — count each unique term once per doc
		for term := range tf {
			ti.df[term]++
		}

		ti.docIDs = append(ti.docIDs, id)
		ti.docLens = append(ti.docLens, docLen)
		ti.docTerms = append(ti.docTerms, tf)
		ti.states = append(ti.states, state)
	}

	// Average document length
	var totalLen float64
	for _, dl := range ti.docLens {
		totalLen += dl
	}
	ti.avgDL = totalLen / float64(n)
}

// Search returns the top-K results for a query, ranked by BM25 score.
func (ti *TextIndex) Search(query string, topK int) []SearchResult {
	if ti.docCount == 0 || query == "" {
		return nil
	}

	queryTerms := tokenize(query)
	if len(queryTerms) == 0 {
		return nil
	}

	type scored struct {
		idx   int
		score float64
	}

	var hits []scored
	for i := 0; i < ti.docCount; i++ {
		s := ti.score(queryTerms, i)
		if s > 0 {
			hits = append(hits, scored{i, s})
		}
	}

	sort.Slice(hits, func(a, b int) bool {
		return hits[a].score > hits[b].score
	})

	if topK > 0 && len(hits) > topK {
		hits = hits[:topK]
	}

	results := make([]SearchResult, len(hits))
	for i, h := range hits {
		results[i] = SearchResult{
			ID:    ti.docIDs[h.idx],
			Score: h.score,
			State: ti.states[h.idx],
		}
	}
	return results
}

// SearchFiltered returns results filtered by FindOptions, then ranked by BM25.
func (ti *TextIndex) SearchFiltered(query string, opts SearchOptions) []SearchResult {
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	// Get all scored results first
	all := ti.Search(query, 0) // 0 = no limit

	// Filter by FindOptions
	var filtered []SearchResult
	for _, r := range all {
		if !matchesFind(r.State, opts.FindOptions) {
			continue
		}
		filtered = append(filtered, r)
		if len(filtered) >= limit {
			break
		}
	}
	return filtered
}

// score computes BM25 score for a single document against query terms.
func (ti *TextIndex) score(queryTerms []string, docIdx int) float64 {
	tf := ti.docTerms[docIdx]
	dl := ti.docLens[docIdx]
	var s float64

	for _, q := range queryTerms {
		f := tf[q]
		if f == 0 {
			continue
		}

		// IDF: log((N - df + 0.5) / (df + 0.5) + 1)
		df := float64(ti.df[q])
		N := float64(ti.docCount)
		idf := math.Log((N-df+0.5)/(df+0.5) + 1)

		// TF saturation with length normalization
		num := f * (bm25K1 + 1)
		den := f + bm25K1*(1-bm25B+bm25B*dl/ti.avgDL)
		s += idf * num / den
	}

	return s
}

// matchesFind checks if a state matches FindOptions filters.
func matchesFind(state *State, opts FindOptions) bool {
	if opts.Kind != "" && state.Kind != opts.Kind {
		return false
	}
	if opts.Status != "" && opts.Status != "all" && state.Status != opts.Status {
		return false
	}
	if opts.Tag != "" {
		found := false
		for _, t := range state.Tags {
			if t == opts.Tag {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if opts.Type != "" && state.Type != opts.Type {
		return false
	}
	if opts.Assignee != "" && state.Assignee != opts.Assignee {
		return false
	}
	if opts.Target != "" {
		found := false
		for _, tgt := range state.Targets {
			if tgt == opts.Target {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// tokenize splits text into lowercase tokens.
// No stemming — raw tokens proved sufficient in prototype testing.
func tokenize(text string) []string {
	if text == "" {
		return nil
	}
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
	})
	return words
}
