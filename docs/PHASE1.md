---
mai-id: mai-s9i2
---
# Phase 1 — Notes Engine

## Scope

Three packages: `pkg/git`, `pkg/guard`, `pkg/notes`. When Phase 1 is done, you can create notes with IDs, append events and comments, fold current state, query by kind/status/target/tag, and branch-scope notes — with hook-based write protection on every write.

Everything has an ID. Everything changes via events. Nothing is mutated.

## pkg/git

### Purpose

Thin wrapper around git CLI plumbing. No domain concepts. No note format knowledge.

### Why git CLI, not libgit2/go-git

- Zero dependency beyond git itself
- Notes plumbing commands are simple and fast
- We need `git notes`, `git rev-parse`, `git log`, `git cat-file` — not a full object model
- Easier to test (mock command output)

### Types

```go
package git

// OID is a full 40-char hex SHA.
type OID string

// ObjectType is the git object type.
type ObjectType string

const (
    ObjectCommit ObjectType = "commit"
    ObjectBlob   ObjectType = "blob"
    ObjectTree   ObjectType = "tree"
    ObjectTag    ObjectType = "tag"
)

// NotesRef is a fully qualified notes reference.
type NotesRef string

const DefaultNotesRef NotesRef = "refs/notes/maitake"

// Object is a resolved git object.
type Object struct {
    OID  OID
    Type ObjectType
}

// NoteEntry is one entry from `git notes list`.
type NoteEntry struct {
    NoteOID   OID // the blob containing the note text
    TargetOID OID // the object the note is attached to
}

// RepoInfo describes the current repository state.
type RepoInfo struct {
    RootDir     string
    GitDir      string
    WorktreeDir string
    IsWorktree  bool
    IsJJ        bool
    IsBare      bool
}
```

### Repo interface

```go
type Repo interface {
    Info() (*RepoInfo, error)
    Resolve(rev string) (*Object, error)
    ResolveFilePath(path string) (*Object, error)
    CatBlob(oid OID) ([]byte, error)
    NoteGet(ref NotesRef, target OID) ([]byte, error)
    NoteAdd(ref NotesRef, target OID, content []byte) error
    NoteOverwrite(ref NotesRef, target OID, content []byte) error
    NoteRemove(ref NotesRef, target OID) error
    NoteList(ref NotesRef) ([]NoteEntry, error)
    NoteRefs() ([]NotesRef, error)
    CurrentBranch() (string, error)
    JJChangeID(commitOID OID) (string, error)
    TreeEntries(treeOID OID) ([]TreeEntry, error)
}
```

### Implementations

- `RealRepo` — shells out to git with timeouts and stderr capture
- `MockRepo` — in-memory, for testing consumers

```go
func Open(dir string) (*RealRepo, error)
func OpenFromCwd() (*RealRepo, error)
```

### Errors

```go
var (
    ErrNotARepo       = errors.New("not inside a git repository")
    ErrObjectNotFound = errors.New("object not found")
    ErrNoteExists     = errors.New("note already exists on this object")
    ErrNoteNotFound   = errors.New("no note on this object")
)

// GitError wraps a failed git command with stderr context.
type GitError struct {
    Cmd    string
    Args   []string
    Stderr string
    Err    error
}
```

### Files

```
pkg/git/
├── AGENTS.md
├── types.go
├── errors.go
├── repo.go              ← Repo interface
├── real_repo.go         ← RealRepo (git CLI)
├── real_repo_test.go
├── mock_repo.go         ← MockRepo (testing)
├── exec.go              ← command runner with timeout, stderr capture
├── exec_test.go
└── integration_test.go  ← tests against real git repos
```

---

## pkg/guard

### Purpose

Run hooks before writes. That's it.

### Implementation

```go
package guard

// RunHook executes .maitake/hooks/<hookName> with content on stdin.
// Returns nil if hook passes or doesn't exist.
// Returns error with stderr message if hook rejects.
func RunHook(maitakeDir string, hookName string, content []byte, env map[string]string) error

// HookExists checks if a hook is installed and executable.
func HookExists(maitakeDir string, hookName string) bool

// DefaultHookContent returns the default pre-write hook script.
func DefaultHookContent() []byte
```

See [HOOKS.md](HOOKS.md) for the full hook contract.

### Files

```
pkg/guard/
├── AGENTS.md
├── hook.go
└── hook_test.go
```

---

## pkg/notes

### Purpose

The substrate. Create notes with IDs, append events, fold state, query. Built on `pkg/git` and `pkg/guard`. Does NOT know about tickets, reviews, or docs — those are just kinds.

