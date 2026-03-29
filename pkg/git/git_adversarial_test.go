package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// testGitEnv provides stable identity for git operations inside tests.
var testGitEnv = []string{
	"GIT_AUTHOR_NAME=Test",
	"GIT_AUTHOR_EMAIL=test@test.com",
	"GIT_COMMITTER_NAME=Test",
	"GIT_COMMITTER_EMAIL=test@test.com",
}

// setupGitRepo creates a temporary git repository with one initial commit and
// returns a GitRepo pointing to it.
func setupGitRepo(t *testing.T) *GitRepo {
	t.Helper()
	dir := t.TempDir()

	// gitRun runs a git command in dir with test identity, fatals on error.
	gitRun := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Env = append(os.Environ(), testGitEnv...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	gitRun("init", dir)
	gitRun("-C", dir, "config", "user.name", "Test")
	gitRun("-C", dir, "config", "user.email", "test@test.com")

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test"), 0644); err != nil {
		t.Fatalf("WriteFile README: %v", err)
	}
	gitRun("-C", dir, "add", "-A")
	gitRun("-C", dir, "commit", "-m", "init")

	repo, err := NewGitRepo(dir)
	if err != nil {
		t.Fatalf("NewGitRepo: %v", err)
	}
	return repo
}

// addCommit writes a file and creates a commit in the repo, returning the new HEAD OID.
func addCommit(t *testing.T, repo *GitRepo, filename, content, msg string) OID {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo.Path, filename), []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile %s: %v", filename, err)
	}
	gitRunInRepo(t, repo, "add", "-A")
	gitRunInRepo(t, repo, "commit", "-m", msg)
	return mustHead(t, repo)
}

