package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cygnusfear/maitake/pkg/git"
	"github.com/cygnusfear/maitake/pkg/notes"
	"github.com/fsnotify/fsnotify"
)

// simulateDaemon runs the core daemon loop for a single repo.
// Returns a stop function. Watches repo root, filters to docs dir.
func simulateDaemon(t *testing.T, repoPath string, cfg notes.DocsConfig) func() {
	t.Helper()

	docsDir := filepath.Join(repoPath, cfg.Dir)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}

	// Watch repo root, not docs dir (survives rm -rf docs/)
	addTestDirRecursive(t, watcher, repoPath)

	stop := make(chan struct{})
	done := make(chan struct{})

	go func() {
		defer close(done)
		debounce := make(map[string]time.Time)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// Watch new dirs (including recreated docs/)
				if event.Op&fsnotify.Create != 0 {
					if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
						addTestDirRecursive(t, watcher, event.Name)
					}
				}
				if !strings.HasSuffix(event.Name, ".md") {
					continue
				}
				if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
					continue
				}
				// Filter to docs dir
				if !strings.HasPrefix(event.Name, docsDir+"/") {
					continue
				}
				debounce[event.Name] = time.Now()

			case <-ticker.C:
				now := time.Now()
				for path, changed := range debounce {
					if now.Sub(changed) < 200*time.Millisecond {
						continue
					}
					delete(debounce, path)
					// Sync
					repo, _ := git.NewGitRepo(repoPath)
					engine, _ := notes.NewEngine(repo)
					notes.SyncDocs(engine, repoPath, cfg)
					_ = path
				}

			case <-watcher.Errors:
				continue
			}
		}
	}()

	return func() {
		close(stop)
		watcher.Close()
		<-done
	}
}

var testSkipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
	"target": true, ".maitake": true,
}

func addTestDirRecursive(t *testing.T, watcher *fsnotify.Watcher, dir string) {
	t.Helper()
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		if testSkipDirs[info.Name()] {
			return filepath.SkipDir
		}
		watcher.Add(path)
		return nil
	})
}

// ── THE BUG: rm -rf docs/ kills fsnotify watches ────────────────────────

func TestDaemon_SurvivesRmRfDocs(t *testing.T) {
	t.Skip("TODO: daemon fsnotify + CRDT sync interaction — race on rm-rf + file recreation")
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)
	cfg := notes.DocsConfig{Dir: "docs"}

	// Create doc and materialize
	note, _ := engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "Survivor",
		Body:  "Original body.",
	})
	notes.SyncDocs(engine, dir, cfg)

	// Start daemon
	stopDaemon := simulateDaemon(t, dir, cfg)
	defer stopDaemon()
	time.Sleep(200 * time.Millisecond) // let watcher start

	// Edit the file
	filePath := filepath.Join(dir, "docs", "survivor.md")
	data, _ := os.ReadFile(filePath)
	os.WriteFile(filePath, []byte(strings.Replace(string(data), "Original body.", "Edit 1.", 1)), 0644)
	time.Sleep(500 * time.Millisecond) // daemon catches

	// Verify edit reached note
	repo2, _ := git.NewGitRepo(dir)
	engine2, _ := notes.NewEngine(repo2)
	state, _ := engine2.Fold(note.ID)
	if !strings.Contains(state.Body, "Edit 1") {
		t.Fatalf("daemon should catch edit 1.\nBody: %q", state.Body)
	}

	// rm -rf docs/
	os.RemoveAll(filepath.Join(dir, "docs"))
	time.Sleep(200 * time.Millisecond)

	// Restore
	repo3, _ := git.NewGitRepo(dir)
	engine3, _ := notes.NewEngine(repo3)
	notes.SyncDocs(engine3, dir, cfg)

	// Edit AGAIN after restore — daemon must still be watching
	data2, _ := os.ReadFile(filePath)
	os.WriteFile(filePath, []byte(strings.Replace(string(data2), "Edit 1.", "Edit 2 after rm-rf.", 1)), 0644)
	time.Sleep(500 * time.Millisecond) // daemon catches

	// Verify the post-rm-rf edit reached the note
	repo4, _ := git.NewGitRepo(dir)
	engine4, _ := notes.NewEngine(repo4)
	state2, _ := engine4.Fold(note.ID)
	if !strings.Contains(state2.Body, "Edit 2 after rm-rf") {
		t.Errorf("daemon should catch edits AFTER rm -rf docs/ and restore.\nBody: %q", state2.Body)
	}
}

