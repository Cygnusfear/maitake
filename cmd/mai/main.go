// Command mai is the maitake CLI — git-native notes, tickets, and reviews.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cygnusfear/maitake/pkg/git"
	"github.com/cygnusfear/maitake/pkg/notes"
)

// globalJSON is set by --json flag for machine-readable output.
var globalJSON bool

func main() {
	// Extract global flags before subcommand dispatch
	var rawArgs []string
	for i := 1; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "-C":
			i++
			if i < len(os.Args) {
				globalDir = os.Args[i]
			}
		case "--json":
			globalJSON = true
		default:
			rawArgs = append(rawArgs, os.Args[i])
		}
	}

	if len(rawArgs) < 1 {
		runList(nil, []string{})
		return
	}

	cmd := rawArgs[0]
	args := rawArgs[1:]

	switch cmd {
	case "init":
		runInit(args)
	case "sync":
		withEngine(func(e notes.Engine) { runSync(e, args) })
	case "migrate":
		withEngine(func(e notes.Engine) { runMigrate(e, args) })
	case "docs":
		if len(args) > 0 && args[0] == "sync" {
			withEngine(func(e notes.Engine) { runDocsSync(e, args[1:]) })
		} else {
			fatal("usage: mai docs sync [--dir PATH]")
		}
	case "daemon":
		runDaemon(args)
	case "check":
		withEngine(func(e notes.Engine) { runCheck(e, args) })
	case "refs":
		withEngine(func(e notes.Engine) { runRefs(e, args) })
	case "expand":
		withEngine(func(e notes.Engine) { runExpand(e, args) })
	case "create":
		withEngine(func(e notes.Engine) { runCreate(e, args) })
	case "show":
		withEngine(func(e notes.Engine) { runShow(e, args) })
	case "ls", "list":
		withEngine(func(e notes.Engine) { runList(e, args) })
	case "start":
		withEngine(func(e notes.Engine) { runLifecycle(e, "in_progress", args) })
	case "close":
		withEngine(func(e notes.Engine) { runClose(e, args) })
	case "reopen":
		withEngine(func(e notes.Engine) { runLifecycle(e, "open", args) })
	case "add-note":
		withEngine(func(e notes.Engine) { runAddNote(e, args) })
	case "tag":
		withEngine(func(e notes.Engine) { runTag(e, args) })
	case "assign":
		withEngine(func(e notes.Engine) { runAssign(e, args) })
	case "dep":
		if len(args) > 0 && args[0] == "tree" {
			withEngine(func(e notes.Engine) { runDepTree(e, args[1:]) })
		} else {
			withEngine(func(e notes.Engine) { runDep(e, args) })
		}
	case "undep":
		withEngine(func(e notes.Engine) { runUndep(e, args) })
	case "link":
		withEngine(func(e notes.Engine) { runLink(e, args) })
	case "unlink":
		withEngine(func(e notes.Engine) { runUnlink(e, args) })
	case "context":
		withEngine(func(e notes.Engine) { runContext(e, args) })
	case "kinds":
		withEngine(func(e notes.Engine) { runKinds(e) })
	case "doctor":
		withEngine(func(e notes.Engine) { runDoctor(e) })
	case "closed":
		withEngine(func(e notes.Engine) { runClosed(e, args) })
	case "ready":
		withEngine(func(e notes.Engine) { runReady(e, args) })
	case "blocked":
		withEngine(func(e notes.Engine) { runBlocked(e, args) })
	// Shortcuts
	case "ticket":
		withEngine(func(e notes.Engine) { runShortcut(e, "ticket", "task", args) })
	case "warn":
		withEngine(func(e notes.Engine) { runWarn(e, args) })
	case "review":
		withEngine(func(e notes.Engine) { runShortcut(e, "review", "", args) })
	case "artifact":
		withEngine(func(e notes.Engine) { runShortcut(e, "artifact", "artifact", args) })
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

// globalDir is set by -C flag to override working directory.
var globalDir string

func withEngine(fn func(notes.Engine)) {
	dir := globalDir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			fatal("getting working directory: %v", err)
		}
	}
	repo, err := git.NewGitRepo(dir)
	if err != nil {
		fatal("not a git repository (or any parent)")
	}
	engine, err := notes.NewEngine(repo)
	if err != nil {
		fatal("initializing engine: %v", err)
	}

	// Register this repo for daemon discovery
	registerRepo(repo.GetPath())

	fn(engine)
}

