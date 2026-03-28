# Phase 1 — Notes Engine

## Scope

Three packages: `pkg/git`, `pkg/guard`, `pkg/notes`. When Phase 1 is done, you can read, write, query, compost, and branch-scope structured notes on any git object in any git repo, with PII protection on every write.

## pkg/git

### Purpose

Thin wrapper around git CLI plumbing. No domain concepts. No note format knowledge. Just "talk to git and return structured results."

### Why shell out to git CLI (not libgit2)

- libgit2/go-git add massive dependency surface
- git CLI is always available where maitake runs
- Notes plumbing commands are simple and fast
- We need `git notes`, `git rev-parse`, `git log`, `git cat-file` — not a full object model
- Easier to test (mock command output, not a C library)

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
    RootDir       string // absolute path to repo root (.git parent)
    GitDir        string // absolute path to .git directory
    WorktreeDir   string // absolute path to current worktree (may differ from RootDir)
    IsWorktree    bool   // true if cwd is inside a linked worktree
    IsJJ          bool   // true if .jj/ exists (jj colocated repo)
    IsBare        bool   // true if bare repo
}

// Repo is the interface to git operations.
// Implementations: RealRepo (shells out to git), MockRepo (testing).
type Repo interface {
    // Info returns repository metadata.
    Info() (*RepoInfo, error)

    // Resolve resolves a revision/path to an object.
    // Examples: "HEAD", "HEAD:src/auth.ts", "main~3", a full OID.
    Resolve(rev string) (*Object, error)

    // ResolveFilePath resolves a file path to its current blob OID.
    // Returns the blob OID for the file at HEAD of the current worktree.
    ResolveFilePath(path string) (*Object, error)

    // CatBlob returns the content of a blob object.
    CatBlob(oid OID) ([]byte, error)

    // NoteGet reads the note on a target object.
    // Returns nil, nil if no note exists.
    NoteGet(ref NotesRef, target OID) ([]byte, error)

    // NoteAdd writes a note on a target object.
    // Fails if a note already exists (use NoteOverwrite for replacement).
    NoteAdd(ref NotesRef, target OID, content []byte) error

    // NoteOverwrite writes a note, replacing any existing note.
    NoteOverwrite(ref NotesRef, target OID, content []byte) error

    // NoteRemove removes the note on a target object.
    NoteRemove(ref NotesRef, target OID) error

    // NoteList lists all notes in a ref.
    NoteList(ref NotesRef) ([]NoteEntry, error)

    // NoteRefs lists all maitake-related notes refs.
    // Matches: refs/notes/maitake, refs/notes/maitake-*
    NoteRefs() ([]NotesRef, error)

    // LogWithNotes returns recent commits that have notes.
    LogWithNotes(ref NotesRef, limit int) ([]CommitWithNote, error)

    // CurrentBranch returns the current branch name (empty if detached).
    CurrentBranch() (string, error)

    // JJChangeID returns the jj change ID for a commit OID (empty if not jj).
    JJChangeID(commitOID OID) (string, error)

    // TreeEntries lists files in a tree object (for directory notes).
    TreeEntries(treeOID OID) ([]TreeEntry, error)
}

// CommitWithNote pairs a commit with its note content.
type CommitWithNote struct {
    CommitOID OID
    Subject   string
    NoteOID   OID
    Note      []byte // raw note content
}

// TreeEntry is one entry in a git tree.
type TreeEntry struct {
    Mode string
    Type ObjectType
    OID  OID
    Name string
}
```

### Implementation: RealRepo

```go
// RealRepo implements Repo by shelling out to git.
type RealRepo struct {
    dir string // working directory for git commands
}

func Open(dir string) (*RealRepo, error)     // validates dir is inside a git repo
func OpenFromCwd() (*RealRepo, error)        // uses os.Getwd(), walks parents
```

Every method runs `exec.CommandContext` with a timeout. Stderr is captured for error messages. Stdout is parsed into the return types.

### Error types

```go
var (
    ErrNotARepo      = errors.New("not inside a git repository")
    ErrObjectNotFound = errors.New("object not found")
    ErrNoteExists     = errors.New("note already exists on this object")
    ErrNoteNotFound   = errors.New("no note on this object")
    ErrGitCommand     = errors.New("git command failed")
)

