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
		os.Exit(0) // no repos, exit silently
	}

	// Filter to repos that exist and have docs.watch enabled
	var watched []watchedRepo
	for _, repoPath := range repos {
		// Skip dead/temp repos
		if _, err := os.Stat(repoPath); err != nil {
			continue
		}
		maitakeDir := filepath.Join(repoPath, ".maitake")
		cfg := notes.ReadConfig(maitakeDir)
		if !cfg.Docs.Watch {
			continue
		}
		docsDir := filepath.Join(repoPath, cfg.Docs.Dir)
		watched = append(watched, watchedRepo{
			path:    repoPath,
			docsDir: docsDir,
			cfg:     cfg,
		})
	}

	if len(watched) == 0 {
		os.Exit(0) // nothing to watch, exit silently
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fatal("creating watcher: %v", err)
	}
	defer watcher.Close()

	// Watch repo root (shallow) + docs dir (recursive).
	// Repo root is watched shallow so we catch docs/ being recreated after rm -rf.
	// Docs dir is watched recursive for file changes.
	for _, w := range watched {
		watcher.Add(w.path) // repo root — shallow, catches dir creation
		if _, err := os.Stat(w.docsDir); err == nil {
			addDirRecursive(watcher, w.docsDir) // docs dir — recursive
		}
		fmt.Printf("watching %s (%s)\n", w.docsDir, filepath.Base(w.path))
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

			// Watch new/recreated directories — especially the docs dir
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					// Only recursively watch if it's under a docs dir
					for _, w := range watched {
						if strings.HasPrefix(event.Name, w.docsDir) || event.Name == w.docsDir {
							addDirRecursive(watcher, event.Name)
							break
						}
					}
				}
			}

			if !strings.HasSuffix(event.Name, ".md") {
				continue
			}

			// Only process files under a watched repo's docs dir
			var matchedRepo *watchedRepo
			for i := range watched {
				if strings.HasPrefix(event.Name, watched[i].docsDir+"/") {
					matchedRepo = &watched[i]
					break
				}
			}
			if matchedRepo == nil {
				continue
			}

			if event.Op&fsnotify.Remove != 0 {
				// File intentionally deleted — tombstone so sync doesn't recreate
				handleFileDelete(*matchedRepo, event.Name)
				continue
			}

			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
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

func handleFileDelete(w watchedRepo, filePath string) {
	// Read the file content from git to find the mai-id
	// The file is already gone, but we can figure out which note it was
	// by scanning the note index for docs targeting this path
	rel, _ := filepath.Rel(w.path, filePath)

	repo, err := git.NewGitRepo(w.path)
	if err != nil {
		return
	}
	engine, err := notes.NewEngine(repo)
	if err != nil {
		return
	}

	// Find doc note that targets this file
	states, _ := engine.Find(notes.FindOptions{Kind: "doc"})
	for _, state := range states {
		for _, target := range state.Targets {
			if target == rel {
				notes.AddTombstone(w.path, state.ID)
				fmt.Printf("  ✗ %s (tombstoned %s) [%s]\n", rel, state.ID, filepath.Base(w.path))
				return
			}
		}
		// Also check derived path
		targetPath := notes.DocTargetPathExported(&state, w.cfg.Docs.Dir)
		if targetPath == rel {
			notes.AddTombstone(w.path, state.ID)
			fmt.Printf("  ✗ %s (tombstoned %s) [%s]\n", rel, state.ID, filepath.Base(w.path))
			return
		}
	}
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
