---
name: mai-agent
description: Use when working in any git repo with maitake. Check for notes before touching files, leave notes after meaningful work. This is the base agent contract — all other mai skills build on it.
---

# Mai Agent — Arrival and Departure

The contract: **read before you work, leave notes after.**

## Setup

First time in a repo:

```bash
mai init                            # local only — nothing pushes anywhere
mai init --remote forgejo           # enable auto-push to a specific remote
```

This creates `.maitake/hooks/` (PII scanning), `.maitake/config.toml` (sync config), and adds `.maitake/` to `.gitignore`. Without `--remote`, everything stays local. With `--remote`, notes auto-push after every write. `github.com` is blocked by default either way.

## On arrival

Before touching any file:

```bash
mai context <file-you-will-touch>     # open tickets, warnings, constraints, review findings
mai ls                                 # open work queue
mai ls -k constraint                   # hard rules you must follow
mai ls -k warning                      # known fragile areas
mai search "topic you're working on"   # find related notes, decisions, history
mai ready                              # what can start next
mai blocked                            # what's stuck on deps
```

`mai context` is the most important command. It shows everything targeting a file — including file-located comments from other agents on tickets that span multiple files.

## During work

### Working on an existing ticket

```bash
mai show <ticket-id>                                    # read the full ticket
mai start <ticket-id>                                   # mark in_progress
mai add-note <ticket-id> "what you're doing"            # general progress
mai add-note <ticket-id> --file src/auth.ts "details"   # file-specific comment
mai add-note <ticket-id> --file src/auth.ts --line 42 "line-level detail"
```

### Creating new work

```bash
mai ticket "Fix auth race condition" -p 1 -l auth --target src/auth.ts \
  -d "Token refresh has a race condition."
```

### Attaching things directly to files

Warnings, decisions, and artifacts can target files without a pre-existing ticket:

```bash
# Warning on a fragile file
mai warn src/auth.ts "Token cache not thread-safe"

# Decision (ADR) explaining why code is the way it is
mai adr "Use mutex for token refresh" --target src/auth.ts \
  -d "Chose mutex over single-flight. SF propagates errors to all waiters."

# Artifact — research, analysis, post-mortem (born closed)
mai artifact "Perf analysis" --target src/physics/rebuild.rs -d "..."
```

`mai context <file>` shows all of these. This is how you stick the *why* onto the *what*.

### Tagging and assigning

```bash
mai tag <id> +critical        # add tag
mai tag <id> -wontfix         # remove tag
mai assign <id> "Alice"       # assign
```

### Dependencies

```bash
mai dep <parent-id> <dep-id>  # parent depends on dep
mai dep tree <id>             # visualize
mai undep <id> <dep-id>       # remove dependency
mai ready                     # what's unblocked
mai blocked                   # what's waiting
```

### Linking

```bash
mai link <id-a> <id-b>       # symmetric link
mai unlink <id-a> <id-b>     # remove
```

## On departure

```bash
mai close <ticket-id> -m "What was done and the result"
```

If you found something fragile:

```bash
mai warn src/auth.ts "Token cache not thread-safe — hold mutex during refresh"
```

If you made a decision worth recording:

```bash
mai create "Use single-flight for token refresh" -k decision --target src/auth.ts \
  -d "Chose single-flight over mutex — coalesces concurrent callers."
```

## Artifact tickets

For non-work outputs (research, reviews, ADRs):

```bash
mai review "Auth hardening review" --target src/auth.ts \
  -d "Review findings for the auth changes."
```

Artifacts are born closed — they don't pollute `mai ls`. Query them explicitly:

```bash
mai ls --status=all           # includes closed
mai closed                    # recently closed
mai ls --status=all -k review # all reviews
```

## Sync

If a remote is configured, notes auto-push after every write (debounced,
conflict-safe). Without a remote, nothing syncs — everything stays local.

Manual sync:

```bash
mai sync                      # fetch + merge + push
```

## Querying

```bash
mai show <id>                 # full state with comments
mai ls                        # open work queue (default: open + in_progress)
mai ls -k <kind>              # filter by kind
mai ls -l <tag>               # filter by tag
mai ls --status=all           # everything including closed
mai search "query"            # BM25 full-text search across all notes
mai search "auth" -k ticket   # search within a kind
mai search "fix" --limit 5    # top N results
mai closed                    # recently closed
mai context <path>            # everything about a file
mai ready                     # unblocked work
mai blocked                   # blocked work
mai dep tree <id>             # dependency tree
mai kinds                     # what kinds exist
mai doctor                    # graph health stats
```

## Rules

1. **Always check context before editing a file.** Warnings, constraints, and review findings are there for a reason.
2. **Always leave a note after meaningful work.** The next agent needs context.
3. **Close things you resolve.** Keep the queue clean.
4. **Use `--file` on comments** when they're about a specific file, not the ticket in general.
5. **Use `--line` on comments** when you can point to a specific location.
