<p align="center">
  <img src="assets/logo.png" width="200" />
</p>

<h1 align="center">maitake</h1>

<p align="center">
  One primitive for your entire decision trail.<br>
  Tickets, reviews, PRs, docs — all stored as git notes. Nothing leaves your machine unless you say so.
</p>

```bash
go install github.com/cygnusfear/maitake/cmd/mai@latest
```

## Why

Your tickets are in Jira. Reviews on GitHub. Docs in Notion. ADRs in a wiki.
None of it travels with the code.

`maitake` stores everything — tickets, code reviews, pull requests, docs,
warnings, decisions — as **`git notes`** attached to your repo. One format,
one CLI, one place.

🤖 **Agent-native**</br>
Agents create tickets, leave file-level findings, open PRs,
and update docs through the same CLI. Every command supports `--json` output.
No API keys, no platform auth, no network required. If you can run `git`, you
can run `mai`.

🕵️ **Private by default**</br> Your draft tickets, internal conversations, and PII stay local unless you choose otherwise. Git notes don't push with `git push`. maitake won't sync anything until you explicitly configure a remote — and even then, `github.com` is blocked out of the box.

🔖 **One primitive.**</br> A ticket, a review finding, a doc, and a PR are all the
same thing: a JSON line in `refs/notes/maitake` with a `kind` field. The
vocabulary is open. The storage is append-only. Concurrent writes merge
without conflicts via set-union.

💼 **Forge-agnostic.**</br> Sync issues, PRs, docs to anything: GitHub, Forgejo, Gitea, or your own server via hooks. Switch forges without losing a single event. The decision trail of _why_ and _how_ stays with your codebase forever.

