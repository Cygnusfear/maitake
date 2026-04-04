package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMaiChangelog_Migrate_BasicEntry migrates a single tinychange file.
func TestMaiChangelog_Migrate_BasicEntry(t *testing.T) {
	bin := buildMaiChangelog(t)
	dir := setupRepo(t)

	// Create a .tinychange dir with one entry
	tcDir := filepath.Join(dir, ".tinychange")
	os.MkdirAll(tcDir, 0755)
	os.WriteFile(filepath.Join(tcDir, "sample-entry-abc1234.md"), []byte(`- Author: Alice
- Kind: fix
---
Fix the thing that was broken
`), 0644)

	cmd := exec.Command(bin, "migrate", "--dir", ".tinychange")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MAI_REPO_PATH="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("migrate failed: %v\n%s", err, out)
	}

	output := string(out)
	if !strings.Contains(output, "1") {
		t.Errorf("output should report 1 entry migrated: %s", output)
	}

	// Verify the entry exists as a changelog artifact
	cmd = exec.Command(bin, "ls")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MAI_REPO_PATH="+dir)
	out, _ = cmd.CombinedOutput()
	if !strings.Contains(string(out), "Fix the thing") {
		t.Errorf("migrated entry should appear in ls: %s", string(out))
	}
	if !strings.Contains(string(out), "fix") {
		t.Errorf("category 'fix' should be tagged: %s", string(out))
	}
}

// TestMaiChangelog_Migrate_MultipleEntries migrates a batch.
func TestMaiChangelog_Migrate_MultipleEntries(t *testing.T) {
	bin := buildMaiChangelog(t)
	dir := setupRepo(t)

	tcDir := filepath.Join(dir, ".tinychange")
	os.MkdirAll(tcDir, 0755)

	entries := []struct{ file, author, kind, body string }{
		{"aa-bb-cc-1234567.md", "Alice", "fix", "Fix A"},
		{"xx-yy-zz-abcdef0.md", "Bob", "feat", "Add B"},
		{"pp-qq-rr-fedcba9.md", "Carol", "chore", "Update C"},
	}
	for _, e := range entries {
		content := "- Author: " + e.author + "\n- Kind: " + e.kind + "\n---\n" + e.body + "\n"
		os.WriteFile(filepath.Join(tcDir, e.file), []byte(content), 0644)
	}

	cmd := exec.Command(bin, "migrate", "--dir", ".tinychange")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MAI_REPO_PATH="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("migrate failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "3") {
		t.Errorf("should migrate 3 entries: %s", string(out))
	}
}

// TestMaiChangelog_Migrate_DryRun doesn't write anything.
func TestMaiChangelog_Migrate_DryRun(t *testing.T) {
	bin := buildMaiChangelog(t)
	dir := setupRepo(t)

	tcDir := filepath.Join(dir, ".tinychange")
	os.MkdirAll(tcDir, 0755)
	os.WriteFile(filepath.Join(tcDir, "test-xxx.md"), []byte(`- Author: X
- Kind: fix
---
Something
`), 0644)

	cmd := exec.Command(bin, "migrate", "--dir", ".tinychange", "--dry-run")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MAI_REPO_PATH="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("migrate dry-run failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "would migrate") && !strings.Contains(string(out), "dry") {
		t.Errorf("dry-run output should indicate preview: %s", string(out))
	}

	// Verify no notes were created
	cmd = exec.Command(bin, "ls")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MAI_REPO_PATH="+dir)
	out, _ = cmd.CombinedOutput()
	if strings.Contains(string(out), "Something") {
		t.Errorf("dry-run should not create entries: %s", string(out))
	}
}

// TestMaiChangelog_Migrate_SkipBadFiles tolerates malformed files.
func TestMaiChangelog_Migrate_SkipBadFiles(t *testing.T) {
	bin := buildMaiChangelog(t)
	dir := setupRepo(t)

	tcDir := filepath.Join(dir, ".tinychange")
	os.MkdirAll(tcDir, 0755)

	// Good one
	os.WriteFile(filepath.Join(tcDir, "good-abc1234.md"), []byte(`- Author: A
- Kind: fix
---
Good entry
`), 0644)
	// No frontmatter
	os.WriteFile(filepath.Join(tcDir, "bad1-def5678.md"), []byte("Just some text, no structure"), 0644)
	// Empty
	os.WriteFile(filepath.Join(tcDir, "bad2-ghi9012.md"), []byte(""), 0644)

	cmd := exec.Command(bin, "migrate", "--dir", ".tinychange")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MAI_REPO_PATH="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("migrate should not fail on bad files: %v\n%s", err, out)
	}

	// Should migrate the 1 good entry, skip the 2 bad
	cmd = exec.Command(bin, "ls")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MAI_REPO_PATH="+dir)
	out, _ = cmd.CombinedOutput()
	if !strings.Contains(string(out), "Good entry") {
		t.Errorf("good entry should be migrated: %s", string(out))
	}
}
