package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/cygnusfear/maitake/pkg/guard"
	"github.com/cygnusfear/maitake/pkg/notes"
)

func runInit(args []string) {
	cwd, _ := os.Getwd()
	maitakeDir := cwd + "/.maitake"
	if err := guard.InitHooks(maitakeDir); err != nil {
		fatal("init: %v", err)
	}
	fmt.Println("Initialized .maitake/hooks/")
}

func runCreate(e notes.Engine, args []string) {
	f, pos := parseFlags(args)

	title := ""
	if len(pos) > 0 {
		title = pos[0]
	}

	if f.kind == "" {
		f.kind = "ticket"
	}

	body := f.body
	if body == "" && title != "" {
		body = "# " + title
	}

	note, err := e.Create(notes.CreateOptions{
		Kind:     f.kind,
		Title:    title,
		Type:     f.typ,
		Priority: f.priority,
		Assignee: f.assignee,
		Tags:     f.tags,
		Body:     body,
		Targets:  f.targets,
	})
	if err != nil {
		fatal("create: %v", err)
	}
	fmt.Println(note.ID)
}

func runShow(e notes.Engine, args []string) {
	if len(args) < 1 {
		fatal("usage: mai show <id>")
	}
	state, err := e.Fold(args[0])
	if err != nil {
		fatal("show: %v", err)
	}
	printState(state)
}

func runList(e notes.Engine, args []string) {
	if e == nil {
		withEngine(func(eng notes.Engine) { runList(eng, args) })
		return
	}

	f, _ := parseFlags(args)
	opts := notes.ListOptions{
		FindOptions: notes.FindOptions{
			Kind:   f.kind,
			Status: f.status,
			Tag:    "",
		},
		SortBy: "created",
	}
	if len(f.tags) > 0 {
		opts.FindOptions.Tag = f.tags[0]
	}

	summaries, err := e.List(opts)
	if err != nil {
		fatal("ls: %v", err)
	}

	for _, s := range summaries {
		printSummaryLine(s)
	}
}

func runLifecycle(e notes.Engine, status string, args []string) {
	if len(args) < 1 {
		fatal("usage: mai %s <id>", status)
	}
	_, err := e.Append(notes.AppendOptions{
		TargetID: args[0],
		Kind:     "event",
		Field:    "status",
		Value:    status,
	})
	if err != nil {
		fatal("%v", err)
	}
	state, _ := e.Fold(args[0])
	if state != nil {
		fmt.Printf("%s → %s\n", state.ID, state.Status)
	}
}

func runClose(e notes.Engine, args []string) {
	if len(args) < 1 {
		fatal("usage: mai close <id> [-m message]")
	}
	id := args[0]
	f, _ := parseFlags(args[1:])

	_, err := e.Append(notes.AppendOptions{
		TargetID: id,
		Kind:     "event",
		Field:    "status",
		Value:    "closed",
		Body:     f.body,
	})
	if err != nil {
		fatal("close: %v", err)
	}
	state, _ := e.Fold(id)
	if state != nil {
		fmt.Printf("%s → closed\n", state.ID)
	}
}

func runAddNote(e notes.Engine, args []string) {
	if len(args) < 1 {
		fatal("usage: mai add-note <id> [--file path] [--line N] [text]")
	}
	id := args[0]
	remaining := args[1:]

	// Parse --file and --line flags
	var filePath string
	var startLine, endLine uint32
	var positional []string
	for i := 0; i < len(remaining); i++ {
		switch remaining[i] {
		case "--file", "-f":
			i++
			if i < len(remaining) {
				filePath = remaining[i]
			}
		case "--line", "-l":
			i++
			if i < len(remaining) {
				fmt.Sscanf(remaining[i], "%d", &startLine)
			}
		case "--end-line":
			i++
			if i < len(remaining) {
				fmt.Sscanf(remaining[i], "%d", &endLine)
			}
		default:
			positional = append(positional, remaining[i])
		}
	}

	body := ""
	if len(positional) > 0 {
		body = strings.Join(positional, " ")
	} else {
		buf, _ := os.ReadFile("/dev/stdin")
		body = string(buf)
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
		fatal("add-note: %v", err)
	}
	if filePath != "" {
		fmt.Printf("Comment added to %s on %s\n", id, filePath)
	} else {
		fmt.Printf("Comment added to %s\n", id)
	}
}

func runTag(e notes.Engine, args []string) {
	if len(args) < 2 {
		fatal("usage: mai tag <id> +tag / -tag")
	}
	_, err := e.Append(notes.AppendOptions{
		TargetID: args[0],
		Kind:     "event",
		Field:    "tags",
		Value:    args[1],
	})
	if err != nil {
		fatal("tag: %v", err)
	}
	fmt.Printf("Tagged %s: %s\n", args[0], args[1])
}

