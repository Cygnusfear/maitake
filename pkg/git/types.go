package git

// OID is a 40-character hex SHA git object identifier.
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

// NotesRef is a fully qualified git notes reference.
type NotesRef string

// String implements fmt.Stringer.
func (r NotesRef) String() string { return string(r) }

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
