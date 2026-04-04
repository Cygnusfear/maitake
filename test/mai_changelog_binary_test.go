package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var maiChangelogBinaryDir string

func buildMaiChangelog(t *testing.T) string {
	t.Helper()
	if maiChangelogBinaryDir != "" {
		bin := filepath.Join(maiChangelogBinaryDir, "mai-changelog")
		if _, err := os.Stat(bin); err == nil {
			return bin
		}
	}
	dir, err := os.MkdirTemp("", "mai-changelog-test-*")
	if err != nil {
		t.Fatal(err)
	}
	maiChangelogBinaryDir = dir
	bin := filepath.Join(dir, "mai-changelog")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/mai-changelog/")
	cmd.Dir = projectRoot()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build mai-changelog: %v\n%s", err, out)
	}
	return bin
}

// TestMaiChangelog_Binary_Builds confirms cmd/mai-changelog/ compiles.
func TestMaiChangelog_Binary_Builds(t *testing.T) {
	bin := buildMaiChangelog(t)
	if _, err := os.Stat(bin); err != nil {
		t.Fatal("mai-changelog binary should exist after build")
	}
}

// TestMaiChangelog_Binary_Help confirms it shows help.
func TestMaiChangelog_Binary_Help(t *testing.T) {
	bin := buildMaiChangelog(t)
	cmd := exec.Command(bin, "--help")
	out, _ := cmd.CombinedOutput()
	output := string(out)
	if !strings.Contains(output, "changelog") {
		t.Errorf("mai-changelog --help should mention changelog: %s", output)
	}
}

// TestMaiChangelog_NewAndList creates changelog entries and lists them.
func TestMaiChangelog_NewAndList(t *testing.T) {
	bin := buildMaiChangelog(t)
	dir := setupRepo(t)

	// Create entries
	for _, entry := range []struct{ kind, desc string }{
		{"fix", "Fix token refresh race condition"},
		{"feat", "Add webhook support"},
		{"chore", "Update dependencies"},
	} {
		cmd := exec.Command(bin, "new", entry.desc, "-k", entry.kind)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "MAI_REPO_PATH="+dir)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("mai-changelog new failed: %v\n%s", err, out)
		}
	}

	// List
	cmd := exec.Command(bin, "ls")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MAI_REPO_PATH="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mai-changelog ls failed: %v\n%s", err, out)
	}
	output := string(out)
	if !strings.Contains(output, "Fix token refresh") {
		t.Errorf("ls should list entries: %s", output)
	}
	if !strings.Contains(output, "webhook") {
		t.Errorf("ls should list all entries: %s", output)
	}
}

// TestMaiChangelog_Merge renders changelog entries to markdown.
func TestMaiChangelog_Merge(t *testing.T) {
	bin := buildMaiChangelog(t)
	dir := setupRepo(t)

	// Create entries
	for _, entry := range []struct{ kind, desc string }{
		{"fix", "Fix auth bug"},
		{"feat", "Add search"},
		{"fix", "Fix memory leak"},
	} {
		cmd := exec.Command(bin, "new", entry.desc, "-k", entry.kind)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "MAI_REPO_PATH="+dir)
		cmd.CombinedOutput()
	}

	// Merge
	cmd := exec.Command(bin, "merge")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MAI_REPO_PATH="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mai-changelog merge failed: %v\n%s", err, out)
	}
	output := string(out)

	// Should have category headers and entries
	if !strings.Contains(output, "fix") && !strings.Contains(output, "Fix") {
		t.Errorf("merge output should contain fix entries: %s", output)
	}
	if !strings.Contains(output, "feat") && !strings.Contains(output, "Feat") {
		t.Errorf("merge output should contain feat entries: %s", output)
	}
}

// TestMaiChangelog_MergeToFile writes changelog to a file.
func TestMaiChangelog_MergeToFile(t *testing.T) {
	bin := buildMaiChangelog(t)
	dir := setupRepo(t)

	cmd := exec.Command(bin, "new", "Something changed", "-k", "feat")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MAI_REPO_PATH="+dir)
	cmd.CombinedOutput()

	outFile := filepath.Join(dir, "CHANGELOG.md")
	cmd = exec.Command(bin, "merge", "--output", outFile)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MAI_REPO_PATH="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mai-changelog merge --output failed: %v\n%s", err, out)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal("CHANGELOG.md should exist")
	}
	if !strings.Contains(string(data), "Something changed") {
		t.Errorf("CHANGELOG.md should contain entry: %s", string(data))
	}
}

// TestMaiChangelog_DispatchViaMai confirms `mai changelog` dispatches.
func TestMaiChangelog_DispatchViaMai(t *testing.T) {
	bin := buildMaiChangelog(t)
	dir := setupRepo(t)
	binDir := filepath.Dir(bin)

	maitakeDir := filepath.Join(dir, ".maitake")
	os.MkdirAll(maitakeDir, 0755)
	os.WriteFile(filepath.Join(maitakeDir, "plugins.toml"), []byte(`[plugins]
changelog = "mai-changelog"
`), 0644)

	cmd := exec.Command(maiBinary, "changelog", "new", "Dispatch test entry", "-k", "fix")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+binDir+":"+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mai changelog new (dispatch) failed: %v\n%s", err, out)
	}
}
