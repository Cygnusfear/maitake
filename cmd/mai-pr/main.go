// mai-pr is the PR plugin for maitake.
// It is discovered and dispatched by `mai pr` via .maitake/plugins.toml.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cygnusfear/maitake/internal/cli"
	"github.com/cygnusfear/maitake/pkg/git"
	"github.com/cygnusfear/maitake/pkg/notes"
)

var jsonOutput bool

func main() {
	args := os.Args[1:]

	// Strip --json / check MAI_JSON
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

	// Help
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		printHelp()
		return
	}

	e, repo := initEngine()

	if len(args) == 0 {
		runList(e)
		return
	}

	switch args[0] {
	case "show":
		runShow(e, repo, args[1:])
	case "accept":
		runAccept(e, args[1:])
	case "reject":
		runReject(e, args[1:])
	case "submit":
		runSubmit(e, repo, args[1:])
	case "diff":
		runDiff(e, repo, args[1:])
	case "comment":
		runComment(e, args[1:])
	default:
		// Bare text = create
		runCreate(e, args)
	}
}

func printHelp() {
	fmt.Print(`mai-pr — git-native pull requests

Usage: mai-pr [subcommand] [args]

  mai-pr "title" --into main    Create a PR
  mai-pr                        List PRs
  mai-pr show <id> [--diff]     Show PR details
  mai-pr diff <id> [--stat]     Show diff
  mai-pr accept <id> [-m msg]   Accept PR
  mai-pr reject <id> -m reason  Request changes
  mai-pr submit <id> [--force]  Merge and close PR
  mai-pr comment <id> -m msg    Add comment

Flags:
  --into <branch>   Target branch (default: main)
  --json            JSON output
  -d, --description Body text
  -a, --assignee    Reviewer
  -l, --tags        Tags
  --force           Skip unresolved comment check
`)
}

func initEngine() (notes.Engine, git.Repo) {
	dir := os.Getenv("MAI_REPO_PATH")
	if dir == "" {
		dir, _ = os.Getwd()
		// Walk up to find .git
		for {
			if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				cli.Fatal("not a git repository")
			}
			dir = parent
		}
	}

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

// --- PR helpers ---

func prBranches(s *notes.State) (from, into string) {
	if len(s.Targets) >= 2 {
		return s.Targets[0], s.Targets[1]
	}
	if len(s.Targets) == 1 {
		return s.Targets[0], "main"
	}
	return "?", "?"
}

func prResolvedStatus(s *notes.State) *bool {
	var last *bool
	for _, c := range s.Comments {
		if c.Resolved != nil {
			last = c.Resolved
		}
	}
	return last
}

// --- Commands ---

func runCreate(e notes.Engine, args []string) {
	intoBranch := "main"
	var filteredArgs []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--into" && i+1 < len(args) {
			intoBranch = args[i+1]
			i++
		} else {
			filteredArgs = append(filteredArgs, args[i])
		}
	}

	f, pos := cli.ParseFlags(filteredArgs)

	title := f.Title
	if title == "" && len(pos) > 0 {
		title = pos[0]
	}

	fromBranch := e.GitBranch()
	if fromBranch == "" || fromBranch == "HEAD" {
		cli.Fatal("pr: not on a branch (detached HEAD)")
	}
	if fromBranch == intoBranch {
		cli.Fatal("pr: source and target are the same branch (%s)", fromBranch)
	}
	if title == "" {
		title = fmt.Sprintf("%s → %s", fromBranch, intoBranch)
	}

	body := f.Body
	if body == "" {
		body = fmt.Sprintf("# %s\n\nMerge `%s` into `%s`.", title, fromBranch, intoBranch)
	}

	note, err := e.Create(notes.CreateOptions{
		Kind:     "pr",
		Title:    title,
		Body:     body,
		Priority: f.Priority,
		Assignee: f.Assignee,
		Tags:     f.Tags,
		Targets:  []string{fromBranch, intoBranch},
	})
	if err != nil {
		cli.Fatal("pr: %v", err)
	}

	fmt.Printf("%s  %s → %s\n", note.ID, fromBranch, intoBranch)
}

func runList(e notes.Engine) {
	states, _ := e.Find(notes.FindOptions{Kind: "pr"})
	if len(states) == 0 {
		fmt.Println("No open PRs.")
		return
	}

	for i := range states {
		s := &states[i]
		from, into := prBranches(s)
		merged := e.IsMerged(from, into)

		if merged && s.Status != "closed" {
			_, err := e.Append(notes.AppendOptions{
				TargetID: s.ID,
				Kind:     "event",
				Field:    "status",
				Value:    "closed",
				Body:     "Automatically closed: branch merged into target",
			})
			if err == nil {
				s.Status = "closed"
			}
		}

		mergedStr := ""
		if merged {
			mergedStr = " ✓ merged"
		}

		if jsonOutput {
			continue
		}
		fmt.Printf("%s [%-11s] %s → %s%s  %s\n", s.ID, s.Status, from, into, mergedStr, s.Title)
	}

	if jsonOutput {
		cli.PrintJSON(states)
	}
}