func runAssign(e notes.Engine, args []string) {
	if len(args) < 2 {
		fatal("usage: mai assign <id> <name>")
	}
	_, err := e.Append(notes.AppendOptions{
		TargetID: args[0],
		Kind:     "event",
		Field:    "assignee",
		Value:    args[1],
	})
	if err != nil {
		fatal("assign: %v", err)
	}
	fmt.Printf("%s assigned to %s\n", args[0], args[1])
}

func runDep(e notes.Engine, args []string) {
	if len(args) < 2 {
		fatal("usage: mai dep <id> <dep-id>")
	}
	_, err := e.Append(notes.AppendOptions{
		TargetID: args[0],
		Kind:     "event",
		Field:    "deps",
		Value:    "+" + args[1],
	})
	if err != nil {
		fatal("dep: %v", err)
	}
	fmt.Printf("%s depends on %s\n", args[0], args[1])
}

func runLink(e notes.Engine, args []string) {
	if len(args) < 2 {
		fatal("usage: mai link <id> <id>")
	}
	// Symmetric — add link on both
	e.Append(notes.AppendOptions{
		TargetID: args[0],
		Kind:     "event",
		Field:    "links",
		Value:    "+" + args[1],
	})
	e.Append(notes.AppendOptions{
		TargetID: args[1],
		Kind:     "event",
		Field:    "links",
		Value:    "+" + args[0],
	})
	fmt.Printf("%s ↔ %s\n", args[0], args[1])
}

func runContext(e notes.Engine, args []string) {
	if len(args) < 1 {
		fatal("usage: mai context <path>")
	}
	states, err := e.Context(args[0])
	if err != nil {
		fatal("context: %v", err)
	}
	if len(states) == 0 {
		fmt.Printf("No notes on %s\n", args[0])
		return
	}
	fmt.Printf("=== %s ===\n\n", args[0])
	for _, s := range states {
		printContextLine(&s, args[0])
	}
}

func runKinds(e notes.Engine) {
	kinds, err := e.Kinds()
	if err != nil {
		fatal("kinds: %v", err)
	}
	for _, k := range kinds {
		fmt.Printf("%-20s %d\n", k.Kind, k.Count)
	}
}

func runDoctor(e notes.Engine) {
	report, err := e.Doctor()
	if err != nil {
		fatal("doctor: %v", err)
	}
	fmt.Printf("Notes:       %d\n", report.TotalNotes)
	fmt.Printf("  Creation:  %d\n", report.CreationNotes)
	fmt.Printf("  Events:    %d\n", report.Events)
	fmt.Printf("  Comments:  %d\n", report.Comments)
	if report.BrokenEdges > 0 {
		fmt.Printf("  Broken:    %d ⚠\n", report.BrokenEdges)
	}
	fmt.Println()
	fmt.Println("By kind:")
	for kind, count := range report.ByKind {
		fmt.Printf("  %-16s %d\n", kind, count)
	}
	fmt.Println()
	fmt.Println("By status:")
	for status, count := range report.ByStatus {
		fmt.Printf("  %-16s %d\n", status, count)
	}
}

func runReady(e notes.Engine, args []string) {
	states, err := e.Find(notes.FindOptions{Status: "open"})
	if err != nil {
		fatal("ready: %v", err)
	}
	for _, s := range states {
		if allDepsResolved(e, &s) {
			printSummaryFromState(&s)
		}
	}
	// Also show in_progress
	states, _ = e.Find(notes.FindOptions{Status: "in_progress"})
	for _, s := range states {
		if allDepsResolved(e, &s) {
			printSummaryFromState(&s)
		}
	}
}

func runBlocked(e notes.Engine, args []string) {
	states, err := e.Find(notes.FindOptions{Status: "open"})
	if err != nil {
		fatal("blocked: %v", err)
	}
	for _, s := range states {
		if !allDepsResolved(e, &s) {
			printSummaryFromState(&s)
		}
	}
	states, _ = e.Find(notes.FindOptions{Status: "in_progress"})
	for _, s := range states {
		if !allDepsResolved(e, &s) {
			printSummaryFromState(&s)
		}
	}
}

func allDepsResolved(e notes.Engine, s *notes.State) bool {
	for _, dep := range s.Deps {
		depState, err := e.Fold(dep)
		if err != nil || depState.Status != "closed" {
			return false
		}
	}
	return true
}

// Shortcuts

func runShortcut(e notes.Engine, kind, typ string, args []string) {
	// Prepend kind and type flags
	newArgs := []string{"-k", kind, "-t", typ}
	newArgs = append(newArgs, args...)
	runCreate(e, newArgs)
}

func runWarn(e notes.Engine, args []string) {
	if len(args) < 1 {
		fatal("usage: mai warn <path> [message]")
	}
	target := args[0]
	body := ""
	if len(args) > 1 {
		body = strings.Join(args[1:], " ")
	}
	note, err := e.Create(notes.CreateOptions{
		Kind:    "warning",
		Body:    body,
		Targets: []string{target},
	})
	if err != nil {
		fatal("warn: %v", err)
	}
	fmt.Println(note.ID)
}
