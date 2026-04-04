package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cygnusfear/maitake/pkg/git"
	"github.com/cygnusfear/maitake/pkg/docs"
	"github.com/cygnusfear/maitake/pkg/notes"
)

func TestDocsSync_NoteToFile(t *testing.T) {
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)

	// Create a doc note (auto-sync may write file immediately)
	note, err := engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "Architecture",
		Body:  "# Architecture\n\nMicroservices.",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Sync to ensure file exists (may be no-op if auto-sync already wrote it)
	docs.SyncDocs(engine, dir, docs.Config{Dir: "docs"})

	// File should exist with frontmatter
	files, _ := filepath.Glob(filepath.Join(dir, "docs", "*.md"))
	if len(files) == 0 {
		t.Fatal("no doc files found")
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "mai-id: "+note.ID) {
		t.Errorf("missing mai-id frontmatter: %s", content)
	}
	if !strings.Contains(content, "# Architecture") {
		t.Errorf("missing body: %s", content)
	}
}

func TestDocsSync_FileToNote(t *testing.T) {
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)

	// Create a markdown file without frontmatter
	docsDir := filepath.Join(dir, "docs")
	os.MkdirAll(docsDir, 0755)
	os.WriteFile(filepath.Join(docsDir, "guide.md"), []byte("# Guide\n\nHow to use."), 0644)

	// Sync — should import
	result, err := docs.SyncDocs(engine, dir, docs.Config{Dir: "docs"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Imported) != 1 {
		t.Fatalf("Imported = %d, want 1", len(result.Imported))
	}

	// File should now have frontmatter
	data, _ := os.ReadFile(filepath.Join(docsDir, "guide.md"))
	if !strings.Contains(string(data), "mai-id:") {
		t.Error("imported file should have mai-id frontmatter")
	}

	// Note should exist
	summaries, _ := engine.List(notes.ListOptions{FindOptions: notes.FindOptions{Kind: "doc"}})
	if len(summaries) != 1 {
		t.Fatalf("doc notes = %d, want 1", len(summaries))
	}
	if summaries[0].Title != "Guide" {
		t.Errorf("title = %q, want Guide", summaries[0].Title)
	}
}

func TestDocsSync_FileEditUpdatesNote(t *testing.T) {
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)

	// Create doc note and materialize
	note, _ := engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "Test",
		Body:  "Original content.",
	})
	docs.SyncDocs(engine, dir, docs.Config{Dir: "docs"})

	// Edit the file on disk
	filePath := filepath.Join(dir, "docs", "test.md")
	data, _ := os.ReadFile(filePath)
	newContent := strings.Replace(string(data), "Original content.", "Updated content from Obsidian.", 1)
	os.WriteFile(filePath, []byte(newContent), 0644)

	// Sync — should update the note
	result, err := docs.SyncDocs(engine, dir, docs.Config{Dir: "docs"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Updated) != 1 {
		t.Fatalf("Updated = %d, want 1", len(result.Updated))
	}

	// THE CRITICAL TEST: note body should reflect the file edit
	state, err := engine.Fold(note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(state.Body, "Updated content from Obsidian") {
		t.Errorf("note body not updated from file edit.\nGot: %q", state.Body)
	}
	if strings.Contains(state.Body, "Original content") {
		t.Errorf("note body still has original content.\nGot: %q", state.Body)
	}
}

func TestDocsSync_FileEditSurvivesRestart(t *testing.T) {
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)

	// Create doc note and materialize
	note, _ := engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "Persist",
		Body:  "Before edit.",
	})
	docs.SyncDocs(engine, dir, docs.Config{Dir: "docs"})

	// Edit file
	filePath := filepath.Join(dir, "docs", "persist.md")
	data, _ := os.ReadFile(filePath)
	os.WriteFile(filePath, []byte(strings.Replace(string(data), "Before edit.", "After edit.", 1)), 0644)

	// Sync
	docs.SyncDocs(engine, dir, docs.Config{Dir: "docs"})

	// NEW ENGINE — simulates restarting mai
	repo2, _ := git.NewGitRepo(dir)
	engine2, _ := notes.NewEngine(repo2)

	state, err := engine2.Fold(note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(state.Body, "After edit") {
		t.Errorf("body not persisted across restart.\nGot: %q", state.Body)
	}
}

