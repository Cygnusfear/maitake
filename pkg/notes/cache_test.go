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
