package notes

import (
	"testing"
)

// ── Phase 1: BM25 scorer ────────────────────────────────────────────

func TestBM25_EmptyCorpus(t *testing.T) {
	idx := &TextIndex{}
	idx.build(nil) // no states

	results := idx.Search("anything", 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty corpus, got %d", len(results))
	}
}

func TestBM25_SingleDocSingleTerm(t *testing.T) {
	states := map[string]*State{
		"t-1": {ID: "t-1", Title: "fix the authentication bug", Body: "the login page was broken"},
	}
	idx := &TextIndex{}
	idx.build(states)

	results := idx.Search("authentication", 10)
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].ID != "t-1" {
		t.Errorf("expected t-1, got %s", results[0].ID)
	}
	if results[0].Score <= 0 {
		t.Errorf("expected positive score, got %f", results[0].Score)
	}
}

func TestBM25_TermFrequencyBoost(t *testing.T) {
	states := map[string]*State{
		"t-once":  {ID: "t-once", Title: "auth", Body: "one mention of auth"},
		"t-many":  {ID: "t-many", Title: "auth auth auth", Body: "auth is mentioned auth many auth times"},
	}
	idx := &TextIndex{}
	idx.build(states)

	results := idx.Search("auth", 10)
	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Doc with more mentions should score higher
	if results[0].ID != "t-many" {
		t.Errorf("expected t-many first (more TF), got %s", results[0].ID)
	}
	if results[0].Score <= results[1].Score {
		t.Errorf("expected higher score for t-many (%f) vs t-once (%f)",
			results[0].Score, results[1].Score)
	}
}

func TestBM25_IDFRareTermsScoreHigher(t *testing.T) {
	states := map[string]*State{
		"t-1": {ID: "t-1", Title: "common rare", Body: ""},
		"t-2": {ID: "t-2", Title: "common", Body: "common everywhere"},
		"t-3": {ID: "t-3", Title: "common", Body: "common again"},
	}
	idx := &TextIndex{}
	idx.build(states)

	// "rare" appears in 1 doc, "common" in all 3
	// Searching for "rare" on t-1 should score higher than "common" on t-1
	rareResults := idx.Search("rare", 10)
	commonResults := idx.Search("common", 10)

	if len(rareResults) == 0 || len(commonResults) == 0 {
		t.Fatal("expected results for both queries")
	}

	// The doc matching "rare" should have a higher score than any doc matching "common"
	if rareResults[0].Score <= commonResults[0].Score {
		t.Errorf("expected rare term (%f) to score higher than common term (%f)",
			rareResults[0].Score, commonResults[0].Score)
	}
}

func TestBM25_DocLengthNormalization(t *testing.T) {
	shortBody := "bug in auth"
	longBody := "this is a very long document about many things including various topics " +
		"that go on and on with lots of words and content that dilutes the relevance " +
		"of any single term including auth which appears here somewhere in the middle " +
		"of all this other content that makes the document much longer than it needs to be"

	states := map[string]*State{
		"t-short": {ID: "t-short", Title: "short doc", Body: shortBody},
		"t-long":  {ID: "t-long", Title: "long doc", Body: longBody},
	}
	idx := &TextIndex{}
	idx.build(states)

	results := idx.Search("auth", 10)
	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "t-short" {
		t.Errorf("expected short doc first (length normalization), got %s", results[0].ID)
	}
}

func TestBM25_MultiTermQuery(t *testing.T) {
	states := map[string]*State{
		"t-both": {ID: "t-both", Title: "auth login", Body: "authentication and login flow"},
		"t-one":  {ID: "t-one", Title: "auth only", Body: "authentication module"},
	}
	idx := &TextIndex{}
	idx.build(states)

	results := idx.Search("auth login", 10)
	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Doc matching both terms should score higher
	if results[0].ID != "t-both" {
		t.Errorf("expected t-both first (matches both terms), got %s", results[0].ID)
	}
}

func TestBM25_NoMatchReturnsZero(t *testing.T) {
	states := map[string]*State{
		"t-1": {ID: "t-1", Title: "hello world", Body: "greeting program"},
	}
	idx := &TextIndex{}
	idx.build(states)

	results := idx.Search("nonexistent", 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-matching term, got %d", len(results))
	}
}

func TestBM25_TopKLimitsResults(t *testing.T) {
	states := make(map[string]*State)
	for i := 0; i < 20; i++ {
		id := "t-" + string(rune('a'+i))
		states[id] = &State{ID: id, Title: "search term", Body: "matching content"}
	}
	idx := &TextIndex{}
	idx.build(states)

	results := idx.Search("search", 5)
	if len(results) != 5 {
		t.Errorf("expected 5 results (top-K limit), got %d", len(results))
	}
}

// ── Phase 2: TextIndex + field weighting ────────────────────────────

