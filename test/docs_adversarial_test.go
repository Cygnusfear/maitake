package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cygnusfear/maitake/pkg/git"
	"github.com/cygnusfear/maitake/pkg/notes"
)

func docEngine(t *testing.T) (string, notes.Engine) {
	t.Helper()
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)
	return dir, engine
}

var docsCfg = notes.DocsConfig{Dir: "docs"}

// ── Frontmatter corruption ───────────────────────────────────────────────

func TestDocs_CorruptedFrontmatter_StillImports(t *testing.T) {
	dir, engine := docEngine(t)
	docsDir := filepath.Join(dir, "docs")
	os.MkdirAll(docsDir, 0755)

	// File with broken frontmatter (unclosed ---)
	os.WriteFile(filepath.Join(docsDir, "broken.md"), []byte("---\nmai-id: fake\ntitle: oops\n# No closing frontmatter\n\nContent here.\n"), 0644)

	result, err := notes.SyncDocs(engine, dir, docsCfg)
	if err != nil {
		t.Fatal(err)
	}
	// Should treat as a new file (broken frontmatter = no mai-id parsed)
	if len(result.Imported) != 1 {
		t.Errorf("broken frontmatter should import as new, got Imported=%d", len(result.Imported))
	}
}

func TestDocs_FrontmatterPointsToNonexistentNote(t *testing.T) {
	dir, engine := docEngine(t)
	docsDir := filepath.Join(dir, "docs")
	os.MkdirAll(docsDir, 0755)

	// File with valid frontmatter but note doesn't exist
	os.WriteFile(filepath.Join(docsDir, "ghost.md"), []byte("---\nmai-id: nonexistent-note\n---\n# Ghost\n\nThis note doesn't exist.\n"), 0644)

	result, _ := notes.SyncDocs(engine, dir, docsCfg)
	// Should not crash. The file has a mai-id that points nowhere.
	// It should NOT be imported as new (it has a mai-id).
	// It should NOT be written (no matching note).
	if len(result.Imported) != 0 {
		t.Errorf("ghost frontmatter should not be imported, got %d", len(result.Imported))
	}
}

// ── Empty and whitespace files ───────────────────────────────────────────

func TestDocs_EmptyFile_Imports(t *testing.T) {
	dir, engine := docEngine(t)
	docsDir := filepath.Join(dir, "docs")
	os.MkdirAll(docsDir, 0755)

	os.WriteFile(filepath.Join(docsDir, "empty.md"), []byte(""), 0644)

	result, err := notes.SyncDocs(engine, dir, docsCfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Imported) != 1 {
		t.Errorf("empty file should import, got Imported=%d", len(result.Imported))
	}
}

func TestDocs_WhitespaceOnlyFile(t *testing.T) {
	dir, engine := docEngine(t)
	docsDir := filepath.Join(dir, "docs")
	os.MkdirAll(docsDir, 0755)

	os.WriteFile(filepath.Join(docsDir, "spaces.md"), []byte("   \n\n   \n"), 0644)

	result, _ := notes.SyncDocs(engine, dir, docsCfg)
	if len(result.Imported) != 1 {
		t.Errorf("whitespace file should import, got Imported=%d", len(result.Imported))
	}
}

// ── Special characters in filenames and content ──────────────────────────

func TestDocs_SpecialCharsInFilename(t *testing.T) {
	dir, engine := docEngine(t)
	docsDir := filepath.Join(dir, "docs")
	os.MkdirAll(docsDir, 0755)

	os.WriteFile(filepath.Join(docsDir, "my notes & stuff (2026).md"), []byte("# Notes\n\nWith special chars."), 0644)

	result, err := notes.SyncDocs(engine, dir, docsCfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Imported) != 1 {
		t.Fatalf("special filename should import, got Imported=%d", len(result.Imported))
	}
}

