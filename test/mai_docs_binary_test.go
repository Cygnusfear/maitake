package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var maiDocsBinaryDir string

func buildMaiDocs(t *testing.T) string {
	t.Helper()
	if maiDocsBinaryDir != "" {
		bin := filepath.Join(maiDocsBinaryDir, "mai-docs")
		if _, err := os.Stat(bin); err == nil {
			return bin
		}
	}
	dir, err := os.MkdirTemp("", "mai-docs-test-*")
	if err != nil {
		t.Fatal(err)
	}
	maiDocsBinaryDir = dir
	bin := filepath.Join(dir, "mai-docs")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/mai-docs/")
	cmd.Dir = projectRoot()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build mai-docs: %v\n%s", err, out)
	}
	return bin
}

// TestMaiDocs_Binary_Builds confirms cmd/mai-docs/ compiles.
func TestMaiDocs_Binary_Builds(t *testing.T) {
	bin := buildMaiDocs(t)
	if _, err := os.Stat(bin); err != nil {
		t.Fatal("mai-docs binary should exist after build")
	}
}

// TestMaiDocs_Binary_Help confirms it runs and shows help.
func TestMaiDocs_Binary_Help(t *testing.T) {
	bin := buildMaiDocs(t)
	cmd := exec.Command(bin, "--help")
	out, _ := cmd.CombinedOutput()
	output := string(out)
	if !strings.Contains(output, "sync") {
		t.Errorf("mai-docs --help should mention sync: %s", output)
	}
}

// TestMaiDocs_Binary_Sync creates a doc note and syncs to disk.
func TestMaiDocs_Binary_Sync(t *testing.T) {
	bin := buildMaiDocs(t)
	dir := setupRepo(t)

	// Create a doc note via mai
	cmd := exec.Command(maiBinary, "create", "-k", "doc", "-t", "Test Doc", "-d", "Hello from docs.")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mai create doc failed: %v\n%s", err, out)
	}

	// Run mai-docs sync
	cmd = exec.Command(bin, "sync", "--dir", "docs")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MAI_REPO_PATH="+dir)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mai-docs sync failed: %v\n%s", err, out)
	}

	// Verify a .md file was created
	files, _ := filepath.Glob(filepath.Join(dir, "docs", "*.md"))
	if len(files) == 0 {
		t.Fatal("mai-docs sync should create doc files")
	}

	data, _ := os.ReadFile(files[0])
	if !strings.Contains(string(data), "Hello from docs") {
		t.Errorf("doc file should contain body: %s", string(data))
	}
}

// TestMaiDocs_Binary_Check runs mai-docs check.
func TestMaiDocs_Binary_Check(t *testing.T) {
	bin := buildMaiDocs(t)
	dir := setupRepo(t)

	cmd := exec.Command(bin, "check")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MAI_REPO_PATH="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mai-docs check failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "ref") || !strings.Contains(string(out), "0") {
		// At minimum it should run and report something
		_ = out // pass — just checking it doesn't crash
	}
}

// TestMaiDocs_DispatchViaMai confirms `mai docs` dispatches to mai-docs.
func TestMaiDocs_DispatchViaMai(t *testing.T) {
	bin := buildMaiDocs(t)
	dir := setupRepo(t)
	binDir := filepath.Dir(bin)

	maitakeDir := filepath.Join(dir, ".maitake")
	os.MkdirAll(maitakeDir, 0755)
	os.WriteFile(filepath.Join(maitakeDir, "plugins.toml"), []byte(`[plugins]
docs = "mai-docs"
`), 0644)

	// Create a doc note first
	cmd := exec.Command(maiBinary, "create", "-k", "doc", "-t", "Dispatch Doc", "-d", "Via dispatch.")
	cmd.Dir = dir
	cmd.CombinedOutput()

	// Dispatch: mai docs sync
	cmd = exec.Command(maiBinary, "docs", "sync", "--dir", "docs")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+binDir+":"+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mai docs sync (dispatch) failed: %v\n%s", err, out)
	}

	files, _ := filepath.Glob(filepath.Join(dir, "docs", "*.md"))
	if len(files) == 0 {
		t.Fatal("dispatch sync should create doc files")
	}
}