func TestTextIndex_DocCount(t *testing.T) {
	states := map[string]*State{
		"t-1": {ID: "t-1", Title: "one"},
		"t-2": {ID: "t-2", Title: "two"},
		"t-3": {ID: "t-3", Title: "three"},
	}
	idx := &TextIndex{}
	idx.build(states)

	if idx.docCount != 3 {
		t.Errorf("expected 3 docs, got %d", idx.docCount)
	}
}

func TestTextIndex_TokenizesAllFields(t *testing.T) {
	states := map[string]*State{
		"t-1": {
			ID:    "t-1",
			Title: "unique_title_token",
			Body:  "unique_body_token",
			Tags:  []string{"unique_tag_token"},
			Comments: []Note{
				{Body: "unique_comment_token"},
			},
		},
	}
	idx := &TextIndex{}
	idx.build(states)

	// Each unique token should be findable
	for _, query := range []string{"unique_title_token", "unique_body_token", "unique_tag_token", "unique_comment_token"} {
		results := idx.Search(query, 10)
		if len(results) == 0 {
			t.Errorf("expected to find %q in indexed fields", query)
		}
	}
}

func TestTextIndex_TitleWeightHigherThanComment(t *testing.T) {
	states := map[string]*State{
		"t-title": {
			ID:    "t-title",
			Title: "kubernetes deployment",
			Body:  "some unrelated content about cooking recipes",
		},
		"t-comment": {
			ID:    "t-comment",
			Title: "some unrelated title about gardening",
			Body:  "unrelated body",
			Comments: []Note{
				{Body: "kubernetes deployment mentioned in passing"},
			},
		},
	}
	idx := &TextIndex{}
	idx.build(states)

	results := idx.Search("kubernetes deployment", 10)
	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "t-title" {
		t.Errorf("expected title match first, got %s", results[0].ID)
	}
}

func TestTextIndex_SearchWithFindOptions_Kind(t *testing.T) {
	states := map[string]*State{
		"t-1": {ID: "t-1", Kind: "ticket", Title: "auth bug"},
		"d-1": {ID: "d-1", Kind: "doc", Title: "auth documentation"},
	}
	idx := &TextIndex{}
	idx.build(states)

	results := idx.SearchFiltered("auth", SearchOptions{
		FindOptions: FindOptions{Kind: "ticket"},
		Limit:       10,
	})
	if len(results) != 1 {
		t.Fatalf("expected 1 filtered result, got %d", len(results))
	}
	if results[0].ID != "t-1" {
		t.Errorf("expected t-1 (ticket), got %s", results[0].ID)
	}
}

func TestTextIndex_SearchWithFindOptions_Status(t *testing.T) {
	states := map[string]*State{
		"t-open":   {ID: "t-open", Status: "open", Title: "auth bug open"},
		"t-closed": {ID: "t-closed", Status: "closed", Title: "auth bug closed"},
	}
	idx := &TextIndex{}
	idx.build(states)

	results := idx.SearchFiltered("auth", SearchOptions{
		FindOptions: FindOptions{Status: "open"},
		Limit:       10,
	})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "t-open" {
		t.Errorf("expected t-open, got %s", results[0].ID)
	}
}

func TestTextIndex_SearchWithFindOptions_Tag(t *testing.T) {
	states := map[string]*State{
		"t-tagged":   {ID: "t-tagged", Tags: []string{"critical"}, Title: "auth bug"},
		"t-untagged": {ID: "t-untagged", Title: "auth issue"},
	}
	idx := &TextIndex{}
	idx.build(states)

	results := idx.SearchFiltered("auth", SearchOptions{
		FindOptions: FindOptions{Tag: "critical"},
		Limit:       10,
	})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "t-tagged" {
		t.Errorf("expected t-tagged, got %s", results[0].ID)
	}
}

func TestTextIndex_EmptyQuery(t *testing.T) {
	states := map[string]*State{
		"t-1": {ID: "t-1", Title: "something"},
	}
	idx := &TextIndex{}
	idx.build(states)

	results := idx.Search("", 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty query, got %d", len(results))
	}
}

func TestTextIndex_AfterIngestAndRebuild(t *testing.T) {
	// Simulate the lifecycle: build index, then add a note and rebuild
	states := map[string]*State{
		"t-1": {ID: "t-1", Title: "original note"},
	}
	idx := &TextIndex{}
	idx.build(states)

	// Add a new state and rebuild
	states["t-2"] = &State{ID: "t-2", Title: "newly added note about kubernetes"}
	idx.build(states)

	results := idx.Search("kubernetes", 10)
	if len(results) == 0 {
		t.Fatal("expected to find newly added note")
	}
	if results[0].ID != "t-2" {
		t.Errorf("expected t-2, got %s", results[0].ID)
	}
}
