package notes

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cygnusfear/maitake/pkg/git"
)

func TestCache_WriteAndLoad(t *testing.T) {
	// Use a temp dir as fake repo path for isolation
	repoPath := t.TempDir()
	tipOID := git.OID("abc123def456")

	notes := []*Note{
		{
			ID:        "t-1",
			Kind:      "ticket",
			Title:     "Test ticket",
			Timestamp: "2026-03-15T14:30:00Z",
			TargetOID: "oid-aaa",
			Ref:       "refs/notes/maitake",
			Branch:    "main",
		},
		{
			Kind:      "event",
			Field:     "status",
			Value:     "closed",
			Timestamp: "2026-03-16T10:00:00Z",
			TargetOID: "oid-aaa",
			Ref:       "refs/notes/maitake",
			Branch:    "main",
			Edges: []Edge{
				{Type: "closes", Target: EdgeTarget{Kind: "note", Ref: "t-1"}},
			},
		},
	}

	writeCache(repoPath, tipOID, notes)

	// Load it back
	loaded := loadCache(repoPath, tipOID)
	if loaded == nil {
		t.Fatal("loadCache returned nil — cache miss")
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded %d notes, want 2", len(loaded))
	}

	// Check first note
	n := loaded[0]
	if n.ID != "t-1" {
		t.Errorf("ID = %q", n.ID)
	}
	if n.Title != "Test ticket" {
		t.Errorf("Title = %q", n.Title)
	}
	if n.TargetOID != "oid-aaa" {
		t.Errorf("TargetOID = %q", n.TargetOID)
	}
	if n.Ref != "refs/notes/maitake" {
		t.Errorf("Ref = %q", n.Ref)
	}
	if n.Branch != "main" {
		t.Errorf("Branch = %q", n.Branch)
	}

	// Time should be hydrated from Timestamp
	want := time.Date(2026, 3, 15, 14, 30, 0, 0, time.UTC)
	if !n.Time.Equal(want) {
		t.Errorf("Time = %v, want %v (should be hydrated from cache)", n.Time, want)
	}

	// Check event note
	ev := loaded[1]
	if ev.Field != "status" || ev.Value != "closed" {
		t.Errorf("event: field=%q value=%q", ev.Field, ev.Value)
	}
	if len(ev.Edges) != 1 {
		t.Errorf("event edges: %d", len(ev.Edges))
	}
}

func TestCache_MissOnWrongTip(t *testing.T) {
	repoPath := t.TempDir()
	writeCache(repoPath, git.OID("tip-1"), []*Note{
		{ID: "t-1", Kind: "ticket", TargetOID: "oid", Ref: "ref"},
	})

	// Load with different tip — should miss
	loaded := loadCache(repoPath, git.OID("tip-2"))
	if loaded != nil {
		t.Error("should be cache miss on different tip")
	}
}

func TestCache_MissOnEmpty(t *testing.T) {
	loaded := loadCache("/nonexistent/repo", git.OID("anything"))
	if loaded != nil {
		t.Error("should be nil for nonexistent cache")
	}
}

func TestCache_Prune(t *testing.T) {
	dir := t.TempDir()

	// Create 5 files
	for i := 0; i < 5; i++ {
		name := filepath.Join(dir, "file"+string(rune('a'+i))+".json")
		os.WriteFile(name, []byte("{}"), 0644)
		// Small sleep to ensure different mod times
		time.Sleep(10 * time.Millisecond)
	}

	pruneCache(dir, 2)

	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("after prune: %d files, want 2", len(entries))
	}
}

func TestCache_RepoHash_Stable(t *testing.T) {
	h1 := repoHash("/Users/test/project")
	h2 := repoHash("/Users/test/project")
	if h1 != h2 {
		t.Errorf("hash not stable: %s != %s", h1, h2)
	}

	h3 := repoHash("/Users/test/other")
	if h1 == h3 {
		t.Error("different paths should have different hashes")
	}
}

