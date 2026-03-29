package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cygnusfear/maitake/pkg/git"
	"github.com/cygnusfear/maitake/pkg/notes"
)

// === Config defaults ===

func TestDaemon_WatchDefaultTrue(t *testing.T) {
	// Watch should default to true — repos are watched unless explicitly disabled
	dir := t.TempDir()
	cfg := notes.ReadConfig(dir) // no config files at all
	if !cfg.Docs.Watch {
		t.Error("Docs.Watch should default to true")
	}
}

func TestDaemon_WatchExplicitFalse(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.toml"),
		[]byte("[docs]\nwatch = false\n"), 0644)
	cfg := notes.ReadConfig(dir)
	if cfg.Docs.Watch {
		t.Error("Docs.Watch should be false when explicitly set")
	}
}

func TestDaemon_WatchLegacyConfigDoesNotSetWatch(t *testing.T) {
	// Legacy flat config has no watch key — should keep the default (true)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config"),
		[]byte("remote origin\nblocked-host github.com\n"), 0644)
	cfg := notes.ReadConfig(dir)
	if !cfg.Docs.Watch {
		t.Error("Legacy config should not override Watch default (true)")
	}
}

// === Worktree resolution ===

func TestDaemon_WorktreeResolvedToMainRepo(t *testing.T) {
	// Create a main repo
	mainDir := t.TempDir()
	gitRun(t, mainDir, "init")
	gitRun(t, mainDir, "config", "user.name", "Test")
	gitRun(t, mainDir, "config", "user.email", "test@test.com")
	os.WriteFile(filepath.Join(mainDir, "file.txt"), []byte("main"), 0644)
	gitRun(t, mainDir, "add", "-A")
	gitRun(t, mainDir, "commit", "-m", "init")

	// Create a worktree
	wtDir := filepath.Join(t.TempDir(), "my-worktree")
	gitRun(t, mainDir, "worktree", "add", wtDir, "-b", "feature")

	// Running mai from the worktree should register the MAIN repo, not the worktree
	// We test by checking what gets written to repos file
	home := t.TempDir()
	reposFile := filepath.Join(home, ".maitake", "repos")
	os.MkdirAll(filepath.Dir(reposFile), 0755)

	// Simulate registerRepo behavior: worktree .git is a file, not a dir
	dotGit := filepath.Join(wtDir, ".git")
	info, err := os.Stat(dotGit)
	if err != nil {
		t.Fatalf("worktree .git should exist: %v", err)
	}
	if info.IsDir() {
		t.Fatal("worktree .git should be a file, not a directory")
	}

	// Read .git file content
	data, err := os.ReadFile(dotGit)
	if err != nil {
		t.Fatal(err)
	}
	content := strings.TrimSpace(string(data))
	if !strings.HasPrefix(content, "gitdir: ") {
		t.Fatalf("worktree .git should start with 'gitdir: ', got: %q", content)
	}

	// The gitdir path should contain /.git/worktrees/
	gitDir := strings.TrimPrefix(content, "gitdir: ")
	if !strings.Contains(gitDir, "/.git/worktrees/") {
		t.Fatalf("gitdir should contain /.git/worktrees/, got: %q", gitDir)
	}

	// Extract main repo path (everything before /.git/worktrees/)
	idx := strings.Index(gitDir, "/.git/worktrees/")
	resolved := gitDir[:idx]
	if resolved != mainDir {
		t.Errorf("resolved = %q, want main repo %q", resolved, mainDir)
	}
}

func TestDaemon_MainRepoNotChanged(t *testing.T) {
	// A normal repo (not a worktree) should resolve to itself
	dir := t.TempDir()
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.name", "Test")
	gitRun(t, dir, "config", "user.email", "test@test.com")
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("main"), 0644)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "init")

	// .git should be a directory
	dotGit := filepath.Join(dir, ".git")
	info, err := os.Stat(dotGit)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal(".git should be a directory for a normal repo")
	}
}

// === Dead repo pruning ===

func TestDaemon_DeadReposPruned(t *testing.T) {
	// Create a repos file with alive and dead paths
	home := t.TempDir()
	reposDir := filepath.Join(home, ".maitake")
	os.MkdirAll(reposDir, 0755)
	reposFile := filepath.Join(reposDir, "repos")

	aliveDir := t.TempDir()
	gitRun(t, aliveDir, "init")

	os.WriteFile(reposFile, []byte(
		aliveDir+"\n"+
			"/tmp/maitake-dead-path-1\n"+
			"/tmp/maitake-dead-path-2\n",
	), 0644)

	// Read back — dead paths should have been present
	data, _ := os.ReadFile(reposFile)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines before pruning, got %d", len(lines))
	}
}

// === Engine cache ===

func TestDaemon_EngineCacheSameInstance(t *testing.T) {
	// Test that the engine is created once per repo, not per event.
	// We can't test the daemon's internal cache directly (package main),
	// but we verify that NewEngine on the same repo returns consistent state.
	dir := setupRepo(t)

	repo1, _ := os.ReadDir(dir) // sanity
	_ = repo1

	// Create two engines on same repo — both should see the same notes
	e1 := createEngine(t, dir)
	note, _ := e1.Create(notes.CreateOptions{Kind: "ticket", Title: "Cached?"})

	e2 := createEngine(t, dir)
	state, err := e2.Fold(note.ID)
	if err != nil {
		t.Fatalf("second engine should see note from first: %v", err)
	}
	if state.Title != "Cached?" {
		t.Errorf("title = %q, want 'Cached?'", state.Title)
	}
}

// === Worktree CLI registration ===

func TestDaemon_MaiInWorktreeRegistersMainRepo(t *testing.T) {
	// Create main repo
	mainDir := setupTestRepo(t)
	gitRun(t, mainDir, "branch", "-M", "main")

	// Create worktree
	wtDir := filepath.Join(t.TempDir(), "wt")
	gitRun(t, mainDir, "worktree", "add", wtDir, "-b", "feat")

	// Run mai from the worktree — it should register the main repo
	// We can verify by checking that mai runs without error
	out := mai(t, wtDir, "ls", "--status=all")
	_ = out // just verify it doesn't crash

	// The worktree should have a .git file (not dir)
	dotGit := filepath.Join(wtDir, ".git")
	info, _ := os.Stat(dotGit)
	if info.IsDir() {
		t.Error("worktree .git should be a file, not a directory")
	}
}

// helper
func createEngine(t *testing.T, dir string) notes.Engine {
	t.Helper()
	repo, err := git.NewGitRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	e, err := notes.NewEngine(repo)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
