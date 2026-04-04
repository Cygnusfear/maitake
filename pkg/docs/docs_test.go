package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cygnusfear/maitake/pkg/git"
	"github.com/cygnusfear/maitake/pkg/notes"
)

func setupRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.InitRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Need an initial commit for notes to attach to
	os.WriteFile(filepath.Join(dir, "init"), []byte("init"), 0644)
	if err := repo.Add("."); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit("init"); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSyncDocs_NoteToFile(t *testing.T) {
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)

	note, err := engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "Architecture",
		Body:  "# Architecture\n\nMicroservices.",
	})
	if err != nil {
		t.Fatal(err)
	}

	cfg := Config{Dir: "docs"}
	result, err := SyncDocs(engine, dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Written) == 0 {
		t.Fatal("expected file to be written")
	}

	files, _ := filepath.Glob(filepath.Join(dir, "docs", "*.md"))
	if len(files) == 0 {
		t.Fatal("no doc files found")
	}
	data, _ := os.ReadFile(files[0])
	content := string(data)
	if !strings.Contains(content, "mai-id: "+note.ID) {
		t.Errorf("missing mai-id frontmatter: %s", content)
	}
	if !strings.Contains(content, "# Architecture") {
		t.Errorf("missing body: %s", content)
	}
}

func TestSyncDocs_FileToNote(t *testing.T) {
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)

	docsDir := filepath.Join(dir, "docs")
	os.MkdirAll(docsDir, 0755)
	os.WriteFile(filepath.Join(docsDir, "guide.md"), []byte("# Guide\n\nHow to use."), 0644)

	cfg := Config{Dir: "docs"}
	result, err := SyncDocs(engine, dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Imported) != 1 {
		t.Fatalf("Imported = %d, want 1", len(result.Imported))
	}

	data, _ := os.ReadFile(filepath.Join(docsDir, "guide.md"))
	if !strings.Contains(string(data), "mai-id:") {
		t.Error("imported file should have mai-id frontmatter")
	}
}

func TestSyncDocs_RoundTrip(t *testing.T) {
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)

	note, _ := engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "Test",
		Body:  "Original content.",
	})

	cfg := Config{Dir: "docs"}
	SyncDocs(engine, dir, cfg)

	// Edit file
	filePath := filepath.Join(dir, "docs", "test.md")
	data, _ := os.ReadFile(filePath)
	newContent := strings.Replace(string(data), "Original content.", "Updated from Obsidian.", 1)
	os.WriteFile(filePath, []byte(newContent), 0644)

	// Sync back
	result, err := SyncDocs(engine, dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Updated) != 1 {
		t.Fatalf("Updated = %d, want 1", len(result.Updated))
	}

	state, _ := engine.Fold(note.ID)
	if !strings.Contains(state.Body, "Updated from Obsidian") {
		t.Errorf("note body not updated: %q", state.Body)
	}
}