// GitError wraps a failed git command with stderr.
type GitError struct {
    Cmd    string
    Args   []string
    Stderr string
    Err    error
}
```

### Testing

- `MockRepo` implements `Repo` interface with in-memory state
- Integration tests in `pkg/git/git_integration_test.go` — create temp git repos, run real commands
- Every `Repo` method has both unit (mock) and integration tests
- Build tag `//go:build integration` for tests that need real git

### Files

```
pkg/git/
├── AGENTS.md          ← package purpose and boundaries
├── types.go           ← OID, Object, NoteEntry, RepoInfo, etc.
├── errors.go          ← error types
├── repo.go            ← Repo interface
├── real_repo.go       ← RealRepo implementation
├── real_repo_test.go  ← unit tests with command mocking
├── mock_repo.go       ← MockRepo for testing consumers
├── integration_test.go ← tests against real git repos
└── exec.go            ← command runner with timeout, stderr capture
```

---

## pkg/guard

### Purpose

Scan content for secrets and PII before it enters a notes ref. Every write path in `pkg/notes` goes through guard. Hard gate — if guard rejects, the write does not happen.

### Types

```go
package guard

// Severity indicates how dangerous the finding is.
type Severity int

const (
    SeverityCritical Severity = iota // API keys, passwords, tokens
    SeverityHigh                      // emails, phone numbers
    SeverityMedium                    // possible PII patterns
)

// Finding is a single guard violation.
type Finding struct {
    Severity Severity
    Rule     string // which rule matched (e.g., "aws-secret-key", "email-address")
    Match    string // the matched text (redacted in logs)
    Line     int    // 1-indexed line number in content
}

// Result is the outcome of a guard scan.
type Result struct {
    Clean    bool
    Findings []Finding
}

// Scanner scans content for secrets and PII.
type Scanner interface {
    Scan(content []byte) (*Result, error)
}

// Config controls scanner behavior.
type Config struct {
    UseGitleaks    bool     // shell out to gitleaks if available
    BuiltinRules   bool     // use built-in regex patterns (default: true)
    AllowList      []string // regex patterns to ignore (e.g., test fixtures)
    MaxNoteSize    int      // reject notes larger than this (default: 65536)
}
```

### Implementation

Two scanner backends, composable:

```go
// BuiltinScanner uses compiled regex patterns.
type BuiltinScanner struct { ... }

// GitleaksScanner shells out to gitleaks.
type GitleaksScanner struct { ... }

// CompositeScanner runs multiple scanners and merges findings.
type CompositeScanner struct {
    scanners []Scanner
}

// NewScanner creates the appropriate scanner based on config and available tools.
func NewScanner(cfg Config) Scanner
```

### Built-in patterns

| Pattern | What it catches |
|---|---|
| AWS access key | `AKIA[0-9A-Z]{16}` |
| AWS secret key | 40-char base64 near "secret" |
| GitHub token | `ghp_`, `gho_`, `ghs_`, `ghr_` prefixes |
| Generic API key | `[Aa]pi[_-]?[Kk]ey.*[=:]\s*["']?[A-Za-z0-9]{20,}` |
| Private key | `-----BEGIN.*PRIVATE KEY-----` |
| Email address | standard email regex |
| Phone number | international format patterns |
| JWT | `eyJ[A-Za-z0-9-_]+\.eyJ[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+` |

Not exhaustive. Gitleaks is more thorough. Built-in patterns are the safety net when gitleaks is not installed.

### Testing

- Test each built-in pattern with positive and negative cases
- Test gitleaks integration with mock command
- Test composite scanner merging
- Test that clean content passes
- Test that allowlist suppresses findings
- Benchmark: scanning 64KB of content should be <1ms

### Files

