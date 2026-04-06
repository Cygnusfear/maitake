// mai-changelog is the changelog plugin for maitake.
// Replaces tinychange — stores changelog entries as mai artifacts.
// Discovered and dispatched by `mai changelog` via .maitake/plugins.toml.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cygnusfear/maitake/internal/cli"
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

	e, _ := initEngine()

	switch args[0] {
	case "new":
		runNew(e, args[1:])
	case "ls", "list":
		runList(e, args[1:])
	case "merge":
		runMerge(e, args[1:])
	case "migrate":
		runMigrate(e, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown changelog subcommand: %s\n", args[0])
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Print(`mai-changelog — changelog management via mai artifacts

Usage: mai-changelog <subcommand> [args]

  mai-changelog new "description" -k fix    Create a changelog entry
  mai-changelog ls                          List unreleased entries
  mai-changelog merge [--output FILE]       Render changelog to markdown
  mai-changelog migrate [--dir .tinychange] Import from tinychange format

Categories: fix, feat, chore, security, docs, refactor, perf, deploy, test

Flags:
  -k <category>     Category (fix, feat, chore, etc.)
  --output <file>   Write merged changelog to file (default: stdout)
  --json            JSON output
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
	return engine, repo
}

// --- New ---

func runNew(e notes.Engine, args []string) {
	var category string
	var positional []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-k", "--kind", "--category":
			i++
			if i < len(args) {
				category = args[i]
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				cli.Fatal("unknown flag: %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}

	if len(positional) == 0 {
		cli.Fatal("usage: mai-changelog new \"description\" -k <category>")
	}

	description := strings.Join(positional, " ")

	if category == "" {
		category = "chore"
	}

	tags := []string{"changelog", category}

	note, err := e.Create(notes.CreateOptions{
		Kind: "artifact",
		Type: "artifact",
		Title: description,
		Body:  description,
		Tags:  tags,
	})
	if err != nil {
		cli.Fatal("new: %v", err)
	}

	// Close immediately — artifacts are born closed
	_, err = e.Append(notes.AppendOptions{
		TargetID: note.ID,
		Kind:     "event",
		Field:    "status",
		Value:    "closed",
	})
	if err != nil {
		cli.Fatal("close: %v", err)
	}

	fmt.Printf("%s [%s] %s\n", note.ID, category, description)
}

// --- List ---

func runList(e notes.Engine, args []string) {
	entries := findEntries(e)

	if jsonOutput {
		cli.PrintJSON(entries)
		return
	}

	if len(entries) == 0 {
		fmt.Println("No changelog entries.")
		return
	}

	for _, entry := range entries {
		cat := entryCategory(entry)
		fmt.Printf("%-8s [%-10s] %s\n", entry.ID, cat, entry.Title)
	}
}

// --- Merge ---

func runMerge(e notes.Engine, args []string) {
	var outputFile string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--output", "-o":
			i++
			if i < len(args) {
				outputFile = args[i]
			}
		}
	}

	entries := findEntries(e)
	if len(entries) == 0 {
		fmt.Println("No changelog entries to merge.")
		return
	}

	md := renderChangelog(entries)

	if outputFile != "" {
		if err := os.MkdirAll(filepath.Dir(outputFile), 0755); err != nil {
			cli.Fatal("merge: %v", err)
		}
		if err := os.WriteFile(outputFile, []byte(md), 0644); err != nil {
			cli.Fatal("merge: %v", err)
		}
		fmt.Printf("Changelog written to %s (%d entries)\n", outputFile, len(entries))
	} else {
		fmt.Print(md)
	}
}

// --- Helpers ---

func findEntries(e notes.Engine) []notes.State {
	// Find all artifacts tagged "changelog"
	states, _ := e.Find(notes.FindOptions{Kind: "artifact", Tag: "changelog"})
	closedStates, _ := e.Find(notes.FindOptions{Kind: "artifact", Tag: "changelog", Status: "closed"})

	// Merge and dedupe
	seen := make(map[string]bool)
	var all []notes.State
	for _, s := range append(states, closedStates...) {
		if !seen[s.ID] {
			seen[s.ID] = true
			all = append(all, s)
		}
	}
	return all
}

func entryCategory(s notes.State) string {
	for _, tag := range s.Tags {
		if tag != "changelog" {
			return tag
		}
	}
	return "chore"
}

// --- Migrate ---

// tinychangeEntry represents a parsed .tinychange/*.md file.
type tinychangeEntry struct {
	File   string
	Author string
	Kind   string
	Body   string
}

func parseTinychangeFile(path string) (*tinychangeEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := string(data)
	lines := strings.Split(content, "\n")

	entry := &tinychangeEntry{File: filepath.Base(path)}

	// Parse header lines until "---"
	var bodyStart int
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "---" {
			bodyStart = i + 1
			break
		}
		if strings.HasPrefix(line, "- Author:") {
			entry.Author = strings.TrimSpace(strings.TrimPrefix(line, "- Author:"))
		} else if strings.HasPrefix(line, "- Kind:") {
			entry.Kind = strings.TrimSpace(strings.TrimPrefix(line, "- Kind:"))
		}
	}

	if bodyStart == 0 || bodyStart >= len(lines) {
		return nil, fmt.Errorf("no frontmatter separator")
	}

	body := strings.TrimSpace(strings.Join(lines[bodyStart:], "\n"))
	if body == "" {
		return nil, fmt.Errorf("empty body")
	}
	entry.Body = body

	if entry.Kind == "" {
		entry.Kind = "chore"
	}
	return entry, nil
}

func runMigrate(e notes.Engine, args []string) {
	dir := ".tinychange"
	dryRun := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dir":
			i++
			if i < len(args) {
				dir = args[i]
			}
		case "--dry-run", "-n":
			dryRun = true
		}
	}

	sourceDir := filepath.Join(repoPath(), dir)
	info, err := os.Stat(sourceDir)
	if err != nil || !info.IsDir() {
		cli.Fatal("migrate: %s does not exist or is not a directory", sourceDir)
	}

	matches, err := filepath.Glob(filepath.Join(sourceDir, "*.md"))
	if err != nil {
		cli.Fatal("migrate: %v", err)
	}
	if len(matches) == 0 {
		fmt.Printf("No .md files found in %s\n", sourceDir)
		return
	}

	sort.Strings(matches)

	// Batch mode: defer index rebuild until all entries are ingested.
	// Without this, migrating N entries is O(N²) — each write triggers a full
	// index rebuild including BM25 reindexing of all existing notes.
	if !dryRun {
		e.BeginBatch()
		defer e.EndBatch()
	}

	migrated := 0
	skipped := 0
	for _, path := range matches {
		entry, err := parseTinychangeFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  skip %s: %v\n", filepath.Base(path), err)
			skipped++
			continue
		}

		if dryRun {
			fmt.Printf("  would migrate [%s] %s\n", entry.Kind, firstLineOf(entry.Body))
			migrated++
			continue
		}

		note, err := e.Create(notes.CreateOptions{
			Kind:     "artifact",
			Type:     "artifact",
			Title:    firstLineOf(entry.Body),
			Body:     entry.Body,
			Assignee: entry.Author,
			Tags:     []string{"changelog", entry.Kind},
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "  error creating %s: %v\n", filepath.Base(path), err)
			skipped++
			continue
		}

		_, err = e.Append(notes.AppendOptions{
			TargetID: note.ID,
			Kind:     "event",
			Field:    "status",
			Value:    "closed",
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "  error closing %s: %v\n", note.ID, err)
		}

		migrated++
	}

	if dryRun {
		fmt.Printf("\nDry-run: would migrate %d entries from %s (%d skipped)\n", migrated, dir, skipped)
	} else {
		fmt.Printf("\nMigrated %d entries from %s (%d skipped)\n", migrated, dir, skipped)
	}
}

func firstLineOf(s string) string {
	for i, c := range s {
		if c == '\n' {
			return strings.TrimSpace(s[:i])
		}
	}
	return strings.TrimSpace(s)
}

func renderChangelog(entries []notes.State) string {
	// Group by category
	grouped := make(map[string][]notes.State)
	for _, entry := range entries {
		cat := entryCategory(entry)
		grouped[cat] = append(grouped[cat], entry)
	}

	// Sort categories
	var categories []string
	for cat := range grouped {
		categories = append(categories, cat)
	}
	sort.Strings(categories)

	var sb strings.Builder
	sb.WriteString("# Changelog\n\n")

	for _, cat := range categories {
		sb.WriteString(fmt.Sprintf("## %s\n\n", strings.Title(cat)))
		for _, entry := range grouped[cat] {
			sb.WriteString(fmt.Sprintf("- %s\n", entry.Title))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
