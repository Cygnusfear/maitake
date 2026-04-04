// Command mai is the maitake CLI — git-native notes, tickets, and reviews.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/cygnusfear/maitake/pkg/docs"
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

	if cmd == "-h" || cmd == "--help" || cmd == "help" {
		printUsage()
		return
	}

	switch cmd {
	case "init":
		runInit(args)
	case "sync":
		withEngine(func(e notes.Engine) { runSync(e, args) })
	case "migrate":
		withEngine(func(e notes.Engine) { runMigrate(e, args) })
	case "docs":
		if !dispatchPlugin("docs", args) {
			fatal("mai-docs not found. Install: go install github.com/cygnusfear/maitake/cmd/mai-docs@latest")
		}
	case "daemon":
		if !dispatchPlugin("docs", append([]string{"daemon"}, args...)) {
			fatal("mai-docs not found. Install: go install github.com/cygnusfear/maitake/cmd/mai-docs@latest")
		}
	case "check":
		if !dispatchPlugin("docs", append([]string{"check"}, args...)) {
			fatal("mai-docs not found. Install: go install github.com/cygnusfear/maitake/cmd/mai-docs@latest")
		}
	case "refs":
		if !dispatchPlugin("docs", append([]string{"refs"}, args...)) {
			fatal("mai-docs not found. Install: go install github.com/cygnusfear/maitake/cmd/mai-docs@latest")
		}
	case "expand":
		if !dispatchPlugin("docs", append([]string{"expand"}, args...)) {
			fatal("mai-docs not found. Install: go install github.com/cygnusfear/maitake/cmd/mai-docs@latest")
		}
	case "create":
		withEngine(func(e notes.Engine) { runCreate(e, args) })
	case "show":
		withEngine(func(e notes.Engine) { runShow(e, args) })
	case "search":
		withEngine(func(e notes.Engine) { runSearch(e, args) })
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
		withEngine(func(e notes.Engine) { runDoctor(e, args) })
	case "closed":
		withEngine(func(e notes.Engine) { runClosed(e, args) })
	case "ready":
		withEngine(func(e notes.Engine) { runReady(e, args) })
	case "blocked":
		withEngine(func(e notes.Engine) { runBlocked(e, args) })
	case "priority":
		withEngine(func(e notes.Engine) { runPriority(e, args) })
	case "title":
		withEngine(func(e notes.Engine) { runTitle(e, args) })
	case "type":
		withEngine(func(e notes.Engine) { runType(e, args) })
	case "edit":
		withEngine(func(e notes.Engine) { runEdit(e, args) })
	// Shortcuts
	case "ticket":
		withEngine(func(e notes.Engine) { runShortcut(e, "ticket", "task", args) })
	case "warn":
		withEngine(func(e notes.Engine) { runWarn(e, args) })
	case "review":
		withEngine(func(e notes.Engine) { runShortcut(e, "review", "", args) })
	case "artifact":
		withEngine(func(e notes.Engine) { runShortcut(e, "artifact", "artifact", args) })
	case "adr":
		if len(args) == 0 {
			withEngine(func(e notes.Engine) { runList(e, []string{"-k", "decision"}) })
		} else {
			withEngine(func(e notes.Engine) { runShortcut(e, "decision", "", args) })
		}
	case "pr":
		if !dispatchPlugin("pr", args) {
			fatal("mai-pr not found. Install: go install github.com/cygnusfear/maitake/cmd/mai-pr@latest")
		}
	case "help", "--help", "-h":
		printUsage()
	default:
		// Plugin dispatch: check .maitake/plugins.toml → exec binary
		if dispatchPlugin(cmd, args) {
			return
		}
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

	// Register doc auto-sync hooks (CRDT + disk materialization)
	docs.RegisterAutoSync(engine)

	// Register this repo for daemon discovery
	registerRepo(repo.GetPath())

	// Auto-start daemon if any repo has docs.watch enabled
	ensureDaemon()

	fn(engine)
}

// dispatchPR and withEngineAndRepo removed — PR logic lives in cmd/mai-pr/