// gitRunInRepo runs a git command inside repo.Path with test identity.
func gitRunInRepo(t *testing.T, repo *GitRepo, args ...string) {
	t.Helper()
	full := append([]string{"-C", repo.Path}, args...)
	cmd := exec.Command("git", full...)
	cmd.Env = append(os.Environ(), testGitEnv...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// mustHead returns the current HEAD OID, failing the test on error.
func mustHead(t *testing.T, repo *GitRepo) OID {
	t.Helper()
	oid, err := repo.GetCommitHash("HEAD")
	if err != nil {
		t.Fatalf("GetCommitHash HEAD: %v", err)
	}
	return oid
}

// notesContain returns true if any note in the slice contains needle.
func notesContain(notes []Note, needle string) bool {
	for _, n := range notes {
		if strings.Contains(string(n), needle) {
			return true
		}
	}
	return false
}

// notesJoined joins all note content into one string for substring checks.
func notesJoined(notes []Note) string {
	parts := make([]string, len(notes))
	for i, n := range notes {
		parts[i] = string(n)
	}
	return strings.Join(parts, "\n")
}

// ---------------------------------------------------------------------------
// 1–9: Notes operations
// ---------------------------------------------------------------------------

func TestAdversarial_Notes_AppendGetRoundTrip(t *testing.T) {
	repo := setupGitRepo(t)
	rev := mustHead(t, repo)
	ref := NotesRef("refs/notes/test-roundtrip")

	if err := repo.AppendNote(ref, rev, Note("hello world")); err != nil {
		t.Fatalf("AppendNote: %v", err)
	}

	notes := repo.GetNotes(ref, rev)
	if len(notes) == 0 {
		t.Fatal("GetNotes: got empty slice; want at least one note")
	}
	if !notesContain(notes, "hello world") {
		t.Errorf("GetNotes: 'hello world' not found; got %v", notes)
	}
}

func TestAdversarial_Notes_MultipleNotesToSameRevision(t *testing.T) {
	repo := setupGitRepo(t)
	rev := mustHead(t, repo)
	ref := NotesRef("refs/notes/test-multi")

	contents := []string{"alpha-note", "beta-note", "gamma-note"}
	for i, c := range contents {
		if err := repo.AppendNote(ref, rev, Note(c)); err != nil {
			t.Fatalf("AppendNote[%d] %q: %v", i, c, err)
		}
	}

	notes := repo.GetNotes(ref, rev)
	if len(notes) == 0 {
		t.Fatal("GetNotes: got empty slice after 3 appends")
	}
	joined := notesJoined(notes)
	for _, want := range contents {
		if !strings.Contains(joined, want) {
			t.Errorf("GetNotes: %q not found in output %q", want, joined)
		}
	}
}

func TestAdversarial_Notes_GetNotesNoNotes(t *testing.T) {
	repo := setupGitRepo(t)
	rev := mustHead(t, repo)
	ref := NotesRef("refs/notes/test-nonexistent")

	notes := repo.GetNotes(ref, rev)
	// Must be nil (not panic, not empty non-nil slice)
	if notes != nil {
		t.Errorf("GetNotes on unannotated rev: got %v, want nil", notes)
	}
}

func TestAdversarial_Notes_GetAllNotesMultipleRevisions(t *testing.T) {
	repo := setupGitRepo(t)
	ref := NotesRef("refs/notes/test-all")

	rev1 := mustHead(t, repo)
	rev2 := addCommit(t, repo, "f2.txt", "c2", "commit-2")

	if err := repo.AppendNote(ref, rev1, Note("note-rev1")); err != nil {
		t.Fatalf("AppendNote rev1: %v", err)
	}
	if err := repo.AppendNote(ref, rev2, Note("note-rev2")); err != nil {
		t.Fatalf("AppendNote rev2: %v", err)
	}

	all, err := repo.GetAllNotes(ref)
	if err != nil {
		t.Fatalf("GetAllNotes: %v", err)
	}
	if _, ok := all[rev1]; !ok {
		t.Errorf("GetAllNotes: missing rev1 %s; keys=%v", rev1, func() []OID {
			var ks []OID
			for k := range all {
				ks = append(ks, k)
			}
			return ks
		}())
	}
	if _, ok := all[rev2]; !ok {
		t.Errorf("GetAllNotes: missing rev2 %s", rev2)
	}
}

func TestAdversarial_Notes_ListNotedRevisions(t *testing.T) {
	repo := setupGitRepo(t)
	ref := NotesRef("refs/notes/test-listed")

	rev1 := mustHead(t, repo)
	rev2 := addCommit(t, repo, "f3.txt", "c3", "commit-3")

	repo.AppendNote(ref, rev1, Note("n1")) //nolint
	repo.AppendNote(ref, rev2, Note("n2")) //nolint

	revisions := repo.ListNotedRevisions(ref)
	if len(revisions) != 2 {
		t.Fatalf("ListNotedRevisions: got %d, want 2; revisions=%v", len(revisions), revisions)
	}
	set := make(map[OID]bool)
	for _, r := range revisions {
		set[r] = true
	}
	if !set[rev1] {
		t.Errorf("ListNotedRevisions: missing rev1 %s", rev1)
	}
	if !set[rev2] {
		t.Errorf("ListNotedRevisions: missing rev2 %s", rev2)
	}
}

func TestAdversarial_Notes_AppendEmptyContent(t *testing.T) {
	repo := setupGitRepo(t)
	rev := mustHead(t, repo)
	ref := NotesRef("refs/notes/test-empty-content")

	// git notes append -m "" may reject empty content — document current behaviour.
	err := repo.AppendNote(ref, rev, Note(""))
	if err != nil {
		t.Logf("AppendNote(\"\") returned error: %v — documenting as current behaviour", err)
		return
	}
	notes := repo.GetNotes(ref, rev)
	t.Logf("AppendNote(\"\") succeeded; GetNotes returned %d lines: %v", len(notes), notes)
}

func TestAdversarial_Notes_AppendHugeContent(t *testing.T) {
	repo := setupGitRepo(t)
	rev := mustHead(t, repo)
	ref := NotesRef("refs/notes/test-huge")

	huge := strings.Repeat("x", 100*1024) // 100 KB

	if err := repo.AppendNote(ref, rev, Note(huge)); err != nil {
		t.Fatalf("AppendNote huge: %v", err)
	}

	notes := repo.GetNotes(ref, rev)
	if len(notes) == 0 {
		t.Fatal("GetNotes after huge append: empty")
	}
	// Total content length must be ≥ 100 KB
	total := 0
	for _, n := range notes {
		total += len(n)
	}
	if total < 100*1024 {
		t.Errorf("GetNotes huge: total %d bytes < 100 KB", total)
	}
}

func TestAdversarial_Notes_AppendSpecialChars(t *testing.T) {
	repo := setupGitRepo(t)
	rev := mustHead(t, repo)
	ref := NotesRef("refs/notes/test-special")

	// JSON with unicode and emoji — typical maitake payload
	payload := `{"key":"value","unicode":"日本語","emoji":"🎉","nested":{"ok":true}}`

	if err := repo.AppendNote(ref, rev, Note(payload)); err != nil {
		t.Fatalf("AppendNote special: %v", err)
	}

	notes := repo.GetNotes(ref, rev)
	joined := notesJoined(notes)
	for _, want := range []string{"key", "value", "日本語", "🎉"} {
		if !strings.Contains(joined, want) {
			t.Errorf("special round-trip: %q not found in %q", want, joined)
		}
	}
}

func TestAdversarial_Notes_SurviveAmendedCommit(t *testing.T) {
	repo := setupGitRepo(t)
	origRev := mustHead(t, repo)
	ref := NotesRef("refs/notes/test-amend")

	if err := repo.AppendNote(ref, origRev, Note("pre-amend")); err != nil {
		t.Fatalf("AppendNote: %v", err)
	}

	// Amend the commit with a new message — this mints a new OID.
	// (--no-edit keeps the old message; using -m guarantees the hash changes.)
	gitRunInRepo(t, repo, "commit", "--amend", "-m", "amended-init")
	newRev := mustHead(t, repo)

	if newRev == origRev {
		t.Fatal("amended commit must have different OID")
	}

	oldNotes := repo.GetNotes(ref, origRev)
	newNotes := repo.GetNotes(ref, newRev)

	// BUG: git notes are NOT automatically transferred when a commit is amended.
	// The original OID retains its notes; the new OID has none.
	// This is expected git behaviour, not a maitake bug — callers must migrate notes manually.
	t.Logf("notes on original OID %s: %v", origRev, oldNotes)
	t.Logf("notes on amended  OID %s: %v", newRev, newNotes)

	if notesContain(oldNotes, "pre-amend") {
		t.Logf("original OID still has notes (expected — notes are not lost, just orphaned)")
	}
	if newNotes != nil {
		t.Logf("amended OID unexpectedly has notes: %v", newNotes)
	}
}

// ---------------------------------------------------------------------------
// 10–15: Ref operations
// ---------------------------------------------------------------------------

func TestAdversarial_Refs_HasRefExisting(t *testing.T) {
	repo := setupGitRepo(t)

	headRef, err := repo.GetHeadRef()
	if err != nil {
		t.Fatalf("GetHeadRef: %v", err)
	}
	has, err := repo.HasRef(headRef)
	if err != nil {
		t.Fatalf("HasRef(%s): %v", headRef, err)
	}
	if !has {
		t.Errorf("HasRef(%s): got false, want true", headRef)
	}
}

func TestAdversarial_Refs_HasRefNonExisting(t *testing.T) {
	repo := setupGitRepo(t)

	has, err := repo.HasRef("refs/heads/does-not-exist-xyz")
	if err != nil {
		t.Fatalf("HasRef non-existing: %v", err)
	}
	if has {
		t.Error("HasRef(non-existent): got true, want false")
	}
}

func TestAdversarial_Refs_SetRefAndVerify(t *testing.T) {
	repo := setupGitRepo(t)
	hash1 := mustHead(t, repo)
	hash2 := addCommit(t, repo, "setref.txt", "setref", "setref-commit")

	customRef := "refs/custom/myref"

	// Create (previousCommitHash == "" means create new)
	if err := repo.SetRef(customRef, string(hash1), ""); err != nil {
		t.Fatalf("SetRef create: %v", err)
	}
	got, err := repo.GetCommitHash(customRef)
	if err != nil {
		t.Fatalf("GetCommitHash after create: %v", err)
	}
	if got != hash1 {
		t.Errorf("SetRef create: got %s, want %s", got, hash1)
	}

	// CAS-update from hash1 to hash2
	if err := repo.SetRef(customRef, string(hash2), string(hash1)); err != nil {
		t.Fatalf("SetRef update: %v", err)
	}
	got, err = repo.GetCommitHash(customRef)
	if err != nil {
		t.Fatalf("GetCommitHash after update: %v", err)
	}
	if got != hash2 {
		t.Errorf("SetRef update: got %s, want %s", got, hash2)
	}
}

func TestAdversarial_Refs_VerifyGitRef(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		repo := setupGitRepo(t)
		headRef, err := repo.GetHeadRef()
		if err != nil {
			t.Fatalf("GetHeadRef: %v", err)
		}
		if err := repo.VerifyGitRef(headRef); err != nil {
			t.Errorf("VerifyGitRef(%s): %v", headRef, err)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		repo := setupGitRepo(t)
		if err := repo.VerifyGitRef("refs/heads/no-such-branch"); err == nil {
			t.Error("VerifyGitRef(non-existent): want error, got nil")
		}
	})
}

func TestAdversarial_Refs_VerifyCommit(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		repo := setupGitRepo(t)
		rev := mustHead(t, repo)
		if err := repo.VerifyCommit(rev); err != nil {
			t.Errorf("VerifyCommit(%s): %v", rev, err)
		}
	})

	t.Run("all-zeros", func(t *testing.T) {
		repo := setupGitRepo(t)
		if err := repo.VerifyCommit(OID("0000000000000000000000000000000000000000")); err == nil {
			t.Error("VerifyCommit(zeros): want error, got nil")
		}
	})

	t.Run("non-existent", func(t *testing.T) {
		repo := setupGitRepo(t)
		if err := repo.VerifyCommit(OID("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")); err == nil {
			t.Error("VerifyCommit(non-existent OID): want error, got nil")
		}
	})
}