### Note format

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
field <name>
value <value>
<key> <value>

<blank line>
<free-form body, markdown>
```

First line MUST be `id` for creation notes. Events and comments have `kind` first (no `id` — they're anonymous).

### Core types

```go
package notes

import "github.com/cygnusfear/maitake/pkg/git"

// Note is a parsed note (creation note, event, or comment).
type Note struct {
    ID      string            // human-readable ID (creation notes only)
    Kind    string            // required
    Title   string            // optional
    Type    string            // optional (task, bug, artifact, etc.)
    Status  string            // optional
    Priority int              // optional
    Assignee string           // optional
    Tags    []string          // optional
    Field   string            // for events: which field changed
    Value   string            // for events: new value
    Edges   []Edge            // typed links
    Headers map[string]string // all other headers (extensible)
    Body    string            // free-form content after blank line

    // Metadata from git (not in note content)
    OID       git.OID
    TargetOID git.OID
    Ref       git.NotesRef
    Slot      string
    Timestamp time.Time       // from git log or note header
    Author    string          // from git
}

// Edge is a typed link.
type Edge struct {
    Type   string     // "targets", "closes", "on", "part-of", etc.
    Target EdgeTarget
}

// EdgeTarget identifies what an edge points at.
type EdgeTarget struct {
    Kind string // "commit", "blob", "tree", "path", "note", "change"
    Ref  string // OID, path, or ID
}

// State is the computed current state of a note after folding all events.
type State struct {
    ID        string
    Kind      string
    Status    string     // open, in_progress, closed — computed from events
    Title     string
    Type      string
    Priority  int
    Assignee  string
    Tags      []string
    Body      string
    Targets   []string   // file paths, commits targeted
    Deps      []string   // note IDs this depends on
    Links     []string   // note IDs linked to
    ParentID  string     // parent note ID (from part-of edge)
    Events    []Note     // all events, ordered by timestamp
    Comments  []Note     // all comments, ordered by timestamp
    CreatedAt time.Time
    UpdatedAt time.Time  // timestamp of last event
    NoteOID   git.OID    // creation note's blob OID
}

// StateSummary is a lightweight version of State for list views.
type StateSummary struct {
    ID        string
    Kind      string
    Status    string
    Type      string
    Priority  int
    Title     string
    Tags      []string
    Targets   []string
    CreatedAt time.Time
    UpdatedAt time.Time
}

// KindCount is a kind with its usage count.
type KindCount struct {
    Kind  string
    Count int
}

// DoctorReport contains graph health statistics.
type DoctorReport struct {
    TotalNotes   int
    CreationNotes int
    Events       int
    Comments     int
    ByKind       map[string]int
    ByStatus     map[string]int
    BrokenEdges  int
    Slots        []string
    BranchScopes []string
    IndexFresh   bool
}
```

### Engine interface

```go
type Engine interface {
    // Create writes a new creation note with a generated ID. Runs pre-write hook.
    Create(opts CreateOptions) (*Note, error)

    // Append writes an event or comment on an existing note. Runs pre-write hook.
    Append(opts AppendOptions) (*Note, error)

    // Get returns the raw creation note by ID (not folded).
    Get(id string) (*Note, error)

    // Fold returns the computed current state of a note.
    Fold(id string) (*State, error)

    // Context returns all open notes targeting a file path.
    Context(path string) ([]State, error)

    // ContextAll returns all notes targeting a file path (open + closed).
    ContextAll(path string) ([]State, error)

    // Find returns all notes matching filters.
    Find(opts FindOptions) ([]State, error)

    // List returns summary state for notes matching filters.
    List(opts ListOptions) ([]StateSummary, error)

    // Refs returns all notes with edges pointing at a target (reverse lookup).
    Refs(target string) ([]State, error)

    // Kinds returns all kinds in use with counts.
    Kinds() ([]KindCount, error)

    // BranchUse switches the active notes scope.
    BranchUse(name string) error

    // BranchMerge merges a branch scope into main.
    BranchMerge(name string) error

    // CurrentBranch returns the active scope name (empty = main).
    CurrentBranch() string

    // Doctor reports graph health.
    Doctor() (*DoctorReport, error)

    // Rebuild forces a full index rebuild.
    Rebuild() error
}
```

### Create and Append options

```go
type CreateOptions struct {
    Kind     string   // required
    Title    string   // optional
    Type     string   // optional
    Priority int
    Assignee string
    Tags     []string
    Body     string   // required
    Targets  []string // file paths, commit refs — auto-resolved to edges
    Edges    []Edge   // additional manual edges
    Slot     string   // optional parallel write lane
}