func runShow(e notes.Engine, repo git.Repo, args []string) {
	if len(args) < 1 {
		cli.Fatal("usage: mai-pr show <id> [--diff]")
	}
	showDiff := false
	id := args[0]
	for _, a := range args[1:] {
		if a == "--diff" {
			showDiff = true
		}
	}

	state, err := e.Fold(id)
	if err != nil {
		cli.Fatal("pr show: %v", err)
	}
	if state.Kind != "pr" {
		cli.Fatal("pr show: %s is not a PR (kind: %s)", id, state.Kind)
	}

	if jsonOutput {
		cli.PrintJSON(state)
		return
	}

	from, into := prBranches(state)
	merged := e.IsMerged(from, into)

	fmt.Printf("%s [%s] %s\n", state.ID, state.Status, state.Title)
	fmt.Printf("  %s → %s", from, into)
	if merged {
		fmt.Print("  ✓ merged")
	}
	fmt.Println()

	if state.Assignee != "" {
		fmt.Printf("  reviewer: %s\n", state.Assignee)
	}
	if len(state.Tags) > 0 {
		fmt.Printf("  tags: %s\n", strings.Join(state.Tags, ", "))
	}

	verdict := prResolvedStatus(state)
	if verdict != nil {
		if *verdict {
			fmt.Println("  review: ✅ accepted")
		} else {
			fmt.Println("  review: ❌ changes requested")
		}
	} else {
		fmt.Println("  review: ⏳ pending")
	}

	if !state.CreatedAt.IsZero() {
		fmt.Printf("  created: %s\n", state.CreatedAt.Format("2006-01-02 15:04"))
	}

	diffStat, err := repo.Diff(into, from, "--stat")
	if err == nil && diffStat != "" {
		fmt.Println()
		fmt.Println("## Diff")
		fmt.Println(diffStat)
	}

	if state.Body != "" {
		fmt.Println()
		fmt.Println(state.Body)
	}

	if len(state.Comments) > 0 {
		fmt.Println()
		fmt.Println("## Comments")
		for _, c := range state.Comments {
			author := c.Author
			if author == "" {
				author = "anonymous"
			}
			ts := c.Timestamp
			resolvedMark := ""
			if c.Resolved != nil {
				if *c.Resolved {
					resolvedMark = " ✅"
				} else {
					resolvedMark = " ❌"
				}
			}
			loc := ""
			if c.Location != nil && c.Location.Path != "" {
				loc = fmt.Sprintf(" 📌 %s", c.Location.Path)
				if c.Location.Range != nil && c.Location.Range.StartLine > 0 {
					loc = fmt.Sprintf(" 📌 %s:%d", c.Location.Path, c.Location.Range.StartLine)
				}
			}
			fmt.Printf("\n**%s** (%s)%s%s\n", ts, author, resolvedMark, loc)
			if c.Body != "" {
				fmt.Println(c.Body)
			}
		}
	}

	if showDiff {
		fullDiff, err := repo.Diff(into, from)
		if err == nil && fullDiff != "" {
			fmt.Println()
			fmt.Println("## Full Diff")
			fmt.Println(fullDiff)
		}
	}
}

func runAccept(e notes.Engine, args []string) {
	if len(args) < 1 {
		cli.Fatal("usage: mai-pr accept <id> [-m message]")
	}
	id := args[0]
	f, _ := cli.ParseFlags(args[1:])

	state, err := e.Fold(id)
	if err != nil {
		cli.Fatal("pr accept: %v", err)
	}
	if state.Kind != "pr" {
		cli.Fatal("pr accept: %s is not a PR (kind: %s)", id, state.Kind)
	}

	body := f.Body
	if body == "" {
		body = "LGTM"
	}
	resolved := true
	_, err = e.Append(notes.AppendOptions{
		TargetID: id,
		Kind:     "comment",
		Body:     body,
		Resolved: &resolved,
	})
	if err != nil {
		cli.Fatal("pr accept: %v", err)
	}

	from, into := prBranches(state)
	fmt.Printf("PR %s accepted. %s → %s ready to merge.\n", id, from, into)
}

func runReject(e notes.Engine, args []string) {
	if len(args) < 1 {
		cli.Fatal("usage: mai-pr reject <id> -m 'reason'")
	}
	id := args[0]
	f, _ := cli.ParseFlags(args[1:])

	state, err := e.Fold(id)
	if err != nil {
		cli.Fatal("pr reject: %v", err)
	}
	if state.Kind != "pr" {
		cli.Fatal("pr reject: %s is not a PR (kind: %s)", id, state.Kind)
	}
	if f.Body == "" {
		cli.Fatal("pr reject: reason required (-m 'reason')")
	}

	resolved := false
	_, err = e.Append(notes.AppendOptions{
		TargetID: id,
		Kind:     "comment",
		Body:     f.Body,
		Resolved: &resolved,
	})
	if err != nil {
		cli.Fatal("pr reject: %v", err)
	}
	fmt.Printf("Changes requested on %s: %s\n", id, f.Body)
}

