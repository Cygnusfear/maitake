package test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cygnusfear/maitake/pkg/git"
	"github.com/cygnusfear/maitake/pkg/notes"
)

// setupRepo creates a temp git repo with one commit and a file.
func setupRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s\n%s", args, err, out)
		}
	}

	run("init")
	run("config", "user.name", "Test")
	run("config", "user.email", "test@test.com")

	// Create a file and commit
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "auth.ts"), []byte("export function refreshToken() {}"), 0644)
	run("add", "-A")
	run("commit", "-m", "init")

	return dir
}

func TestEngine_CreateAndFold(t *testing.T) {
	dir := setupRepo(t)
	repo, err := git.NewGitRepo(dir)
	if err != nil {
		t.Fatal(err)
	}

	engine, err := notes.NewEngine(repo)
	if err != nil {
		t.Fatal(err)
	}

	// Create a ticket
	note, err := engine.Create(notes.CreateOptions{
		Kind:     "ticket",
		Title:    "Fix auth race condition",
		Type:     "task",
		Priority: 1,
		Tags:     []string{"auth", "backend"},
		Body:     "The token refresh has a race condition.",
		Targets:  []string{"src/auth.ts"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if note.ID == "" {
		t.Fatal("ID should not be empty")
	}
	t.Logf("Created ticket: %s", note.ID)

	// Fold it — should be open
	state, err := engine.Fold(note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "open" {
		t.Errorf("Status = %q, want open", state.Status)
	}
	if state.Title != "Fix auth race condition" {
		t.Errorf("Title = %q", state.Title)
	}
	if state.Priority != 1 {
		t.Errorf("Priority = %d", state.Priority)
	}
}

func TestEngine_AppendEventAndFold(t *testing.T) {
	dir := setupRepo(t)
	repo, err := git.NewGitRepo(dir)
	if err != nil {
		t.Fatal(err)
	}

	engine, err := notes.NewEngine(repo)
	if err != nil {
		t.Fatal(err)
	}

	// Create
	note, err := engine.Create(notes.CreateOptions{
		Kind:  "ticket",
		Title: "Test ticket",
		Type:  "task",
		Body:  "A test ticket.",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Start it
	_, err = engine.Append(notes.AppendOptions{
		TargetID: note.ID,
		Kind:     "event",
		Field:    "status",
		Value:    "in_progress",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Fold — should be in_progress
	state, err := engine.Fold(note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "in_progress" {
		t.Errorf("Status = %q, want in_progress", state.Status)
	}

	// Close it
	_, err = engine.Append(notes.AppendOptions{
		TargetID: note.ID,
		Kind:     "event",
		Field:    "status",
		Value:    "closed",
	})
	if err != nil {
		t.Fatal(err)
	}

	state, err = engine.Fold(note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "closed" {
		t.Errorf("Status = %q, want closed", state.Status)
	}
}

func TestEngine_Comment(t *testing.T) {
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)

	note, _ := engine.Create(notes.CreateOptions{
		Kind: "ticket",
		Body: "A ticket.",
	})

	_, err := engine.Append(notes.AppendOptions{
		TargetID: note.ID,
		Kind:     "comment",
		Body:     "Found the root cause.",
	})
	if err != nil {
		t.Fatal(err)
	}

	state, _ := engine.Fold(note.ID)
	if len(state.Comments) != 1 {
		t.Fatalf("Comments = %d, want 1", len(state.Comments))
	}
	if state.Comments[0].Body != "Found the root cause." {
		t.Errorf("Comment body = %q", state.Comments[0].Body)
	}
}

func TestEngine_Context(t *testing.T) {
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)

	// Create two notes targeting the same file
	engine.Create(notes.CreateOptions{
		Kind:    "warning",
		Body:    "Race condition here.",
		Targets: []string{"src/auth.ts"},
	})
	engine.Create(notes.CreateOptions{
		Kind:    "ticket",
		Body:    "Fix the race condition.",
		Targets: []string{"src/auth.ts"},
	})
	// One targeting a different file
	engine.Create(notes.CreateOptions{
		Kind:    "ticket",
		Body:    "Unrelated.",
		Targets: []string{"src/http.ts"},
	})

	ctx, err := engine.Context("src/auth.ts")
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx) != 2 {
		t.Errorf("Context(src/auth.ts) = %d notes, want 2", len(ctx))
	}
}

func TestEngine_ArtifactBornClosed(t *testing.T) {
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)

	note, err := engine.Create(notes.CreateOptions{
		Kind: "review",
		Type: "artifact",
		Body: "Review findings.",
	})
	if err != nil {
		t.Fatal(err)
	}

	state, _ := engine.Fold(note.ID)
	if state.Status != "closed" {
		t.Errorf("Artifact status = %q, want closed", state.Status)
	}
}

func TestEngine_FindAndList(t *testing.T) {
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)

	engine.Create(notes.CreateOptions{Kind: "ticket", Type: "task", Body: "Task 1."})
	engine.Create(notes.CreateOptions{Kind: "ticket", Type: "bug", Body: "Bug 1."})
	engine.Create(notes.CreateOptions{Kind: "warning", Body: "Warning 1."})

	// Find by kind
	results, _ := engine.Find(notes.FindOptions{Kind: "ticket"})
	if len(results) != 2 {
		t.Errorf("Find(ticket) = %d, want 2", len(results))
	}

	// List all
	summaries, _ := engine.List(notes.ListOptions{})
	if len(summaries) != 3 {
		t.Errorf("List() = %d, want 3", len(summaries))
	}

	// Find by type
	results, _ = engine.Find(notes.FindOptions{Type: "bug"})
	if len(results) != 1 {
		t.Errorf("Find(bug) = %d, want 1", len(results))
	}
}

func TestEngine_Doctor(t *testing.T) {
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)

	engine.Create(notes.CreateOptions{Kind: "ticket", Body: "One."})
	engine.Create(notes.CreateOptions{Kind: "warning", Body: "Two."})

	report, err := engine.Doctor()
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalNotes != 2 {
		t.Errorf("TotalNotes = %d, want 2", report.TotalNotes)
	}
	if report.ByKind["ticket"] != 1 {
		t.Errorf("ticket count = %d", report.ByKind["ticket"])
	}
}

func TestEngine_PersistsAcrossRestart(t *testing.T) {
	dir := setupRepo(t)

	// Session 1: create a note
	repo1, _ := git.NewGitRepo(dir)
	engine1, _ := notes.NewEngine(repo1)
	note, _ := engine1.Create(notes.CreateOptions{
		Kind: "ticket",
		Body: "Persistent ticket.",
	})
	noteID := note.ID

	// Session 2: new engine instance should see the note from git
	repo2, _ := git.NewGitRepo(dir)
	engine2, _ := notes.NewEngine(repo2)

	state, err := engine2.Fold(noteID)
	if err != nil {
		t.Fatalf("Second engine can't find note %q: %v", noteID, err)
	}
	if state.Body != "Persistent ticket." {
		t.Errorf("Body = %q", state.Body)
	}
}


// TestSummaryCacheSelfHeal ensures that when the full cache exists for a ref tip
// but the summary cache does not, the next engine invocation rebuilds the summary
// cache as a side-effect of Rebuild's cache-hit branch. This regresses the
// real-world bug where updateCache wrote only the full cache, leaving subsequent
// ls/kinds/show commands stuck on the slow path until a cache miss occurred.
func TestSummaryCacheSelfHeal(t *testing.T) {
	dir := setupRepo(t)
	repo, err := git.NewGitRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := notes.NewEngine(repo)
	if err != nil {
		t.Fatal(err)
	}

	// Create a note so the notes ref exists and has a tip.
	if _, err := engine.Create(notes.CreateOptions{
		Kind:  "ticket",
		Title: "self-heal target",
		Type:  "task",
		Body:  "body",
	}); err != nil {
		t.Fatal(err)
	}

	// Prime the cache — first List call triggers Rebuild which writes both
	// the full cache and the summary cache.
	if _, err := engine.List(notes.ListOptions{}); err != nil {
		t.Fatal(err)
	}

	// Locate the cache directory the engine uses. Mirror the hashing strategy
	// in pkg/notes/cache.go: first 8 bytes of sha256(repoPath), hex.
	home, _ := os.UserHomeDir()
	sum := sha256.Sum256([]byte(dir))
	hash := fmt.Sprintf("%x", sum[:8])
	repoCacheDir := filepath.Join(home, ".maitake", "cache", hash)

	// Delete the summary cache file(s) only, leaving the full cache in place.
	entries, err := os.ReadDir(repoCacheDir)
	if err != nil {
		t.Fatalf("read cache dir: %v", err)
	}
	deletedSummary := false
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".summary.json") {
			if err := os.Remove(filepath.Join(repoCacheDir, e.Name())); err != nil {
				t.Fatalf("remove summary: %v", err)
			}
			deletedSummary = true
		}
	}
	if !deletedSummary {
		t.Fatal("no summary cache file found to delete — engine never wrote one")
	}

	// Fresh engine forces Rebuild from the existing full cache.
	engine2, err := notes.NewEngine(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine2.List(notes.ListOptions{}); err != nil {
		t.Fatal(err)
	}

	// After the List call, the summary cache should have been re-created.
	entries2, _ := os.ReadDir(repoCacheDir)
	foundSummary := false
	for _, e := range entries2 {
		if strings.HasSuffix(e.Name(), ".summary.json") {
			foundSummary = true
			break
		}
	}
	if !foundSummary {
		t.Fatal("summary cache was NOT rewritten after Rebuild — self-heal regression")
	}
}


// TestUpdateCacheWritesSummary ensures that a write (which triggers
// scheduleAsyncRebuild -> eventually updateCache) results in both the full
// cache AND the summary cache being written for the new ref tip. This
// regresses the bug where updateCache wrote only the full cache, forcing
// every ls/show on the new tip through a 200MB deserialization path.
func TestUpdateCacheWritesSummary(t *testing.T) {
	dir := setupRepo(t)
	repo, err := git.NewGitRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := notes.NewEngine(repo)
	if err != nil {
		t.Fatal(err)
	}

	// First write — creates the notes ref.
	if _, err := engine.Create(notes.CreateOptions{
		Kind: "ticket", Title: "first", Type: "task", Body: "b",
	}); err != nil {
		t.Fatal(err)
	}
	// Prime: List triggers Rebuild which writes both caches.
	if _, err := engine.List(notes.ListOptions{}); err != nil {
		t.Fatal(err)
	}

	// Second write — bumps the notes ref tip.
	if _, err := engine.Create(notes.CreateOptions{
		Kind: "ticket", Title: "second", Type: "task", Body: "b",
	}); err != nil {
		t.Fatal(err)
	}
	// Next read — updateCache OR Rebuild writes caches for the new tip.
	if _, err := engine.List(notes.ListOptions{}); err != nil {
		t.Fatal(err)
	}

	home, _ := os.UserHomeDir()
	sum := sha256.Sum256([]byte(dir))
	hash := fmt.Sprintf("%x", sum[:8])
	repoCacheDir := filepath.Join(home, ".maitake", "cache", hash)

	entries, err := os.ReadDir(repoCacheDir)
	if err != nil {
		t.Fatalf("read cache dir: %v", err)
	}

	// Each tip should have both a .json and a .summary.json counterpart.
	byPrefix := map[string]map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".summary.json") {
			prefix := strings.TrimSuffix(name, ".summary.json")
			if byPrefix[prefix] == nil {
				byPrefix[prefix] = map[string]bool{}
			}
			byPrefix[prefix]["summary"] = true
		} else if strings.HasSuffix(name, ".json") {
			prefix := strings.TrimSuffix(name, ".json")
			if byPrefix[prefix] == nil {
				byPrefix[prefix] = map[string]bool{}
			}
			byPrefix[prefix]["full"] = true
		}
	}

	if len(byPrefix) == 0 {
		t.Fatal("no cache files written at all")
	}
	for prefix, kinds := range byPrefix {
		if !kinds["full"] {
			t.Errorf("tip %q: missing full cache", prefix)
		}
		if !kinds["summary"] {
			t.Errorf("tip %q: missing summary cache (updateCache did not write it)", prefix)
		}
	}
}