type AppendOptions struct {
    TargetID string   // the note ID this applies to — required
    Kind     string   // "event" or "comment" — required
    Body     string   // optional for events, required for comments
    Field    string   // for events: which field changed
    Value    string   // for events: new value
    Edges    []Edge   // additional edges beyond the auto-generated one
    Slot     string
}
```

### Query options

```go
type FindOptions struct {
    Kind     string
    Status   string
    Tag      string
    Type     string
    Target   string   // file path
    Assignee string
}

type ListOptions struct {
    FindOptions
    Limit  int
    SortBy string // "priority", "created", "updated"
}
```

### Fold rules

For a note with ID X, fold reads the creation note plus all events/comments referencing X, ordered by timestamp.

```
Event has edge "closes note:X"     → status = "closed"
Event has edge "reopens note:X"    → status = "open"
Event has edge "starts note:X"     → status = "in_progress"
Event has field=tags value=+foo    → tags = append(tags, "foo")
Event has field=tags value=-foo    → tags = remove(tags, "foo")
Event has field=priority value=N   → priority = N
Event has field=assignee value=X   → assignee = X
Event has field=deps value=+ID     → deps = append(deps, ID)
Event has field=deps value=-ID     → deps = remove(deps, ID)
Event has field=type value=X       → type = X
```

Scalars: last-writer-wins by timestamp.
Collections: operations applied in timestamp order.
Comments: collected, ordered by timestamp.

### How notes attach to git objects

Every note is a git note blob. Git notes attach to git objects (commits, blobs, trees). The question is: what git object does each note attach to?

**Creation notes:** attached to the git object they target.
- Note targeting `src/auth.ts` → attached to that file's current blob OID
- Note targeting `HEAD` → attached to that commit OID
- Note targeting a directory → attached to that tree OID
- Standalone note (no target) → attached to a deterministic synthetic OID derived from the note ID

**Events and comments:** attached to the same target OID as their parent creation note. Distinguished from the creation note by not having an `id` header (they have `kind` as first line instead).

Multiple notes on the same git object: git notes only allows one note per object per ref. Solution: use `git notes append` which concatenates. We delimit with a separator line (`---maitake---`) and parse them apart on read.

Alternative: use slots (separate refs) for truly concurrent writes.

### Slots

```
refs/notes/maitake              ← default
refs/notes/maitake-slot-<name>  ← named slot
```

- Create/Append with Slot → writes to slot ref
- Context, Find, List → aggregate across all slots
- Fold aggregates events from all slots by timestamp
- Same note ID should not exist in multiple slots (enforced on Create, not on Append)

### Branch-scoped notes

```
refs/notes/maitake                    ← main scope
refs/notes/maitake-branch-<name>      ← branch scope
```

- BranchUse persists in `.maitake/scope` (gitignored)
- All operations target the active scope
- BranchMerge = set-union of note blobs from branch into main

### Auto-edges

On Create:
- Each entry in `Targets` → resolved to `edge targets <kind>:<ref>`
- File path → `edge targets path:<filepath>` + `edge targets blob:<current-blob-oid>`
- Commit → `edge targets commit:<oid>`
- In jj repo, commit → also `edge targets change:<jj-change-id>`

On Append:
- Auto-add edge based on kind:
  - `kind event` with close semantics → `edge closes note:<target-id>`
  - `kind event` with field change → `edge updates note:<target-id>`
  - `kind comment` → `edge on note:<target-id>`

### Index

Local cache for fast queries. Stored in `.maitake/index` (gitignored, binary or JSON).

```go
type Index struct {
    Version  int
    RefTips  map[git.NotesRef]git.OID    // invalidation key
    ByID     map[string]git.OID          // human ID → creation note OID
    ByKind   map[string][]string         // kind → note IDs
    ByTarget map[string][]string         // target path/OID → note IDs
    ByStatus map[string][]string         // computed status → note IDs
    States   map[string]*StateSummary    // ID → cached summary (pre-folded)
}
```

Invalidation: compare stored ref tips against current. Rebuild if any differ.
Rebuild: list all notes across all refs → parse → fold → populate maps.

Performance targets:
- Rebuild 10,000 notes: <1s
- Warm query (list, find): <10ms
- Fold single note: <1ms from warm index

### Parse and Serialize

```go
// Parse parses raw note bytes into a Note.
// Returns error if kind is missing or format is invalid.
func Parse(raw []byte) (*Note, error)

// ParseMulti parses a concatenated note blob (multiple notes joined by separator).
func ParseMulti(raw []byte) ([]*Note, error)