func registerRepo(repoPath string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	reposFile := filepath.Join(home, ".maitake", "repos")
	os.MkdirAll(filepath.Dir(reposFile), 0755)

	data, _ := os.ReadFile(reposFile)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == repoPath {
			return
		}
	}

	f, err := os.OpenFile(reposFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(repoPath + "\n")
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "mai: "+format+"\n", args...)
	os.Exit(1)
}

func printUsage() {
	fmt.Print(`mai — git-native notes, tickets, and reviews

Usage: mai <command> [args]

Create:
  create [title] [options]   Create a note with a generated ID
  ticket [title] [options]   Shortcut: create -k ticket -t task
  warn <path> [message]      Shortcut: create -k warning --target <path>
  review [title] [options]   Shortcut: create -k review (open, needs response)
  artifact [title] [options] Shortcut: create -k artifact -t artifact (born closed)

Lifecycle:
  start <id>                 Status → in_progress
  close <id> [-m message]    Status → closed
  reopen <id>                Status → open
  add-note <id> [text]       Append comment (--file path, --line N for file-level)
  tag <id> +tag / -tag       Add or remove tag
  assign <id> <name>         Set assignee
  dep <id> <dep-id>          Add dependency
  dep tree <id>              Show dependency tree
  undep <id> <dep-id>        Remove dependency
  link <id> <id>             Symmetric link
  unlink <id> <id>           Remove link

Query:
  show <id>                  Full note state
  ls [--status=X] [-k kind]  List notes (default: open + in_progress)
  closed [-k kind]           Recently closed notes
  context <path>             Everything about a file
  ready                      Open notes with deps resolved
  blocked                    Open notes with unresolved deps
  kinds                      List all kinds in use
  doctor                     Graph health report

Setup:
  init [--remote R] [--block H]  Create .maitake/ with hooks, config, gitignore
  sync                           Manual fetch + merge + push
  migrate [--dir PATH] [--dry-run]  Import tk .tickets/*.md into maitake

Docs:
  docs sync [--dir PATH]         Bidirectional sync: doc notes ↔ markdown files
  daemon                         Watch all repos, sync file changes to notes

Knowledge graph:
  check                          Validate all [[refs]] in notes + // @mai: in code
  refs <id>                      Find code + notes referencing a target
  expand <text>                  Resolve [[refs]] in text with note context

Create options:
  -k, --kind KIND            Note kind (ticket, warning, review, etc.)
  -t, --type TYPE            Ticket type (task, bug, feature, artifact, etc.)
  -p, --priority N           Priority 0-4
  -a, --assignee NAME        Assignee
  --tags TAG,TAG             Comma-separated tags
  --target PATH              File path this note targets
  -d, --description TEXT     Body text
  -m, --message TEXT         Alias for -d
`)
}

// parseFlags extracts known flags from args, returns remaining positional args.
type flagSet struct {
	kind     string
	typ      string
	priority int
	assignee string
	tags     []string
	targets  []string
	body     string
	status   string
	message  string
}

func parseFlags(args []string) (flagSet, []string) {
	var f flagSet
	var positional []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-k", "--kind":
			i++; if i < len(args) { f.kind = args[i] }
		case "-t", "--type":
			i++; if i < len(args) { f.typ = args[i] }
		case "-p", "--priority":
			i++; if i < len(args) { fmt.Sscanf(args[i], "%d", &f.priority) }
		case "-a", "--assignee":
			i++; if i < len(args) { f.assignee = args[i] }
		case "--tags":
			i++; if i < len(args) { f.tags = strings.Split(args[i], ",") }
		case "--target":
			i++; if i < len(args) { f.targets = append(f.targets, args[i]) }
		case "-d", "--description", "-m", "--message":
			i++; if i < len(args) { f.body = args[i] }
		case "--status":
			i++; if i < len(args) { f.status = args[i] }
		default:
			if strings.HasPrefix(args[i], "--status=") {
				f.status = strings.TrimPrefix(args[i], "--status=")
			} else if strings.HasPrefix(args[i], "-k=") {
				f.kind = strings.TrimPrefix(args[i], "-k=")
			} else {
				positional = append(positional, args[i])
			}
		}
	}
	return f, positional
}