func TestSummaryCache_WriteAndLoad(t *testing.T) {
	repoPath := t.TempDir()
	tipOID := git.OID("summary123")
	entries := []summaryCacheEntry{
		{
			Summary: StateSummary{
				ID:        "t-1",
				Kind:      "ticket",
				Status:    "open",
				Title:     "Fast path",
				Tags:      []string{"perf", "cache"},
				Targets:   []string{"pkg/notes/cache.go"},
				CreatedAt: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 4, 10, 13, 0, 0, 0, time.UTC),
			},
			TargetOID: "oid-1",
		},
	}
	kinds := []KindCount{{Kind: "ticket", Count: 1}}

	writeSummaryCache(repoPath, tipOID, entries, kinds)
	loaded := loadSummaryCache(repoPath, tipOID)
	if loaded == nil {
		t.Fatal("loadSummaryCache returned nil")
	}
	if len(loaded.Entries) != 1 {
		t.Fatalf("loaded %d entries, want 1", len(loaded.Entries))
	}
	if loaded.Entries[0].Summary.ID != "t-1" {
		t.Fatalf("summary ID = %q, want t-1", loaded.Entries[0].Summary.ID)
	}
	if loaded.Entries[0].TargetOID != "oid-1" {
		t.Fatalf("targetOID = %q, want oid-1", loaded.Entries[0].TargetOID)
	}
	if len(loaded.KindCounts) != 1 || loaded.KindCounts[0].Count != 1 {
		t.Fatalf("kind counts = %#v", loaded.KindCounts)
	}
	if !loaded.Entries[0].Summary.UpdatedAt.Equal(time.Date(2026, 4, 10, 13, 0, 0, 0, time.UTC)) {
		t.Fatalf("updatedAt not preserved: %v", loaded.Entries[0].Summary.UpdatedAt)
	}
}

func TestResolveSummaryEntry(t *testing.T) {
	entries := []summaryCacheEntry{
		{Summary: StateSummary{ID: "cf-n9r4"}, TargetOID: "oid-1"},
		{Summary: StateSummary{ID: "cf-1234"}, TargetOID: "oid-2"},
	}

	entry, fullID, err := resolveSummaryEntry(entries, "cf-n9r4")
	if err != nil {
		t.Fatalf("exact resolve error: %v", err)
	}
	if entry == nil || fullID != "cf-n9r4" || entry.TargetOID != "oid-1" {
		t.Fatalf("exact resolve mismatch: %#v %q", entry, fullID)
	}

	entry, fullID, err = resolveSummaryEntry(entries, "1234")
	if err != nil {
		t.Fatalf("partial resolve error: %v", err)
	}
	if entry == nil || fullID != "cf-1234" || entry.TargetOID != "oid-2" {
		t.Fatalf("partial resolve mismatch: %#v %q", entry, fullID)
	}

	entry, fullID, err = resolveSummaryEntry(entries, "cf-")
	if err == nil {
		t.Fatal("expected ambiguous resolve error")
	}
	if entry != nil || fullID != "" {
		t.Fatalf("ambiguous resolve mismatch: %#v %q", entry, fullID)
	}
}

func TestQuerySummaries_FiltersSortsAndLimits(t *testing.T) {
	summaries := []StateSummary{
		{ID: "t-1", Kind: "ticket", Status: "open", Priority: 2, Tags: []string{"perf"}, Targets: []string{"a.go"}, CreatedAt: time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)},
		{ID: "t-2", Kind: "ticket", Status: "open", Priority: 1, Tags: []string{"perf", "cache"}, Targets: []string{"b.go"}, CreatedAt: time.Date(2026, 4, 10, 11, 0, 0, 0, time.UTC)},
		{ID: "d-1", Kind: "doc", Status: "closed", Priority: 3, Tags: []string{"docs"}, Targets: []string{"README.md"}, CreatedAt: time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC)},
	}

	results := querySummaries(summaries, ListOptions{
		FindOptions: FindOptions{Kind: "ticket", Tag: "perf"},
		SortBy:      "priority",
		Limit:       1,
	})

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].ID != "t-2" {
		t.Fatalf("got %q first, want t-2 (lowest priority number)", results[0].ID)
	}

	results = querySummaries(summaries, ListOptions{FindOptions: FindOptions{Target: "README.md"}})
	if len(results) != 1 || results[0].ID != "d-1" {
		t.Fatalf("target filter mismatch: %#v", results)
	}
}


