package notes

import (
	"testing"
)

func buildTestIndex() *Index {
	idx := NewIndex()

	// Ticket
	idx.Ingest(&Note{
		ID:   "tre-5c4a",
		Kind: "ticket",
		Type: "task",
		Tags: []string{"auth", "backend"},
		Edges: []Edge{
			{Type: "targets", Target: EdgeTarget{Kind: "path", Ref: "src/auth.ts"}},
			{Type: "depends-on", Target: EdgeTarget{Kind: "note", Ref: "wrn-a4f2"}},
		},
		Body:      "# Fix auth race condition",
		Time: ts("2026-03-01T10:00:00Z"),
	})
	// Start event
	idx.Ingest(&Note{
		Kind:      "event",
		Edges:     []Edge{{Type: "starts", Target: EdgeTarget{Kind: "note", Ref: "tre-5c4a"}}},
		Time: ts("2026-03-01T11:00:00Z"),
	})
	// Comment
	idx.Ingest(&Note{
		Kind:      "comment",
		Body:      "Found root cause",
		Edges:     []Edge{{Type: "on", Target: EdgeTarget{Kind: "note", Ref: "tre-5c4a"}}},
		Time: ts("2026-03-01T12:00:00Z"),
	})

	// Warning
	idx.Ingest(&Note{
		ID:   "wrn-a4f2",
		Kind: "warning",
		Edges: []Edge{
			{Type: "targets", Target: EdgeTarget{Kind: "path", Ref: "src/auth.ts"}},
		},
		Body:      "Race condition in token refresh",
		Time: ts("2026-03-01T09:00:00Z"),
	})

	// Closed review artifact
	idx.Ingest(&Note{
		ID:     "rev-b3d1",
		Kind:   "review",
		Status: "closed",
		Tags:   []string{"review"},
		Edges: []Edge{
			{Type: "targets", Target: EdgeTarget{Kind: "path", Ref: "src/auth.ts"}},
		},
		Body:      "Add mutex to refresh",
		Time: ts("2026-03-02T10:00:00Z"),
	})

	// Unrelated ticket
	idx.Ingest(&Note{
		ID:        "tre-9b2f",
		Kind:      "ticket",
		Type:      "bug",
		Tags:      []string{"http"},
		Body:      "# Retry logic missing",
		Time: ts("2026-03-03T10:00:00Z"),
	})

	idx.Build()
	return idx
}

func TestIndex_Build(t *testing.T) {
	idx := buildTestIndex()

	if len(idx.States) != 4 {
		t.Fatalf("States = %d, want 4", len(idx.States))
	}

	state := idx.States["tre-5c4a"]
	if state.Status != "in_progress" {
		t.Errorf("tre-5c4a status = %q, want in_progress", state.Status)
	}
	if len(state.Comments) != 1 {
		t.Errorf("tre-5c4a comments = %d, want 1", len(state.Comments))
	}
}

func TestIndex_FindByKind(t *testing.T) {
	idx := buildTestIndex()

	tickets := idx.FindByKind("ticket")
	if len(tickets) != 2 {
		t.Errorf("tickets = %d, want 2", len(tickets))
	}

	warnings := idx.FindByKind("warning")
	if len(warnings) != 1 {
		t.Errorf("warnings = %d, want 1", len(warnings))
	}
}

func TestIndex_FindByTarget(t *testing.T) {
	idx := buildTestIndex()

	authNotes := idx.FindByTarget("src/auth.ts")
	if len(authNotes) != 3 {
		t.Errorf("notes on src/auth.ts = %d, want 3 (ticket + warning + review)", len(authNotes))
	}
}

func TestIndex_FindByStatus(t *testing.T) {
	idx := buildTestIndex()

	open := idx.FindByStatus("open")
	inProgress := idx.FindByStatus("in_progress")
	closed := idx.FindByStatus("closed")

	if len(inProgress) != 1 {
		t.Errorf("in_progress = %d, want 1", len(inProgress))
	}
	if len(closed) != 1 {
		t.Errorf("closed = %d, want 1", len(closed))
	}
	// wrn-a4f2 and tre-9b2f are open
	if len(open) != 2 {
		t.Errorf("open = %d, want 2", len(open))
	}
}

func TestIndex_Query(t *testing.T) {
	idx := buildTestIndex()

	// Kind + Status
	results := idx.Query(FindOptions{Kind: "ticket", Status: "in_progress"})
	if len(results) != 1 || results[0].ID != "tre-5c4a" {
		t.Errorf("query ticket+in_progress = %v", results)
	}

	// Tag
	results = idx.Query(FindOptions{Tag: "auth"})
	if len(results) != 1 {
		t.Errorf("query tag=auth = %d, want 1", len(results))
	}

	// Target
	results = idx.Query(FindOptions{Target: "src/auth.ts"})
	if len(results) != 3 {
		t.Errorf("query target=src/auth.ts = %d, want 3", len(results))
	}

	// Type
	results = idx.Query(FindOptions{Type: "bug"})
	if len(results) != 1 || results[0].ID != "tre-9b2f" {
		t.Errorf("query type=bug = %v", results)
	}
}

func TestIndex_ContextForPath(t *testing.T) {
	idx := buildTestIndex()

	// Context = open notes only
	ctx := idx.ContextForPath("src/auth.ts")
	if len(ctx) != 2 {
		t.Errorf("context src/auth.ts = %d, want 2 (ticket + warning, not closed review)", len(ctx))
	}

	// ContextAll = includes closed
	all := idx.ContextAllForPath("src/auth.ts")
	if len(all) != 3 {
		t.Errorf("contextAll src/auth.ts = %d, want 3", len(all))
	}
}

func TestIndex_ResolveID(t *testing.T) {
	idx := buildTestIndex()

	// Exact
	id, err := idx.ResolveID("tre-5c4a")
	if err != nil || id != "tre-5c4a" {
		t.Errorf("exact resolve: %q, %v", id, err)
	}

	// Partial
	id, err = idx.ResolveID("5c4a")
	if err != nil || id != "tre-5c4a" {
		t.Errorf("partial resolve: %q, %v", id, err)
	}

	// Not found
	id, err = idx.ResolveID("nonexistent")
	if err != nil || id != "" {
		t.Errorf("not found: %q, %v", id, err)
	}
}

func TestIndex_KindCounts(t *testing.T) {
	idx := buildTestIndex()
	counts := idx.KindCounts()

	found := map[string]int{}
	for _, c := range counts {
		found[c.Kind] = c.Count
	}
	if found["ticket"] != 2 {
		t.Errorf("ticket count = %d, want 2", found["ticket"])
	}
	if found["warning"] != 1 {
		t.Errorf("warning count = %d, want 1", found["warning"])
	}
}

func TestIndex_QueryList_Sorted(t *testing.T) {
	idx := buildTestIndex()

	summaries := idx.QueryList(ListOptions{
		FindOptions: FindOptions{Kind: "ticket"},
		SortBy:      "created",
	})
	if len(summaries) != 2 {
		t.Fatalf("got %d summaries, want 2", len(summaries))
	}
	// Newer first
	if summaries[0].ID != "tre-9b2f" {
		t.Errorf("first should be tre-9b2f (newer), got %s", summaries[0].ID)
	}
}