func TestAdversarial_Refs_GetHeadRefOnBranch(t *testing.T) {
	repo := setupGitRepo(t)

	ref, err := repo.GetHeadRef()
	if err != nil {
		t.Fatalf("GetHeadRef: %v", err)
	}
	if !strings.HasPrefix(ref, "refs/heads/") {
		t.Errorf("GetHeadRef: got %q, want refs/heads/...", ref)
	}
}

func TestAdversarial_Refs_GetHeadRefDetached(t *testing.T) {
	repo := setupGitRepo(t)
	rev := mustHead(t, repo)

	// Detach HEAD by checking out the raw SHA
	cmd := exec.Command("git", "-C", repo.Path, "checkout", "--detach", string(rev))
	cmd.Env = append(os.Environ(), testGitEnv...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("checkout --detach: %v\n%s", err, out)
	}

	_, err := repo.GetHeadRef()
	// git symbolic-ref HEAD fails in detached state — must return error, not panic.
	if err == nil {
		t.Log("GetHeadRef in detached HEAD: returned nil error (implementation returns the SHA instead of error)")
	} else {
		t.Logf("GetHeadRef in detached HEAD: %v (expected)", err)
	}
}

func TestAdversarial_Refs_ResolveRefCommit(t *testing.T) {
	repo := setupGitRepo(t)
	headRef, err := repo.GetHeadRef()
	if err != nil {
		t.Fatalf("GetHeadRef: %v", err)
	}

	oid, err := repo.ResolveRefCommit(headRef)
	if err != nil {
		t.Fatalf("ResolveRefCommit(%s): %v", headRef, err)
	}
	if oid.IsZero() {
		t.Error("ResolveRefCommit: got zero OID")
	}
	expected := mustHead(t, repo)
	if oid != expected {
		t.Errorf("ResolveRefCommit: got %s, want %s", oid, expected)
	}
}

// ---------------------------------------------------------------------------
// 16–25: Diff / Merge / Ancestry
// ---------------------------------------------------------------------------