// Serialize converts a Note to bytes.
func Serialize(note *Note) []byte
```

Format is strict:
- Creation notes: first line must be `id <value>`
- Events/comments: first line must be `kind <value>`
- Headers before first blank line
- Body after first blank line
- Unknown headers preserved in `Headers` map
- Parse errors are real errors, not silent fallbacks

### ID generation

```go
// GenerateID creates a human-readable ID from the directory name + random suffix.
func GenerateID(dir string) (string, error)
```

Same algorithm as current tk:
- Prefix = first letter of each hyphenated/underscored segment of directory name
- Suffix = 4 random lowercase alphanumeric chars
- Example: `tre-5c4a`, `mai-b2f1`

### ID resolution

```go
// ResolveID finds a note by full or partial ID.
// Returns ErrNotFound if no match, ErrAmbiguous if multiple matches.
func (e *RealEngine) ResolveID(partial string) (string, error)
```

Scans index. Supports substring matching anywhere in the ID.

### Files

```
pkg/notes/
├── AGENTS.md
├── types.go              ← Note, Edge, EdgeTarget, State, StateSummary
├── parse.go              ← Parse, ParseMulti, Serialize
├── parse_test.go         ← round-trip tests, invalid input, property tests
├── engine.go             ← Engine interface
├── real_engine.go        ← RealEngine struct, constructor
├── create.go             ��� Create implementation
├── create_test.go
├── append.go             ← Append implementation (events + comments)
├── append_test.go
├── fold.go               ← Fold implementation
├── fold_test.go          ← fold rules, edge cases, ordering
├── read.go               ← Get, Context, ContextAll
├── read_test.go
├── query.go              ← Find, List, Refs, Kinds
├── query_test.go
├── branch.go             ← BranchUse, BranchMerge
├── branch_test.go
├── index.go              ← Index type, build, invalidation, persistence
├── index_test.go
├── id.go                 ← GenerateID, ResolveID
├── id_test.go
├── doctor.go             ← Doctor report
├── doctor_test.go
└── integration_test.go   ← end-to-end against real git repos
```

---

## Testing strategy

### Unit tests

Every file has a `*_test.go` companion. Uses `MockRepo` for git operations. Tests:
- Parse/Serialize round-trips for every header combination
- Parse rejects invalid notes (missing kind, malformed edges, bad headers)
- Fold produces correct state for every event type
- Fold handles out-of-order timestamps correctly
- Fold handles duplicate events (idempotent)
- ID generation produces valid IDs
- ID resolution handles exact, partial, ambiguous, and missing cases
- Index invalidation triggers on ref tip change
- Branch scope isolation (branch notes don't appear in main)
- Branch merge (notes move to main)

### Integration tests

Require real git binary. Create temp repos, write real notes, read them back.
- Create a note on a file, read it back
- Append events, fold, verify state
- Write in two slots, verify aggregation
- Branch scope: create in branch, verify not in main, merge, verify in main
- Worktree: notes visible from linked worktree
- 10,000 notes: benchmark rebuild, list, fold

### Property-based tests

- `Parse(Serialize(note)) == note` for random valid notes
- `Fold(events_in_any_order)` produces same result (for commutative operations)
- Index rebuild produces same state as cold scan

---

## Phase 1 exit criteria

All of the following must be true before Phase 1 is complete:

1. **pkg/git** — Repo interface implemented. RealRepo passes integration tests against real git repos. MockRepo works for all consumer tests.

2. **pkg/guard** — RunHook executes `.maitake/hooks/pre-write`, returns error with stderr on rejection, passes through when hook is missing. Default hook script ships with `mai init`.

3. **pkg/notes** — Full Engine interface implemented:
   - Create notes with generated IDs on commits, blobs, trees, paths
   - Append events (close, reopen, start, field changes) and comments
   - Fold computes correct state from creation + events
   - Context returns all open notes targeting a file
   - Find/List with filtering by kind, status, tag, type, target, assignee
   - Refs reverse-lookup
   - Slots (parallel write lanes across separate refs)
   - Branch scope (use, merge)
   - Index with invalidation and rebuild
   - Doctor report

4. **Writes go through hooks** — no write bypasses pre-write hook unless `--skip-hooks` is passed (and that's logged)

5. **Parse/Serialize** property tests pass

6. **Performance** — 10,000 note benchmark: rebuild <1s, list <200ms, fold <1ms

7. **Integration tests** pass against real git repos, including worktrees

8. **Every exported function** has a test

9. **No file** exceeds 500 lines

10. **`go vet ./...` and `go test ./...`** clean

