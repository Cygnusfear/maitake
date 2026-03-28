// Package notes provides the maitake note substrate: create, append events,
// fold state, query, branch-scope, and slots.
package notes

import "time"

// Note is a parsed note — creation note, event, or comment.
type Note struct {
	// Core fields (from headers)
	ID       string            // human-readable ID (creation notes only; events/comments have "")
	Kind     string            // required: "ticket", "warning", "event", "comment", etc.
	Title    string            // optional short label
	Type     string            // optional: "task", "bug", "feature", "artifact", etc.
	Status   string            // optional: "open", "in_progress", "closed"
	Priority int               // optional (0 = unset)
	Assignee string            // optional
	Tags     []string          // optional
	Field    string            // for events: which field changed
	Value    string            // for events: new value
	Edges    []Edge            // typed links to other objects
	Headers  map[string]string // unknown headers preserved here

	// Body (everything after the blank line)
	Body string

	// Git metadata (populated on read from git, not serialized)
	OID       string    // this note's blob OID
	TargetOID string    // the git object this note is attached to
	Ref       string    // which notes ref this came from
	Slot      string    // slot name (empty = default)
	Timestamp time.Time // from git or header
	Author    string    // from git
}

// Edge is a typed link to another git object or note.
type Edge struct {
	Type   string     // "targets", "closes", "on", "depends-on", etc.
	Target EdgeTarget // what the edge points at
}

// EdgeTarget identifies what an edge points at.
type EdgeTarget struct {
	Kind string // "commit", "blob", "tree", "path", "note", "change"
	Ref  string // the OID, file path, or note ID
}

// State is the computed current state of a note after folding all events.
type State struct {
	ID        string
	Kind      string
	Status    string // computed: "open", "in_progress", "closed"
	Title     string
	Type      string
	Priority  int
	Assignee  string
	Tags      []string
	Body      string
	Targets   []string // file paths, commits targeted
	Deps      []string // note IDs this depends on
	Links     []string // note IDs linked to
	ParentID  string   // parent note ID (from part-of edge)
	Events    []Note   // all events, ordered by timestamp
	Comments  []Note   // all comments, ordered by timestamp
	CreatedAt time.Time
	UpdatedAt time.Time // timestamp of last event
	NoteOID   string    // creation note's blob OID
}

// StateSummary is a lightweight State for list views.
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
	TotalNotes    int
	CreationNotes int
	Events        int
	Comments      int
	ByKind        map[string]int
	ByStatus      map[string]int
	BrokenEdges   int
	Slots         []string
	BranchScopes  []string
	IndexFresh    bool
}

// CreateOptions controls note creation.
type CreateOptions struct {
	Kind     string
	Title    string
	Type     string
	Priority int
	Assignee string
	Tags     []string
	Body     string
	Targets  []string // file paths, commit refs — auto-resolved to edges
	Edges    []Edge
	Slot     string
}

// AppendOptions controls event/comment appending.
type AppendOptions struct {
	TargetID string // the note ID this applies to
	Kind     string // "event" or "comment"
	Body     string
	Field    string // for events: which field changed
	Value    string // for events: new value
	Edges    []Edge
	Slot     string
}

// FindOptions filters for queries.
type FindOptions struct {
	Kind     string
	Status   string
	Tag      string
	Type     string
	Target   string
	Assignee string
}

// ListOptions extends FindOptions with pagination and sorting.
type ListOptions struct {
	FindOptions
	Limit  int
	SortBy string // "priority", "created", "updated"
}

// Engine is the main notes API.
type Engine interface {
	Create(opts CreateOptions) (*Note, error)
	Append(opts AppendOptions) (*Note, error)
	Get(id string) (*Note, error)
	Fold(id string) (*State, error)
	Context(path string) ([]State, error)
	ContextAll(path string) ([]State, error)
	Find(opts FindOptions) ([]State, error)
	List(opts ListOptions) ([]StateSummary, error)
	Refs(target string) ([]State, error)
	Kinds() ([]KindCount, error)
	BranchUse(name string) error
	BranchMerge(name string) error
	CurrentBranch() string
	Doctor() (*DoctorReport, error)
	Rebuild() error
}
