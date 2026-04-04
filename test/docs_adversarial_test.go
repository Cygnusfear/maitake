package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cygnusfear/maitake/pkg/git"
	"github.com/cygnusfear/maitake/pkg/docs"
	"github.com/cygnusfear/maitake/pkg/notes"
)

func docEngine(t *testing.T) (string, notes.Engine) {
	t.Helper()
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)
	return dir, engine
}

var docsCfg = docs.Config{Dir: "docs"}

// ── Frontmatter corruption ───────────────────────────────────────────────

func TestDocs_CorruptedFrontmatter_StillImports(t *testing.T) {
	dir, engine := docEngine(t)
	docsDir := filepath.Join(dir, "docs")
	os.MkdirAll(docsDir, 0755)

	// File with broken frontmatter (unclosed ---)
	os.WriteFile(filepath.Join(docsDir, "broken.md"), []byte("---\nmai-id: fake\ntitle: oops\n# No closing frontmatter\n\nContent here.\n"), 0644)

	result, err := docs.SyncDocs(engine, dir, docsCfg)
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

	result, _ := docs.SyncDocs(engine, dir, docsCfg)
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

	result, err := docs.SyncDocs(engine, dir, docsCfg)
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

	result, _ := docs.SyncDocs(engine, dir, docsCfg)
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

	result, err := docs.SyncDocs(engine, dir, docsCfg)
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
	docs.SyncDocs(engine, dir, docsCfg)

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
	docs.SyncDocs(engine2, dir, docsCfg)

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

	result, _ := docs.SyncDocs(engine, dir, docsCfg)
	if len(result.Imported) != 2 {
		t.Fatalf("should import 2 nested files, got %d", len(result.Imported))
	}

	// Delete and restore — subdirectories should be recreated
	os.RemoveAll(filepath.Join(dir, "docs"))
	repo2, _ := git.NewGitRepo(dir)
	engine2, _ := notes.NewEngine(repo2)
	docs.SyncDocs(engine2, dir, docsCfg)

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
	docs.SyncDocs(engine, dir, docsCfg)

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
	result, _ := docs.SyncDocs(engine, dir, docsCfg)

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
	docs.SyncDocs(engine, dir, docsCfg)

	filePath := filepath.Join(dir, "docs", "rapid.md")

	// 10 rapid edits
	for i := 0; i < 10; i++ {
		data, _ := os.ReadFile(filePath)
		os.WriteFile(filePath, []byte(string(data)+"\nEdit "+string(rune('A'+i))), 0644)
		docs.SyncDocs(engine, dir, docsCfg)
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
	docs.SyncDocs(engine2, dir, docsCfg)

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
	docs.SyncDocs(engine, dir, docsCfg)

	// Count files
	files1, _ := filepath.Glob(filepath.Join(dir, "docs", "*.md"))
	if len(files1) != 20 {
		t.Fatalf("should have 20 files, got %d", len(files1))
	}

	// rm -rf and restore
	os.RemoveAll(filepath.Join(dir, "docs"))
	repo2, _ := git.NewGitRepo(dir)
	engine2, _ := notes.NewEngine(repo2)
	docs.SyncDocs(engine2, dir, docsCfg)

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
	docs.SyncDocs(engine, dir, docsCfg)

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
	docs.AddTombstone(dir, unwanted.ID)

	// Sync again — unwanted should NOT come back
	repo2, _ := git.NewGitRepo(dir)
	engine2, _ := notes.NewEngine(repo2)
	docs.SyncDocs(engine2, dir, docsCfg)

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
	docs.SyncDocs(engine, dir, docsCfg)

	// Tombstone it
	filePath := filepath.Join(dir, "docs", "revived.md")
	os.Remove(filePath)
	docs.AddTombstone(dir, note.ID)

	// Verify tombstoned
	docs.SyncDocs(engine, dir, docsCfg)
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatal("should be tombstoned")
	}

	// Remove tombstone
	docs.RemoveTombstone(dir, note.ID)

	// Sync — should come back
	docs.SyncDocs(engine, dir, docsCfg)
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

	result, _ := docs.SyncDocs(engine, dir, docsCfg)
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

	result, _ := docs.SyncDocs(engine, dir, docsCfg)
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

	docs.SyncDocs(engine, dir, docsCfg)

	// Do a second sync (simulates note update propagation).
	docs.SyncDocs(engine, dir, docsCfg)

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

	result, _ := docs.SyncDocs(engine, dir, docsCfg)
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
	docs.SyncDocs(engine, dir, docsCfg)

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

	result, _ := docs.SyncDocs(engine, dir, docsCfg)
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

func TestDocs_CreateRejectsDuplicateTargetPath(t *testing.T) {
	dir, engine := docEngine(t)
	_ = dir

	_, err := engine.Create(notes.CreateOptions{
		Kind:    "doc",
		Title:   "One",
		Body:    "first",
		Targets: []string{"docs/shared.md"},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = engine.Create(notes.CreateOptions{
		Kind:    "doc",
		Title:   "Two",
		Body:    "second",
		Targets: []string{"docs/shared.md"},
	})
	if err == nil {
		t.Fatal("expected duplicate doc target to be rejected")
	}
	if !strings.Contains(err.Error(), "already owned") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDocsSync_StrippedFrontmatterAdoptsExistingDoc(t *testing.T) {
	dir, engine := docEngine(t)

	note, err := engine.Create(notes.CreateOptions{
		Kind:    "doc",
		Title:   "Guide",
		Body:    "# Guide\n\nOriginal body.\n",
		Targets: []string{"docs/guide.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := docs.SyncDocs(engine, dir, docsCfg); err != nil {
		t.Fatal(err)
	}

	// Simulate a user or tool replacing the file without mai frontmatter.
	stripped := "# Guide\n\nEdited on disk without frontmatter.\n"
	if err := os.WriteFile(filepath.Join(dir, "docs", "guide.md"), []byte(stripped), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := docs.SyncDocs(engine, dir, docsCfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Imported) != 0 {
		t.Fatalf("should not import a duplicate doc, imported=%v", result.Imported)
	}
	if len(result.Updated) != 1 || result.Updated[0] != "docs/guide.md" {
		t.Fatalf("expected guide.md to update existing note, got %+v", result)
	}

	summaries, _ := engine.List(notes.ListOptions{FindOptions: notes.FindOptions{Kind: "doc"}})
	if len(summaries) != 1 {
		t.Fatalf("expected one doc note, got %d", len(summaries))
	}
	state, err := engine.Fold(note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(state.Body, "Edited on disk without frontmatter") {
		t.Fatalf("expected existing note body to adopt disk edit, got %q", state.Body)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "docs", "guide.md"))
	content := string(data)
	if !strings.Contains(content, "mai-id: "+note.ID) {
		t.Fatalf("expected file to be reattached to original note id, got:\n%s", content)
	}
}

func TestDocsSync_DuplicateTargetsReportConflict(t *testing.T) {
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)

	first, err := engine.Create(notes.CreateOptions{
		Kind:    "doc",
		Title:   "One",
		Body:    "first",
		Targets: []string{"docs/conflict.md"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Materialize first doc to disk before creating the conflict
	docs.SyncDocs(engine, dir, docsCfg)

	second := &notes.Note{
		ID:        "doc-conflict-2",
		Kind:      "doc",
		Title:     "Two",
		Body:      "second",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Time:      time.Now().UTC(),
		Edges: []notes.Edge{{
			Type:   "targets",
			Target: notes.EdgeTarget{Kind: "path", Ref: "docs/conflict.md"},
		}},
	}
	data, err := notes.Serialize(second)
	if err != nil {
		t.Fatal(err)
	}
	targetOID, err := repo.StoreBlob("maitake:" + second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.AppendNote(git.DefaultNotesRef, targetOID, git.Note(data)); err != nil {
		t.Fatal(err)
	}
	if err := engine.Rebuild(); err != nil {
		t.Fatal(err)
	}

	result, err := docs.SyncDocs(engine, dir, docsCfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Conflicts) != 1 || result.Conflicts[0] != "docs/conflict.md" {
		t.Fatalf("expected one conflict on docs/conflict.md, got %+v", result)
	}
	data, err = os.ReadFile(filepath.Join(dir, "docs", "conflict.md"))
	if err != nil {
		t.Fatalf("existing conflicted file should be left in place: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "mai-id: "+first.ID) {
		t.Fatalf("conflicted file should stay attached to the original note.\nGot:\n%s", content)
	}
	if strings.Contains(content, second.ID) || strings.Contains(content, "second") {
		t.Fatalf("conflicted file should not be overwritten by the duplicate note.\nGot:\n%s", content)
	}
}

// ── CRDT merge regression: repeated syncs must not duplicate content ─────

// TestDocs_RepeatedSyncs_NoDuplication is the regression test for the
// stale-merge-base bug. Before the fix, mergeViaCRDT used creationBody
// as the 3-way merge base forever. After N sync cycles both peers would
// diff from that ancient base, producing overlapping inserts. The CRDT
// preserved both → exponential duplication (2x per cycle).
func TestDocs_RepeatedSyncs_NoDuplication(t *testing.T) {
	dir, engine := docEngine(t)

	note, _ := engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "NoDup",
		Body:  "if one day we can merge safely, we need the right base.",
	})
	docs.SyncDocs(engine, dir, docsCfg)

	filePath := filepath.Join(dir, "docs", "nodup.md")

	// Simulate 6 rounds of Obsidian-style full-file saves with small edits.
	edits := []string{
		"if one day we can merge safely, we need the right base.\nEdit 1.",
		"if one day we can merge safely, we need the right base.\nEdit 1.\nEdit 2.",
		"if one day we can merge safely, we need the right base.\nEdit 1.\nEdit 2.\nEdit 3.",
		"if one day we can merge safely, we need the right base.\nEdit 1.\nEdit 2.\nEdit 3.\nEdit 4.",
		"if one day we can merge safely, we need the right base.\nEdit 1.\nEdit 2.\nEdit 3.\nEdit 4.\nEdit 5.",
		"if one day we can merge safely, we need the right base.\nEdit 1.\nEdit 2.\nEdit 3.\nEdit 4.\nEdit 5.\nEdit 6.",
	}

	for i, content := range edits {
		// Full-file rewrite (what Obsidian does)
		data, _ := os.ReadFile(filePath)
		_, body := docs.ParseFrontmatter(string(data))
		_ = body
		// Reconstruct file with frontmatter + new body
		id, _ := docs.ParseFrontmatter(string(data))
		fm := "---\nmai-id: " + id + "\n---\n"
		os.WriteFile(filePath, []byte(fm+content+"\n"), 0644)

		result, err := docs.SyncDocs(engine, dir, docsCfg)
		if err != nil {
			t.Fatalf("round %d: sync error: %v", i+1, err)
		}

		// After each sync, the note body must exactly match what we wrote
		state, _ := engine.Fold(note.ID)
		if strings.TrimSpace(state.Body) != strings.TrimSpace(content) {
			t.Fatalf("round %d: body mismatch.\nExpected: %q\nGot:      %q", i+1, content, state.Body)
		}

		// Content must NOT be duplicated — check that key phrase appears once
		count := strings.Count(state.Body, "if one day we can merge safely")
		if count != 1 {
			t.Fatalf("round %d: duplication detected! 'if one day we can merge safely' appears %d times.\nBody: %q",
				i+1, count, state.Body)
		}

		_ = result
	}
}

// TestDocs_ConcurrentNoteAndFileEdits_NoDuplication tests that both note-side
// and file-side edits merge cleanly across multiple sync cycles without
// duplication.
func TestDocs_ConcurrentNoteAndFileEdits_NoDuplication(t *testing.T) {
	dir, engine := docEngine(t)

	note, _ := engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "Concurrent",
		Body:  "Base content.\n",
	})
	docs.SyncDocs(engine, dir, docsCfg)

	filePath := filepath.Join(dir, "docs", "concurrent.md")

	for i := 0; i < 5; i++ {
		// Agent edits note body
		state, _ := engine.Fold(note.ID)
		newNoteBody := state.Body + "Agent line " + string(rune('A'+i)) + ".\n"
		engine.Append(notes.AppendOptions{
			TargetID: note.ID,
			Kind:     "event",
			Field:    "body",
			Body:     newNoteBody,
		})

		// User edits file (adds a different line)
		data, _ := os.ReadFile(filePath)
		id, body := docs.ParseFrontmatter(string(data))
		userBody := body + "User line " + string(rune('1'+i)) + ".\n"
		fm := "---\nmai-id: " + id + "\n---\n"
		os.WriteFile(filePath, []byte(fm+userBody+"\n"), 0644)

		// Sync
		_, err := docs.SyncDocs(engine, dir, docsCfg)
		if err != nil {
			t.Fatalf("round %d: %v", i+1, err)
		}

		// No duplication — "Base content" should appear exactly once
		merged, _ := engine.Fold(note.ID)
		count := strings.Count(merged.Body, "Base content")
		if count != 1 {
			t.Fatalf("round %d: 'Base content' appears %d times (expected 1).\nBody: %q",
				i+1, count, merged.Body)
		}

		// Body size should grow linearly, not exponentially
		// Each round adds ~2 lines (~30 chars). After 5 rounds: ~250 chars max.
		if len(merged.Body) > 1000 {
			t.Fatalf("round %d: body size explosion (%d chars). Likely duplication.\nBody: %q",
				i+1, len(merged.Body), merged.Body)
		}
	}
}

// TestDocs_LargeDocument_StableSize ensures a large document doesn't explode
// in size across sync cycles.
func TestDocs_LargeDocument_StableSize(t *testing.T) {
	dir, engine := docEngine(t)

	// Build a ~2700 word document
	var body strings.Builder
	body.WriteString("# Large Document\n\n")
	for i := 0; i < 100; i++ {
		body.WriteString("This is paragraph number ")
		body.WriteString(string(rune('0' + i/100)))
		body.WriteString(string(rune('0' + (i/10)%10)))
		body.WriteString(string(rune('0' + i%10)))
		body.WriteString(". It contains enough words to simulate a real document that someone would edit in Obsidian.\n\n")
	}
	originalBody := body.String()
	originalSize := len(originalBody)

	note, _ := engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "LargeDoc",
		Body:  originalBody,
	})
	docs.SyncDocs(engine, dir, docsCfg)

	filePath := filepath.Join(dir, "docs", "largedoc.md")

	// 5 rounds of small edits on a large document
	for i := 0; i < 5; i++ {
		data, _ := os.ReadFile(filePath)
		id, currentBody := docs.ParseFrontmatter(string(data))
		// Append one line
		editedBody := currentBody + "Edit round " + string(rune('A'+i)) + ".\n"
		fm := "---\nmai-id: " + id + "\n---\n"
		os.WriteFile(filePath, []byte(fm+editedBody+"\n"), 0644)

		docs.SyncDocs(engine, dir, docsCfg)
	}

	state, _ := engine.Fold(note.ID)
	finalSize := len(state.Body)

	// Should be within 20% of original + edits (~50 chars of edits)
	maxExpected := int(float64(originalSize)*1.2) + 200
	if finalSize > maxExpected {
		t.Fatalf("document size exploded: original=%d, final=%d, max=%d.\nLikely duplication.",
			originalSize, finalSize, maxExpected)
	}
}

// TestDocs_FileOnlyEdit_NoFileRewrite verifies that when only the file
// changed (user editing in Obsidian), the daemon updates the note but does
// NOT rewrite the file. This prevents Obsidian's "file changed externally"
// dialog from appearing during normal editing.
func TestDocs_FileOnlyEdit_NoFileRewrite(t *testing.T) {
	dir, engine := docEngine(t)

	note, _ := engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "NoRewrite",
		Body:  "Original content.",
	})
	docs.SyncDocs(engine, dir, docsCfg)

	filePath := filepath.Join(dir, "docs", "norewrite.md")

	// User edits the file in Obsidian (full file rewrite)
	data, _ := os.ReadFile(filePath)
	id, _ := docs.ParseFrontmatter(string(data))
	newBody := "Original content.\nUser added this line."
	fm := "---\nmai-id: " + id + "\n---\n"
	os.WriteFile(filePath, []byte(fm+newBody+"\n"), 0644)

	// Record the file's modification time BEFORE sync
	info1, _ := os.Stat(filePath)
	mtime1 := info1.ModTime()

	// Small delay to ensure mtime would differ if file were rewritten
	time.Sleep(50 * time.Millisecond)

	// Sync — should update note but NOT rewrite the file
	docs.SyncDocs(engine, dir, docsCfg)

	// Verify the note was updated
	state, _ := engine.Fold(note.ID)
	if !strings.Contains(state.Body, "User added this line") {
		t.Fatalf("note should have file's content after sync.\nBody: %q", state.Body)
	}

	// Verify the file was NOT rewritten (mtime unchanged)
	info2, _ := os.Stat(filePath)
	mtime2 := info2.ModTime()
	if !mtime1.Equal(mtime2) {
		t.Fatalf("file should NOT be rewritten when only the file changed.\nmtime before: %v\nmtime after:  %v", mtime1, mtime2)
	}

	// Verify file content is exactly what the user wrote (not reconstructed)
	data2, _ := os.ReadFile(filePath)
	expected := fm + newBody + "\n"
	if string(data2) != expected {
		t.Fatalf("file content should be untouched.\nExpected: %q\nGot:      %q", expected, string(data2))
	}
}

// TestDocs_NoteOnlyEdit_WritesFile verifies that when only the note
// changed (agent edit), the file IS updated.
func TestDocs_NoteOnlyEdit_WritesFile(t *testing.T) {
	dir, engine := docEngine(t)

	note, _ := engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "NoteOnly",
		Body:  "Original content.",
	})
	docs.SyncDocs(engine, dir, docsCfg)

	// Agent edits the note (not the file)
	engine.Append(notes.AppendOptions{
		TargetID: note.ID,
		Kind:     "event",
		Field:    "body",
		Body:     "Original content.\nAgent added this line.",
	})

	// Sync — should update the file
	docs.SyncDocs(engine, dir, docsCfg)

	// Verify the file was updated
	filePath := filepath.Join(dir, "docs", "noteonly.md")
	data, _ := os.ReadFile(filePath)
	if !strings.Contains(string(data), "Agent added this line") {
		t.Fatalf("file should have note's content after sync.\nFile: %s", string(data))
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
	docs.SyncDocs(engine, dir, docsCfg)

	files, _ := filepath.Glob(filepath.Join(dir, "docs", "*.md"))
	if len(files) == 0 {
		t.Fatal("no file written")
	}

	// Read it back — the --- in the body shouldn't confuse the parser
	data, _ := os.ReadFile(files[0])
	noteID, body := docs.ParseFrontmatter(string(data))
	if noteID == "" {
		t.Error("should parse mai-id from frontmatter")
	}
	if !strings.Contains(body, "key: value") {
		t.Errorf("body content with --- should survive round-trip.\nBody: %q", body)
	}
}
