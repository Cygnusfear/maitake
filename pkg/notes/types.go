// Package notes defines the maitake note format and core engine types.
package notes

import (
	"time"

	"github.com/cygnusfear/maitake/pkg/git"
)

// Note is a parsed maitake note.
type Note struct {
	ID       string            // Human-readable ID for creation notes only.
	Kind     string            // Required kind such as ticket, event, or comment.
	Title    string            // Optional short title.
	Type     string            // Optional type such as task, bug, or artifact.
	Status   string            // Optional raw status header.
	Priority int               // Optional priority.
	Assignee string            // Optional assignee.
	Tags     []string          // Optional tags.
	Field    string            // Event field name.
	Value    string            // Event field value.
	Edges    []Edge            // Typed links.
	Headers  map[string]string // Unknown headers preserved verbatim.
	Body     string            // Free-form body after the blank line.

	// Metadata populated from git rather than note headers.
	OID       git.OID
	TargetOID git.OID
	Ref       git.NotesRef
	Slot      string
	Timestamp time.Time
	Author    string
}

// Edge is a typed link from one note to another target.
type Edge struct {
	Type   string
	Target EdgeTarget
}

// EdgeTarget identifies what an edge points at.
type EdgeTarget struct {
	Kind string
	Ref  string
}

// State is the computed state after folding a creation note with its events.
type State struct {
	ID        string
	Kind      string
	Status    string
	Title     string
	Type      string
	Priority  int
	Assignee  string
	Tags      []string
	Body      string
	Targets   []string
	Deps      []string
	Links     []string
	ParentID  string
	Events    []Note
	Comments  []Note
	CreatedAt time.Time
	UpdatedAt time.Time
	NoteOID   git.OID
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

// KindCount reports how many notes exist for a given kind.
type KindCount struct {
	Kind  string
	Count int
}

// DoctorReport contains graph and index health details.
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

// CreateOptions controls creation of a new note.
type CreateOptions struct {
	Kind     string
	Title    string
	Type     string
	Priority int
	Assignee string
	Tags     []string
	Body     string
	Targets  []string
	Edges    []Edge
	Slot     string
}

// AppendOptions controls appending an event or comment to an existing note.
type AppendOptions struct {
	TargetID string
	Kind     string
	Body     string
	Field    string
	Value    string
	Edges    []Edge
	Slot     string
}

// FindOptions filters folded note state results.
type FindOptions struct {
	Kind     string
	Status   string
	Tag      string
	Type     string
	Target   string
	Assignee string
}

// ListOptions filters and sorts summary results.
type ListOptions struct {
	FindOptions
	Limit  int
	SortBy string
}

// Engine is the package boundary consumed by higher-level maitake features.
type Engine interface {
	// Create writes a new creation note with a generated ID.
	Create(opts CreateOptions) (*Note, error)

	// Append writes an event or comment on an existing note.
	Append(opts AppendOptions) (*Note, error)

	// Get returns the raw creation note by ID.
	Get(id string) (*Note, error)

	// Fold returns the computed current state of a note.
	Fold(id string) (*State, error)

	// Context returns open notes targeting a file path.
	Context(path string) ([]State, error)

	// ContextAll returns all notes targeting a file path.
	ContextAll(path string) ([]State, error)

	// Find returns all notes matching the supplied filters.
	Find(opts FindOptions) ([]State, error)

	// List returns summary state for notes matching the supplied filters.
	List(opts ListOptions) ([]StateSummary, error)

	// Refs returns all notes with edges pointing at a target.
	Refs(target string) ([]State, error)

	// Kinds returns all kinds in use with counts.
	Kinds() ([]KindCount, error)

	// BranchUse switches the active notes scope.
	BranchUse(name string) error

	// BranchMerge merges a branch scope into the main scope.
	BranchMerge(name string) error

	// CurrentBranch returns the active scope name, or an empty string for main.
	CurrentBranch() string

	// Doctor reports graph health.
	Doctor() (*DoctorReport, error)

	// Rebuild forces a full index rebuild.
	Rebuild() error
}
