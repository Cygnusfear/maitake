<p align="center">
  <img src="assets/logo.png" width="200" />
</p>

<h1 align="center">maitake</h1>

Git-native tickets, notes, and code review. One binary, zero dependencies beyond git. Storage lives in `refs/notes/maitake` — invisible to your working tree, pushed only where you choose.

```bash
go install github.com/cygnusfear/maitake/cmd/mai@latest
```

## 30-second tour

```bash
mai init --remote origin --block github.com   # set up hooks + sync config
mai ticket "Fix auth race" -p 1 --tags auth --target src/auth.ts
mai start mt-5c4a
mai add-note mt-5c4a --file src/auth.ts --line 42 "Race condition here"
mai context src/auth.ts                       # see everything about a file
mai close mt-5c4a -m "Fixed with mutex"
```

Every write auto-pushes to the configured remote. Every note records the current git branch. Everything is JSON, append-only, and mergeable.

## How it works

Each ticket, warning, review finding, or comment is one JSON line in a git note. Nothing is mutated — state is computed by folding events:

```
{"id":"mt-5c4a","kind":"ticket","title":"Fix auth race","branch":"main","timestamp":"..."}
{"kind":"event","field":"status","value":"in_progress","branch":"feature/auth","timestamp":"..."}
{"kind":"comment","body":"Found root cause","branch":"feature/auth","timestamp":"..."}
{"kind":"event","field":"status","value":"closed","branch":"main","timestamp":"..."}
```

Closed from `main` — the branch was merged. The event stream tells the story.

## Commands

### Create

| Command                     | What                          |
| --------------------------- | ----------------------------- |
| `mai ticket [title] [opts]` | Ticket (task by default)      |
| `mai warn <path> [message]` | Warning on a file             |
| `mai review [title] [opts]` | Code review (open, needs response) |
| `mai artifact [title] [opts]` | Record/output (born closed — ADRs, research, mid-mortems) |
| `mai create [title] [opts]` | Any kind — use `-k`           |

**Options:** `-k kind`, `-t type`, `-p priority`, `-a assignee`, `--tags a,b`, `--target path`, `-d description`

### Lifecycle

```bash
mai start <id>                                  # → in_progress
mai close <id> [-m message]                     # → closed
mai reopen <id>                                 # → open
mai add-note <id> [text]                        # comment
mai add-note <id> --file <path> [text]          # file-level comment
mai add-note <id> --file <path> --line N [text] # line-level comment
mai tag <id> +tag / -tag                        # add/remove tag
mai assign <id> <name>                          # set assignee
mai dep <id> <dep-id>                           # add dependency
mai undep <id> <dep-id>                         # remove dependency
mai link <id> <id>                              # symmetric link
mai unlink <id> <id>                            # remove link
```

### Query

```bash
mai show <id>                   # full state with comments
mai ls                          # open + in_progress (work queue)
mai ls --status=all             # everything
mai ls -k warning               # filter by kind
mai closed                      # recently closed
mai context <path>              # everything targeting a file
mai ready                       # unblocked work
mai blocked                     # stuck on deps
mai dep tree <id>               # dependency graph
mai kinds                       # all kinds in use
mai doctor                      # graph health
```

### Machine-readable output

```bash
mai --json ls                   # JSON array of summaries
mai --json show <id>            # JSON state with events + comments
mai --json context <path>       # JSON array of states
mai -C /path/to/repo --json ls  # query a different repo
```

### Setup & sync

```bash
mai init [--remote R] [--block H]   # hooks + config + .gitignore
mai sync                            # manual fetch + merge + push
mai migrate [--dir .tickets/] [--dry-run]  # import tk tickets
```

## Setup

```bash
mai init --remote forgejo --block github.com
```

This creates three things:

1. **`.maitake/hooks/pre-write`** — scans notes for secrets before every write (gitleaks with regex fallback)
2. **`.maitake/config`** — sync remote + blocked hosts
3. **`.gitignore` entry** — keeps `.maitake/` out of the repo

### Config

