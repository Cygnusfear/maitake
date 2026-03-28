// Package git provides a thin wrapper around git CLI plumbing.
// It knows nothing about notes format, tickets, or reviews.
package git

import "time"

// OID is a full 40-character hex SHA-1 object identifier.
type OID string

// Short returns the first 7 characters for display.
func (o OID) Short() string {
	if len(o) >= 7 {
		return string(o[:7])
	}
	return string(o)
}

// String implements fmt.Stringer.
func (o OID) String() string { return string(o) }

// IsZero returns true if the OID is empty.
func (o OID) IsZero() bool { return o == "" }

// ObjectType is the type of a git object.
type ObjectType string

const (
	ObjectCommit ObjectType = "commit"
	ObjectBlob   ObjectType = "blob"
	ObjectTree   ObjectType = "tree"
	ObjectTag    ObjectType = "tag"
)

// NotesRef is a fully qualified git notes reference.
type NotesRef string

const (
	// DefaultNotesRef is the primary maitake notes ref.
	DefaultNotesRef NotesRef = "refs/notes/maitake"

	// NotesSeparator delimits multiple notes concatenated on a single object.
	NotesSeparator = "\n---maitake---\n"
)

// SlotRef returns the notes ref for a named slot.
func SlotRef(slot string) NotesRef {
	return NotesRef("refs/notes/maitake-slot-" + slot)
}

// BranchRef returns the notes ref for a branch scope.
func BranchRef(branch string) NotesRef {
	return NotesRef("refs/notes/maitake-branch-" + branch)
}

// Object is a resolved git object.
type Object struct {
	OID  OID
	Type ObjectType
}

// NoteEntry is one entry from `git notes list` — maps a note blob to its target.
type NoteEntry struct {
	NoteOID   OID // the blob containing the note text
	TargetOID OID // the object the note is attached to
}

// RepoInfo describes the current repository state.
type RepoInfo struct {
	RootDir     string // absolute path to repo root (.git parent)
	GitDir      string // absolute path to .git directory
	WorktreeDir string // absolute path to current worktree root
	IsWorktree  bool   // true if cwd is a linked worktree
	IsJJ        bool   // true if .jj/ exists (jj colocated repo)
	IsBare      bool   // true if bare repository
}

// CommitInfo holds metadata for a single commit.
type CommitInfo struct {
	OID       OID
	Subject   string
	Author    string
	Timestamp time.Time
}

// CommitWithNote pairs a commit with its note content.
type CommitWithNote struct {
	Commit CommitInfo
	Note   []byte // raw note content (may be nil if no note)
}

// TreeEntry is one entry in a git tree.
type TreeEntry struct {
	Mode string
	Type ObjectType
	OID  OID
	Name string
}
