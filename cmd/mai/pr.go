package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/cygnusfear/maitake/pkg/git"
	"github.com/cygnusfear/maitake/pkg/notes"
)

// prBranches extracts source and target branches from a PR note's targets.
func prBranches(s *notes.State) (from, into string) {
	if len(s.Targets) >= 2 {
		return s.Targets[0], s.Targets[1]
	}
	if len(s.Targets) == 1 {
		return s.Targets[0], "main"
	}
	return "?", "?"
}

// prResolvedStatus returns the aggregate review verdict for a PR.
// Returns nil if no review comments, *true if accepted, *false if rejected.
func prResolvedStatus(s *notes.State) *bool {
	var last *bool
	for _, c := range s.Comments {
		if c.Resolved != nil {
			last = c.Resolved
		}
	}
	return last
}

func runPRCreate(e notes.Engine, args []string) {
	// Extract --into before parseFlags (PR-specific flag)
	intoBranch := "main"
	var filteredArgs []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--into" && i+1 < len(args) {
			intoBranch = args[i+1]
			i++ // skip value
		} else {
			filteredArgs = append(filteredArgs, args[i])
		}
	}

	f, pos := parseFlags(filteredArgs)

	if f.help {
		fmt.Fprintln(os.Stderr, `Usage: mai pr [title] [flags]

Create a pull request note for the current branch.

Flags:
      --into <branch>   Target branch (default: main)
  -d, --description     PR description
  -a, --assignee        Reviewer
  -l, --tags            Tags

Examples:
  mai pr "Add auth middleware" --into main
  mai pr "Fix login" -a reviewer -d "Fixes the token refresh race"`)
		return
	}

	title := f.title
	if title == "" && len(pos) > 0 {
		title = pos[0]
	}

	// Detect source branch from HEAD
	fromBranch := e.GitBranch()
	if fromBranch == "" || fromBranch == "HEAD" {
		fatal("pr: not on a branch (detached HEAD)")
	}

	if fromBranch == intoBranch {
		fatal("pr: source and target are the same branch (%s)", fromBranch)
	}

	if title == "" {
		title = fmt.Sprintf("%s → %s", fromBranch, intoBranch)
	}

	body := f.body
	if body == "" {
		body = fmt.Sprintf("# %s\n\nMerge `%s` into `%s`.", title, fromBranch, intoBranch)
	}

	// Create the PR note
	note, err := e.Create(notes.CreateOptions{
		Kind:     "pr",
		Title:    title,
		Body:     body,
		Priority: f.priority,
		Assignee: f.assignee,
		Tags:     f.tags,
		Targets:  []string{fromBranch, intoBranch},
	})
	if err != nil {
		fatal("pr: %v", err)
	}

	fmt.Printf("%s  %s → %s\n", note.ID, fromBranch, intoBranch)
}

func runPRList(e notes.Engine) {
	states, _ := e.Find(notes.FindOptions{Kind: "pr"})

	if len(states) == 0 {
		fmt.Println("No open PRs.")
		return
	}

	for i := range states {
		s := &states[i]
		from, into := prBranches(s)
		merged := e.IsMerged(from, into)

		// Auto-close: if merged but still open, close it
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

		if globalJSON {
			continue // handled below
		}
		fmt.Printf("%s [%-11s] %s → %s%s  %s\n", s.ID, s.Status, from, into, mergedStr, s.Title)
	}

	if globalJSON {
		printJSON(states)
	}
}

func runPRShow(e notes.Engine, repo git.Repo, args []string) {
	if len(args) < 1 {
		fatal("usage: mai pr show <id> [--diff]")
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
		fatal("pr show: %v", err)
	}
	if state.Kind != "pr" {
		fatal("pr show: %s is not a PR (kind: %s)", id, state.Kind)
	}

	if globalJSON {
		printJSON(state)
		return
	}

	from, into := prBranches(state)
	merged := e.IsMerged(from, into)

	// Header
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

	// Review verdict
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

	// Diff summary
	diffStat, err := repo.Diff(into, from, "--stat")
	if err == nil && diffStat != "" {
		fmt.Println()
		fmt.Println("## Diff")
		fmt.Println(diffStat)
	}

	// Body
	if state.Body != "" {
		fmt.Println()
		fmt.Println(state.Body)
	}

	// Comments
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

	// Full diff if requested
	if showDiff {
		fullDiff, err := repo.Diff(into, from)
		if err == nil && fullDiff != "" {
			fmt.Println()
			fmt.Println("## Full Diff")
			fmt.Println(fullDiff)
		}
	}
}