```
pkg/guard/
├── AGENTS.md
├── scanner.go          ← Scanner interface, Config, CompositeScanner
├── builtin.go          ← BuiltinScanner with regex patterns
├── builtin_test.go
├── gitleaks.go         ← GitleaksScanner (shells out)
├── gitleaks_test.go
├── patterns.go         ← compiled regex table
├── patterns_test.go
└── finding.go          ← Finding, Result, Severity types
```

---

## pkg/notes

### Purpose

The substrate. Read, write, query, compost, branch-scope structured notes on git objects. Built on `pkg/git` and `pkg/guard`. Does NOT know about tickets, reviews, or docs.

### Types — Note format

```go
package notes

// Note is a parsed note.
type Note struct {
    Kind        string            // required (e.g., "context", "warning", "ticket")
    Title       string            // optional short label
    Status      string            // optional (used by tickets, reviews)
    Edges       []Edge            // typed links to other objects
    Supersedes  OID               // optional: previous version of this note
    Headers     map[string]string // all other headers (extensible)
    Body        string            // free-form content after headers

    // Metadata (not in note content — from git)
    OID         OID               // this note's blob OID
    TargetOID   OID               // the object this note is attached to
    Ref         NotesRef          // which notes ref this came from
    Slot        string            // slot name (empty = default)
}

// Edge is a typed link to another git object or note.
type Edge struct {
    Type   string    // "explains", "applies-to", "depends-on", "targets-path", etc.
    Target EdgeTarget
}

// EdgeTarget identifies what an edge points at.
type EdgeTarget struct {
    Kind string // "commit", "blob", "tree", "path", "note", "change"
    Ref  string // the OID, path, or change ID
}

// Freshness describes a note's relationship to the current file content.
type Freshness int

const (
    FreshnessCurrent   Freshness = iota // note's target matches current blob
    FreshnessStale                       // target blob differs from current
    FreshnessComposted                   // explicitly marked as absorbed
    FreshnessOrphaned                    // target object no longer exists
)

// AnnotatedNote is a Note with computed metadata.
type AnnotatedNote struct {
    Note
    Freshness   Freshness
    FilePath    string    // resolved from targets-path edge (if any)
    BranchScope string    // which branch scope this came from (empty = main)
}
```

### Types — Index

```go
// Index is the cached note state for fast queries.
type Index struct {
    Version   int                       // schema version
    RefOIDs   map[NotesRef]OID          // tip OID per notes ref (invalidation key)
    ByKind    map[string][]OID          // kind → note OIDs
    ByTarget  map[OID][]OID             // target OID → note OIDs attached to it
    ByPath    map[string][]OID          // file path → note OIDs targeting it
    ByEdge    map[string][]OID          // "edgetype:targetref" → note OIDs containing that edge
    Notes     map[OID]*Note             // OID → parsed note (cache)
}

// IndexStore handles persistence of the index.
type IndexStore interface {
    Load() (*Index, error)
    Save(idx *Index) error
}

// FileIndexStore stores the index in .maitake/index.json (gitignored).
type FileIndexStore struct { ... }
```

### Types — Write options

```go
// WriteOptions controls note creation.
type WriteOptions struct {
    Target     string   // git rev, file path, or "." for project-level
    Kind       string   // required
    Title      string   // optional
    Status     string   // optional
    Body       string   // required
    Edges      []Edge   // optional additional edges
    Slot       string   // optional slot name
    Supersede  bool     // if true, supersede existing note on same target+slot
    SkipGuard  bool     // emergency bypass (logged)
}

// WriteResult is returned after a successful write.
type WriteResult struct {
    NoteOID   OID
    TargetOID OID
    Ref       NotesRef
    AutoEdges []Edge   // edges automatically added (targets-path, targets-change for jj)
}
```

### Engine interface