func TestDocs_UnicodeContent(t *testing.T) {
	dir, engine := docEngine(t)

	note, _ := engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "Unicode",
		Body:  "# 日本語\n\n🍄 maitake はキノコです。\n\nEmoji: 🎉🔥💀\n",
	})
	notes.SyncDocs(engine, dir, docsCfg)

	// Read back
	files, _ := filepath.Glob(filepath.Join(dir, "docs", "*.md"))
	if len(files) == 0 {
		t.Fatal("no files")
	}
	data, _ := os.ReadFile(files[0])
	content := string(data)
	if !strings.Contains(content, "🍄") {
		t.Errorf("unicode lost: %s", content)
	}

	// Round-trip: delete and restore
	os.RemoveAll(filepath.Join(dir, "docs"))
	repo2, _ := git.NewGitRepo(dir)
	engine2, _ := notes.NewEngine(repo2)
	notes.SyncDocs(engine2, dir, docsCfg)

	data2, _ := os.ReadFile(files[0])
	if !strings.Contains(string(data2), "🍄") {
		t.Errorf("unicode lost after round-trip: %s", string(data2))
	}
	_ = note
}

// ── Subdirectories ───────────────────────────────────────────────────────

func TestDocs_NestedSubdirectories(t *testing.T) {
	dir, engine := docEngine(t)
	nested := filepath.Join(dir, "docs", "features", "auth")
	os.MkdirAll(nested, 0755)

	os.WriteFile(filepath.Join(nested, "oauth.md"), []byte("# OAuth\n\nOAuth2 flow."), 0644)
	os.WriteFile(filepath.Join(nested, "jwt.md"), []byte("# JWT\n\nToken handling."), 0644)

	result, _ := notes.SyncDocs(engine, dir, docsCfg)
	if len(result.Imported) != 2 {
		t.Fatalf("should import 2 nested files, got %d", len(result.Imported))
	}

	// Delete and restore — subdirectories should be recreated
	os.RemoveAll(filepath.Join(dir, "docs"))
	repo2, _ := git.NewGitRepo(dir)
	engine2, _ := notes.NewEngine(repo2)
	notes.SyncDocs(engine2, dir, docsCfg)

	if _, err := os.Stat(filepath.Join(nested, "oauth.md")); err != nil {
		t.Error("nested file should be restored with directory structure")
	}
}

// ── Concurrent note + file edits ─────────────────────────────────────────

func TestDocs_BothNoteAndFileChanged(t *testing.T) {
	dir, engine := docEngine(t)

	note, _ := engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "Conflict",
		Body:  "Original.",
	})
	notes.SyncDocs(engine, dir, docsCfg)

	// Edit the file on disk
	filePath := filepath.Join(dir, "docs", "conflict.md")
	data, _ := os.ReadFile(filePath)
	os.WriteFile(filePath, []byte(strings.Replace(string(data), "Original.", "File edit.", 1)), 0644)

	// ALSO change the note body via event (simulate agent edit)
	engine.Append(notes.AppendOptions{
		TargetID: note.ID,
		Kind:     "event",
		Field:    "body",
		Value:    "Note edit.",
	})

	// Sync — both changed. Current behavior: file wins.
	result, _ := notes.SyncDocs(engine, dir, docsCfg)

	// After sync, note should have file content (file wins)
	state, _ := engine.Fold(note.ID)
	if !strings.Contains(state.Body, "File edit") {
		t.Errorf("file should win conflict.\nBody: %q", state.Body)
	}

	_ = result
}

// ── Rapid successive edits ───────────────────────────────────────────────

func TestDocs_RapidEditsAllPersist(t *testing.T) {
	dir, engine := docEngine(t)

	note, _ := engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "Rapid",
		Body:  "Start.",
	})
	notes.SyncDocs(engine, dir, docsCfg)

	filePath := filepath.Join(dir, "docs", "rapid.md")

	// 10 rapid edits
	for i := 0; i < 10; i++ {
		data, _ := os.ReadFile(filePath)
		os.WriteFile(filePath, []byte(string(data)+"\nEdit "+string(rune('A'+i))), 0644)
		notes.SyncDocs(engine, dir, docsCfg)
	}

	// All 10 should be in the note
	state, _ := engine.Fold(note.ID)
	for i := 0; i < 10; i++ {
		marker := "Edit " + string(rune('A'+i))
		if !strings.Contains(state.Body, marker) {
			t.Errorf("missing %q in body after rapid edits", marker)
		}
	}

	// Survives restart
	os.RemoveAll(filepath.Join(dir, "docs"))
	repo2, _ := git.NewGitRepo(dir)
	engine2, _ := notes.NewEngine(repo2)
	notes.SyncDocs(engine2, dir, docsCfg)

	restored, _ := os.ReadFile(filePath)
	if !strings.Contains(string(restored), "Edit J") {
		t.Errorf("last rapid edit lost after restore.\nGot: %s", string(restored))
	}
}

