package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// === mai check ===

func TestLattice_Check_EmptyRepo(t *testing.T) {
	dir := setupTestRepo(t)
	out := mai(t, dir, "check")
	if !strings.Contains(out, "All refs resolve") && !strings.Contains(out, "0 code refs") {
		t.Errorf("check on empty repo should pass cleanly:\n%s", out)
	}
}

func TestLattice_Check_ValidCodeRef(t *testing.T) {
	dir := setupTestRepo(t)

	// Create a note
	id := mai(t, dir, "ticket", "Auth fix")

	// Add a code ref pointing at it
	goFile := filepath.Join(dir, "src", "main.go")
	os.WriteFile(goFile, []byte("package main\n// @mai: [["+id+"]]\nfunc main() {}\n"), 0644)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "add code ref")

	out := mai(t, dir, "check")
	if !strings.Contains(out, "All refs resolve") {
		t.Errorf("check should pass with valid code ref:\n%s", out)
	}
}

func TestLattice_Check_BrokenCodeRef(t *testing.T) {
	dir := setupTestRepo(t)

	// Add code ref pointing at nothing
	goFile := filepath.Join(dir, "src", "main.go")
	os.WriteFile(goFile, []byte("package main\n// @mai: [[nonexistent-note]]\nfunc main() {}\n"), 0644)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "add broken code ref")

	out := maiFail(t, dir, "check")
	if !strings.Contains(out, "broken") {
		t.Errorf("check should report broken ref:\n%s", out)
	}
	if !strings.Contains(out, "nonexistent-note") {
		t.Errorf("check should mention the broken target:\n%s", out)
	}
}

func TestLattice_Check_ValidWikiRef(t *testing.T) {
	dir := setupTestRepo(t)

	// Create two notes, one referencing the other
	id1 := mai(t, dir, "ticket", "First note")
	mai(t, dir, "ticket", "Second note", "-d", "See [["+id1+"]] for context")

	out := mai(t, dir, "check")
	if !strings.Contains(out, "All refs resolve") {
		t.Errorf("check should pass with valid wiki ref:\n%s", out)
	}
}

func TestLattice_Check_BrokenWikiRef(t *testing.T) {
	dir := setupTestRepo(t)

	mai(t, dir, "ticket", "Note with broken link", "-d", "See [[zzz-not-real]] for nothing")

	out := maiFail(t, dir, "check")
	if !strings.Contains(out, "broken") {
		t.Errorf("check should report broken wiki link:\n%s", out)
	}
	if !strings.Contains(out, "zzz-not-real") {
		t.Errorf("check should mention the broken target:\n%s", out)
	}
}

func TestLattice_Check_WikiRefToFile(t *testing.T) {
	dir := setupTestRepo(t)

	// Create a note referencing an actual file
	mai(t, dir, "ticket", "File ref", "-d", "See [[src/auth.ts]] for implementation")

	out := mai(t, dir, "check")
	if !strings.Contains(out, "All refs resolve") {
		t.Errorf("check should pass — src/auth.ts exists:\n%s", out)
	}
}

func TestLattice_Check_JSON(t *testing.T) {
	dir := setupTestRepo(t)
	mai(t, dir, "ticket", "Test note")

	out := mai(t, dir, "--json", "check")
	if !strings.Contains(out, "{") {
		t.Errorf("--json check should return JSON:\n%s", out)
	}
}

// === mai refs ===

func TestLattice_Refs_CodeRefFound(t *testing.T) {
	dir := setupTestRepo(t)

	id := mai(t, dir, "ticket", "Auth fix")

	// Add code ref
	goFile := filepath.Join(dir, "src", "main.go")
	os.WriteFile(goFile, []byte("package main\n// @mai: [["+id+"]]\nfunc main() {}\n"), 0644)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "add code ref")

	out := mai(t, dir, "refs", id)
	if !strings.Contains(out, "main.go") {
		t.Errorf("refs should find code reference in main.go:\n%s", out)
	}
}

func TestLattice_Refs_WikiRefFound(t *testing.T) {
	dir := setupTestRepo(t)

	id1 := mai(t, dir, "ticket", "Target note")
	mai(t, dir, "ticket", "Referencing note", "-d", "See [["+id1+"]]")

	out := mai(t, dir, "refs", id1)
	if !strings.Contains(out, "[["+id1+"]]") || !strings.Contains(out, "Note refs") {
		t.Errorf("refs should find wiki reference:\n%s", out)
	}
}