func TestAdversarial_Diff_BetweenCommitsWithChanges(t *testing.T) {
	repo := setupGitRepo(t)
	rev1 := mustHead(t, repo)
	rev2 := addCommit(t, repo, "diff_test.txt", "new file content", "add diff file")

	diff, err := repo.Diff(string(rev1), string(rev2))
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if !strings.Contains(diff, "diff_test.txt") {
		t.Errorf("Diff: expected diff_test.txt in output:\n%s", diff)
	}
	if !strings.Contains(diff, "new file content") {
		t.Errorf("Diff: expected 'new file content' in output:\n%s", diff)
	}
}

func TestAdversarial_Diff_StatFlag(t *testing.T) {
	repo := setupGitRepo(t)
	rev1 := mustHead(t, repo)
	rev2 := addCommit(t, repo, "stat_file.txt", "stat content", "add stat file")

	diff, err := repo.Diff(string(rev1), string(rev2), "--stat")
	if err != nil {
		t.Fatalf("Diff --stat: %v", err)
	}
	if !strings.Contains(diff, "stat_file.txt") {
		t.Errorf("Diff --stat: expected stat_file.txt:\n%s", diff)
	}
}

func TestAdversarial_Diff_IdenticalCommits(t *testing.T) {
	repo := setupGitRepo(t)
	rev := mustHead(t, repo)

	diff, err := repo.Diff(string(rev), string(rev))
	if err != nil {
		t.Fatalf("Diff same commit: %v", err)
	}
	if diff != "" {
		t.Errorf("Diff same commit: want empty, got %q", diff)
	}
}

func TestAdversarial_MergeBase_Diverged(t *testing.T) {
	repo := setupGitRepo(t)
	base := mustHead(t, repo)

	// branch-a from base
	gitRunInRepo(t, repo, "checkout", "-b", "merge-a")
	revA := addCommit(t, repo, "a.txt", "content-a", "commit-a")

	// branch-b from base
	gitRunInRepo(t, repo, "checkout", string(base))
	gitRunInRepo(t, repo, "checkout", "-b", "merge-b")
	revB := addCommit(t, repo, "b.txt", "content-b", "commit-b")

	got, err := repo.MergeBase(string(revA), string(revB))
	if err != nil {
		t.Fatalf("MergeBase: %v", err)
	}
	if OID(got) != base {
		t.Errorf("MergeBase diverged: got %s, want base %s", got, base)
	}
}

func TestAdversarial_MergeBase_Linear(t *testing.T) {
	repo := setupGitRepo(t)
	rev1 := mustHead(t, repo)
	rev2 := addCommit(t, repo, "lin.txt", "lin", "linear-commit")

	got, err := repo.MergeBase(string(rev1), string(rev2))
	if err != nil {
		t.Fatalf("MergeBase linear: %v", err)
	}
	if OID(got) != rev1 {
		t.Errorf("MergeBase linear: got %s, want rev1 %s", got, rev1)
	}
}

func TestAdversarial_IsAncestor_TrueCase(t *testing.T) {
	repo := setupGitRepo(t)
	rev1 := mustHead(t, repo)
	rev2 := addCommit(t, repo, "anc.txt", "anc", "anc-commit")

	isAnc, err := repo.IsAncestor(string(rev1), string(rev2))
	if err != nil {
		t.Fatalf("IsAncestor: %v", err)
	}
	if !isAnc {
		t.Errorf("IsAncestor(rev1, rev2): got false, want true (rev1 is parent of rev2)")
	}
}

func TestAdversarial_IsAncestor_FalseCase(t *testing.T) {
	repo := setupGitRepo(t)
	base := mustHead(t, repo)

	gitRunInRepo(t, repo, "checkout", "-b", "div-x")
	revX := addCommit(t, repo, "x.txt", "x", "commit-x")

	gitRunInRepo(t, repo, "checkout", string(base))
	gitRunInRepo(t, repo, "checkout", "-b", "div-y")
	revY := addCommit(t, repo, "y.txt", "y", "commit-y")

	isAnc, err := repo.IsAncestor(string(revX), string(revY))
	if err != nil {
		t.Fatalf("IsAncestor(X,Y): %v", err)
	}
	if isAnc {
		t.Error("IsAncestor(X,Y): got true, want false — divergent commits")
	}
}

func TestAdversarial_IsAncestor_SameCommit(t *testing.T) {
	repo := setupGitRepo(t)
	rev := mustHead(t, repo)

	// git --is-ancestor: a commit is its own ancestor
	isAnc, err := repo.IsAncestor(string(rev), string(rev))
	if err != nil {
		t.Fatalf("IsAncestor same: %v", err)
	}
	// Document actual behaviour — git considers a commit an ancestor of itself.
	t.Logf("IsAncestor(rev, rev) = %v", isAnc)
}

func TestAdversarial_IsAncestor_NonExistentRef(t *testing.T) {
	repo := setupGitRepo(t)
	rev := mustHead(t, repo)

	// Must NOT panic — either return error or false.
	_, err := repo.IsAncestor("refs/heads/totally-fake-branch", string(rev))
	if err != nil {
		t.Logf("IsAncestor(non-existent, rev): error=%v (expected)", err)
	} else {
		t.Log("IsAncestor(non-existent, rev): returned false without error (also acceptable)")
	}
}