// ── Multiple doc notes ───────────────────────────────────────────────────

func TestDocs_ManyDocsAllSurviveRmRf(t *testing.T) {
	dir, engine := docEngine(t)

	// Create 20 doc notes
	for i := 0; i < 20; i++ {
		engine.Create(notes.CreateOptions{
			Kind:  "doc",
			Title: strings.Repeat("Doc", 1) + string(rune('A'+i)),
			Body:  "Content " + string(rune('A'+i)),
		})
	}
	notes.SyncDocs(engine, dir, docsCfg)

	// Count files
	files1, _ := filepath.Glob(filepath.Join(dir, "docs", "*.md"))
	if len(files1) != 20 {
		t.Fatalf("should have 20 files, got %d", len(files1))
	}

	// rm -rf and restore
	os.RemoveAll(filepath.Join(dir, "docs"))
	repo2, _ := git.NewGitRepo(dir)
	engine2, _ := notes.NewEngine(repo2)
	notes.SyncDocs(engine2, dir, docsCfg)

	files2, _ := filepath.Glob(filepath.Join(dir, "docs", "*.md"))
	if len(files2) != 20 {
		t.Errorf("should restore 20 files, got %d", len(files2))
	}
}

// ── Intentional file deletion (tombstone) ────────────────────────────────

func TestDocs_IntentionalDelete_NotResurrected(t *testing.T) {
	dir, engine := docEngine(t)

	// Create 2 doc notes
	engine.Create(notes.CreateOptions{Kind: "doc", Title: "Keep", Body: "Keeper."})
	unwanted, _ := engine.Create(notes.CreateOptions{Kind: "doc", Title: "Unwanted", Body: "Delete me."})
	notes.SyncDocs(engine, dir, docsCfg)

	// Both files exist
	keepFile := filepath.Join(dir, "docs", "keep.md")
	unwantedFile := filepath.Join(dir, "docs", "unwanted.md")
	if _, err := os.Stat(keepFile); err != nil {
		t.Fatal("keep.md should exist")
	}
	if _, err := os.Stat(unwantedFile); err != nil {
		t.Fatal("unwanted.md should exist")
	}

	// User deletes unwanted.md and adds tombstone
	os.Remove(unwantedFile)
	notes.AddTombstone(dir, unwanted.ID)

	// Sync again — unwanted should NOT come back
	repo2, _ := git.NewGitRepo(dir)
	engine2, _ := notes.NewEngine(repo2)
	notes.SyncDocs(engine2, dir, docsCfg)

	if _, err := os.Stat(unwantedFile); !os.IsNotExist(err) {
		t.Error("tombstoned file should NOT be recreated by sync")
	}
	if _, err := os.Stat(keepFile); err != nil {
		t.Error("non-tombstoned file should still exist")
	}
}

func TestDocs_TombstoneRemoval_Resurrects(t *testing.T) {
	dir, engine := docEngine(t)

	note, _ := engine.Create(notes.CreateOptions{Kind: "doc", Title: "Revived", Body: "Back from the dead."})
	notes.SyncDocs(engine, dir, docsCfg)

	// Tombstone it
	filePath := filepath.Join(dir, "docs", "revived.md")
	os.Remove(filePath)
	notes.AddTombstone(dir, note.ID)

	// Verify tombstoned
	notes.SyncDocs(engine, dir, docsCfg)
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatal("should be tombstoned")
	}

	// Remove tombstone
	notes.RemoveTombstone(dir, note.ID)

	// Sync — should come back
	notes.SyncDocs(engine, dir, docsCfg)
	if _, err := os.Stat(filePath); err != nil {
		t.Error("removing tombstone should allow file to be recreated")
	}
}

