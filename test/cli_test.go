package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// maiBinary is the path to the compiled mai binary used for CLI tests.
var maiBinary string

func TestMain(m *testing.M) {
	// Build mai binary once for all CLI tests
	tmp, err := os.MkdirTemp("", "mai-test-bin")
	if err != nil {
		panic(err)
	}
	maiBinary = filepath.Join(tmp, "mai")

	build := exec.Command("go", "build", "-o", maiBinary, "./cmd/mai/")
	build.Dir = filepath.Join(mustGetwd(), "..")
	if out, err := build.CombinedOutput(); err != nil {
		panic("building mai: " + err.Error() + "\n" + string(out))
	}

	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

func mustGetwd() string {
	d, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return d
}

// mai runs the mai CLI in the given directory and returns stdout.
func mai(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(maiBinary, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
		// Prevent git from opening an interactive editor (e.g. during merge -e)
		"GIT_EDITOR=true",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mai %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// maiFail runs the mai CLI expecting failure. Returns stderr+stdout.
func maiFail(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(maiBinary, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
		// Prevent git from opening an interactive editor (e.g. during merge -e)
		"GIT_EDITOR=true",
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected mai %s to fail, but it succeeded:\n%s", strings.Join(args, " "), out)
	}
	return strings.TrimSpace(string(out))
}

// gitRun runs a git command in the given directory.
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
		"GIT_EDITOR=true",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// setupTestRepo creates a git repo with an initial commit and some files.
func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.name", "Test")
	gitRun(t, dir, "config", "user.email", "test@test.com")

	os.MkdirAll(filepath.Join(dir, "src"), 0755)
	os.WriteFile(filepath.Join(dir, "src", "auth.ts"), []byte("export function refreshToken() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "src", "http.ts"), []byte("export function fetch() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test Project\n"), 0644)

	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "init")
	return dir
}

// === TICKET LIFECYCLE ===

func TestCLI_CreateAndShow(t *testing.T) {
	dir := setupTestRepo(t)
	id := mai(t, dir, "ticket", "Fix auth bug", "-p", "1", "--tags", "auth,critical", "--target", "src/auth.ts", "-d", "Race condition in token refresh")

	out := mai(t, dir, "show", id)
	if !strings.Contains(out, "Fix auth bug") {
		t.Errorf("show missing title:\n%s", out)
	}
	if !strings.Contains(out, "auth, critical") {
		t.Errorf("show missing tags:\n%s", out)
	}
	if !strings.Contains(out, "src/auth.ts") {
		t.Errorf("show missing target:\n%s", out)
	}
	if !strings.Contains(out, "[open]") {
		t.Errorf("show missing open status:\n%s", out)
	}
}

func TestCLI_FullLifecycle(t *testing.T) {
	dir := setupTestRepo(t)
	id := mai(t, dir, "ticket", "Lifecycle test")

	// Start
	out := mai(t, dir, "start", id)
	if !strings.Contains(out, "in_progress") {
		t.Errorf("start: %s", out)
	}

	// Comment
	mai(t, dir, "add-note", id, "Working on it now")

	// Tag
	mai(t, dir, "tag", id, "+urgent")

	// Assign
	mai(t, dir, "assign", id, "Alice")

	// Show — verify all mutations
	out = mai(t, dir, "show", id)
	if !strings.Contains(out, "in_progress") {
		t.Errorf("expected in_progress:\n%s", out)
	}
	if !strings.Contains(out, "urgent") {
		t.Errorf("expected urgent tag:\n%s", out)
	}
	if !strings.Contains(out, "Alice") {
		t.Errorf("expected Alice assignee:\n%s", out)
	}
	if !strings.Contains(out, "Working on it now") {
		t.Errorf("expected comment:\n%s", out)
	}

	// Close
	out = mai(t, dir, "close", id, "-m", "Done and shipped")
	if !strings.Contains(out, "closed") {
		t.Errorf("close: %s", out)
	}

	// Reopen
	out = mai(t, dir, "reopen", id)
	if !strings.Contains(out, "open") {
		t.Errorf("reopen: %s", out)
	}
}

func TestCLI_ReviewBornOpen(t *testing.T) {
	dir := setupTestRepo(t)
	id := mai(t, dir, "review", "Code review findings", "-d", "Add mutex around token refresh")

	out := mai(t, dir, "show", id)
	if !strings.Contains(out, "[open]") {
		t.Errorf("review should be born open:\n%s", out)
	}
}

func TestCLI_ArtifactBornClosed(t *testing.T) {
	dir := setupTestRepo(t)
	id := mai(t, dir, "artifact", "Research findings", "-d", "Oracle output")

	out := mai(t, dir, "show", id)
	if !strings.Contains(out, "[closed]") {
		t.Errorf("artifact should be born closed:\n%s", out)
	}
}

// === CONTEXT ===

func TestCLI_Context_FiltersOpenOnly(t *testing.T) {
	dir := setupTestRepo(t)

	// Open ticket targeting auth.ts
	id1 := mai(t, dir, "ticket", "Open ticket", "--target", "src/auth.ts")
	// Warning on auth.ts (open)
	id2 := mai(t, dir, "warn", "src/auth.ts", "Fragile code here")
	// Closed artifact on auth.ts (born closed)
	_ = mai(t, dir, "artifact", "Old review", "--target", "src/auth.ts")

	out := mai(t, dir, "context", "src/auth.ts")

	if !strings.Contains(out, id1) {
		t.Errorf("context should show open ticket %s:\n%s", id1, out)
	}
	if !strings.Contains(out, id2) {
		t.Errorf("context should show warning %s:\n%s", id2, out)
	}
	if strings.Contains(out, "Old review") {
		t.Errorf("context should NOT show closed review:\n%s", out)
	}
}

func TestCLI_Context_MultipleFiles(t *testing.T) {
	dir := setupTestRepo(t)
	mai(t, dir, "ticket", "Auth bug", "--target", "src/auth.ts")
	mai(t, dir, "ticket", "HTTP bug", "--target", "src/http.ts")

	authCtx := mai(t, dir, "context", "src/auth.ts")
	httpCtx := mai(t, dir, "context", "src/http.ts")

	if strings.Contains(authCtx, "HTTP bug") {
		t.Error("auth context should not show http ticket")
	}
	if strings.Contains(httpCtx, "Auth bug") {
		t.Error("http context should not show auth ticket")
	}
}

// === DEPS AND BLOCKING ===

func TestCLI_DepsReadyBlocked(t *testing.T) {
	dir := setupTestRepo(t)
	parentID := mai(t, dir, "ticket", "Parent task")
	childID := mai(t, dir, "ticket", "Child task")
	mai(t, dir, "dep", parentID, childID)

	// Parent is blocked because child is open
	blocked := mai(t, dir, "blocked")
	if !strings.Contains(blocked, parentID) {
		t.Errorf("parent should be blocked:\n%s", blocked)
	}

	ready := mai(t, dir, "ready")
	if strings.Contains(ready, parentID) {
		t.Errorf("parent should NOT be ready:\n%s", ready)
	}
	if !strings.Contains(ready, childID) {
		t.Errorf("child should be ready (no deps):\n%s", ready)
	}

	// Close the child
	mai(t, dir, "close", childID)

	// Now parent is ready
	ready = mai(t, dir, "ready")
	if !strings.Contains(ready, parentID) {
		t.Errorf("parent should be ready after child closed:\n%s", ready)
	}

	blocked = mai(t, dir, "blocked")
	if strings.Contains(blocked, parentID) {
		t.Errorf("parent should no longer be blocked:\n%s", blocked)
	}
}

func TestCLI_Link(t *testing.T) {
	dir := setupTestRepo(t)
	id1 := mai(t, dir, "ticket", "Ticket A")
	id2 := mai(t, dir, "ticket", "Ticket B")
	mai(t, dir, "link", id1, id2)

	out1 := mai(t, dir, "show", id1)
	if !strings.Contains(out1, id2) {
		t.Errorf("ticket A should link to B:\n%s", out1)
	}
	out2 := mai(t, dir, "show", id2)
	if !strings.Contains(out2, id1) {
		t.Errorf("ticket B should link to A:\n%s", out2)
	}
}

// === PERSISTENCE ===

func TestCLI_PersistenceAcrossInvocations(t *testing.T) {
	dir := setupTestRepo(t)

	// Create in one invocation
	id := mai(t, dir, "ticket", "Persistent ticket", "-d", "Should survive restarts")

	// Start in another
	mai(t, dir, "start", id)

	// Comment in another
	mai(t, dir, "add-note", id, "Progress update")

	// Show in yet another — all state should be present
	out := mai(t, dir, "show", id)
	if !strings.Contains(out, "in_progress") {
		t.Errorf("status not persisted:\n%s", out)
	}
	if !strings.Contains(out, "Progress update") {
		t.Errorf("comment not persisted:\n%s", out)
	}
}

// === MULTIPLE NOTES ON SAME OBJECT ===

func TestCLI_MultipleNotesOnSameFile(t *testing.T) {
	dir := setupTestRepo(t)

	// Multiple notes targeting the same file
	id1 := mai(t, dir, "warn", "src/auth.ts", "Race condition")
	id2 := mai(t, dir, "warn", "src/auth.ts", "Missing input validation")
	id3 := mai(t, dir, "ticket", "Fix auth", "--target", "src/auth.ts")

	if id1 == id2 || id2 == id3 {
		t.Error("IDs should be unique")
	}

	out := mai(t, dir, "context", "src/auth.ts")
	if !strings.Contains(out, id1) || !strings.Contains(out, id2) || !strings.Contains(out, id3) {
		t.Errorf("context should show all 3 notes:\n%s", out)
	}
}

// === AFTER FILE CHANGES ===

func TestCLI_NotesOnChangedFile(t *testing.T) {
	dir := setupTestRepo(t)

	// Create note targeting file
	id := mai(t, dir, "warn", "src/auth.ts", "Race condition here")

	// Modify the file and commit
	os.WriteFile(filepath.Join(dir, "src", "auth.ts"), []byte("export function refreshToken() { /* fixed */ }\n"), 0644)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "fix auth")

	// Note should still be findable by ID
	out := mai(t, dir, "show", id)
	if !strings.Contains(out, "Race condition") {
		t.Errorf("note should survive file change:\n%s", out)
	}

	// Context might not show it (blob OID changed) — that's OK for now
	// The important thing is the note isn't lost
}

// === CONCURRENT NOTES (SEPARATE OBJECTS) ===

func TestCLI_ManyNotesScaleTest(t *testing.T) {
	dir := setupTestRepo(t)

	// Create 50 tickets
	var ids []string
	for i := 0; i < 50; i++ {
		id := mai(t, dir, "ticket", "Ticket "+strings.Repeat("X", i%10))
		ids = append(ids, id)
	}

	// List should show all 50
	out := mai(t, dir, "ls")
	lines := strings.Split(out, "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	if count != 50 {
		t.Errorf("ls showed %d notes, want 50", count)
	}

	// Show each one
	for _, id := range ids {
		out := mai(t, dir, "show", id)
		if !strings.Contains(out, id) {
			t.Errorf("show %s failed:\n%s", id, out)
		}
	}
}

// === DOCTOR ===

func TestCLI_Doctor(t *testing.T) {
	dir := setupTestRepo(t)
	mai(t, dir, "ticket", "One")
	mai(t, dir, "warn", "src/auth.ts", "Two")
	id := mai(t, dir, "ticket", "Three")
	mai(t, dir, "add-note", id, "A comment")
	mai(t, dir, "start", id)

	out := mai(t, dir, "doctor")
	if !strings.Contains(out, "Notes:") {
		t.Errorf("doctor output malformed:\n%s", out)
	}
	if !strings.Contains(out, "ticket") {
		t.Errorf("doctor should show ticket kind:\n%s", out)
	}
}

// === KINDS ===

func TestCLI_Kinds(t *testing.T) {
	dir := setupTestRepo(t)
	mai(t, dir, "ticket", "T1")
	mai(t, dir, "ticket", "T2")
	mai(t, dir, "warn", "src/auth.ts", "W1")

	out := mai(t, dir, "kinds")
	if !strings.Contains(out, "ticket") || !strings.Contains(out, "warning") {
		t.Errorf("kinds missing:\n%s", out)
	}
}

// === ERROR CASES ===

func TestCLI_ShowNonexistent(t *testing.T) {
	dir := setupTestRepo(t)
	out := maiFail(t, dir, "show", "nonexistent-id")
	if !strings.Contains(out, "not found") {
		t.Errorf("expected 'not found' error:\n%s", out)
	}
}

func TestCLI_StartNonexistent(t *testing.T) {
	dir := setupTestRepo(t)
	out := maiFail(t, dir, "start", "nonexistent-id")
	if !strings.Contains(out, "not found") {
		t.Errorf("expected error:\n%s", out)
	}
}

// === DEP TREE / UNDEP / UNLINK ===

func TestCLI_DepTree(t *testing.T) {
	dir := setupTestRepo(t)
	a := mai(t, dir, "ticket", "Root task")
	b := mai(t, dir, "ticket", "Child task")
	c := mai(t, dir, "ticket", "Grandchild")
	mai(t, dir, "dep", a, b)
	mai(t, dir, "dep", b, c)

	out := mai(t, dir, "dep", "tree", a)
	if !strings.Contains(out, a) {
		t.Errorf("tree should show root:\n%s", out)
	}
	if !strings.Contains(out, b) {
		t.Errorf("tree should show child:\n%s", out)
	}
	if !strings.Contains(out, c) {
		t.Errorf("tree should show grandchild:\n%s", out)
	}
	if !strings.Contains(out, "└──") {
		t.Errorf("tree should have connectors:\n%s", out)
	}
}

func TestCLI_Undep(t *testing.T) {
	dir := setupTestRepo(t)
	a := mai(t, dir, "ticket", "Parent")
	b := mai(t, dir, "ticket", "Dep")
	mai(t, dir, "dep", a, b)

	blocked := mai(t, dir, "blocked")
	if !strings.Contains(blocked, a) {
		t.Errorf("should be blocked before undep:\n%s", blocked)
	}

	mai(t, dir, "undep", a, b)

	ready := mai(t, dir, "ready")
	if !strings.Contains(ready, a) {
		t.Errorf("should be ready after undep:\n%s", ready)
	}
}

func TestCLI_Unlink(t *testing.T) {
	dir := setupTestRepo(t)
	a := mai(t, dir, "ticket", "A")
	b := mai(t, dir, "ticket", "B")
	mai(t, dir, "link", a, b)

	out := mai(t, dir, "show", a)
	if !strings.Contains(out, b) {
		t.Errorf("should show link:\n%s", out)
	}

	mai(t, dir, "unlink", a, b)

	out = mai(t, dir, "show", a)
	if strings.Contains(out, "links: "+b) {
		t.Errorf("link should be removed:\n%s", out)
	}
}

func TestCLI_DefaultLsShowsOnlyOpen(t *testing.T) {
	dir := setupTestRepo(t)
	open := mai(t, dir, "ticket", "Open one")
	closed := mai(t, dir, "ticket", "Will close")
	mai(t, dir, "close", closed)

	out := mai(t, dir, "ls")
	if !strings.Contains(out, open) {
		t.Errorf("ls should show open ticket:\n%s", out)
	}
	if strings.Contains(out, closed) {
		t.Errorf("ls should NOT show closed ticket:\n%s", out)
	}

	out = mai(t, dir, "ls", "--status=all")
	if !strings.Contains(out, closed) {
		t.Errorf("ls --status=all should show closed:\n%s", out)
	}
}

// === PARTIAL ID MATCHING ===

func TestCLI_PartialIDMatch(t *testing.T) {
	dir := setupTestRepo(t)
	id := mai(t, dir, "ticket", "Partial match test")

	// Use last 4 chars as partial
	partial := id[len(id)-4:]
	out := mai(t, dir, "show", partial)
	if !strings.Contains(out, "Partial match test") {
		t.Errorf("partial ID %q should match %q:\n%s", partial, id, out)
	}
}

// === INIT ===

func TestCLI_Init(t *testing.T) {
	dir := setupTestRepo(t)
	out := mai(t, dir, "init")
	if !strings.Contains(out, "hooks") {
		t.Errorf("init output: %s", out)
	}

	hookPath := filepath.Join(dir, ".maitake", "hooks", "pre-write")
	if _, err := os.Stat(hookPath); err != nil {
		t.Errorf("pre-write hook not created: %v", err)
	}
}

// === FILE-LOCATED COMMENTS ===

func TestCLI_FileLocatedComment(t *testing.T) {
	dir := setupTestRepo(t)

	// Ticket targeting two files
	id := mai(t, dir, "ticket", "Auth hardening", "--target", "src/auth.ts", "--target", "src/http.ts")

	// Add file-specific comments
	mai(t, dir, "add-note", id, "--file", "src/auth.ts", "Add mutex around token refresh")
	mai(t, dir, "add-note", id, "--file", "src/http.ts", "Add backoff to retry logic")
	mai(t, dir, "add-note", id, "General progress comment")

	// Show should have all 3 comments
	out := mai(t, dir, "show", id)
	if !strings.Contains(out, "mutex") {
		t.Errorf("show missing auth comment:\n%s", out)
	}
	if !strings.Contains(out, "backoff") {
		t.Errorf("show missing http comment:\n%s", out)
	}
	if !strings.Contains(out, "General progress") {
		t.Errorf("show missing general comment:\n%s", out)
	}

	// Context for auth.ts should show the ticket + the auth-specific comment
	authCtx := mai(t, dir, "context", "src/auth.ts")
	if !strings.Contains(authCtx, id) {
		t.Errorf("auth context should show ticket:\n%s", authCtx)
	}
	if !strings.Contains(authCtx, "mutex") {
		t.Errorf("auth context should show auth comment:\n%s", authCtx)
	}
	if strings.Contains(authCtx, "backoff") {
		t.Errorf("auth context should NOT show http comment:\n%s", authCtx)
	}

	// Context for http.ts should show the ticket + the http-specific comment
	httpCtx := mai(t, dir, "context", "src/http.ts")
	if !strings.Contains(httpCtx, "backoff") {
		t.Errorf("http context should show http comment:\n%s", httpCtx)
	}
	if strings.Contains(httpCtx, "mutex") {
		t.Errorf("http context should NOT show auth comment:\n%s", httpCtx)
	}
}

func TestCLI_FileLocatedComment_LineLevel(t *testing.T) {
	dir := setupTestRepo(t)
	id := mai(t, dir, "ticket", "Line level test", "--target", "src/auth.ts")

	mai(t, dir, "add-note", id, "--file", "src/auth.ts", "--line", "42", "Race condition on this line")

	authCtx := mai(t, dir, "context", "src/auth.ts")
	if !strings.Contains(authCtx, ":42") {
		t.Errorf("context should show line number:\n%s", authCtx)
	}
	if !strings.Contains(authCtx, "Race condition") {
		t.Errorf("context should show comment body:\n%s", authCtx)
	}
}

func TestCLI_FileLocatedComment_TicketWithoutTarget(t *testing.T) {
	dir := setupTestRepo(t)

	// Ticket with NO file target
	id := mai(t, dir, "ticket", "General task")

	// But add a file-specific comment
	mai(t, dir, "add-note", id, "--file", "src/auth.ts", "This file needs attention")

	// Context for auth.ts should pick up the ticket because of the located comment
	authCtx := mai(t, dir, "context", "src/auth.ts")
	if !strings.Contains(authCtx, "General task") {
		t.Errorf("context should show ticket with file-located comment:\n%s", authCtx)
	}
	if !strings.Contains(authCtx, "needs attention") {
		t.Errorf("context should show the comment:\n%s", authCtx)
	}
}

// === JSON OUTPUT ===

func TestCLI_JSON_Ls(t *testing.T) {
	dir := setupTestRepo(t)
	id := mai(t, dir, "ticket", "JSON test", "-p", "1", "--tags", "test")

	out := mai(t, dir, "--json", "ls")
	if !strings.Contains(out, `"id"`) {
		t.Errorf("--json ls should output JSON:\n%s", out)
	}
	if !strings.Contains(out, id) {
		t.Errorf("--json ls should contain ticket ID:\n%s", out)
	}
	if !strings.Contains(out, `"createdAt"`) {
		t.Errorf("--json ls should have createdAt field:\n%s", out)
	}
}

func TestCLI_JSON_Ls_TimestampsNotZero(t *testing.T) {
	dir := setupTestRepo(t)
	mai(t, dir, "ticket", "Timestamp check")

	out := mai(t, dir, "--json", "ls")
	if strings.Contains(out, "0001-01-01") {
		t.Errorf("--json ls has zero timestamps — Time not hydrated:\n%s", out)
	}
}

func TestCLI_JSON_Show(t *testing.T) {
	dir := setupTestRepo(t)
	id := mai(t, dir, "ticket", "Show JSON", "-d", "Body text")
	mai(t, dir, "start", id)
	mai(t, dir, "add-note", id, "A comment")

	out := mai(t, dir, "--json", "show", id)
	if !strings.Contains(out, `"status"`) {
		t.Errorf("--json show should have status:\n%s", out)
	}
	if !strings.Contains(out, `"in_progress"`) {
		t.Errorf("--json show should be in_progress:\n%s", out)
	}
	if !strings.Contains(out, `"A comment"`) {
		t.Errorf("--json show should have comment:\n%s", out)
	}
	if strings.Contains(out, "0001-01-01") {
		t.Errorf("--json show has zero timestamps:\n%s", out)
	}
}

func TestCLI_JSON_Show_DepsLinks(t *testing.T) {
	dir := setupTestRepo(t)
	a := mai(t, dir, "ticket", "A")
	b := mai(t, dir, "ticket", "B")
	mai(t, dir, "dep", a, b)

	out := mai(t, dir, "--json", "show", a)
	if !strings.Contains(out, b) {
		t.Errorf("--json show should have dep ID:\n%s", out)
	}
	if !strings.Contains(out, `"deps"`) {
		t.Errorf("--json show should have deps field:\n%s", out)
	}
}

func TestCLI_JSON_Context(t *testing.T) {
	dir := setupTestRepo(t)
	mai(t, dir, "ticket", "Auth fix", "--target", "src/auth.ts")

	out := mai(t, dir, "--json", "context", "src/auth.ts")
	if !strings.Contains(out, `"id"`) {
		t.Errorf("--json context should output JSON:\n%s", out)
	}
	if !strings.Contains(out, "Auth fix") {
		t.Errorf("--json context should contain ticket:\n%s", out)
	}
}

func TestCLI_JSON_Ls_DepsLinksInSummary(t *testing.T) {
	dir := setupTestRepo(t)
	a := mai(t, dir, "ticket", "Parent")
	b := mai(t, dir, "ticket", "Child")
	c := mai(t, dir, "ticket", "Related")
	mai(t, dir, "dep", a, b)
	mai(t, dir, "link", a, c)

	out := mai(t, dir, "--json", "ls")
	// The summary for 'a' should contain deps and links
	if !strings.Contains(out, `"deps"`) {
		t.Errorf("--json ls summary should include deps:\n%s", out)
	}
	if !strings.Contains(out, `"links"`) {
		t.Errorf("--json ls summary should include links:\n%s", out)
	}
}

func TestCLI_JSON_BranchStamped(t *testing.T) {
	dir := setupTestRepo(t)
	mai(t, dir, "ticket", "Branch test")

	out := mai(t, dir, "--json", "ls")
	if !strings.Contains(out, `"branch"`) {
		t.Errorf("--json ls should include branch:\n%s", out)
	}
}

// === -C FLAG ===

func TestCLI_CFlag(t *testing.T) {
	dir := setupTestRepo(t)
	id := mai(t, dir, "ticket", "Remote dir test")

	// Query from a different directory using -C
	other := t.TempDir()
	out := mai(t, other, "-C", dir, "show", id)
	if !strings.Contains(out, "Remote dir test") {
		t.Errorf("-C flag should query the target dir:\n%s", out)
	}
}

func TestCLI_CFlag_JSON(t *testing.T) {
	dir := setupTestRepo(t)
	mai(t, dir, "ticket", "C flag JSON")

	other := t.TempDir()
	out := mai(t, other, "-C", dir, "--json", "ls")
	if !strings.Contains(out, "C flag JSON") {
		t.Errorf("-C + --json should work:\n%s", out)
	}
}

// === TAG REMOVAL ===

func TestCLI_TagAddRemove(t *testing.T) {
	dir := setupTestRepo(t)
	id := mai(t, dir, "ticket", "Tag test", "--tags", "initial")

	mai(t, dir, "tag", id, "+added")
	out := mai(t, dir, "show", id)
	if !strings.Contains(out, "added") {
		t.Errorf("tag +added not applied:\n%s", out)
	}

	mai(t, dir, "tag", id, "-initial")
	out = mai(t, dir, "show", id)
	if strings.Contains(out, "initial") {
		t.Errorf("tag -initial not removed:\n%s", out)
	}
	if !strings.Contains(out, "added") {
		t.Errorf("tag added should remain:\n%s", out)
	}
}
