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


func TestTextIndexCache_WriteAndLoad(t *testing.T) {
	repoPath := t.TempDir()
	tipOID := git.OID("textidxtip")

	// Build a small TextIndex manually
	ti := &TextIndex{
		docIDs:   []string{"t-1", "t-2"},
		docLens:  []float64{6, 4},
		docTerms: []map[string]float64{{"auth": 3, "bug": 1}, {"auth": 2}},
		df:       map[string]int{"auth": 2, "bug": 1},
		avgDL:    5,
		docCount: 2,
	}
	writeTextIndexCache(repoPath, tipOID, ti)

	env := loadTextIndexCache(repoPath, tipOID)
	if env == nil {
		t.Fatal("loadTextIndexCache returned nil")
	}
	if env.DocCount != 2 {
		t.Errorf("DocCount = %d, want 2", env.DocCount)
	}
	if env.AvgDL != 5 {
		t.Errorf("AvgDL = %v, want 5", env.AvgDL)
	}
	if env.DF["auth"] != 2 {
		t.Errorf("DF[auth] = %d, want 2", env.DF["auth"])
	}
	if len(env.Docs) != 2 {
		t.Fatalf("docs = %d, want 2", len(env.Docs))
	}
}

func TestTextIndexCache_MissOnWrongTip(t *testing.T) {
	repoPath := t.TempDir()
	ti := &TextIndex{docIDs: []string{"t"}, docLens: []float64{1}, docTerms: []map[string]float64{{"x": 1}}, df: map[string]int{"x": 1}, avgDL: 1, docCount: 1}
	writeTextIndexCache(repoPath, git.OID("tip-a"), ti)
	if env := loadTextIndexCache(repoPath, git.OID("tip-b")); env != nil {
		t.Error("should miss on wrong tip")
	}
}

func TestTextIndexCache_SkipsEmpty(t *testing.T) {
	repoPath := t.TempDir()
	writeTextIndexCache(repoPath, git.OID("nothing"), &TextIndex{docCount: 0})
	if textIndexCacheExists(repoPath, git.OID("nothing")) {
		t.Error("should not write cache for empty index")
	}
}

func TestTextIndex_HydrateFromCache(t *testing.T) {
	env := &textIndexEnvelope{
		DocCount: 2,
		AvgDL:    5,
		DF:       map[string]int{"alpha": 1, "beta": 2},
		Docs: []persistedDoc{
			{ID: "t-1", DocLen: 6, Terms: map[string]float64{"alpha": 3, "beta": 1}},
			{ID: "t-2", DocLen: 4, Terms: map[string]float64{"beta": 1}},
		},
	}
	states := map[string]*State{
		"t-1": {ID: "t-1", Title: "one"},
		"t-2": {ID: "t-2", Title: "two"},
	}

	ti := &TextIndex{}
	ti.hydrateFromCache(env, func(id string) *State { return states[id] })

	if ti.docCount != 2 {
		t.Errorf("docCount = %d", ti.docCount)
	}
	if ti.avgDL != 5 {
		t.Errorf("avgDL = %v", ti.avgDL)
	}
	if ti.df["beta"] != 2 {
		t.Errorf("df[beta] = %d", ti.df["beta"])
	}
	// Query "alpha" should only match t-1
	results := ti.Search("alpha", 10)
	if len(results) != 1 || results[0].ID != "t-1" {
		t.Errorf("search(alpha) = %v", results)
	}
}

func TestTextIndex_HydrateSkipsMissingStates(t *testing.T) {
	env := &textIndexEnvelope{
		DocCount: 2,
		AvgDL:    5,
		DF:       map[string]int{"alpha": 2},
		Docs: []persistedDoc{
			{ID: "have", DocLen: 3, Terms: map[string]float64{"alpha": 3}},
			{ID: "gone", DocLen: 3, Terms: map[string]float64{"alpha": 3}},
		},
	}
	states := map[string]*State{"have": {ID: "have", Title: "survivor"}}

	ti := &TextIndex{}
	ti.hydrateFromCache(env, func(id string) *State { return states[id] })

	if ti.docCount != 1 {
		t.Errorf("docCount = %d, want 1 (missing state should be dropped)", ti.docCount)
	}
	if len(ti.states) != 1 || ti.states[0].ID != "have" {
		t.Errorf("states = %v", ti.states)
	}
}

func TestStateFromSummary_CopiesFilterFields(t *testing.T) {
	resolved := true
	s := StateSummary{
		ID: "t-1", Kind: "ticket", Status: "open", Type: "task",
		Priority: 2, Title: "find me", Tags: []string{"perf"},
		Targets: []string{"pkg/notes/search.go"}, Deps: []string{"t-0"},
		Assignee: "alex", Resolved: &resolved, Branch: "main",
	}
	st := stateFromSummary(s)
	if st.ID != "t-1" || st.Title != "find me" || st.Status != "open" {
		t.Errorf("scalar fields not copied: %+v", st)
	}
	if st.Resolved == nil || *st.Resolved != true {
		t.Error("resolved pointer lost")
	}
	if len(st.Tags) != 1 || st.Tags[0] != "perf" {
		t.Errorf("tags = %v", st.Tags)
	}
}

func TestPruneCache_KeepsCompanionFilesOfRecentTips(t *testing.T) {
	dir := t.TempDir()
	// Three tips, each with a .json + .summary.json + .textindex.json triple.
	// Oldest tip gets all three files dropped; newer two keep everything.
	tips := []string{"tipA", "tipB", "tipC"}
	for _, tip := range tips {
		for _, suffix := range []string{".json", ".summary.json", ".textindex.gob"} {
			name := filepath.Join(dir, tip+suffix)
			if err := os.WriteFile(name, []byte("{}"), 0644); err != nil {
				t.Fatal(err)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}

	pruneCache(dir, 2)

	remaining, _ := os.ReadDir(dir)
	names := map[string]bool{}
	for _, e := range remaining {
		names[e.Name()] = true
	}
	if len(names) != 6 {
		t.Fatalf("remaining files = %d, want 6 (two tips × three suffixes): %v", len(names), names)
	}
	// oldest tip gone
	for _, suffix := range []string{".json", ".summary.json", ".textindex.gob"} {
		if names["tipA"+suffix] {
			t.Errorf("tipA%s should be pruned", suffix)
		}
	}
	// newest two present
	for _, tip := range []string{"tipB", "tipC"} {
		for _, suffix := range []string{".json", ".summary.json", ".textindex.gob"} {
			if !names[tip+suffix] {
				t.Errorf("%s%s should be kept", tip, suffix)
			}
		}
	}
}

func TestTipFromCacheFileName(t *testing.T) {
	cases := map[string]string{
		"abc123.json":           "abc123",
		"abc123.summary.json":   "abc123",
		"abc123.textindex.gob":  "abc123",
		"weird":                 "",
		"abc123.unknown":        "",
	}
	for in, want := range cases {
		if got := tipFromCacheFileName(in); got != want {
			t.Errorf("tipFromCacheFileName(%q) = %q, want %q", in, got, want)
		}
	}
}