func TestSummaryEntriesFromIndex(t *testing.T) {
	idx := NewIndex()
	idx.Ingest(&Note{
		ID: "t-fast", Kind: "ticket", Title: "fast",
		Timestamp: "2026-04-15T10:00:00Z",
		TargetOID: "oid-fast", Ref: "refs/notes/maitake",
	})
	idx.Build()

	entries := summaryEntriesFromIndex(idx)
	if len(entries) != 1 {
		t.Fatalf("entries=%d, want 1", len(entries))
	}
	if entries[0].Summary.ID != "t-fast" {
		t.Errorf("id=%q", entries[0].Summary.ID)
	}
	if entries[0].TargetOID != "oid-fast" {
		t.Errorf("targetOID=%q", entries[0].TargetOID)
	}
}

func TestSummaryCacheExists(t *testing.T) {
	repoPath := t.TempDir()
	tipOID := git.OID("existtip")

	if summaryCacheExists(repoPath, tipOID) {
		t.Fatal("should not exist yet")
	}

	writeSummaryCache(repoPath, tipOID,
		[]summaryCacheEntry{{Summary: StateSummary{ID: "t-1"}, TargetOID: "oid"}},
		nil,
	)
	if !summaryCacheExists(repoPath, tipOID) {
		t.Fatal("should exist after write")
	}
}

func TestPruneStaleRepoCaches_DeletesOld(t *testing.T) {
	root := t.TempDir()

	old := filepath.Join(root, "0000000000000001")
	fresh := filepath.Join(root, "ffffffffffffffff")
	notHash := filepath.Join(root, "definitely-not-a-hash")

	for _, d := range []string{old, fresh, notHash} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(d, "x.json"), []byte("{}"), 0644)
	}

	// Backdate "old" mtime by 60 days.
	past := time.Now().Add(-60 * 24 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}

	pruneStaleRepoCaches(root, 30*24*time.Hour, 0, time.Now())

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("old dir should be gone: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("fresh dir removed unexpectedly: %v", err)
	}
	if _, err := os.Stat(notHash); err != nil {
		t.Errorf("non-hash dir removed unexpectedly: %v", err)
	}
}

func TestPruneStaleRepoCaches_RespectsCooldown(t *testing.T) {
	root := t.TempDir()

	old := filepath.Join(root, "0000000000000002")
	if err := os.MkdirAll(old, 0755); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-60 * 24 * time.Hour)
	os.Chtimes(old, past, past)

	// Touch the .last-gc marker as if a recent run happened.
	marker := filepath.Join(root, ".last-gc")
	os.WriteFile(marker, []byte("recent"), 0644)

	pruneStaleRepoCaches(root, 30*24*time.Hour, 24*time.Hour, time.Now())

	if _, err := os.Stat(old); err != nil {
		t.Errorf("stale dir should NOT be pruned during cooldown: %v", err)
	}
}

func TestIsRepoHashDir(t *testing.T) {
	ok := []string{"0123456789abcdef", "ffffffffffffffff", "0000000000000000"}
	bad := []string{"", "short", "0123456789ABCDEF", "0123456789abcdeg", "0123456789abcdef0"}
	for _, n := range ok {
		if !isRepoHashDir(n) {
			t.Errorf("%q should be a hash dir", n)
		}
	}
	for _, n := range bad {
		if isRepoHashDir(n) {
			t.Errorf("%q should NOT be a hash dir", n)
		}
	}
}