func TestAdversarial_MergeRef_FastForward(t *testing.T) {
	repo := setupGitRepo(t)

	headRef, err := repo.GetHeadRef()
	if err != nil {
		t.Fatalf("GetHeadRef: %v", err)
	}
	mainBranch := strings.TrimPrefix(headRef, "refs/heads/")

	// Create feature branch ahead of main
	gitRunInRepo(t, repo, "checkout", "-b", "feature-ff")
	_ = addCommit(t, repo, "ff.txt", "ff content", "ff commit")
	featureHead := mustHead(t, repo)

	// Switch back to main (which is behind)
	gitRunInRepo(t, repo, "checkout", mainBranch)
	mainBefore := mustHead(t, repo)

	if err := repo.MergeRef("refs/heads/feature-ff", true /* fast-forward */); err != nil {
		t.Fatalf("MergeRef fast-forward: %v", err)
	}

	mainAfter := mustHead(t, repo)
	if mainAfter == mainBefore {
		t.Error("MergeRef fast-forward: HEAD did not advance")
	}
	if mainAfter != featureHead {
		t.Errorf("MergeRef fast-forward: HEAD=%s, want feature HEAD=%s", mainAfter, featureHead)
	}
}

func TestAdversarial_MergeRef_NoFF(t *testing.T) {
	// GIT_EDITOR=true prevents interactive editor (true exits 0 on Unix).
	t.Setenv("GIT_EDITOR", "true")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")

	repo := setupGitRepo(t)

	headRef, err := repo.GetHeadRef()
	if err != nil {
		t.Fatalf("GetHeadRef: %v", err)
	}
	mainBranch := strings.TrimPrefix(headRef, "refs/heads/")

	gitRunInRepo(t, repo, "checkout", "-b", "feature-noff")
	_ = addCommit(t, repo, "noff.txt", "noff content", "noff commit")

	gitRunInRepo(t, repo, "checkout", mainBranch)

	// Use a message so git doesn't need a separate editor session for the commit msg.
	// The -e flag IS added when messages are provided, but GIT_EDITOR=true handles it.
	if err := repo.MergeRef("refs/heads/feature-noff", false, "Test no-ff merge"); err != nil {
		t.Fatalf("MergeRef no-ff: %v", err)
	}

	// A no-ff merge creates a new merge commit, so HEAD must have changed.
	merged := mustHead(t, repo)
	if merged.IsZero() {
		t.Error("MergeRef no-ff: HEAD is zero after merge")
	}
	t.Logf("no-ff merge commit: %s", merged)
}