🍄‍🟫 **A substrate for other apps.**</br> `maitake` is a primitive you want to build on. A [kanban board](https://github.com/Cygnusfear/ramboard), [multi-agent coordinator](https://github.com/Cygnusfear/pi-extensions), a diff viewer, an Obsidian clone — all powered by `maitake` underneath.

## 30-second tour

```bash
mai init                                      # hooks + config (local only)
mai ticket "Fix auth race" -p 1 -l auth --target src/auth.ts
mai start mt-5c4a
mai add-note mt-5c4a --file src/auth.ts --line 42 "Race condition here if not implemented correctly"
mai context src/auth.ts                       # see everything about a file
mai close mt-5c4a -m "Fixed with mutex"
```

Everything is JSON, append-only, and mergeable. Every event records the
current git branch automatically.

## How it works

Each ticket, warning, review finding, or comment is one JSON line in a git
note. Nothing is mutated — state is computed by folding events:

```
{"id":"mt-5c4a","kind":"ticket","title":"Fix auth race","branch":"main","timestamp":"..."}
{"kind":"event","field":"status","value":"in_progress","branch":"feature/auth","timestamp":"..."}
{"kind":"comment","body":"Found root cause","branch":"feature/auth","timestamp":"..."}
{"kind":"event","field":"status","value":"closed","branch":"main","timestamp":"..."}
```

Closed from `main` — the branch was merged. The event stream tells the story.

## Comparison

|                           | maitake                                     | [tk](https://github.com/wedow/ticket) | [lat.md](https://github.com/1st1/lat.md) | [mycelium](https://github.com/openprose/mycelium) | [entire.io](https://entire.io) | [git-bug](https://github.com/git-bug/git-bug) | [git-appraise](https://github.com/google/git-appraise) |
| ------------------------- | ------------------------------------------- | ------------------------------------- | ---------------------------------------- | ------------------------------------------------- | ------------------------------ | --------------------------------------------- | ------------------------------------------------------ |
| **Storage**               | git notes                                   | `.tickets/` files                     | `lat.md/` files                          | git notes                                         | shadow branch                  | custom git refs                               | git notes                                              |
| **Scope**                 | tickets, reviews, PRs, docs, warnings, ADRs | tickets                               | knowledge graph                          | open-vocabulary notes                             | session checkpoints            | issues                                        | reviews                                                |
| **Unified primitive**     | ✓                                           | —                                     | —                                        | ✓ (notes only)                                    | —                              | —                                             | —                                                      |
| **Private by default**    | ✓ (nothing pushes without config)           | — (files in working tree)             | — (files in working tree)                | manual setup                                      | configurable                   | —                                             | ✓                                                      |
| **PII / secret scanning** | built-in hooks, blocked hosts               | —                                     | —                                        | warns only                                        | —                              | —                                             | —                                                      |
| **File-level targeting**  | ✓ (file + line)                             | —                                     | `@lat:` comments                         | ✓                                                 | —                              | —                                             | ✓                                                      |
| **Agent-native CLI**      | ✓ (JSON)                                    | ✓ (markdown)                          | ✓ (needs OpenAI key)                     | ✓ (bash)                                          | background capture             | partial                                       | —                                                      |
| **Doc sync**              | ✓ (CRDT, Obsidian-compatible)               | —                                     | —                                        | —                                                 | —                              | —                                             | —                                                      |
| **Event-sourced**         | ✓ (append-only, set-union merge)            | — (mutable files)                     | — (mutable files)                        | — (mutable notes)                                 | ✓                              | ✓                                             | ✓                                                      |
| **Language**              | Go                                          | Bash                                  | TypeScript                               | Bash                                              | TypeScript                     | Go                                            | Go                                                     |

## Commands

### Create

| Command                       | What                                                      |
| ----------------------------- | --------------------------------------------------------- |
| `mai ticket [title] [opts]`   | Ticket (task by default)                                  |
| `mai warn <path> [message]`   | Warning on a file                                         |
| `mai review [title] [opts]`   | Code review (open, needs response)                        |
| `mai artifact [title] [opts]` | Record/output (born closed — ADRs, research, mid-mortems) |
| `mai create [title] [opts]`   | Any kind — use `-k`                                       |

**Options:** `-k kind`, `-t title`, `--type type`, `-p priority`, `-a assignee`, `-l a,b` (tags), `--target path`, `-d description`

### Pull Requests

Git-native PRs — no GitHub, no Forgejo, no platform lock-in. Stored as `kind: pr` notes.

```bash
# Create (from a feature branch)
mai pr "Add auth middleware" --into main   # → mai-5c4a  feature/auth → main
mai pr                                     # list open PRs (auto-closes merged ones)

# Inspect
mai pr show <id>                           # details + diff summary + review verdict
mai pr show <id> --diff                    # include full inline diff
mai pr diff <id>                           # full diff between source and target
mai pr diff <id> --stat                    # summary only

# Review
mai pr accept <id> [-m message]            # LGTM (resolved comment)
mai pr reject <id> -m 'reason'             # request changes (unresolved comment)
mai pr comment <id> -m 'msg'               # general comment
mai pr comment <id> -m 'msg' --file <path> --line N  # inline comment

# Merge
mai pr submit <id>                         # merge source → target, close PR
mai pr submit <id> --force                 # skip unresolved comment check
```

PRs that are merged outside mai (via `git merge`, GitHub, etc.) auto-close when listed.

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
mai search "auth race"          # BM25 full-text search across all notes
mai search "fix" -k ticket      # search within a kind
mai search "merge" --limit 5    # top N results
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
mai --json search "query"       # JSON array of {id, score, state}
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
mai init                            # local only — no remote, no push
mai init --remote forgejo           # enable auto-push to a remote
mai init --remote forgejo --block github.com  # push to forgejo, block github
```

This creates:

1. **`.maitake/hooks/pre-write`** — scans for secrets before every write (gitleaks with regex fallback)
2. **`.maitake/config.toml`** — sync remote + blocked hosts
3. **`.gitignore` entry** — keeps `.maitake/` out of the repo

Without `--remote`, nothing syncs anywhere. With `--remote`, every write
auto-pushes `refs/notes/maitake` to that remote (debounced, conflict-safe).
`github.com` is blocked by default even when a remote is configured.

### Config

```toml
[sync]
remote = "forgejo"
blocked-hosts = ["github.com", "gitlab.com"]

[docs]
sync = "auto"
dir = ".mai-docs"

[hooks]
pre-write = true
post-push = true
```

## Sync

When a remote is configured, every write auto-pushes `refs/notes/maitake`.
On conflict: fetch + set-union merge + retry. Push failures warn but never
block.

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

## Attach anything to a file

Tickets, warnings, decisions, and artifacts can target files directly. This is
how you stick the *why* onto the *what*.

```bash
# Warning on a file
mai warn src/auth.ts "Race condition in token refresh"

# ADR explaining a design decision, attached to the file it affects
mai adr "Why topology lives in SpacetimeDB" --target src/physics/rebuild.rs \
  -d "Convergence overhead is lower than mirroring + writeback cost. Revisit at 500+ entities."

# Artifact (born closed) — research, analysis, post-mortem
mai artifact "Perf analysis" --target src/physics/rebuild.rs -d "..."

# Ticket targeting multiple files
mai ticket "Auth hardening" --target src/auth.ts --target src/http.ts
```

Comments within a ticket can also target files and lines:

```bash
mai add-note mt-5c4a --file src/auth.ts "Add mutex around token refresh"
mai add-note mt-5c4a --file src/http.ts --line 15 "Missing backoff"
```

`mai context <path>` shows everything attached to a file — tickets, warnings,
decisions, review findings — filtered to only that file's comments:

```bash
mai context src/auth.ts    # what do we know about this file?
```

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

Git notes don't push by default — `git push` ignores them entirely. maitake
only pushes to the remote you configure in `.maitake/config.toml`. Blocked
hosts are checked before every push. No remote configured = nothing leaves
your machine.

## Design

- **Event-sourced** — immutable JSON lines, state computed by folding
- **Append-only** — changes via events, never mutation
- **Set-union merge** — `cat | sort | uniq` resolves conflicts (inherited from git-appraise)
- **Kind-agnostic** — tickets, warnings, constraints, decisions, reviews are all notes with different `kind` fields
- **Full-text search** — BM25 scoring with field weighting (title 3×, tags 2×, body 1×, comments 0.5×). Combines with kind/status/tag filtering.
- **Performance** — 10,000 notes: index build <20ms, query <1ms. Cache eliminates git reads on warm start.

### References

- [wedow/ticket](https://github.com/wedow/ticket) — git-backed ticket tracker (maitake's predecessor used this as starting point)
- [openprose/mycelium](https://github.com/openprose/mycelium) — git notes substrate for agent communication
- [google/git-appraise](https://github.com/google/git-appraise) — code review on git notes (Apache 2.0, repository package adapted)
- [1st1/lat.md](https://github.com/1st1/lat.md) — markdown knowledge graph for codebases
- [entire.io](https://entire.io) — AI session checkpoints stored in git