// ── Non-md files in docs dir ─────────────────────────────────────────────

func TestDocs_IgnoresNonMarkdownFiles(t *testing.T) {
	dir, engine := docEngine(t)
	docsDir := filepath.Join(dir, "docs")
	os.MkdirAll(docsDir, 0755)

	os.WriteFile(filepath.Join(docsDir, "image.png"), []byte("fake png"), 0644)
	os.WriteFile(filepath.Join(docsDir, "data.json"), []byte(`{"key":"val"}`), 0644)
	os.WriteFile(filepath.Join(docsDir, "real.md"), []byte("# Real\n\nMarkdown."), 0644)

	result, _ := notes.SyncDocs(engine, dir, docsCfg)
	if len(result.Imported) != 1 {
		t.Errorf("should only import .md files, got %d", len(result.Imported))
	}
}

// ── Frontmatter preservation ─────────────────────────────────────────────

// TestDocs_ExtraFrontmatterPreserved checks that Obsidian fields survive an
// import round-trip. The original test only verified mai-id and body content;
// this version also asserts that the extra fields were NOT dropped.
func TestDocs_ExtraFrontmatterPreserved(t *testing.T) {
	dir, engine := docEngine(t)
	docsDir := filepath.Join(dir, "docs")
	os.MkdirAll(docsDir, 0755)

	original := "---\ntitle: My Note\ntags: [a, b]\ndate: 2026-03-28\n---\n# Obsidian Note\n\nWith extra frontmatter.\n"
	os.WriteFile(filepath.Join(docsDir, "obsidian.md"), []byte(original), 0644)

	result, _ := notes.SyncDocs(engine, dir, docsCfg)
	if len(result.Imported) != 1 {
		t.Fatalf("should import, got %d", len(result.Imported))
	}

	data, _ := os.ReadFile(filepath.Join(docsDir, "obsidian.md"))
	content := string(data)

	if !strings.Contains(content, "mai-id:") {
		t.Error("should have mai-id after import")
	}
	if !strings.Contains(content, "Obsidian Note") {
		t.Error("body content should be preserved")
	}
	// Obsidian fields must survive — not dropped on first write.
	if !strings.Contains(content, "title: My Note") {
		t.Errorf("title field dropped after import.\nGot:\n%s", content)
	}
	if !strings.Contains(content, "tags: [a, b]") {
		t.Errorf("tags field dropped after import.\nGot:\n%s", content)
	}
	if !strings.Contains(content, "date: 2026-03-28") {
		t.Errorf("date field dropped after import.\nGot:\n%s", content)
	}
}

// TestDocs_TagsAliasesCssclassesPreserved verifies that tags, aliases,
// and cssclasses (Obsidian-specific fields) survive a full sync round-trip.
func TestDocs_TagsAliasesCssclassesPreserved(t *testing.T) {
	dir, engine := docEngine(t)
	docsDir := filepath.Join(dir, "docs")
	os.MkdirAll(docsDir, 0755)

	original := "---\ntags:\n  - project\n  - active\naliases:\n  - myalias\ncssclasses:\n  - fancy\n---\n# Tagged Note\n\nContent here.\n"
	os.WriteFile(filepath.Join(docsDir, "tagged.md"), []byte(original), 0644)

	notes.SyncDocs(engine, dir, docsCfg)

	// Do a second sync (simulates note update propagation).
	notes.SyncDocs(engine, dir, docsCfg)

	data, _ := os.ReadFile(filepath.Join(docsDir, "tagged.md"))
	content := string(data)

	if !strings.Contains(content, "tags:") {
		t.Errorf("tags block dropped.\nGot:\n%s", content)
	}
	if !strings.Contains(content, "aliases:") {
		t.Errorf("aliases block dropped.\nGot:\n%s", content)
	}
	if !strings.Contains(content, "cssclasses:") {
		t.Errorf("cssclasses block dropped.\nGot:\n%s", content)
	}
	if !strings.Contains(content, "mai-id:") {
		t.Errorf("mai-id missing after sync.\nGot:\n%s", content)
	}
}