func TestLattice_Refs_NoReferences(t *testing.T) {
	dir := setupTestRepo(t)

	id := mai(t, dir, "ticket", "Lonely note")

	out := mai(t, dir, "refs", id)
	if !strings.Contains(out, "No references") {
		t.Errorf("refs should report nothing found:\n%s", out)
	}
}

func TestLattice_Refs_FilePathAsTarget(t *testing.T) {
	dir := setupTestRepo(t)

	// Create a note targeting a file
	mai(t, dir, "ticket", "Auth work", "--target", "src/auth.ts")

	// refs on the file path
	out := mai(t, dir, "refs", "src/auth.ts")
	// The refs command searches for code refs AND wiki refs mentioning the target.
	// A note with --target doesn't create a wiki ref, but we can check code refs.
	t.Logf("refs for file path: %s", out)
}

func TestLattice_Refs_JSON(t *testing.T) {
	dir := setupTestRepo(t)
	id := mai(t, dir, "ticket", "Test")

	out := mai(t, dir, "--json", "refs", id)
	if !strings.Contains(out, "target") {
		t.Errorf("--json refs should return JSON with target field:\n%s", out)
	}
}

// === mai expand ===

func TestLattice_Expand_ResolvesRef(t *testing.T) {
	dir := setupTestRepo(t)

	id := mai(t, dir, "ticket", "Auth race fix", "-d", "Token refresh race condition")

	out := mai(t, dir, "expand", "Check [["+id+"]] for the fix")
	if !strings.Contains(out, id) {
		t.Errorf("expand should include the note ID:\n%s", out)
	}
	if !strings.Contains(out, "mai-context") {
		t.Errorf("expand should include context block:\n%s", out)
	}
	if !strings.Contains(out, "Auth race fix") {
		t.Errorf("expand should include note title in context:\n%s", out)
	}
}

func TestLattice_Expand_NoRefs(t *testing.T) {
	dir := setupTestRepo(t)

	out := mai(t, dir, "expand", "Plain text no links")
	if strings.Contains(out, "mai-context") {
		t.Errorf("expand with no refs should not add context block:\n%s", out)
	}
	if !strings.Contains(out, "Plain text no links") {
		t.Errorf("expand should return input text:\n%s", out)
	}
}

func TestLattice_Expand_BrokenRef(t *testing.T) {
	dir := setupTestRepo(t)

	out := mai(t, dir, "expand", "See [[zzz-fake]] for info")
	// Should handle gracefully — either leave as-is or mark unresolved
	if !strings.Contains(out, "zzz-fake") {
		t.Errorf("expand should preserve broken ref text:\n%s", out)
	}
}

func TestLattice_Expand_MultipleRefs(t *testing.T) {
	dir := setupTestRepo(t)

	id1 := mai(t, dir, "ticket", "First thing")
	id2 := mai(t, dir, "ticket", "Second thing")

	out := mai(t, dir, "expand", "Link [["+id1+"]] and [["+id2+"]]")
	if !strings.Contains(out, id1) || !strings.Contains(out, id2) {
		t.Errorf("expand should resolve both refs:\n%s", out)
	}
}

// === mai docs sync edge cases (beyond what docs_adversarial covers) ===

func TestLattice_DocsSync_DryRunDoesNotWrite(t *testing.T) {
	dir := setupTestRepo(t)

	// Create a doc note via CLI
	mai(t, dir, "create", "Test doc", "-k", "doc", "-d", "Some documentation content")

	// Dry run — should not create the docs dir
	out := mai(t, dir, "docs", "sync", "--dir", "test-docs", "--dry-run")
	t.Logf("dry-run output: %s", out)

	docsDir := filepath.Join(dir, "test-docs")
	if _, err := os.Stat(docsDir); err == nil {
		entries, _ := os.ReadDir(docsDir)
		mdCount := 0
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".md") {
				mdCount++
			}
		}
		if mdCount > 0 {
			t.Errorf("dry-run should not create markdown files, found %d", mdCount)
		}
	}
}

func TestLattice_DocsSync_NothingToSync(t *testing.T) {
	dir := setupTestRepo(t)

	out := mai(t, dir, "docs", "sync")
	if !strings.Contains(out, "in sync") {
		t.Errorf("empty repo docs sync should say everything in sync:\n%s", out)
	}
}