func runSubmit(e notes.Engine, repo git.Repo, args []string) {
	if len(args) < 1 {
		cli.Fatal("usage: mai-pr submit <id> [--force]")
	}
	id := args[0]
	force := false
	for _, a := range args[1:] {
		if a == "--force" {
			force = true
		}
	}

	state, err := e.Fold(id)
	if err != nil {
		cli.Fatal("pr submit: %v", err)
	}
	if state.Kind != "pr" {
		cli.Fatal("pr submit: %s is not a PR (kind: %s)", id, state.Kind)
	}
	if state.Status == "closed" {
		cli.Fatal("pr submit: %s is already closed", id)
	}

	from, into := prBranches(state)

	if !force {
		for _, c := range state.Comments {
			if c.Resolved != nil && !*c.Resolved {
				cli.Fatal("pr submit: unresolved comments exist (use --force to override)")
			}
		}
	}

	if e.IsMerged(from, into) {
		_, err = e.Append(notes.AppendOptions{
			TargetID: id,
			Kind:     "event",
			Field:    "status",
			Value:    "closed",
			Body:     fmt.Sprintf("Merged %s into %s. PR closed.", from, into),
		})
		if err != nil {
			cli.Fatal("pr submit: %v", err)
		}
		fmt.Printf("Already merged. %s → closed.\n", id)
		return
	}

	if err := repo.SwitchToRef(into); err != nil {
		cli.Fatal("pr submit: checkout %s: %v", into, err)
	}

	mergeMsg := fmt.Sprintf("Merge %s into %s\n\nPR: %s — %s", from, into, id, state.Title)
	if err := repo.MergeRef(from, false, mergeMsg); err != nil {
		cli.Fatal("pr submit: merge failed: %v", err)
	}

	_, err = e.Append(notes.AppendOptions{
		TargetID: id,
		Kind:     "event",
		Field:    "status",
		Value:    "closed",
		Body:     fmt.Sprintf("Merged %s into %s. PR closed.", from, into),
	})
	if err != nil {
		cli.Fatal("pr submit: close note: %v", err)
	}
	fmt.Printf("Merged %s into %s. %s → closed.\n", from, into, id)
}

func runDiff(e notes.Engine, repo git.Repo, args []string) {
	if len(args) < 1 {
		cli.Fatal("usage: mai-pr diff <id> [--stat]")
	}
	id := args[0]
	statOnly := false
	for _, a := range args[1:] {
		if a == "--stat" {
			statOnly = true
		}
	}

	state, err := e.Fold(id)
	if err != nil {
		cli.Fatal("pr diff: %v", err)
	}
	if state.Kind != "pr" {
		cli.Fatal("pr diff: %s is not a PR (kind: %s)", id, state.Kind)
	}

	from, into := prBranches(state)
	var diffArgs []string
	if statOnly {
		diffArgs = append(diffArgs, "--stat")
	}

	diff, err := repo.Diff(into, from, diffArgs...)
	if err != nil {
		cli.Fatal("pr diff: %v", err)
	}
	if diff == "" {
		fmt.Println("No differences.")
		return
	}
	fmt.Print(diff)
}

func runComment(e notes.Engine, args []string) {
	if len(args) < 1 {
		cli.Fatal("usage: mai-pr comment <id> -m 'message' [--file path] [--line N]")
	}

	id := args[0]
	remaining := args[1:]

	var body, filePath string
	var startLine, endLine uint32
	for i := 0; i < len(remaining); i++ {
		switch remaining[i] {
		case "-m", "--message", "-d", "--description":
			i++
			if i < len(remaining) {
				body = remaining[i]
			}
		case "--file", "-f":
			i++
			if i < len(remaining) {
				filePath = remaining[i]
			}
		case "--line":
			i++
			if i < len(remaining) {
				fmt.Sscanf(remaining[i], "%d", &startLine)
			}
		case "--end-line":
			i++
			if i < len(remaining) {
				fmt.Sscanf(remaining[i], "%d", &endLine)
			}
		}
	}

	opts := notes.AppendOptions{
		TargetID: id,
		Kind:     "comment",
		Body:     body,
	}
	if filePath != "" {
		opts.Location = &notes.Location{Path: filePath}
		if startLine > 0 {
			opts.Location.Range = &notes.Range{StartLine: startLine, EndLine: endLine}
		}
	}

	_, err := e.Append(opts)
	if err != nil {
		cli.Fatal("pr comment: %v", err)
	}
	if filePath != "" {
		fmt.Printf("Comment added to %s on %s\n", id, filePath)
	} else {
		fmt.Printf("Comment added to %s\n", id)
	}
}