func runPRAccept(e notes.Engine, args []string) {
	if len(args) < 1 {
		fatal("usage: mai pr accept <id> [-m message]")
	}

	id := args[0]
	f, _ := parseFlags(args[1:])

	// Verify it's a PR
	state, err := e.Fold(id)
	if err != nil {
		fatal("pr accept: %v", err)
	}
	if state.Kind != "pr" {
		fatal("pr accept: %s is not a PR (kind: %s)", id, state.Kind)
	}

	body := f.body
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
		fatal("pr accept: %v", err)
	}

	from, into := prBranches(state)
	fmt.Printf("PR %s accepted. %s → %s ready to merge.\n", id, from, into)
}

func runPRReject(e notes.Engine, args []string) {
	if len(args) < 1 {
		fatal("usage: mai pr reject <id> -m 'reason'")
	}

	id := args[0]
	f, _ := parseFlags(args[1:])

	// Verify it's a PR
	state, err := e.Fold(id)
	if err != nil {
		fatal("pr reject: %v", err)
	}
	if state.Kind != "pr" {
		fatal("pr reject: %s is not a PR (kind: %s)", id, state.Kind)
	}

	if f.body == "" {
		fatal("pr reject: reason required (-m 'reason')")
	}

	resolved := false
	_, err = e.Append(notes.AppendOptions{
		TargetID: id,
		Kind:     "comment",
		Body:     f.body,
		Resolved: &resolved,
	})
	if err != nil {
		fatal("pr reject: %v", err)
	}

	fmt.Printf("Changes requested on %s: %s\n", id, f.body)
}

func runPRSubmit(e notes.Engine, repo git.Repo, args []string) {
	if len(args) < 1 {
		fatal("usage: mai pr submit <id> [--force]")
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
		fatal("pr submit: %v", err)
	}
	if state.Kind != "pr" {
		fatal("pr submit: %s is not a PR (kind: %s)", id, state.Kind)
	}
	if state.Status == "closed" {
		fatal("pr submit: %s is already closed", id)
	}

	from, into := prBranches(state)

	// Check for unresolved comments (unless --force)
	if !force {
		for _, c := range state.Comments {
			if c.Resolved != nil && !*c.Resolved {
				fatal("pr submit: unresolved comments exist (use --force to override)")
			}
		}
	}

	// If already merged, just close
	if e.IsMerged(from, into) {
		_, err = e.Append(notes.AppendOptions{
			TargetID: id,
			Kind:     "event",
			Field:    "status",
			Value:    "closed",
			Body:     fmt.Sprintf("Merged %s into %s. PR closed.", from, into),
		})
		if err != nil {
			fatal("pr submit: %v", err)
		}
		fmt.Printf("Already merged. %s → closed.\n", id)
		return
	}

	// Switch to target branch and merge source
	if err := repo.SwitchToRef(into); err != nil {
		fatal("pr submit: checkout %s: %v", into, err)
	}

	mergeMsg := fmt.Sprintf("Merge %s into %s\n\nPR: %s — %s", from, into, id, state.Title)
	if err := repo.MergeRef(from, false, mergeMsg); err != nil {
		fatal("pr submit: merge failed: %v", err)
	}

	// Close the PR note
	_, err = e.Append(notes.AppendOptions{
		TargetID: id,
		Kind:     "event",
		Field:    "status",
		Value:    "closed",
		Body:     fmt.Sprintf("Merged %s into %s. PR closed.", from, into),
	})
	if err != nil {
		fatal("pr submit: close note: %v", err)
	}

	fmt.Printf("Merged %s into %s. %s → closed.\n", from, into, id)
}

func runPRDiff(e notes.Engine, repo git.Repo, args []string) {
	if len(args) < 1 {
		fatal("usage: mai pr diff <id> [--stat]")
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
		fatal("pr diff: %v", err)
	}
	if state.Kind != "pr" {
		fatal("pr diff: %s is not a PR (kind: %s)", id, state.Kind)
	}

	from, into := prBranches(state)

	var diffArgs []string
	if statOnly {
		diffArgs = append(diffArgs, "--stat")
	}

	diff, err := repo.Diff(into, from, diffArgs...)
	if err != nil {
		fatal("pr diff: %v", err)
	}
	if diff == "" {
		fmt.Println("No differences.")
		return
	}
	fmt.Print(diff)
}

func runPRComment(e notes.Engine, args []string) {
	if len(args) < 1 {
		fatal("usage: mai pr comment <id> -m 'message' [--file path] [--line N]")
	}

	// Delegate to runAddNote — same behavior
	runAddNote(e, args)
}