```go
// Engine is the main notes API.
type Engine interface {
    // Write creates a note. Content goes through guard.
    Write(opts WriteOptions) (*WriteResult, error)

    // Read reads the note on a target. Returns nil, nil if none.
    Read(target string) (*AnnotatedNote, error)

    // ReadSlot reads from a specific slot.
    ReadSlot(target string, slot string) (*AnnotatedNote, error)

    // Follow reads a note and recursively resolves all edges.
    Follow(target string) (*AnnotatedNote, map[OID]*AnnotatedNote, error)

    // Context returns everything known about a file path:
    // current note + stale notes + parent dir notes + commit note.
    // Aggregates across all slots.
    Context(path string) ([]AnnotatedNote, error)

    // ContextAll is Context but includes composted notes.
    ContextAll(path string) ([]AnnotatedNote, error)

    // Find returns all notes of a given kind. Aggregates across slots.
    Find(kind string) ([]AnnotatedNote, error)

    // Refs returns all notes with edges pointing at the target (reverse lookup).
    Refs(target string) ([]AnnotatedNote, error)

    // List returns all annotated objects.
    List() ([]NoteEntry, error)

    // Kinds returns all kinds in use.
    Kinds() ([]string, error)

    // Edges returns all edges, optionally filtered by type.
    Edges(edgeType string) ([]Edge, error)

    // Compost marks stale notes as composted.
    Compost(target string) ([]CompostResult, error)

    // CompostDryRun shows what would be composted.
    CompostDryRun(target string) ([]CompostResult, error)

    // Renew reattaches a stale note to the current blob.
    Renew(target string) ([]RenewResult, error)

    // BranchUse switches the active notes scope.
    BranchUse(name string) error

    // BranchMerge merges a branch scope into the main scope.
    BranchMerge(name string) error

    // CurrentBranch returns the active notes scope name (empty = main).
    CurrentBranch() string

    // Doctor reports graph health statistics.
    Doctor() (*DoctorReport, error)

    // Rebuild forces a full index rebuild.
    Rebuild() error
}

// CompostResult describes one composting action.
type CompostResult struct {
    NoteOID  OID
    Kind     string
    Title    string
    FilePath string
    Action   string // "composted" or "would-compost"
}

// RenewResult describes one renewal action.
type RenewResult struct {
    NoteOID    OID
    OldBlobOID OID
    NewBlobOID OID
    FilePath   string
}

// DoctorReport contains graph health statistics.
type DoctorReport struct {
    TotalNotes    int
    ByKind        map[string]int
    ByFreshness   map[Freshness]int
    BrokenEdges   int              // edges pointing at nonexistent objects
    Slots         []string
    BranchScopes  []string
    IndexFresh    bool             // true if index matches current refs
}
```

### Engine implementation

```go
// RealEngine implements Engine.
type RealEngine struct {
    repo    git.Repo
    guard   guard.Scanner
    index   *Index
    store   IndexStore
    scope   string          // current branch scope (empty = main)
}

func NewEngine(repo git.Repo, scanner guard.Scanner) (*RealEngine, error)
```

### Note parsing and serialization

```go
// Parse parses raw note bytes into a Note.
func Parse(raw []byte) (*Note, error)

// Serialize converts a Note back to bytes.
func Serialize(note *Note) []byte
```

Format is strict:
- First line MUST be `kind <value>`
- Headers are `key value`, one per line
- First blank line separates headers from body
- Unknown headers are preserved in `Headers` map
- Parse errors are real errors, not silent fallbacks

### Auto-edges

On every `Write`:
- If target resolves to a file path → auto-add `edge targets-path path:<filepath>`
- If target is a commit in a jj repo → auto-add `edge targets-change change:<id>`

### Composting implementation

Freshness check:
1. Note has `edge targets-path path:foo.ts`
2. Resolve `foo.ts` to current blob OID
3. Compare against note's target OID
4. If different → stale
5. If note has `status composted` → composted
6. If target OID is gone from repo → orphaned

Composting a note:
1. Read existing note
2. Write new note with `supersedes <old-oid>` and `status composted`
3. New note is on the SAME target as the old one

### Slots implementation

- Default ref: `refs/notes/maitake`
- Slot ref: `refs/notes/maitake-<slot-name>`
- `Write` with slot → writes to slot ref
- `Read` without slot → reads default ref
- `Context`, `Find`, `Kinds` → scan all `refs/notes/maitake*` refs
- Supersession is intra-slot: writing to slot A never touches slot B