func ensureDaemon() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	pidFile := filepath.Join(home, ".maitake", "daemon.pid")

	// Check if already running — fast PID check only
	if data, err := os.ReadFile(pidFile); err == nil {
		pid := strings.TrimSpace(string(data))
		if pid != "" && checkPidAlive(pid) {
			return // already running
		}
	}

	// Spawn daemon — it decides what to watch on startup (not us)
	exe, err := os.Executable()
	if err != nil {
		return
	}

	logFile := filepath.Join(home, ".maitake", "daemon.log")
	os.MkdirAll(filepath.Dir(logFile), 0755)
	out, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}

	proc, err := os.StartProcess(exe, []string{exe, "daemon"}, &os.ProcAttr{
		Dir:   "/",
		Files: []*os.File{nil, out, out},
		Sys:   daemonSysProcAttr(),
	})
	if err != nil {
		out.Close()
		return
	}

	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", proc.Pid)), 0644)
	proc.Release()
	out.Close()
}

func checkPidAlive(pid string) bool {
	// Use os.FindProcess + signal 0 (works on macOS and Linux)
	var p int
	if _, err := fmt.Sscanf(pid, "%d", &p); err != nil {
		return false
	}
	proc, err := os.FindProcess(p)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

func registerRepo(repoPath string) {
	// Don't register temp dirs (test repos, CI, etc.)
	if strings.Contains(repoPath, "/tmp/") || strings.Contains(repoPath, "/private/tmp/") ||
		strings.Contains(repoPath, os.TempDir()) {
		return
	}

	// If this is a worktree, register the main repo instead.
	// Worktrees have a .git file (not dir) pointing at the main repo.
	repoPath = resolveMainRepo(repoPath)

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

// resolveMainRepo returns the main repo path if repoPath is a worktree.
// Worktrees have a .git file containing "gitdir: /path/to/main/.git/worktrees/name".
// If it's already a main repo (has .git dir), returns the path unchanged.
func resolveMainRepo(repoPath string) string {
	dotGit := filepath.Join(repoPath, ".git")
	info, err := os.Stat(dotGit)
	if err != nil {
		return repoPath
	}
	if info.IsDir() {
		return repoPath // normal repo, not a worktree
	}
	// .git is a file — this is a worktree
	data, err := os.ReadFile(dotGit)
	if err != nil {
		return repoPath
	}
	content := strings.TrimSpace(string(data))
	if !strings.HasPrefix(content, "gitdir: ") {
		return repoPath
	}
	// Parse: "gitdir: /path/to/main/.git/worktrees/name"
	gitDir := strings.TrimPrefix(content, "gitdir: ")
	// Walk up from .git/worktrees/name to find the main repo
	// The main .git dir is the parent of "worktrees/"
	if idx := strings.Index(gitDir, "/.git/worktrees/"); idx >= 0 {
		return gitDir[:idx]
	}
	return repoPath
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
  ticket [title] [type]      Shortcut: create -k ticket (default type: task)
  warn <path> [message]      Shortcut: create -k warning --target <path>
  review [title] [options]   Shortcut: create -k review (open, needs response)
  artifact [title] [options] Shortcut: create -k artifact (born closed)
  adr [title] [flags]        List decisions, or create one
  pr [title] [flags]         List PRs, or create one (--into <branch>)
  pr show <id> [--diff]      PR details, diff summary, review verdict
  pr accept <id> [-m msg]    Accept PR (resolved comment)
  pr reject <id> -m 'reason' Request changes (unresolved comment)
  pr submit <id> [--force]   Merge source into target, close PR
  pr diff <id> [--stat]      Diff between source and target branches
  pr comment <id> -m 'msg'   Comment on a PR (--file, --line for inline)

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
  search <query> [flags]     BM25 full-text search across all notes
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
	title    string
	priority int
	assignee string
	tags     []string
	targets  []string
	body     string
	status   string
	message  string
	help     bool
	limit    int
}

func parseFlags(args []string) (flagSet, []string) {
	var f flagSet
	var positional []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			f.help = true
		case "--limit":
			i++; if i < len(args) { fmt.Sscanf(args[i], "%d", &f.limit) }
		case "-k", "--kind":
			i++; if i < len(args) { f.kind = args[i] }
		case "-t", "--title":
			i++; if i < len(args) { f.title = args[i] }
		case "--type":
			i++; if i < len(args) { f.typ = args[i] }
		case "-p", "--priority":
			i++; if i < len(args) { fmt.Sscanf(args[i], "%d", &f.priority) }
		case "-a", "--assignee":
			i++; if i < len(args) { f.assignee = args[i] }
		case "-l", "--tags":
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
			} else if strings.HasPrefix(args[i], "-") {
				fatal("unknown flag: %s", args[i])
			} else {
				positional = append(positional, args[i])
			}
		}
	}
	return f, positional
}
