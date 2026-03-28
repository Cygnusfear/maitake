package git

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrNotARepo is returned when the directory is not inside a git repository.
	ErrNotARepo = errors.New("not inside a git repository")

	// ErrObjectNotFound is returned when a git object cannot be resolved.
	ErrObjectNotFound = errors.New("object not found")

	// ErrNoteExists is returned when trying to add a note that already exists.
	ErrNoteExists = errors.New("note already exists on this object")

	// ErrNoteNotFound is returned when no note exists on the target object.
	ErrNoteNotFound = errors.New("no note on this object")

	// ErrAmbiguousRef is returned when a partial ref matches multiple objects.
	ErrAmbiguousRef = errors.New("ambiguous reference")
)

// GitError wraps a failed git command with the full command line and stderr.
type GitError struct {
	Cmd    string   // the git subcommand (e.g., "notes", "rev-parse")
	Args   []string // the full argument list
	Stderr string   // captured stderr output
	Err    error    // the underlying exec error (usually *exec.ExitError)
}

func (e *GitError) Error() string {
	stderr := strings.TrimSpace(e.Stderr)
	if stderr != "" {
		return fmt.Sprintf("git %s: %s (%s)", e.Cmd, stderr, e.Err)
	}
	return fmt.Sprintf("git %s: %s", e.Cmd, e.Err)
}

func (e *GitError) Unwrap() error { return e.Err }

// IsNoteExists checks if an error indicates a note already exists.
func IsNoteExists(err error) bool {
	var ge *GitError
	if errors.As(err, &ge) {
		return strings.Contains(ge.Stderr, "Cannot add notes") ||
			strings.Contains(ge.Stderr, "already has notes")
	}
	return errors.Is(err, ErrNoteExists)
}

// IsObjectNotFound checks if an error indicates an object was not found.
func IsObjectNotFound(err error) bool {
	var ge *GitError
	if errors.As(err, &ge) {
		return strings.Contains(ge.Stderr, "not a valid object") ||
			strings.Contains(ge.Stderr, "unknown revision") ||
			strings.Contains(ge.Stderr, "does not exist")
	}
	return errors.Is(err, ErrObjectNotFound)
}
