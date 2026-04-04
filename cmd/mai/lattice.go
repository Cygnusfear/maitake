package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cygnusfear/maitake/pkg/docs"
	"github.com/cygnusfear/maitake/pkg/notes"
)

func runDocsSync(e notes.Engine, args []string) {
	cwd, _ := os.Getwd()
	dir := globalDir
	if dir == "" {
		dir = cwd
	}

	cfg := docs.Config{}
	dryRun := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dir":
			i++
			if i < len(args) {
				cfg.Dir = args[i]
			}
		case "--dry-run", "-n":
			dryRun = true
		}
	}

	// Read docs dir from .maitake/config if not specified
	if cfg.Dir == "" {
		maiCfg := e.GetConfig()
		if maiCfg.Docs.Dir != "" {
			cfg.Dir = maiCfg.Docs.Dir
		}
	}

	// Warn if docs dir isn't gitignored
	warnIfNotGitignored(dir, cfg.Dir)

	// Dry-run first to preview
	preview, err := docs.SyncDocs(e, dir, cfg, docs.SyncOptions{DryRun: true})
	if err != nil {
		fatal("docs sync: %v", err)
	}

	total := len(preview.Written) + len(preview.Imported) + len(preview.Updated) + len(preview.Removed)
	if total == 0 {
		fmt.Println("Everything in sync.")
		return
	}

	// Print preview
	printSyncResult(preview, dryRun)

	if dryRun {
		return
	}

	// Confirm if >10 files affected
	if total > 10 {
		fmt.Printf("\n%d files will be affected. Continue? [y/N] ", total)
		var answer string
		fmt.Scanln(&answer)
		if answer != "y" && answer != "Y" {
			fmt.Println("Aborted.")
			return
		}
	}

	// Execute for real
	result, err := docs.SyncDocs(e, dir, cfg)
	if err != nil {
		fatal("docs sync: %v", err)
	}

	if globalJSON {
		printJSON(result)
		return
	}

	printSyncResult(result, false)
}

func warnIfNotGitignored(repoPath, docsDir string) {
	if docsDir == "" {
		docsDir = ".mai-docs"
	}
	gitignorePath := filepath.Join(repoPath, ".gitignore")
	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  No .gitignore found. Run 'mai init' to gitignore %s/\n", docsDir)
		return
	}
	if !strings.Contains(string(data), docsDir) {
		fmt.Fprintf(os.Stderr, "⚠️  %s/ is not in .gitignore — docs may collide with git. Run 'mai init' to fix.\n", docsDir)
	}
}

func printSyncResult(result *docs.SyncResult, dryRun bool) {
	prefix := ""
	if dryRun {
		prefix = "(dry-run) "
	}

	if len(result.Written) > 0 {
		fmt.Printf("\n%s=== Write from notes → disk (%d files) ===\n", prefix, len(result.Written))
		for _, f := range result.Written {
			fmt.Printf("  → %s\n", f)
		}
	}
	if len(result.Imported) > 0 {
		fmt.Printf("\n%s=== Import from disk → notes (%d files) ===\n", prefix, len(result.Imported))
		for _, f := range result.Imported {
			fmt.Printf("  ← %s\n", f)
		}
	}
	if len(result.Updated) > 0 {
		fmt.Printf("\n%s=== Updated notes from files (%d files) ===\n", prefix, len(result.Updated))
		for _, f := range result.Updated {
			fmt.Printf("  ↔ %s\n", f)
		}
	}
	if len(result.Removed) > 0 {
		fmt.Printf("\n%s=== Removed (%d files) ===\n", prefix, len(result.Removed))
		for _, f := range result.Removed {
			fmt.Printf("  ✗ %s\n", f)
		}
	}

	total := len(result.Written) + len(result.Imported) + len(result.Updated) + len(result.Removed)
	fmt.Printf("\n%s%d written, %d imported, %d updated, %d removed\n",
		prefix, len(result.Written), len(result.Imported), len(result.Updated), len(result.Removed))
	_ = total
}