// TestDocsSync_CloseMarksFrontmatter verifies that closing a doc note keeps
// the file on disk and adds closed: true to frontmatter (not os.Remove).
func TestDocsSync_CloseMarksFrontmatter(t *testing.T) {
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)

	note, _ := engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "Temporary",
		Body:  "Will be marked closed.",
	})
	docs.SyncDocs(engine, dir, docs.Config{Dir: "docs"})

	filePath := filepath.Join(dir, "docs", "temporary.md")
	if _, err := os.Stat(filePath); err != nil {
		t.Fatal("file should exist after sync")
	}

	// Close the note.
	engine.Append(notes.AppendOptions{
		TargetID: note.ID,
		Kind:     "event",
		Field:    "status",
		Value:    "closed",
	})

	// Sync to ensure closed marking.
	docs.SyncDocs(engine, dir, docs.Config{Dir: "docs"})

	// File must still exist — we mark it, not delete it.
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal("file should still exist after closing doc note (not deleted)")
	}
	content := string(data)
	if !strings.Contains(content, "closed: true") {
		t.Errorf("closed frontmatter field missing.\nGot:\n%s", content)
	}
	if !strings.Contains(content, "mai-id:") {
		t.Error("mai-id frontmatter should still be present")
	}
}

func TestDocsSync_AlreadyInSync(t *testing.T) {
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)

	engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "Stable",
		Body:  "No changes.",
	})
	docs.SyncDocs(engine, dir, docs.Config{Dir: "docs"})

	// Second sync — nothing should change
	result, _ := docs.SyncDocs(engine, dir, docs.Config{Dir: "docs"})
	total := len(result.Written) + len(result.Imported) + len(result.Updated) + len(result.Removed)
	if total != 0 {
		t.Errorf("second sync should be no-op, got %d changes", total)
	}
}

func TestDocsSync_DeleteAndRestore(t *testing.T) {
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)

	engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "Resilient",
		Body:  "Survives rm -rf.",
	})
	docs.SyncDocs(engine, dir, docs.Config{Dir: "docs"})

	// rm -rf docs/
	os.RemoveAll(filepath.Join(dir, "docs"))

	// Sync — should restore
	result, _ := docs.SyncDocs(engine, dir, docs.Config{Dir: "docs"})
	if len(result.Written) != 1 {
		t.Fatalf("Written = %d, want 1 (restored)", len(result.Written))
	}

	// File should be back
	data, err := os.ReadFile(filepath.Join(dir, "docs", "resilient.md"))
	if err != nil {
		t.Fatal("file should be restored")
	}
	if !strings.Contains(string(data), "Survives rm -rf") {
		t.Error("restored file should have original content")
	}
}

// TestDocsSync_NoteEditUpdatesFile verifies that a body edit via Append()
// (e.g. mai edit <doc-id>) materializes to disk via auto-sync.
// This was the root bug: autoSyncDoc compared file vs NEW body (always differed
// right after an edit) instead of file vs PREVIOUS body.
func TestDocsSync_NoteEditUpdatesFile(t *testing.T) {
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)

	cfgFile := filepath.Join(dir, ".maitake", "config.yaml")
	os.MkdirAll(filepath.Dir(cfgFile), 0755)
	os.WriteFile(cfgFile, []byte("docs:\n  dir: docs\n  sync: auto\n"), 0644)

	engine, _ := notes.NewEngine(repo)

	// Create doc note (auto-sync writes the file).
	note, err := engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "Autosync",
		Body:  "Original body.",
	})
	if err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join(dir, "docs", "autosync.md")
	if _, err := os.Stat(filePath); err != nil {
		t.Skip("auto-sync not active (no docs.sync=auto config) — skipping")
	}

	// Edit the note body via Append (simulates mai edit).
	_, err = engine.Append(notes.AppendOptions{
		TargetID: note.ID,
		Kind:     "event",
		Field:    "body",
		Body:     "Updated body from mai edit.",
	})
	if err != nil {
		t.Fatal(err)
	}

	// File must reflect the note-side edit — no full docs sync needed.
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal("file should still exist after note edit")
	}
	body := string(data)
	if !strings.Contains(body, "Updated body from mai edit") {
		t.Errorf("note-side body edit should update file via auto-sync.\nFile:\n%s", body)
	}
	if strings.Contains(body, "Original body") {
		t.Errorf("old body should be gone from file.\nFile:\n%s", body)
	}
}
