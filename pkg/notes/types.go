// Package notes provides the maitake note substrate: create, append events,
// fold state, query, branch-scope, and slots.
package notes

import "time"

// Note is the stored unit. One JSON line in a git note.
// Creation notes have an ID. Events and comments do not.
type Note struct {
	// Identity
	ID   string `json:"id,omitempty"`   // human-readable ID (creation notes only)
	Kind string `json:"kind"`           // required: "ticket", "warning", "event", "comment", etc.

	// Metadata (all optional)
	Type     string   `json:"type,omitempty"`     // "task", "bug", "feature", "artifact", etc.
	Status   string   `json:"status,omitempty"`   // "open", "in_progress", "closed"
	Title    string   `json:"title,omitempty"`    // short label
	Priority int      `json:"priority,omitempty"` // 0 = unset
	Assignee string   `json:"assignee,omitempty"`
	Tags     []string `json:"tags,omitempty"`

	// Event fields
	Field string `json:"field,omitempty"` // which field changed
	Value string `json:"value,omitempty"` // new value

	// Edges
	Edges []Edge `json:"edges,omitempty"`

	// Location (for file-level review comments)
	Location *Location `json:"location,omitempty"`

	// Content
	Body string `json:"body,omitempty"` // markdown content

	// Threading
	Parent   string `json:"parent,omitempty"`   // parent comment ID (for threaded replies)
	Original string `json:"original,omitempty"` // original comment ID (for edits)

	// Resolution (tri-state: nil = FYI, true = resolved, false = unresolved)
	Resolved *bool `json:"resolved,omitempty"`

	// Timestamps
	Timestamp string `json:"timestamp,omitempty"` // creation/event time (unix or ISO)
	Author    string `json:"author,omitempty"`
	Branch    string `json:"branch,omitempty"` // git branch at write time (auto-stamped)

	// Git metadata (populated on read, not stored in JSON)
	OID       string `json:"-"`
	TargetOID string `json:"-"`
	Ref       string `json:"-"`
	Slot      string `json:"-"`

	// Parsed timestamp (computed, not stored)
	Time time.Time `json:"-"`
}

// Edge is a typed link to another git object or note.
type Edge struct {
	Type   string     `json:"type"`   // "targets", "closes", "on", "depends-on", etc.
	Target EdgeTarget `json:"target"` // what the edge points at
}

// EdgeTarget identifies what an edge points at.
type EdgeTarget struct {
	Kind string `json:"kind"` // "path", "note", "commit", "blob", "tree", "change"
	Ref  string `json:"ref"`  // the OID, file path, or note ID
}

// Location represents where a comment applies within a file.
type Location struct {
	Path  string `json:"path,omitempty"`
	Range *Range `json:"range,omitempty"`
}

// Range represents a line/column range in a file.
type Range struct {
	StartLine   uint32 `json:"startLine"`
	StartColumn uint32 `json:"startColumn,omitempty"`
	EndLine     uint32 `json:"endLine,omitempty"`
	EndColumn   uint32 `json:"endColumn,omitempty"`
}

// State is the computed current state of a note after folding all events.
type State struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	Title     string    `json:"title,omitempty"`
	Type      string    `json:"type,omitempty"`
	Priority  int       `json:"priority"`
	Assignee  string    `json:"assignee,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	Body      string    `json:"body,omitempty"`
	Targets   []string  `json:"targets,omitempty"`
	Deps      []string  `json:"deps,omitempty"`
	Links     []string  `json:"links,omitempty"`
	ParentID  string    `json:"parentId,omitempty"`
	Events    []Note    `json:"events,omitempty"`
	Comments  []Note    `json:"comments,omitempty"`
	Resolved  *bool     `json:"resolved,omitempty"`
	Branch    string    `json:"branch,omitempty"` // branch at creation time
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	NoteOID   string    `json:"noteOid,omitempty"`
}

// StateSummary is a lightweight State for list views.
type StateSummary struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	Type      string    `json:"type,omitempty"`
	Priority  int       `json:"priority"`
	Title     string    `json:"title,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	Targets   []string  `json:"targets,omitempty"`
	Deps      []string  `json:"deps,omitempty"`
	Links     []string  `json:"links,omitempty"`
	Assignee  string    `json:"assignee,omitempty"`
	Resolved  *bool     `json:"resolved,omitempty"`
	Branch    string    `json:"branch,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
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
	ID        string    // if set, use this ID instead of generating one
	Kind      string
	Title     string
	Type      string
	Priority  int
	Assignee  string
	Tags      []string
	Body      string
	Targets   []string  // file paths — auto-resolved to edges
	Edges     []Edge
	Slot      string
	Timestamp time.Time // if set, use this instead of time.Now()
}

// AppendOptions controls event/comment appending.
type AppendOptions struct {
	TargetID  string    // the note ID this applies to
	Kind      string    // "event" or "comment"
	Body      string
	Field     string    // for events: which field changed
	Value     string    // for events: new value
	Edges     []Edge
	Slot      string
	Timestamp time.Time // if set, use this instead of time.Now()

	// For comments
	Location *Location
	Parent   string // parent comment ID for threading
	Resolved *bool
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
	Sync() error
	GetConfig() Config
}
