# Hongos — Design Specification

## Overview

Hongos is a git-native notes engine with ticket and code review semantics. One Go binary. Storage is `refs/notes/*`. No shadow branches, no working-tree files, no external services.

Design references:
- [openprose/mycelium](https://github.com/openprose/mycelium) — note format, composting, slots, branch-scope, jj support
- [google/git-appraise](https://github.com/google/git-appraise) — code review on git notes
- Current `tk` (github.com/kardianos/ticket) — ticket commands, ID generation, listing/filtering

## 1. Storage Model

### 1.1 Everything is a git note

A note is structured text attached to a git object (commit, blob, tree) via `refs/notes/*`.

```
refs/notes/hongos          ← default notes ref
refs/notes/hongos-<slot>   ← per-slot refs (parallel writes)
refs/notes/hongos-<branch> ← branch-scoped refs
```

### 1.2 Note format

```
kind <string>              ← required, first line
title <string>             ← optional
status <string>            ← optional (tickets)
edge <type> <target>       ← zero or more
supersedes <oid>           ← optional (points at previous version)
<header> <value>           ← extensible headers

<blank line>
<free-form body>           ← markdown encouraged
```

Headers are `key value` pairs, one per line, before the first blank line. Body is everything after.

### 1.3 Edge targets

```
commit:<oid>     ← a commit
blob:<oid>       ← a file at a specific version
tree:<oid>       ← a directory at a specific version
path:<filepath>  ← a file regardless of version
note:<oid>       ← another note's blob OID
change:<id>      ← jj change ID (stable across rewrites)
```

### 1.4 Kinds

Open vocabulary. No schema, no enum. These are defaults:

| Kind | Purpose |
|---|---|
| `note` | General annotation |
| `decision` | Why something was chosen |
| `context` | Background needed before touching this code |
| `summary` | What a file/module does |
| `warning` | Fragile areas, footguns |
| `constraint` | Hard rules |
| `observation` | Noticed but not acted on |
| `ticket` | Work item (immutable creation record) |
| `ticket-event` | Status/field change on a ticket |
| `ticket-comment` | Timestamped comment on a ticket |
| `review-request` | Opens a review |
| `review` | File-level review finding |
| `review-verdict` | Approve/request-changes |

## 2. Notes Engine (`pkg/notes`)

The core. Knows about note format, edges, kinds, composting, slots. Does NOT know about tickets or reviews.

### 2.1 Operations

| Operation | Description |
|---|---|
| `Write(target, note)` | Attach a note to a git object. Goes through guard. Auto-adds `targets-path` edge for file targets. |
| `Read(target)` | Read the note on an object. Returns parsed headers + body. |
| `ReadSlot(target, slot)` | Read from a specific slot. |
| `Follow(target)` | Read + resolve all edges recursively. |
| `Context(path)` | Aggregate: current note + stale notes + parent dir notes + commit notes. Across all slots. |
| `Find(kind)` | All notes of a given kind. |
| `Refs(target)` | All notes with edges pointing at target (reverse lookup). |
| `List()` | All annotated objects. |
| `Kinds()` | All kinds in use. |
| `Edges(edgeType)` | All edges of a given type. |

### 2.2 Composting

Notes go stale when the file they describe changes (blob OID differs from current).

| State | Meaning |
|---|---|
| Current | Note's target blob matches the file's current blob |
| Stale | Target blob differs from current — note is about an older version |
| Composted | Explicitly marked as absorbed/obsolete |

Operations:
- `Compost(target)` — mark stale notes as composted
- `Renew(target)` — reattach stale note to current blob
- `Report()` — count stale/composted across repo

`Context()` shows stale notes as one-line summaries. Composted notes are hidden unless `--all`.

### 2.3 Slots

Parallel write lanes. Each slot is a separate notes ref.

```
refs/notes/hongos            ← default slot
refs/notes/hongos-agent-a    ← slot "agent-a"
refs/notes/hongos-agent-b    ← slot "agent-b"
```

Rules:
- `Read` uses default slot unless slot specified
- `Context`, `Find`, `Kinds` aggregate across all slots
- Supersedes is intra-slot only
- Reserved slot names: `main`, `default`

### 2.4 Branch-scoped notes

Feature work gets its own notes namespace:

```
refs/notes/hongos                    ← main scope
refs/notes/hongos-branch-<name>      ← branch scope
```

- `BranchUse(name)` — switch active scope
- `BranchMerge(name)` — merge branch notes into main scope

### 2.5 jj support

When `.jj/` is detected:
- Notes on commits auto-add `edge targets-change change:<jj-change-id>`
- `Read` falls back to change_id lookup when commit OID is gone
- `Migrate()` bulk-reattaches notes after jj rewrites

## 3. Ticket Layer (`pkg/ticket`)

Built on `pkg/notes`. Tickets are event-sourced.

### 3.1 Ticket = creation note + event stream

```
Creation note:
  kind ticket
  id tre-5c4a
  type task
  priority 1
  assignee Alexander Mangel
  tags auth,backend
  edge targets-path path:src/auth.ts

  # Fix auth race condition
  Description body.

Event notes (each is a separate note):
  kind ticket-event
  edge updates note:<creation-note-oid>
  field status
  value in_progress
  author Alexander Mangel
  timestamp 2026-03-27T20:00:00Z

Comment notes:
  kind ticket-comment
  edge updates note:<creation-note-oid>
  author Alexander Mangel
  timestamp 2026-03-27T20:15:00Z

  Found root cause in refresh_token().
```

### 3.2 Folding

Current ticket state = fold(creation + events):

```go
type TicketState struct {
    ID          string
    Title       string
    Type        string     // task, bug, feature, epic, chore, artifact
    Status      string     // open, in_progress, closed
    Priority    int
    Assignee    string
    Tags        []string
    Deps        []string   // note OIDs
    Links       []string   // note OIDs
    CreatedAt   time.Time
    UpdatedAt   time.Time  // timestamp of last event
    Comments    []Comment
    Events      []Event    // full history
    NoteOID     string     // creation note OID
    Edges       []Edge     // all edges from creation + events
}
```

Fold rules:
- `field status value X` → status = X
- `field tags value +foo` → append tag
- `field tags value -foo` → remove tag
- `field priority value N` → priority = N
- `field assignee value X` → assignee = X
- `field deps value +<oid>` → add dependency
- `field deps value -<oid>` → remove dependency
- Comments are ordered by timestamp

### 3.3 Ticket types

| Type | Default status | Use |
|---|---|---|
| `task` | `open` | General work |
| `bug` | `open` | Bug fix |
| `feature` | `open` | New feature |
| `epic` | `open` | Parent/umbrella |
| `chore` | `open` | Maintenance |
| `artifact` | `closed` | Non-work output (review, research, ADR, doc) |

### 3.4 Commands

| Command | Description |
|---|---|
| `hongos create [title] [options]` | Create ticket (creation note + optional start event) |
| `hongos start <id>` | Emit status→in_progress event |
| `hongos close <id>` | Emit status→closed event |
| `hongos reopen <id>` | Emit status→open event |
| `hongos show <id>` | Fold and display current state |
| `hongos ls [--status=X] [-T tag]` | List tickets (fold all, filter, sort) |
| `hongos ready` | Open tickets with all deps resolved |
| `hongos blocked` | Open tickets with unresolved deps |
| `hongos add-note <id> [text]` | Emit ticket-comment |
| `hongos dep <id> <dep-id>` | Emit deps event |
| `hongos link <id> <id>` | Emit link events on both |
| `hongos edit <id>` | Open in $EDITOR (rewrite creation note with supersedes) |

### 3.5 ID generation and resolution

Same scheme as current tk:
- ID = directory-prefix + 4-char random alphanumeric
- Stored in creation note header: `id tre-5c4a`
- Partial match: scan creation notes, match substring
- Index: maintain an in-memory ID→OID map, cached per-session

### 3.6 Performance: note index

For repos with thousands of tickets, scanning all notes on every command is too slow.

Build a local index file (gitignored):

```
.hongos/index.json
{
  "version": 1,
  "ref": "refs/notes/hongos",
  "ref_oid": "<tip-of-notes-ref>",
  "tickets": {
    "tre-5c4a": { "oid": "<creation-note-oid>", "status": "open", "type": "task", "priority": 1, "title": "Fix auth race condition" },
    ...
  }
}
```

Invalidate when notes ref OID changes. Rebuild is: scan all notes, fold events, write index. Should be <1s for 10,000 notes.

## 4. Review Layer (`pkg/review`)

Built on `pkg/notes`. Reviews are note graphs.

### 4.1 Review workflow

```
1. Author opens review:
   kind review-request
   edge compares commit:<head>
   edge base commit:<merge-base>
   edge targets-path path:src/auth.ts
   edge targets-path path:src/http.ts

   Auth hardening — race condition fix + retry logic

2. Reviewer leaves file-level findings:
   kind review
   edge part-of note:<review-request-oid>
   edge targets-path path:src/auth.ts
   status changes-requested

   Race condition in token refresh.
   AC: concurrent refresh requests must not corrupt token state.

3. Reviewer leaves verdict:
   kind review-verdict
   edge part-of note:<review-request-oid>
   status changes-requested

   Two file-level findings. Fix both before merge.

4. Implementer sees findings on files:
   hongos context src/auth.ts
   → [review] Race condition in token refresh. AC: ...

5. After fixing, implementer or reviewer composts the finding:
   hongos compost src/auth.ts
```

### 4.2 Review commands

| Command | Description |
|---|---|
| `hongos review request [--base commit] [--head commit] [paths...]` | Open a review |
| `hongos review find <path>` | File-level finding with acceptance criteria |
| `hongos review verdict <review-id> [approve\|changes-requested]` | Overall verdict |
| `hongos review ls` | List open reviews |
| `hongos review show <id>` | Show review + all findings |

## 5. Guard Layer (`pkg/guard`)

Every write goes through guard before hitting git.

### 5.1 Checks

| Check | How |
|---|---|
| Secret detection | Shell out to `gitleaks detect --pipe` if available |
| PII patterns | Built-in regex: emails, phone numbers, SSNs, API key patterns |
| Note format validation | Headers parseable, kind present, edges well-formed |
| Size limit | Reject notes over configurable max (default: 64KB) |

### 5.2 Behavior

- If gitleaks is installed: use it (most thorough)
- If not: fall back to built-in patterns (good enough for common cases)
- Guard failures are hard errors — the write does not happen
- `--skip-guard` flag for emergencies (logged as a warning note on the repo)

## 6. Sync Layer (`pkg/sync`)

### 6.1 Privacy model

- Notes refs do NOT push/fetch by default (git's built-in behavior)
- `hongos sync-init <remote>` adds refspecs for a specific remote
- `hongos sync-init --remove <remote>` removes refspecs
- No accidental leaks to GitHub

### 6.2 Merge strategy

When the remote notes ref has diverged:

Notes are immutable blobs. "Merging" is set-union on the blob set:
- Both sides added different notes → keep both
- Same note on both sides → deduplicate by OID
- Supersession chains: preserve both, let the longer chain win

No field-level CRDT needed because notes are never mutated — only superseded.

## 7. CLI Surface

```
hongos note [target] -k <kind> -m <body>       Write a note
hongos read [target]                            Read a note
hongos follow [target]                          Read + resolve edges
hongos context <path>                           Everything known about a file
hongos find <kind>                              Find by kind
hongos compost [path|oid] [--dry-run]           Triage stale notes

hongos create [title] [options]                 Create ticket
hongos start <id>                               Status → in_progress
hongos close <id>                               Status → closed
hongos show <id>                                Display ticket
hongos ls [--status=X] [-T tag]                 List tickets
hongos ready / blocked / closed                 Filtered views
hongos add-note <id> [text]                     Comment on ticket
hongos dep / link / undep / unlink              Relationships

hongos review request [options]                 Open review
hongos review find <path> -m <finding>          File-level finding
hongos review verdict <id> [verdict]            Approve or request changes
hongos review ls / show                         Review queries

hongos branch use <name>                        Branch-scoped notes
hongos branch merge <name>                      Merge branch notes
hongos sync-init [remote]                       Configure sync
hongos doctor                                   Health check
hongos migrate                                  Reattach after jj rewrites
```

Alias: `tk` can remain as an alias or symlink for backward compatibility with existing workflows.

## 8. Migration Path

### 8.1 From current tk (.tickets/ shadow branch)

```
hongos migrate-legacy [--dry-run]
```

- Reads all `.tickets/*.md` files
- Parses YAML frontmatter into creation notes
- Parses `## Notes` sections into ticket-comment notes
- Writes to `refs/notes/hongos`
- Preserves ticket IDs, timestamps, deps, links
- After migration: remove `.tickets/` worktree, delete shadow branch

### 8.2 Backward compatibility

During transition, `hongos` can check both:
1. `refs/notes/hongos` (new)
2. `.tickets/` directory (legacy)

And present a unified view. This allows gradual migration.
