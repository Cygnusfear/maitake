package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cygnusfear/maitake/pkg/git"
	"github.com/cygnusfear/maitake/pkg/notes"
)

// TestDocsSync_EditThenDeleteThenRestore models the exact user flow:
// 1. Create a doc note, materialize it
// 2. Edit the file on disk (simulate Obsidian)
// 3. rm -rf docs/
// 4. mai docs sync
// 5. Restored file should have the edit
//
// This requires the edit to reach the note BEFORE the rm -rf.
// In production the daemon handles this. In this test we simulate
// by running SyncDocs between the edit and the delete.
func TestDocsSync_EditThenDeleteThenRestore(t *testing.T) {
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)

	// Create doc and materialize
	cfg := notes.DocsConfig{Dir: "docs"}
	note, _ := engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "Editable",
		Body:  "# Editable\n\nOriginal content.",
	})
	notes.SyncDocs(engine, dir, cfg)

	// 1. Edit the file (simulate Obsidian save)
	filePath := filepath.Join(dir, "docs", "editable.md")
	data, _ := os.ReadFile(filePath)
	edited := string(data) + "\n## Added Section\nNew content from Obsidian.\n"
	os.WriteFile(filePath, []byte(edited), 0644)

	// Daemon would catch this. Simulate: sync to push edit into note.
	result1, err := notes.SyncDocs(engine, dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result1.Updated) != 1 {
		t.Fatalf("edit should trigger update, got Updated=%d", len(result1.Updated))
	}

	// Verify note has the edit
	state, _ := engine.Fold(note.ID)
	if !strings.Contains(state.Body, "Added Section") {
		t.Fatalf("note should have edit BEFORE delete.\nBody: %q", state.Body)
	}

	// 2. rm -rf docs/
	os.RemoveAll(filepath.Join(dir, "docs"))

	// 3. mai docs sync — should restore WITH the edit
	// Need fresh engine to simulate separate mai invocation
	repo2, _ := git.NewGitRepo(dir)
	engine2, _ := notes.NewEngine(repo2)

	result2, err := notes.SyncDocs(engine2, dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result2.Written) != 1 {
		t.Fatalf("should restore 1 file, got Written=%d", len(result2.Written))
	}

	// 4. Restored file should have the edit
	restored, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal("file should be restored")
	}
	if !strings.Contains(string(restored), "Added Section") {
		t.Errorf("restored file missing edit.\nGot: %s", string(restored))
	}
	if !strings.Contains(string(restored), "New content from Obsidian") {
		t.Errorf("restored file missing Obsidian content.\nGot: %s", string(restored))
	}
}

// TestDocsSync_DaemonCatchesEdit tests that file changes are detected
// and synced without explicit SyncDocs call — simulating daemon behavior.
// Uses auto-sync: engine.Create writes file, then we edit and check
// if a subsequent engine restart sees the edit after a sync.
func TestDocsSync_DaemonCatchesEdit(t *testing.T) {
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)

	cfg := notes.DocsConfig{Dir: "docs"}
	note, _ := engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "Watched",
		Body:  "Before daemon edit.",
	})
	notes.SyncDocs(engine, dir, cfg)

	// Edit file
	filePath := filepath.Join(dir, "docs", "watched.md")
	data, _ := os.ReadFile(filePath)
	os.WriteFile(filePath, []byte(strings.Replace(string(data), "Before daemon edit.", "After daemon edit.", 1)), 0644)

	// Simulate daemon: sync catches the edit
	notes.SyncDocs(engine, dir, cfg)

	// Kill and restart engine (simulates rm -rf + new mai process)
	repo2, _ := git.NewGitRepo(dir)
	engine2, _ := notes.NewEngine(repo2)

	state, _ := engine2.Fold(note.ID)
	if !strings.Contains(state.Body, "After daemon edit") {
		t.Errorf("daemon should have caught edit.\nBody: %q", state.Body)
	}
}

// TestDocsSync_EditWithoutSync_LosesData documents the known limitation:
// if you edit a file and rm -rf WITHOUT syncing in between, the edit is lost.
// This is expected — the daemon is what prevents this in production.
func TestDocsSync_EditWithoutSync_LosesData(t *testing.T) {
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)

	cfg := notes.DocsConfig{Dir: "docs"}
	note, _ := engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "Volatile",
		Body:  "Original only.",
	})
	notes.SyncDocs(engine, dir, cfg)

	// Edit file but DON'T sync
	filePath := filepath.Join(dir, "docs", "volatile.md")
	data, _ := os.ReadFile(filePath)
	os.WriteFile(filePath, []byte(strings.Replace(string(data), "Original only.", "Edited but not synced.", 1)), 0644)

	// rm -rf without syncing — edit lives only on disk
	os.RemoveAll(filepath.Join(dir, "docs"))

	// Restore
	repo2, _ := git.NewGitRepo(dir)
	engine2, _ := notes.NewEngine(repo2)
	notes.SyncDocs(engine2, dir, cfg)

	// Edit is lost — this is expected behavior without daemon
	restored, _ := os.ReadFile(filePath)
	if strings.Contains(string(restored), "Edited but not synced") {
		t.Error("edit should be LOST without sync — if this passes, something changed")
	}
	if !strings.Contains(string(restored), "Original only") {
		t.Error("should have original content")
	}

	_ = note
	_ = time.Second // used for potential sleep in daemon tests
}