### Branch scope implementation

- Main scope: `refs/notes/maitake`
- Branch scope: `refs/notes/maitake-branch-<name>`
- `BranchUse` persists active scope in `.maitake/scope` (gitignored)
- All read/write operations use the active scope's ref
- `BranchMerge` copies notes from branch ref to main ref, handles conflicts

### Index implementation

Stored in `.maitake/index.json` (gitignored). Structure:

```json
{
  "version": 1,
  "ref_oids": {
    "refs/notes/maitake": "abc123...",
    "refs/notes/maitake-agent-a": "def456..."
  },
  "notes": {
    "<note-oid>": {
      "kind": "warning",
      "target_oid": "<target-oid>",
      "title": "Token refresh race",
      "paths": ["src/auth.ts"],
      "edge_targets": ["blob:aaa", "path:src/http.ts"]
    }
  }
}
```

Invalidation: compare stored `ref_oids` against current ref tips. If any differ, rebuild those refs.

Rebuild: `git notes list` on each ref → parse each note → populate maps.

Performance target: rebuild 10,000 notes in <1s. Lookup from warm index in <1ms.

### Testing

**Unit tests (pkg/notes):**
- Parse/Serialize round-trips for every header type
- Parse rejects invalid notes (missing kind, malformed edges)
- Freshness detection with mock repo
- Composting logic with mock repo
- Slot routing (correct ref selection)
- Branch scope switching
- Index invalidation and rebuild
- Auto-edge generation

**Integration tests (test/):**
- Write a note on a file, read it back
- Write notes in two slots, context aggregates both
- Compost a stale note after file changes
- Branch scope: notes in branch don't appear in main
- Branch merge: notes move to main
- Index survives process restart
- 10,000 notes: benchmark listing, filtering, context

**Property-based tests:**
- Parse(Serialize(note)) == note for random valid notes
- Freshness is always correct after any sequence of writes + file changes

### Files

```
pkg/notes/
├── AGENTS.md
├── types.go           ← Note, Edge, EdgeTarget, Freshness, AnnotatedNote
├── parse.go           ← Parse, Serialize
├── parse_test.go
├── engine.go          ← Engine interface
├── real_engine.go     ← RealEngine implementation
├── real_engine_test.go
├── write.go           ← Write implementation (guard gate, auto-edges, slot routing)
├── write_test.go
├── read.go            ← Read, ReadSlot, Follow, Context
├── read_test.go
├── query.go           ← Find, Refs, List, Kinds, Edges
├── query_test.go
├── compost.go         ← Compost, CompostDryRun, Renew, freshness logic
├── compost_test.go
├── branch.go          ← BranchUse, BranchMerge, scope persistence
├── branch_test.go
├── index.go           ← Index type, build, invalidation
├── index_store.go     ← FileIndexStore
├── index_test.go
├── doctor.go          ← Doctor report
└── doctor_test.go
```

---

## Phase 1 exit criteria

All of the following must be true:

1. `pkg/git` — full Repo interface implemented and tested against real git repos
2. `pkg/guard` — built-in patterns + gitleaks integration, tested with positive/negative cases
3. `pkg/notes` — full Engine interface implemented:
   - Write notes on commits, blobs, trees, paths, project root
   - Read notes back with freshness annotation
   - Follow edges recursively
   - Context aggregation across slots and scopes
   - Find by kind, reverse-lookup by refs
   - Composting (compost, renew, dry-run)
   - Slots (parallel write lanes)
   - Branch scope (use, merge)
   - Index with invalidation and rebuild
   - Doctor report
4. All writes go through guard — no bypass without `SkipGuard`
5. Parse/Serialize round-trip property tests pass
6. 10,000-note benchmark passes performance targets (<1s rebuild, <200ms list)
7. Integration tests pass against real git repos (including worktrees)
8. Every exported function has a test
9. No file exceeds 500 lines
10. `go vet ./...` and `go test ./...` clean
