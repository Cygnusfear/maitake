// mai-docs is the docs plugin for maitake.
// Handles doc sync, daemon, check, refs, and expand.
// Discovered and dispatched by `mai docs` via .maitake/plugins.toml.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cygnusfear/maitake/internal/cli"
	"github.com/cygnusfear/maitake/pkg/docs"
	"github.com/cygnusfear/maitake/pkg/git"
	"github.com/cygnusfear/maitake/pkg/notes"
)

var jsonOutput bool

func main() {
	args := os.Args[1:]

	var filtered []string
	for _, a := range args {
		if a == "--json" {
			jsonOutput = true
		} else {
			filtered = append(filtered, a)
		}
	}
	if os.Getenv("MAI_JSON") == "1" {
		jsonOutput = true
	}
	args = filtered

	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		printHelp()
		return
	}

	if len(args) == 0 {
		printHelp()
		return
	}

	switch args[0] {
	case "sync":
		e, _ := initEngine()
		runSync(e, args[1:])
	case "check":
		e, _ := initEngine()
		runCheck(e, args[1:])
	case "refs":
		e, _ := initEngine()
		runRefs(e, args[1:])
	case "expand":
		e, _ := initEngine()
		runExpand(e, args[1:])
	case "daemon":
		runDaemon(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown docs subcommand: %s\n", args[0])
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Print(`mai-docs — document sync and management

Usage: mai-docs <subcommand> [args]

  mai-docs sync [--dir D] [--dry-run] [-y]  Sync doc notes ↔ markdown files
  mai-docs check                        Validate code refs and wiki links
  mai-docs refs <id>                    Find references to a note
  mai-docs expand <text>                Expand [[wiki refs]] in text
  mai-docs daemon                       Watch for doc file changes

Flags:
  --json       JSON output
  --dir <dir>  Docs directory (default: from config or .mai-docs)
  --dry-run    Preview without writing
  -y, --yes    Skip confirmation prompts
`)
}

func repoPath() string {
	dir := os.Getenv("MAI_REPO_PATH")
	if dir != "" {
		return dir
	}
	dir, _ = os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			cli.Fatal("not a git repository")
		}
		dir = parent
	}
}

func initEngine() (notes.Engine, git.Repo) {
	dir := repoPath()
	repo, err := git.NewGitRepo(dir)
	if err != nil {
		cli.Fatal("not a git repository: %v", err)
	}
	engine, err := notes.NewEngine(repo)
	if err != nil {
		cli.Fatal("initializing engine: %v", err)
	}
	docs.RegisterAutoSync(engine)
	return engine, repo
}

// --- Sync ---

func runSync(e notes.Engine, args []string) {
	dir := repoPath()
	cfg := docs.Config{}
	dryRun := false
	assumeYes := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dir":
			i++
			if i < len(args) {
				cfg.Dir = args[i]
			}
		case "--dry-run", "-n":
			dryRun = true
		case "--yes", "-y":
			assumeYes = true
		}
	}

	if cfg.Dir == "" {
		maiCfg := e.GetConfig()
		if maiCfg.Docs.Dir != "" {
			cfg.Dir = maiCfg.Docs.Dir
		}
	}

	warnIfNotGitignored(dir, cfg.Dir)

	preview, err := docs.SyncDocs(e, dir, cfg, docs.SyncOptions{DryRun: true})
	if err != nil {
		cli.Fatal("docs sync: %v", err)
	}

	total := len(preview.Written) + len(preview.Imported) + len(preview.Updated) + len(preview.Removed)
	if total == 0 {
		fmt.Println("Everything in sync.")
		return
	}

	printSyncResult(preview, dryRun)

	if dryRun {
		return
	}

	if total > 10 && !assumeYes {
		fmt.Printf("\n%d files will be affected. Continue? [y/N] ", total)
		var answer string
		fmt.Scanln(&answer)
		if answer != "y" && answer != "Y" {
			fmt.Println("Aborted.")
			return
		}
	}

	result, err := docs.SyncDocs(e, dir, cfg)
	if err != nil {
		cli.Fatal("docs sync: %v", err)
	}

	if jsonOutput {
		cli.PrintJSON(result)
		return
	}

	printSyncResult(result, false)
}

func warnIfNotGitignored(dir, docsDir string) {
	if docsDir == "" {
		docsDir = ".mai-docs"
	}
	gitignorePath := filepath.Join(dir, ".gitignore")
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

// --- Check ---

func runCheck(e notes.Engine, args []string) {
	dir := repoPath()
	result, err := notes.Check(e, dir)
	if err != nil {
		cli.Fatal("check: %v", err)
	}

	if jsonOutput {
		cli.PrintJSON(result)
		return
	}

	errors := len(result.BrokenCode) + len(result.BrokenWiki)
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

// --- Refs ---

func runRefs(e notes.Engine, args []string) {
	if len(args) < 1 {
		cli.Fatal("usage: mai-docs refs <id>")
	}
	target := args[0]
	dir := repoPath()

	codeRefs, err := notes.ScanCodeRefs(dir)
	if err != nil {
		cli.Fatal("refs: %v", err)
	}

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

	if jsonOutput {
		cli.PrintJSON(map[string]interface{}{
			"target": target,
			"code":   codeMatches,
			"wiki":   wikiMatches,
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

// --- Expand ---

func runExpand(e notes.Engine, args []string) {
	text := strings.Join(args, " ")
	if text == "" {
		cli.Fatal("usage: mai-docs expand <text with [[refs]]>")
	}
	result, err := notes.Expand(e, text)
	if err != nil {
		cli.Fatal("expand: %v", err)
	}
	fmt.Print(result)
}

// --- Daemon ---

func runDaemon(args []string) {
	// For now, delegate to mai daemon (daemon logic stays in mai until fully extracted)
	fmt.Println("mai-docs daemon: not yet implemented as standalone — use `mai daemon`")
	os.Exit(1)
}

func matchesRef(refTarget, query string) bool {
	if refTarget == query {
		return true
	}
	if strings.Contains(refTarget, query) {
		return true
	}
	if strings.Contains(query, refTarget) {
		return true
	}
	return false
}