```
remote forgejo
blocked-host github.com
blocked-host gitlab.com
```

## Sync

Every write auto-pushes `refs/notes/maitake` to the configured remote. On conflict: fetch + set-union merge + retry. Push failures warn but never block.

Manual sync pulls remote changes:

```bash
mai sync    # fetch + merge + push
```

## Hooks

Hooks live in `.maitake/hooks/` (per-repo) or `~/.maitake/hooks/` (global fallback). Per-repo wins when both exist.

| Hook        | When                             | Receives                                          |
| ----------- | -------------------------------- | ------------------------------------------------- |
| `pre-write` | Before every note write          | JSON note on stdin                                |
| `post-push` | After every successful auto-push | `MAI_REMOTE`, `MAI_REF`, `MAI_REPO_PATH` env vars |

Exit non-zero from `pre-write` to reject the write. `post-push` failures warn but don't block.

### Example hooks

```bash
# Secret scanning (installed by default with mai init)
cp examples/hooks/pre-write-gitleaks .maitake/hooks/pre-write

# Sync to GitHub Issues (requires gh CLI)
cp examples/hooks/post-push-github .maitake/hooks/post-push

# Sync to Forgejo Issues (requires curl + jq)
cp examples/hooks/post-push-forgejo .maitake/hooks/post-push
```

### Global hooks

Set up once for all repos:

```bash
mkdir -p ~/.maitake/hooks
cp examples/hooks/pre-write-gitleaks ~/.maitake/hooks/pre-write
cp examples/hooks/post-push-github ~/.maitake/hooks/post-push
chmod +x ~/.maitake/hooks/*
```

Every repo gets these unless it provides its own.

## File-located comments

Comments can target specific files (and lines) within a ticket:

```bash
mai ticket "Auth hardening" --target src/auth.ts --target src/http.ts
mai add-note mt-5c4a --file src/auth.ts "Add mutex around token refresh"
mai add-note mt-5c4a --file src/http.ts --line 15 "Missing backoff"
```

`mai context src/auth.ts` shows the ticket and only auth.ts comments — not http.ts comments. Review agents leave findings on files, fix agents see exactly what to address.

## Branch tracking

Every JSON event records the git branch at write time. No flags needed — it's automatic.

```json
{"kind":"ticket","title":"Fix auth","branch":"feature/auth","timestamp":"..."}
{"kind":"event","field":"status","value":"closed","branch":"main","timestamp":"..."}
```

Closed from `main` tells you the feature branch was merged.

## Index cache

The index caches in `~/.maitake/cache/`, keyed by the notes ref tip SHA. Cache invalidates automatically on every write. Cold start reads from git; warm start skips all git round-trips.

## Artifacts

`mai artifact` creates notes with `type: artifact` — born closed. They don't appear in `mai ls` or `mai context` unless you query with `--status=all`. Use for ADRs, research results, oracle findings, mid-mortems, and other records that aren't active work.

Reviews (`mai review`) are open by default — they need a response.

## Migration from tk

```bash
mai migrate --dir .tickets/           # import all tickets
mai migrate --dir .tickets/ --dry-run # preview without writing
```

Preserves original IDs, timestamps, deps, links, parent refs, Forgejo issue numbers, and comments. Old-format files without YAML frontmatter are skipped.

## Privacy

Notes refs don't push by default — git ignores them. Only the remote configured in `.maitake/config` receives notes. Blocked hosts are checked before every push.

## Design

- **Event-sourced** — immutable JSON lines, state computed by folding
- **Append-only** — changes via events, never mutation
- **Set-union merge** — `cat | sort | uniq` resolves conflicts (inherited from git-appraise)
- **Kind-agnostic** — tickets, warnings, constraints, decisions, reviews are all notes with different `kind` fields
- **Performance** — 10,000 notes: index build <20ms, query <1ms. Cache eliminates git reads on warm start.

### References

- [openprose/mycelium](https://github.com/openprose/mycelium) — git notes substrate
- [google/git-appraise](https://github.com/google/git-appraise) — code review on git notes (Apache 2.0, repository package adapted)