func runCheck(e notes.Engine, args []string) {
	cwd, _ := os.Getwd()
	dir := globalDir
	if dir == "" {
		dir = cwd
	}

	result, err := notes.Check(e, dir)
	if err != nil {
		fatal("check: %v", err)
	}

	if globalJSON {
		printJSON(result)
		return
	}

	errors := len(result.BrokenCode) + len(result.BrokenWiki)

	// Summary
	fmt.Printf("Scanned: %d code refs, %d wiki links\n", len(result.CodeRefs), len(result.WikiRefs))

	if errors == 0 {
		fmt.Println("✓ All refs resolve.")
		return
	}

	fmt.Printf("\n%d broken ref(s):\n\n", errors)

	for _, e := range result.BrokenCode {
		fmt.Printf("  %s:%d  %s\n", e.File, e.Line, e.Message)
	}
	for _, e := range result.BrokenWiki {
		if e.NoteID != "" {
			fmt.Printf("  [%s]  %s\n", e.NoteID, e.Message)
		} else {
			fmt.Printf("  %s\n", e.Message)
		}
	}
	fmt.Println()
	os.Exit(1)
}

func runRefs(e notes.Engine, args []string) {
	if len(args) < 1 {
		fatal("usage: mai refs <id>")
	}
	target := args[0]

	cwd, _ := os.Getwd()
	dir := globalDir
	if dir == "" {
		dir = cwd
	}

	// Find code refs pointing at this target
	codeRefs, err := notes.ScanCodeRefs(dir)
	if err != nil {
		fatal("refs: %v", err)
	}

	// Find wiki refs in notes pointing at this target
	states, _ := e.Find(notes.FindOptions{})
	closedStates, _ := e.Find(notes.FindOptions{Status: "closed"})
	states = append(states, closedStates...)

	var codeMatches []notes.CodeRef
	var wikiMatches []notes.WikiRef

	for _, ref := range codeRefs {
		if matchesRef(ref.Target, target) {
			codeMatches = append(codeMatches, ref)
		}
	}

	for _, state := range states {
		for _, ref := range notes.ExtractWikiRefs(state.ID, state.Body) {
			if matchesRef(ref.Target, target) {
				wikiMatches = append(wikiMatches, ref)
			}
		}
		for _, c := range state.Comments {
			for _, ref := range notes.ExtractWikiRefs(state.ID, c.Body) {
				if matchesRef(ref.Target, target) {
					wikiMatches = append(wikiMatches, ref)
				}
			}
		}
	}

	if globalJSON {
		printJSON(map[string]interface{}{
			"target":   target,
			"code":     codeMatches,
			"wiki":     wikiMatches,
		})
		return
	}

	if len(codeMatches) == 0 && len(wikiMatches) == 0 {
		fmt.Printf("No references to %q found.\n", target)
		return
	}

	fmt.Printf("References to %q:\n\n", target)

	if len(codeMatches) > 0 {
		fmt.Println("Code refs:")
		for _, r := range codeMatches {
			fmt.Printf("  %s:%d  %s\n", r.File, r.Line, r.Raw)
		}
	}

	if len(wikiMatches) > 0 {
		if len(codeMatches) > 0 {
			fmt.Println()
		}
		fmt.Println("Note refs:")
		for _, r := range wikiMatches {
			fmt.Printf("  [%s]  [[%s]]\n", r.NoteID, r.Target)
		}
	}
}

func runExpand(e notes.Engine, args []string) {
	text := strings.Join(args, " ")
	if text == "" {
		fatal("usage: mai expand <text with [[refs]]>")
	}

	result, err := notes.Expand(e, text)
	if err != nil {
		fatal("expand: %v", err)
	}

	fmt.Print(result)
}

func matchesRef(refTarget, query string) bool {
	if refTarget == query {
		return true
	}
	if strings.Contains(refTarget, query) {
		return true
	}
	// Partial ID match
	if strings.Contains(query, refTarget) {
		return true
	}
	return false
}
