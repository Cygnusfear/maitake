---
mai-id: mai-yh4w
---
# Maitake — Design Specification

## Overview

Maitake is a git-native notes engine with ticket, review, and documentation semantics. One Go binary. Storage is `refs/notes/*`. No shadow branches, no working-tree files, no external services.

Everything is a note. Every note has an ID. Every change is an event pointing at a note. Nothing is ever mutated or deleted.

Design references:
- [openprose/mycelium](https://github.com/openprose/mycelium) — git notes substrate, edge graph, branch-scoped notes, jj support
- [google/git-appraise](https://github.com/google/git-appraise) — code review on git notes
- Current `tk` (github.com/kardianos/ticket) — ticket commands, ID generation, listing/filtering

## 1. Core Model

### 1.1 Everything is a note with an ID

Every entity in maitake — ticket, warning, review finding, constraint, doc note, observation — is a **creation note** with a human-readable ID.

```
id wrn-a4f2
kind warning
edge targets path:src/auth.ts

Race condition in token refresh. Two concurrent requests can both
see an expired token and both trigger refresh.
```

This is a warning. It has an ID (`wrn-a4f2`). It targets a file. It can be referenced, closed, commented on, linked to, queried — exactly like a ticket.

### 1.2 Everything changes via events

No note is ever rewritten. Changes are new notes pointing at the original.

```
id wrn-a4f2
kind warning
edge targets path:src/auth.ts

Race condition in token refresh.
```

```
kind event
edge closes note:wrn-a4f2

Fixed in commit abc123. Mutex added around refresh.
```

```
kind comment
edge on note:wrn-a4f2

This was caused by the 2024 migration to async refresh.
```

The warning is now closed. Its current state = fold(creation + events + comments). The creation note is untouched. The close event says why. The comment adds context.

### 1.3 Current state = fold(creation + events)

For any note ID, the current state is computed by reading the creation note and all events/comments that reference it, ordered by timestamp.

```go
type State struct {
    ID        string
    Kind      string
    Status    string     // open, in_progress, closed — computed from events
    Title     string     // from body (first # heading) or title header
    Body      string
    Tags      []string   // from creation + tag events
    Priority  int
    Assignee  string
    Type      string     // task, bug, feature, epic, chore, artifact
    Targets   []string   // file paths, commits, etc.
    Deps      []string   // IDs this depends on
    Links     []string   // IDs linked to this
    Events    []Event
    Comments  []Comment
    CreatedAt time.Time
    UpdatedAt time.Time  // timestamp of last event
    NoteOID   OID        // creation note's blob OID
}
```

Fold rules:
- `edge closes` → status = closed
- `edge reopens` → status = open
- `edge starts` → status = in_progress
- `field tags +foo` → append tag
- `field tags -foo` → remove tag
- `field priority N` → priority = N
- `field assignee X` → assignee = X
- `field deps +<id>` → add dependency
- `field deps -<id>` → remove dependency
- Scalars: last-writer-wins by timestamp
- Collections: set operations applied in order
- Comments: ordered by timestamp

### 1.4 CRDT for free

Two agents write events on the same note:

```
Agent A: kind event, edge starts note:tre-5c4a, timestamp T1
Agent B: kind event, field tags +critical on note:tre-5c4a, timestamp T2
```

Both are separate immutable blobs. No conflict. When syncing between machines, merge is set-union on note blobs. Fold produces the same state regardless of order because:
- Scalars use last-writer-wins by timestamp
- Collections use set operations applied in timestamp order

### 1.5 ID generation

Same scheme as current tk:
- Prefix = first letter of each segment of the directory name
- Suffix = 4 random alphanumeric chars
- Examples: `tre-5c4a`, `wrn-a4f2`, `rev-b3d1`

IDs are stored in the creation note's `id` header. Partial matching works everywhere (e.g., `mai show 5c4` matches `tre-5c4a`).

Kind-based prefixes are a convention, not enforced:
- Tickets: directory prefix (`tre-`, `tic-`)
- Warnings/reviews/etc: same generation, different usage

## 2. Note Format

```
id <human-readable-id>
kind <string>
title <string>
type <string>
status <string>
priority <int>
assignee <string>
tags <comma-separated>
edge <type> <target-kind>:<ref>
<key> <value>

<blank line>
<free-form body, markdown>
```

### 2.1 Headers

- First line MUST be `id` for creation notes, `kind` for events/comments
- Headers are `key value` pairs, one per line, before the first blank line
- Unknown headers are preserved (extensible)
- Parse errors are hard errors

### 2.2 Edge targets

```
commit:<oid>       ← a specific commit
blob:<oid>         ← a file at a specific version
tree:<oid>         ← a directory at a specific version
path:<filepath>    ← a file regardless of version
note:<id>          ← another note (by human ID)
change:<jj-id>     ← jj change ID (stable across rewrites)
```

### 2.3 Edge types

Open vocabulary. Common types:

| Edge type | Meaning |
|---|---|
| `targets` | this note is about that object |
| `closes` | this event closes that note |
| `reopens` | this event reopens that note |
| `starts` | this event starts that note (in_progress) |
| `updates` | this event modifies a field on that note |
| `on` | this comment is on that note |
| `depends-on` | this note depends on that note |
| `blocks` | this note blocks that note |
| `links` | symmetric link |
| `part-of` | this note is part of that note (child→parent) |
| `references` | cross-reference |

### 2.4 Kinds

Open vocabulary. Defaults:

| Kind | What it is |
|---|---|
| `ticket` | work item, issue, task |
| `warning` | fragile area, footgun |
| `constraint` | hard rule |
| `context` | background for working on code |
| `summary` | what a file/module does |
| `decision` | why something was chosen (ADR) |
| `observation` | noticed but not acted on |
| `review-request` | opens a code review |
| `review` | file-level review finding |
| `review-verdict` | approve / changes-requested |
| `doc` | documentation |
| `event` | state change on another note |
| `comment` | comment on another note |

## 3. Notes Engine (`pkg/notes`)

### 3.1 Engine interface

```go
type Engine interface {
    // Create writes a new note with a generated ID.
    Create(opts CreateOptions) (*Note, error)

    // Append writes an event or comment on an existing note.
    Append(opts AppendOptions) (*Note, error)

    // Get returns a note by ID (creation note only, not folded).
    Get(id string) (*Note, error)

    // Fold returns the computed current state of a note (creation + all events).
    Fold(id string) (*State, error)

    // Context returns all open notes targeting a file path.
    Context(path string) ([]State, error)

    // ContextAll returns all notes targeting a file path (open + closed).
    ContextAll(path string) ([]State, error)

    // Find returns all notes matching filters.
    Find(opts FindOptions) ([]State, error)

    // List returns all note IDs with summary state.
    List(opts ListOptions) ([]StateSummary, error)

    // Refs returns all notes with edges pointing at a target (reverse lookup).
    Refs(target string) ([]State, error)

    // Kinds returns all kinds in use.
    Kinds() ([]KindCount, error)

    // BranchUse switches the active notes scope.
    BranchUse(name string) error

    // BranchMerge merges a branch scope into the main scope.
    BranchMerge(name string) error

    // Doctor reports graph health.
    Doctor() (*DoctorReport, error)

    // Rebuild forces a full index rebuild.
    Rebuild() error
}
```

### 3.2 Create and Append

```go
type CreateOptions struct {
    Kind     string   // required
    Title    string   // optional (can also be first # heading in body)
    Type     string   // optional (ticket type: task, bug, artifact, etc.)
    Priority int      // optional
    Assignee string   // optional
    Tags     []string // optional
    Body     string   // required
    Edges    []Edge   // optional (auto-edges added for file targets)
    Slot     string   // optional parallel write lane
}

type AppendOptions struct {
    TargetID string   // the note ID this event/comment applies to
    Kind     string   // "event", "comment", or specific kinds
    Body     string   // optional (required for comments)
    Field    string   // for events: which field changed
    Value    string   // for events: new value ("+tag", "-tag", "closed", etc.)
    Edges    []Edge   // auto-generated from TargetID, can add more
    Slot     string   // optional
}
```

### 3.3 Query

```go
type FindOptions struct {
    Kind     string   // filter by kind
    Status   string   // filter by computed status (open, in_progress, closed)
    Tag      string   // filter by tag
    Type     string   // filter by type (task, bug, artifact, etc.)
    Target   string   // filter by target path or OID
    Assignee string   // filter by assignee
}

type ListOptions struct {
    Status   string
    Tag      string
    Type     string
    Assignee string
    Limit    int
    SortBy   string   // "priority", "created", "updated"
}

type StateSummary struct {
    ID        string
    Kind      string
    Status    string
    Type      string
    Priority  int
    Title     string
    Tags      []string
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

### 3.4 Slots

Parallel write lanes for concurrent agents. Each slot is a separate notes ref.

```
refs/notes/maitake              ← default slot
refs/notes/maitake-slot-<name>  ← named slot
```

- `Create`/`Append` with `Slot` → writes to slot ref
- `Context`, `Find`, `List` aggregate across all slots
- Same note ID can exist in multiple slots (rare, but handled)
- Fold aggregates events from all slots by timestamp

### 3.5 Branch-scoped notes

```
refs/notes/maitake                     ← main scope
refs/notes/maitake-branch-<name>       ← branch scope
```

- `BranchUse(name)` → all operations target the branch scope
- `BranchMerge(name)` → copy notes from branch to main (set-union)
- Scope persisted in `.maitake/scope` (gitignored)

### 3.6 jj support

When `.jj/` is detected:
- Notes on commits auto-add `edge targets change:<jj-change-id>`
- `Get`/`Fold` fall back to change_id lookup when commit OID disappears
- `Migrate()` bulk-reattaches orphaned notes after jj rewrites

### 3.7 Index

Local cache for fast queries. Stored in `.maitake/index` (gitignored).

```go
type Index struct {
    Version   int
    RefTips   map[NotesRef]OID              // invalidation key
    ByID      map[string]OID                // human ID → creation note OID
    ByKind    map[string][]string           // kind → note IDs
    ByTarget  map[string][]string           // target path/OID → note IDs
    ByStatus  map[string][]string           // status ��� note IDs
    States    map[string]*StateSummary      // ID → cached summary
}
```

Invalidation: compare stored ref tips against current. If any differ, rebuild.
Performance target: rebuild 10,000 notes in <1s. Warm query in <1ms.

### 3.8 Auto-edges

On every `Create`:
- If target resolves to a file path → `edge targets path:<filepath>`
- If target is a commit in a jj repo → `edge targets change:<jj-change-id>`

On every `Append`:
- Auto-add `edge <type> note:<target-id>` based on the append kind

### 3.9 Storage: what goes where

Every note (creation notes, events, comments) is a git note blob attached to a **synthetic target object**. The target object is determined by:

- Creation notes: attached to the git object they target (blob, commit, tree) or to a null-target ref for standalone notes
- Events/comments: attached to a deterministic OID derived from the target note's ID

This keeps the notes ref as a flat list of git notes, each parseable independently.

## 4. CLI

### 4.1 Everything has an ID, everything is queryable

```bash
# Create things
mai create "Fix auth race condition" -k ticket -t task -p 1 --tags auth
mai create "Race condition in refresh" -k warning --target src/auth.ts
mai create "Must be retryable" -k constraint --target src/auth.ts

# Change things (by ID)
mai start tre-5c4a
mai close wrn-a4f2 -m "Fixed in abc123"
mai add-note tre-5c4a "Found root cause in refresh_token()"
mai tag tre-5c4a +critical
mai assign tre-5c4a "Alice"
mai dep tre-5c4a wrn-a4f2
mai link tre-5c4a rev-b3d1

# Read things
mai show tre-5c4a              # full state: creation + events + comments
mai show wrn-a4f2              # same — warnings are just notes with IDs
mai context src/auth.ts        # all open notes targeting this file
mai context src/auth.ts --all  # open + closed

# Query things
mai ls                          # all open notes
mai ls --status=open            # explicit
mai ls -k ticket                # only tickets
mai ls -k warning               # only warnings
mai ls -k review                # only review findings
mai ls -T auth                  # by tag
mai ls --target src/auth.ts     # everything on a file
mai ready                       # open notes with all deps resolved
mai blocked                     # open notes with unresolved deps

# Review workflow
mai create "Auth hardening review" -k review-request \
  --target src/auth.ts --target src/http.ts
mai create "Add mutex" -k review --target src/auth.ts \
  --part-of rev-b3d1 -m "AC: concurrent refresh safe"
mai create "Add backoff" -k review --target src/http.ts \
  --part-of rev-b3d1 -m "AC: exponential with jitter"
mai close rev-f1a2 -m "Fixed"
mai verdict rev-b3d1 approve

# Explore
mai context src/auth.ts        # what do I need to know about this file?
mai refs tre-5c4a              # what points at this ticket?
mai follow tre-5c4a            # ticket + resolve all edges
mai kinds                       # what kinds are in use?
mai doctor                      # graph health

# Branch scope
mai branch use my-feature       # notes scoped to this branch
mai branch merge my-feature     # merge into main scope

# Sync
mai sync-init forgejo           # push/pull notes to this remote
mai sync                        # push + pull + merge
```

### 4.2 Key principle: uniform interface

There is no separate `ticket` subcommand vs `note` subcommand vs `review` subcommand. Every note is created with `mai create`, queried with `mai ls`, shown with `mai show`. The `kind` is what differentiates them.

```bash
mai create "Fix bug" -k ticket -t task          # ticket
mai create "Footgun here" -k warning             # warning
mai create "Review auth" -k review-request       # PR
mai create "Add mutex" -k review                 # review finding
mai create "Use YAML" -k decision                # ADR
mai create "Module overview" -k summary           # doc
```

Same command. Same flags. Same query surface. The kind is metadata, not a different subsystem.

### 4.3 Shortcuts

Common operations get short aliases:

```bash
# These are equivalent:
mai create "Fix bug" -k ticket -t task -p 1
mai ticket "Fix bug" -t task -p 1

# These are equivalent:
mai create "Footgun" -k warning --target src/auth.ts
mai warn src/auth.ts "Footgun"

# These are equivalent:
mai create "Add mutex" -k review --target src/auth.ts --part-of rev-b3d1
mai review src/auth.ts "Add mutex" --part-of rev-b3d1
```

The shortcuts are sugar over `create` with pre-filled kind and argument order. They use the same code path.

### 4.4 Context is the arrival command

When an agent starts working on a file:

```bash
$ mai context src/auth.ts

wrn-a4f2 [warning] (open)     Race condition in token refresh
con-b1c3 [constraint] (open)  Must be retryable
rev-f1a2 [review] (open)      Add mutex. AC: concurrent refresh safe
tre-5c4a [ticket] (in_progress) Fix auth race condition
```

Everything the agent needs. Warnings, constraints, review findings, related tickets — all targeting this file, all with IDs the agent can reference and close.

### 4.5 Output format

Default: human-readable table (like current `tk ls`).

```bash
$ mai ls -k ticket --status=open
tre-5c4a [P1][open]    Fix auth race condition        auth,backend
tre-9b2f [P2][open]    Add retry logic                http,backend
```

JSON for scripting:

```bash
$ mai ls -k ticket --status=open --json
[{"id":"tre-5c4a","kind":"ticket","status":"open","priority":1,...},...]
```

## 5. Review Workflow

Reviews are notes. The workflow is:

```
1. Author creates review-request targeting changed files
2. Reviewer creates review findings ON the files, linked to the request
3. Each finding has acceptance criteria and rejection criteria in the body
4. Implementer runs `mai context <file>` and sees findings in-place
5. Implementer fixes, then closes findings with a reason
6. Reviewer creates review-verdict (approve / changes-requested)
```

### 5.1 Review finding format

```
id rev-f1a2
kind review
edge targets path:src/auth.ts
edge part-of note:rev-b3d1
status open
tags critical

## Race condition in token refresh

Two concurrent requests can both see an expired token.

## Acceptance Criteria
- [ ] Mutex or single-flight around token refresh
- [ ] Concurrent requests block on in-flight refresh
- [ ] No request gets a revoked token

## Rejection Criteria
- Simple boolean flag instead of proper synchronization
- Retry loop without backoff
- Silencing the error instead of fixing the race
```

This note lives on `src/auth.ts`. When the fix agent runs `mai context src/auth.ts`, they see it. The AC tells them what "done" looks like. The rejection criteria tells them what NOT to do.

### 5.2 Verdict

```
kind event
edge closes note:rev-b3d1
field status approve

All findings addressed. Ship it.
```

Or:

```
kind event
edge updates note:rev-b3d1
field status changes-requested

Two findings still open. See rev-f1a2 and rev-g2b3.
```

## 6. Sync

### 6.1 Privacy

Notes refs don't push/fetch by default. Explicit opt-in per remote:

```bash
mai sync-init forgejo    # adds refspecs for this remote only
mai sync-init --remove github  # removes refspecs
```

### 6.2 Merge

Notes are immutable blobs. Merging diverged refs = set-union on blobs.

Both sides added notes → keep both. Same note on both sides → deduplicate by OID. Fold produces the same state regardless of merge order because event timestamps determine precedence.

## 7. Hooks

See [HOOKS.md](HOOKS.md) for the full spec.

```
.maitake/hooks/
├── pre-write       ← scan content before it enters a ref
├── pre-push        ← last-chance scan before push
├── post-write      ← logging, notifications
└── post-close      ← after a note is closed
```

## 8. Migration

### 8.1 From tk (.tickets/ shadow branch)

```bash
mai migrate-legacy [--dry-run]
```

- Reads `.tickets/*.md` files
- Parses YAML frontmatter into creation notes
- Parses `## Notes` sections into comment notes
- Status/deps/links become events
- Preserves IDs, timestamps
- After: remove `.tickets/` worktree, delete shadow branch

### 8.2 Backward compatibility

During transition, `mai` checks both refs/notes and `.tickets/`. Unified view. Gradual migration.

EDITED CONTENT
REGRESSION CHECK

OBSIDIAN EDIT 1774732766

DAEMON TEST 1774732869

LIVE EDIT 1774732917

