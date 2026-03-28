package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cygnusfear/maitake/pkg/git"
	"github.com/cygnusfear/maitake/pkg/notes"
	"github.com/fsnotify/fsnotify"
)

func runDaemon(args []string) {
	repos := loadRepoList()
	if len(repos) == 0 {
		fatal("no repos registered — run mai in a repo first")
	}

	// Filter to repos with docs.watch enabled
	var watched []watchedRepo
	for _, repoPath := range repos {
		maitakeDir := filepath.Join(repoPath, ".maitake")
		cfg := notes.ReadConfig(maitakeDir)
		if !cfg.Docs.Watch {
			continue
		}
		docsDir := filepath.Join(repoPath, cfg.Docs.Dir)
		if _, err := os.Stat(docsDir); err != nil {
			continue
		}
		watched = append(watched, watchedRepo{
			path:    repoPath,
			docsDir: docsDir,
			cfg:     cfg,
		})
	}

	if len(watched) == 0 {
		fatal("no repos have docs.watch = true")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fatal("creating watcher: %v", err)
	}
	defer watcher.Close()

	// Watch repo roots (not docs dirs — those can be deleted and recreated)
	for _, w := range watched {
		addDirRecursive(watcher, w.path)
		fmt.Printf("watching %s (%s)\n", w.path, filepath.Base(w.path))
	}

	fmt.Printf("\nmai daemon: watching %d repo(s). ctrl-c to stop.\n\n", len(watched))

	// Debounce: collect file changes, sync after 500ms of quiet
	debounce := make(map[string]time.Time) // file path → last change time
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			// Watch new directories (including recreated docs dirs)
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					addDirRecursive(watcher, event.Name)
				}
			}

			if !strings.HasSuffix(event.Name, ".md") {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}

			// Only process files under a watched repo's docs dir
			inDocsDir := false
			for _, w := range watched {
				if strings.HasPrefix(event.Name, w.docsDir+"/") || event.Name == w.docsDir {
					inDocsDir = true
					break
				}
			}
			if !inDocsDir {
				continue
			}

			debounce[event.Name] = time.Now()

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			fmt.Fprintf(os.Stderr, "watch error: %v\n", err)

		case now := <-ticker.C:
			// Process debounced changes
			for path, changed := range debounce {
				if now.Sub(changed) < 500*time.Millisecond {
					continue
				}
				delete(debounce, path)

				// Find which repo this file belongs to
				for _, w := range watched {
					if strings.HasPrefix(path, w.docsDir) {
						syncFile(w, path)
						break
					}
				}
			}
		}
	}
}

type watchedRepo struct {
	path    string
	docsDir string
	cfg     notes.Config
}

func syncFile(w watchedRepo, filePath string) {
	repo, err := git.NewGitRepo(w.path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  error: %s not a git repo\n", w.path)
		return
	}

	engine, err := notes.NewEngine(repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  error: engine init: %v\n", err)
		return
	}

	result, err := notes.SyncDocs(engine, w.path, w.cfg.Docs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  error: sync: %v\n", err)
		return
	}

	rel, _ := filepath.Rel(w.path, filePath)
	for _, f := range result.Imported {
		fmt.Printf("  ← %s (imported) [%s]\n", f, filepath.Base(w.path))
	}
	for _, f := range result.Updated {
		fmt.Printf("  ↔ %s (updated) [%s]\n", f, filepath.Base(w.path))
	}
	// Only print if this file was actually synced
	if len(result.Imported) == 0 && len(result.Updated) == 0 {
		_ = rel // file was in sync
	}
}

var skipWatchDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
	"target": true, "dist": true, "build": true,
	".next": true, "__pycache__": true, ".worktrees": true,
	".maitake": true,
}

func addDirRecursive(watcher *fsnotify.Watcher, dir string) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		if skipWatchDirs[info.Name()] {
			return filepath.SkipDir
		}
		watcher.Add(path)
		return nil
	})
}

func loadRepoList() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(home, ".maitake", "repos"))
	if err != nil {
		return nil
	}
	var repos []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			repos = append(repos, line)
		}
	}
	return repos
}