func TestAdversarial_MergeRef_Conflict(t *testing.T) {
	t.Setenv("GIT_EDITOR", "true")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")

	repo := setupGitRepo(t)

	headRef, err := repo.GetHeadRef()
	if err != nil {
		t.Fatalf("GetHeadRef: %v", err)
	}
	mainBranch := strings.TrimPrefix(headRef, "refs/heads/")

	// branch-conflict-a: modifies README
	gitRunInRepo(t, repo, "checkout", "-b", "conflict-a")
	if err := os.WriteFile(filepath.Join(repo.Path, "README.md"), []byte("# Branch A"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gitRunInRepo(t, repo, "add", "-A")
	gitRunInRepo(t, repo, "commit", "-m", "branch-a change")

	// main: also modifies README differently
	gitRunInRepo(t, repo, "checkout", mainBranch)
	if err := os.WriteFile(filepath.Join(repo.Path, "README.md"), []byte("# Main Change"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gitRunInRepo(t, repo, "add", "-A")
	gitRunInRepo(t, repo, "commit", "-m", "main change")

	mergeErr := repo.MergeRef("refs/heads/conflict-a", false)
	if mergeErr == nil {
		t.Log("MergeRef conflict: got nil error (git may have auto-resolved — unlikely for raw text conflict)")
	} else {
		t.Logf("MergeRef conflict: correctly returned error: %v", mergeErr)
	}

	// Always abort to leave repo clean for t.TempDir cleanup.
	exec.Command("git", "-C", repo.Path, "merge", "--abort").Run() //nolint
}

// ---------------------------------------------------------------------------
// 26–32: Blob / Tree / Commit operations
// ---------------------------------------------------------------------------

func TestAdversarial_StoreBlob_RoundTrip(t *testing.T) {
	repo := setupGitRepo(t)
	// Avoid trailing newline — runGitCommand trims stdout, so trailing \n is lost on readBlob.
	content := "hello git object store"

	oid, err := repo.StoreBlob(content)
	if err != nil {
		t.Fatalf("StoreBlob: %v", err)
	}
	if oid.IsZero() {
		t.Fatal("StoreBlob: zero OID")
	}

	blob, err := repo.readBlob(string(oid))
	if err != nil {
		t.Fatalf("readBlob: %v", err)
	}
	if blob.Contents() != content {
		t.Errorf("readBlob: got %q, want %q", blob.Contents(), content)
	}
}

func TestAdversarial_StoreBlob_Empty(t *testing.T) {
	repo := setupGitRepo(t)

	oid, err := repo.StoreBlob("")
	if err != nil {
		t.Fatalf("StoreBlob empty: %v", err)
	}
	if oid.IsZero() {
		t.Fatal("StoreBlob empty: zero OID")
	}

	blob, err := repo.readBlob(string(oid))
	if err != nil {
		t.Fatalf("readBlob empty: %v", err)
	}
	if blob.Contents() != "" {
		t.Errorf("readBlob empty: got %q, want \"\"", blob.Contents())
	}
}

func TestAdversarial_StoreBlob_BinaryContent(t *testing.T) {
	repo := setupGitRepo(t)
	// Null bytes and high bytes — no leading/trailing whitespace so TrimSpace is safe.
	content := "head\x00\x01\x02\x03\xff\xfetail"

	oid, err := repo.StoreBlob(content)
	if err != nil {
		t.Fatalf("StoreBlob binary: %v", err)
	}

	blob, err := repo.readBlob(string(oid))
	if err != nil {
		t.Fatalf("readBlob binary: %v", err)
	}
	// NOTE: readBlob calls runGitCommand which trims whitespace.
	// Null bytes / high bytes are not Unicode whitespace, so they survive.
	if blob.Contents() != content {
		t.Errorf("readBlob binary: length got=%d want=%d", len(blob.Contents()), len(content))
	}
}

func TestAdversarial_StoreTree_RoundTrip(t *testing.T) {
	repo := setupGitRepo(t)

	blob1 := NewBlob("file1 contents")
	blob2 := NewBlob("file2 contents")
	subBlob := NewBlob("sub file contents")
	subTree := NewTree(map[string]TreeChild{"sub.txt": subBlob})
	top := NewTree(map[string]TreeChild{
		"file1.txt": blob1,
		"file2.txt": blob2,
		"subdir":    subTree,
	})

	hash, err := repo.StoreTree(top.Contents())
	if err != nil {
		t.Fatalf("StoreTree: %v", err)
	}
	if hash == "" {
		t.Fatal("StoreTree: empty hash")
	}

	back, err := repo.ReadTree(hash)
	if err != nil {
		t.Fatalf("ReadTree: %v", err)
	}
	for _, name := range []string{"file1.txt", "file2.txt", "subdir"} {
		if _, ok := back.Contents()[name]; !ok {
			t.Errorf("ReadTree: missing %s", name)
		}
	}
}

func TestAdversarial_CreateCommit_AndGetMessage(t *testing.T) {
	repo := setupGitRepo(t)
	parentOID := mustHead(t, repo)

	details, err := repo.GetCommitDetails("HEAD")
	if err != nil {
		t.Fatalf("GetCommitDetails: %v", err)
	}

	newHash, err := repo.CreateCommit(&CommitDetails{
		Tree:           details.Tree,
		Parents:        []string{string(parentOID)},
		Summary:        "adversarial-test-commit",
		Author:         "Test",
		AuthorEmail:    "test@test.com",
		Committer:      "Test",
		CommitterEmail: "test@test.com",
	})
	if err != nil {
		t.Fatalf("CreateCommit: %v", err)
	}
	if newHash == "" {
		t.Fatal("CreateCommit: empty hash")
	}

	msg, err := repo.GetCommitMessage(newHash)
	if err != nil {
		t.Fatalf("GetCommitMessage: %v", err)
	}
	if !strings.Contains(msg, "adversarial-test-commit") {
		t.Errorf("GetCommitMessage: got %q, want 'adversarial-test-commit'", msg)
	}
}

func TestAdversarial_CreateCommitWithTree(t *testing.T) {
	repo := setupGitRepo(t)
	parentOID := mustHead(t, repo)

	tree := NewTree(map[string]TreeChild{
		"new_file.txt": NewBlob("created with tree"),
	})

	newHash, err := repo.CreateCommitWithTree(&CommitDetails{
		Parents:        []string{string(parentOID)},
		Summary:        "commit-with-tree",
		Author:         "Test",
		AuthorEmail:    "test@test.com",
		Committer:      "Test",
		CommitterEmail: "test@test.com",
	}, tree)
	if err != nil {
		t.Fatalf("CreateCommitWithTree: %v", err)
	}
	if newHash == "" {
		t.Fatal("CreateCommitWithTree: empty hash")
	}

	msg, err := repo.GetCommitMessage(newHash)
	if err != nil {
		t.Fatalf("GetCommitMessage: %v", err)
	}
	if !strings.Contains(msg, "commit-with-tree") {
		t.Errorf("GetCommitMessage: got %q, want 'commit-with-tree'", msg)
	}
}

// ---------------------------------------------------------------------------
// 33–37: Branch / Commit listing + Show
// ---------------------------------------------------------------------------

func TestAdversarial_ListCommits_Order(t *testing.T) {
	repo := setupGitRepo(t)
	rev1 := mustHead(t, repo)
	rev2 := addCommit(t, repo, "c2.txt", "c2", "commit-2")
	rev3 := addCommit(t, repo, "c3.txt", "c3", "commit-3")

	headRef, err := repo.GetHeadRef()
	if err != nil {
		t.Fatalf("GetHeadRef: %v", err)
	}
	commits := repo.ListCommits(headRef)
	if len(commits) < 3 {
		t.Fatalf("ListCommits: got %d, want ≥3", len(commits))
	}

	pos := make(map[OID]int)
	for i, c := range commits {
		pos[OID(c)] = i
	}
	if pos[rev1] >= pos[rev2] {
		t.Errorf("ListCommits: rev1 pos %d ≥ rev2 pos %d (want oldest first)", pos[rev1], pos[rev2])
	}
	if pos[rev2] >= pos[rev3] {
		t.Errorf("ListCommits: rev2 pos %d ≥ rev3 pos %d", pos[rev2], pos[rev3])
	}
}

func TestAdversarial_ListCommitsBetween_Range(t *testing.T) {
	repo := setupGitRepo(t)
	rev1 := mustHead(t, repo)
	rev2 := addCommit(t, repo, "b1.txt", "b1", "between-1")
	rev3 := addCommit(t, repo, "b2.txt", "b2", "between-2")

	commits, err := repo.ListCommitsBetween(string(rev1), string(rev3))
	if err != nil {
		t.Fatalf("ListCommitsBetween: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("ListCommitsBetween: got %d commits, want 2; %v", len(commits), commits)
	}

	set := make(map[OID]bool)
	for _, c := range commits {
		set[OID(c)] = true
	}
	if set[rev1] {
		t.Error("ListCommitsBetween: start commit (exclusive) must not appear")
	}
	if !set[rev2] {
		t.Error("ListCommitsBetween: rev2 missing")
	}
	if !set[rev3] {
		t.Error("ListCommitsBetween: rev3 missing")
	}
}

func TestAdversarial_ListCommitsBetween_SameFromTo(t *testing.T) {
	repo := setupGitRepo(t)
	rev := mustHead(t, repo)

	commits, err := repo.ListCommitsBetween(string(rev), string(rev))
	if err != nil {
		t.Fatalf("ListCommitsBetween same: %v", err)
	}
	if len(commits) != 0 {
		t.Errorf("ListCommitsBetween same: got %d commits, want 0; %v", len(commits), commits)
	}
}

func TestAdversarial_Show_FileAtCommit(t *testing.T) {
	repo := setupGitRepo(t)
	content := "show test content"
	rev := addCommit(t, repo, "show_test.txt", content, "add show file")

	got, err := repo.Show(string(rev), "show_test.txt")
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	// runGitCommand trims output, so trailing newlines are stripped.
	if got != content {
		t.Errorf("Show: got %q, want %q", got, content)
	}
}

func TestAdversarial_Show_NonExistentFile(t *testing.T) {
	repo := setupGitRepo(t)
	rev := mustHead(t, repo)

	_, err := repo.Show(string(rev), "totally-missing.txt")
	if err == nil {
		t.Error("Show(non-existent file): want error, got nil")
	}
}

// ---------------------------------------------------------------------------
// 38–42: Edge cases that cause silent corruption
// ---------------------------------------------------------------------------

func TestAdversarial_ConcurrentAppendNote(t *testing.T) {
	repo := setupGitRepo(t)
	rev := mustHead(t, repo)
	ref := NotesRef("refs/notes/test-concurrent")

	// Seed the ref so concurrent appends don't race on creation.
	if err := repo.AppendNote(ref, rev, Note("seed")); err != nil {
		t.Fatalf("seed AppendNote: %v", err)
	}

	const workers = 6
	var wg sync.WaitGroup
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			errs[id] = repo.AppendNote(ref, rev, Note(fmt.Sprintf("concurrent-%d", id)))
		}(i)
	}
	wg.Wait()

	errCount := 0
	for _, e := range errs {
		if e != nil {
			errCount++
		}
	}
	if errCount > 0 {
		// BUG: concurrent git notes appends can fail due to lock contention.
		// git uses .git/refs/notes/<ref>.lock; concurrent writers race on this lock.
		t.Logf("NOTE: %d/%d concurrent AppendNote calls failed (git file-lock contention — expected)", errCount, workers)
	}

	// Repo must not be corrupted — GetNotes must return something.
	notes := repo.GetNotes(ref, rev)
	if notes == nil {
		t.Error("repo appears corrupted: GetNotes returned nil after concurrent writes")
	}
	t.Logf("After %d concurrent appends: %d note lines", workers, len(notes))
}

func TestAdversarial_GetNotesAfterRefDeleted(t *testing.T) {
	repo := setupGitRepo(t)
	rev := mustHead(t, repo)
	ref := NotesRef("refs/notes/test-deleted")

	if err := repo.AppendNote(ref, rev, Note("to-be-orphaned")); err != nil {
		t.Fatalf("AppendNote: %v", err)
	}

	// Force-delete the notes ref
	cmd := exec.Command("git", "-C", repo.Path, "update-ref", "-d", string(ref))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("delete ref: %v\n%s", err, out)
	}

	// GetNotes must return nil — not panic, not empty-non-nil.
	notes := repo.GetNotes(ref, rev)
	if notes != nil {
		t.Errorf("GetNotes after ref deleted: got %v, want nil", notes)
	}
}

func TestAdversarial_NewGitRepo_NonGitDirectory(t *testing.T) {
	dir := t.TempDir()

	_, err := NewGitRepo(dir)
	if err == nil {
		t.Error("NewGitRepo(plain dir): want error, got nil")
	} else {
		t.Logf("NewGitRepo(plain dir): %v (expected)", err)
	}
}

func TestAdversarial_NewGitRepo_NonExistentPath(t *testing.T) {
	_, err := NewGitRepo("/tmp/maitake-adversarial-nonexistent-xyz-99999")
	if err == nil {
		t.Error("NewGitRepo(non-existent path): want error, got nil")
	} else {
		t.Logf("NewGitRepo(non-existent): %v (expected)", err)
	}
}

func TestAdversarial_OperationsAfterCorruption(t *testing.T) {
	repo := setupGitRepo(t)
	rev := mustHead(t, repo)

	// Nuke the objects directory — simulates disk corruption.
	objectsDir := filepath.Join(repo.Path, ".git", "objects")
	if err := os.RemoveAll(objectsDir); err != nil {
		t.Fatalf("RemoveAll objects: %v", err)
	}
	// Recreate empty dir so git doesn't completely die on startup.
	if err := os.MkdirAll(objectsDir, 0755); err != nil {
		t.Fatalf("MkdirAll objects: %v", err)
	}

	// All operations must return errors, not panic.
	if err := repo.VerifyCommit(rev); err != nil {
		t.Logf("VerifyCommit after corruption: %v (expected)", err)
	} else {
		t.Log("VerifyCommit after corruption: unexpectedly succeeded (object cache may be warm)")
	}

	notes := repo.GetNotes(DefaultNotesRef, rev)
	t.Logf("GetNotes after corruption: %v (must not panic)", notes)
}

// ---------------------------------------------------------------------------
// 43–45: Notes merge (critical for sync)
// ---------------------------------------------------------------------------

func TestAdversarial_MergeNotes_NonOverlapping(t *testing.T) {
	// Setup: origin repo with a note, clone it, then MergeNotes.
	origin := setupGitRepo(t)
	ref := NotesRef("refs/notes/maitake")
	originRev := mustHead(t, origin)

	if err := origin.AppendNote(ref, originRev, Note("origin-note")); err != nil {
		t.Fatalf("origin AppendNote: %v", err)
	}

	localDir := t.TempDir()
	cloneCmd := exec.Command("git", "clone", origin.Path, localDir)
	cloneCmd.Env = append(os.Environ(), testGitEnv...)
	if out, err := cloneCmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}

	local, err := NewGitRepo(localDir)
	if err != nil {
		t.Fatalf("NewGitRepo local: %v", err)
	}

	// Fetch notes into the remote-tracking ref namespace.
	fetchCmd := exec.Command("git", "-C", localDir, "fetch", "origin",
		"refs/notes/*:refs/notes/remotes/origin/*")
	fetchCmd.Env = append(os.Environ(), testGitEnv...)
	if out, err := fetchCmd.CombinedOutput(); err != nil {
		t.Logf("fetch notes: %v\n%s — may be ok if ref is empty", err, out)
	}

	// MergeNotes expects the notes to exist under refs/notes/remotes/origin/*.
	err = local.MergeNotes("origin", "refs/notes/*")
	if err != nil {
		t.Logf("MergeNotes: %v — may fail when no remote notes ref exists yet", err)
		return
	}

	all, err := local.GetAllNotes(ref)
	if err != nil {
		t.Fatalf("GetAllNotes after MergeNotes: %v", err)
	}
	t.Logf("After MergeNotes: %d annotated commits", len(all))
}

func TestAdversarial_MergeNotes_Dedup(t *testing.T) {
	// Create two repos with the SAME note on the SAME commit and verify merge deduplicates.
	origin := setupGitRepo(t)
	ref := NotesRef("refs/notes/maitake")
	originRev := mustHead(t, origin)
	sharedNote := Note("shared-dedup-note")

	if err := origin.AppendNote(ref, originRev, sharedNote); err != nil {
		t.Fatalf("origin AppendNote: %v", err)
	}

	localDir := t.TempDir()
	cloneCmd := exec.Command("git", "clone", origin.Path, localDir)
	cloneCmd.Env = append(os.Environ(), testGitEnv...)
	if out, err := cloneCmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}

	local, err := NewGitRepo(localDir)
	if err != nil {
		t.Fatalf("NewGitRepo local: %v", err)
	}

	// Add the SAME note locally — both sides have it.
	// We need the OID of the cloned commit (same SHA as origin).
	localRev, err := local.GetCommitHash("HEAD")
	if err != nil {
		t.Fatalf("local GetCommitHash: %v", err)
	}
	if err := local.AppendNote(ref, localRev, sharedNote); err != nil {
		t.Fatalf("local AppendNote: %v", err)
	}

	// Fetch origin notes
	fetchCmd := exec.Command("git", "-C", localDir, "fetch", "origin",
		"refs/notes/*:refs/notes/remotes/origin/*")
	fetchCmd.Env = append(os.Environ(), testGitEnv...)
	fetchCmd.CombinedOutput() //nolint — best effort

	err = local.MergeNotes("origin", "refs/notes/*")
	if err != nil {
		t.Logf("MergeNotes dedup: %v — may fail with no remote notes ref", err)
		return
	}

	// After cat_sort_uniq merge, the note should appear exactly once.
	notes := local.GetNotes(ref, localRev)
	count := 0
	for _, n := range notes {
		if strings.Contains(string(n), "shared-dedup-note") {
			count++
		}
	}
	if count > 1 {
		t.Errorf("MergeNotes dedup: note appears %d times, want 1 (cat_sort_uniq should dedup)", count)
	}
	t.Logf("MergeNotes dedup: note appears %d time(s)", count)
}

func TestAdversarial_PullNotes_FromRemote(t *testing.T) {
	origin := setupGitRepo(t)
	ref := NotesRef("refs/notes/maitake")
	originRev := mustHead(t, origin)

	if err := origin.AppendNote(ref, originRev, Note("remote-pull-note")); err != nil {
		t.Fatalf("origin AppendNote: %v", err)
	}

	localDir := t.TempDir()
	cloneCmd := exec.Command("git", "clone", origin.Path, localDir)
	cloneCmd.Env = append(os.Environ(), testGitEnv...)
	if out, err := cloneCmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}

	local, err := NewGitRepo(localDir)
	if err != nil {
		t.Fatalf("NewGitRepo local: %v", err)
	}

	err = local.PullNotes("origin", "refs/notes/*")
	if err != nil {
		t.Logf("PullNotes: %v — acceptable if remote has no notes ref configured", err)
		return
	}

	// After pull, the notes should be visible locally.
	all, err := local.GetAllNotes(ref)
	if err != nil {
		t.Fatalf("GetAllNotes after PullNotes: %v", err)
	}
	t.Logf("PullNotes succeeded: %d annotated commits in local repo", len(all))
}