// TestDocs_UnknownFrontmatterFieldsSurvive verifies that arbitrary unknown
// fields in the frontmatter are preserved through an import + body-update cycle.
func TestDocs_UnknownFrontmatterFieldsSurvive(t *testing.T) {
	dir, engine := docEngine(t)
	docsDir := filepath.Join(dir, "docs")
	os.MkdirAll(docsDir, 0755)

	original := "---\nmy-custom-field: hello\nanother_field: 42\n---\n# Unknown Fields\n\nOriginal content.\n"
	os.WriteFile(filepath.Join(docsDir, "unknown.md"), []byte(original), 0644)

	result, _ := notes.SyncDocs(engine, dir, docsCfg)
	if len(result.Imported) != 1 {
		t.Fatalf("should import, got %d", len(result.Imported))
	}

	// Get the imported note ID, then update the note body.
	summaries, _ := engine.List(notes.ListOptions{FindOptions: notes.FindOptions{Kind: "doc"}})
	if len(summaries) != 1 {
		t.Fatal("doc note not imported")
	}
	engine.Append(notes.AppendOptions{
		TargetID: summaries[0].ID,
		Kind:     "event",
		Field:    "body",
		Body:     "# Unknown Fields\n\nUpdated content.\n",
	})
	notes.SyncDocs(engine, dir, docsCfg)

	data, _ := os.ReadFile(filepath.Join(docsDir, "unknown.md"))
	content := string(data)

	if !strings.Contains(content, "my-custom-field: hello") {
		t.Errorf("custom field dropped after body update.\nGot:\n%s", content)
	}
	if !strings.Contains(content, "another_field: 42") {
		t.Errorf("another_field dropped after body update.\nGot:\n%s", content)
	}
}

// TestDocs_AddMaiIdToExistingFrontmatter verifies that importing a file that
// already has frontmatter (but no mai-id) adds mai-id without destroying the
// existing fields.
func TestDocs_AddMaiIdToExistingFrontmatter(t *testing.T) {
	dir, engine := docEngine(t)
	docsDir := filepath.Join(dir, "docs")
	os.MkdirAll(docsDir, 0755)

	// File has frontmatter but no mai-id.
	original := "---\ntitle: Existing Note\ncreated: 2025-01-01\n---\n# Body\n\nContent.\n"
	os.WriteFile(filepath.Join(docsDir, "existing.md"), []byte(original), 0644)

	result, _ := notes.SyncDocs(engine, dir, docsCfg)
	if len(result.Imported) != 1 {
		t.Fatalf("should import, got %d", len(result.Imported))
	}

	data, _ := os.ReadFile(filepath.Join(docsDir, "existing.md"))
	content := string(data)

	if !strings.Contains(content, "mai-id:") {
		t.Errorf("mai-id not added.\nGot:\n%s", content)
	}
	if !strings.Contains(content, "title: Existing Note") {
		t.Errorf("title dropped when mai-id was added.\nGot:\n%s", content)
	}
	if !strings.Contains(content, "created: 2025-01-01") {
		t.Errorf("created field dropped when mai-id was added.\nGot:\n%s", content)
	}
}

// ── Body with frontmatter-like content ───────────────────────────────────

func TestDocs_BodyContainsDashDashDash(t *testing.T) {
	dir, engine := docEngine(t)

	engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "Tricky",
		Body:  "# Tricky\n\nSome YAML example:\n\n---\nkey: value\n---\n\nEnd.",
	})
	notes.SyncDocs(engine, dir, docsCfg)

	files, _ := filepath.Glob(filepath.Join(dir, "docs", "*.md"))
	if len(files) == 0 {
		t.Fatal("no file written")
	}

	// Read it back — the --- in the body shouldn't confuse the parser
	data, _ := os.ReadFile(files[0])
	noteID, body := notes.ParseMaiFrontmatterExported(string(data))
	if noteID == "" {
		t.Error("should parse mai-id from frontmatter")
	}
	if !strings.Contains(body, "key: value") {
		t.Errorf("body content with --- should survive round-trip.\nBody: %q", body)
	}
}
