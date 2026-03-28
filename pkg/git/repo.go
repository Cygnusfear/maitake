package git

// Repo is the interface to git operations.
// Implementations: RealRepo (shells out to git CLI), MockRepo (testing).
type Repo interface {
	// Info returns repository metadata (root dir, worktree status, jj detection).
	Info() (*RepoInfo, error)

	// Resolve resolves a revision or path spec to a git object.
	// Accepts: "HEAD", "HEAD:src/auth.ts", "main~3", a full OID, etc.
	Resolve(rev string) (*Object, error)

	// ResolveFilePath resolves a file path (relative to repo root) to its
	// current blob OID at HEAD of the current worktree.
	ResolveFilePath(path string) (*Object, error)

	// CatBlob returns the raw content of a blob object.
	CatBlob(oid OID) ([]byte, error)

	// NoteGet reads the note attached to a target object under the given ref.
	// Returns nil, nil if no note exists.
	NoteGet(ref NotesRef, target OID) ([]byte, error)

	// NoteAdd attaches a note to a target object.
	// Returns ErrNoteExists if a note already exists (use NoteAppend instead).
	NoteAdd(ref NotesRef, target OID, content []byte) error

	// NoteAppend appends content to an existing note (or creates one if none exists).
	// Uses the NotesSeparator to delimit concatenated notes.
	NoteAppend(ref NotesRef, target OID, content []byte) error

	// NoteOverwrite replaces the note on a target object, creating or replacing.
	NoteOverwrite(ref NotesRef, target OID, content []byte) error

	// NoteRemove removes the note on a target object.
	// Returns ErrNoteNotFound if no note exists.
	NoteRemove(ref NotesRef, target OID) error

	// NoteList returns all note entries (note blob → target object) in a ref.
	NoteList(ref NotesRef) ([]NoteEntry, error)

	// NoteRefs returns all maitake-related notes refs present in the repo.
	// Matches: refs/notes/maitake, refs/notes/maitake-*
	NoteRefs() ([]NotesRef, error)

	// LogWithNotes returns recent commits that have notes under the given ref.
	LogWithNotes(ref NotesRef, limit int) ([]CommitWithNote, error)

	// CurrentBranch returns the current branch name. Empty if HEAD is detached.
	CurrentBranch() (string, error)

	// JJChangeID returns the jj change ID for a commit OID.
	// Returns empty string if not a jj repo or no change ID found.
	JJChangeID(commitOID OID) (string, error)

	// TreeEntries lists the entries in a tree object.
	TreeEntries(treeOID OID) ([]TreeEntry, error)

	// HashObject writes content to the object store and returns its OID.
	// Useful for creating synthetic target objects.
	HashObject(content []byte) (OID, error)
}
