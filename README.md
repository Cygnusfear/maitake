# maitake 🍄

Git-native notes, tickets, and code review. One binary. Storage is `refs/notes/maitake` — invisible to your working tree, local-only by default.

## Install

```bash
go install github.com/cygnusfear/maitake/cmd/mai@latest
```

## Quick start

```bash
# Create a ticket targeting a file
mai ticket "Fix auth race condition" -p 1 --tags auth --target src/auth.ts

# Start working on it
mai start tre-5c4a

# Add a comment about a specific file
mai add-note tre-5c4a --file src/auth.ts "Add mutex around token refresh"

# Add a line-level comment
mai add-note tre-5c4a --file src/auth.ts --line 42 "Race condition here"

# See everything about a file before touching it
mai context src/auth.ts

# Close when done
mai close tre-5c4a -m "Fixed with mutex"
```

## Commands

### Create

```bash
mai create [title] [options]   # Create a note with a generated ID
mai ticket [title] [options]   # Shortcut: -k ticket -t task
mai warn <path> [message]      # Shortcut: -k warning --target <path>
mai review [title] [options]   # Shortcut: -k review -t artifact (born closed)
```

Create options:

```
-k, --kind KIND            Note kind (ticket, warning, review, constraint, etc.)
-t, --type TYPE            Type (task, bug, feature, epic, chore, artifact)
-p, --priority N           Priority 0-4 (0 = highest)
-a, --assignee NAME        Assignee
--tags TAG,TAG             Comma-separated tags
--target PATH              File this note is about (repeatable)
-d, --description TEXT     Body text
```

### Lifecycle

```bash
mai start <id>                           # Status → in_progress
mai close <id> [-m message]              # Status → closed
mai reopen <id>                          # Status → open
mai add-note <id> [text]                 # Append comment
mai add-note <id> --file <path> [text]   # Comment about a specific file
mai add-note <id> --file <path> --line N [text]  # Line-level comment
mai tag <id> +tag / -tag                 # Add or remove tag
mai assign <id> <name>                   # Set assignee
mai dep <id> <dep-id>                    # Add dependency
mai link <id> <id>                       # Symmetric link
```

### Query

```bash
mai show <id>                  # Full note state with comments
mai ls [--status=X] [-k kind]  # List notes
mai context <path>             # Everything about a file (open notes + file-located comments)
mai ready                      # Open notes with deps resolved
mai blocked                    # Open notes with unresolved deps
mai kinds                      # List all kinds in use
mai doctor                     # Graph health report
```

### Setup

```bash
mai init                       # Create .maitake/hooks/ with default pre-write hook
```

## How it works

Every ticket, warning, review finding, or comment is a **JSON line** stored in `refs/notes/maitake` via git notes.

Nothing is ever mutated. Changes are **event streams**: a ticket's current state is computed by folding its creation note + all events pointing at it.

```
Creation:  {"id":"tre-5c4a","kind":"ticket","status":"open",...}
Event:     {"kind":"event","field":"status","value":"in_progress",...}
Comment:   {"kind":"comment","body":"Found root cause",...}
Event:     {"kind":"event","field":"status","value":"closed",...}
```

Fold result: status=closed, with 1 comment in history.

### File-located comments

Comments can target a specific file (and optionally a line range) within a ticket:

```bash
mai ticket "Auth hardening" --target src/auth.ts --target src/http.ts
mai add-note tre-5c4a --file src/auth.ts "Add mutex around token refresh"
mai add-note tre-5c4a --file src/http.ts "Add backoff to retry logic"
```

When you run `mai context src/auth.ts`, you see the ticket AND only the comments about auth.ts — not the http.ts comments. This means review agents can leave findings on specific files within a ticket, and fix agents see exactly what needs attention when they check context.

### Privacy

Notes refs don't push by default. Explicit opt-in per remote:

```bash
# Push to private forgejo, never to GitHub
git config --add remote.forgejo.push '+refs/notes/maitake:refs/notes/maitake'
```

### Guard hooks

`.maitake/hooks/pre-write` scans every note before it enters git. Default hook catches leaked secrets. Replace with your own scanner.

### Artifact tickets

`mai review` creates tickets with `type: artifact` — born closed by default. They don't show up in `mai ls` or `mai context` unless explicitly queried. Use for review findings, research results, ADRs, and other non-work records.

## Migration from tk

```bash
mai migrate-legacy --tickets-dir .tickets/
```

Preserves original IDs, timestamps, deps, links, comments. Old-format files (no YAML frontmatter) are skipped gracefully.

## Design references

- [openprose/mycelium](https://github.com/openprose/mycelium) — git notes substrate
- [google/git-appraise](https://github.com/google/git-appraise) — code review on git notes (Apache 2.0, repository package adapted)
- [1st1/lat.md](https://github.com/1st1/lat.md) — knowledge graph direction (north star for docs layer)