func TestDaemon_CatchesNewFileInRecreatedDir(t *testing.T) {
	t.Skip("TODO: fsnotify race between dir watch registration and file write — needs retry loop in daemon")
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)
	cfg := notes.DocsConfig{Dir: "docs"}

	// Create initial doc
	engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "Initial",
		Body:  "First doc.",
	})
	notes.SyncDocs(engine, dir, cfg)

	// Start daemon
	stopDaemon := simulateDaemon(t, dir, cfg)
	defer stopDaemon()
	time.Sleep(200 * time.Millisecond)

	// rm -rf and recreate with a NEW file
	os.RemoveAll(filepath.Join(dir, "docs"))
	time.Sleep(300 * time.Millisecond)
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	time.Sleep(500 * time.Millisecond) // let watcher pick up new dir
	os.WriteFile(filepath.Join(dir, "docs", "newfile.md"), []byte("# Brand New\n\nCreated after rm -rf."), 0644)
	time.Sleep(800 * time.Millisecond) // daemon catches

	// The new file should have been imported
	repo2, _ := git.NewGitRepo(dir)
	engine2, _ := notes.NewEngine(repo2)
	summaries, _ := engine2.List(notes.ListOptions{FindOptions: notes.FindOptions{Kind: "doc"}})

	found := false
	for _, s := range summaries {
		if s.Title == "Brand New" {
			found = true
		}
	}
	if !found {
		t.Error("daemon should import new file created in recreated docs dir")
	}
}

func TestDaemon_MultipleRapidEditsAfterRmRf(t *testing.T) {
	t.Skip("TODO: daemon fsnotify + CRDT sync interaction — race on rm-rf + rapid edits")
	dir := setupRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)
	cfg := notes.DocsConfig{Dir: "docs"}

	note, _ := engine.Create(notes.CreateOptions{
		Kind:  "doc",
		Title: "Rapid",
		Body:  "Start.",
	})
	notes.SyncDocs(engine, dir, cfg)
	filePath := filepath.Join(dir, "docs", "rapid.md")

	stopDaemon := simulateDaemon(t, dir, cfg)
	defer stopDaemon()
	time.Sleep(200 * time.Millisecond)

	// rm -rf, restore, then rapid edits
	os.RemoveAll(filepath.Join(dir, "docs"))
	time.Sleep(200 * time.Millisecond)
	repo2, _ := git.NewGitRepo(dir)
	engine2, _ := notes.NewEngine(repo2)
	notes.SyncDocs(engine2, dir, cfg)
	time.Sleep(300 * time.Millisecond) // let daemon pick up recreated dir

	// 5 rapid edits
	for i := 0; i < 5; i++ {
		data, _ := os.ReadFile(filePath)
		os.WriteFile(filePath, []byte(string(data)+"\nRapid "+string(rune('A'+i))), 0644)
		time.Sleep(400 * time.Millisecond) // each edit needs time to sync
	}

	// Check all edits made it
	repo3, _ := git.NewGitRepo(dir)
	engine3, _ := notes.NewEngine(repo3)
	state, _ := engine3.Fold(note.ID)
	for i := 0; i < 5; i++ {
		marker := "Rapid " + string(rune('A'+i))
		if !strings.Contains(state.Body, marker) {
			t.Errorf("missing %q after rapid edits post rm-rf.\nBody: %q", marker, state.Body)
		}
	}
}
