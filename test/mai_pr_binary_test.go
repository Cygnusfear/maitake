package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// maiPrBinaryDir is a persistent temp dir for the mai-pr binary.
var maiPrBinaryDir string

func buildMaiPr(t *testing.T) string {
	t.Helper()
	if maiPrBinaryDir != "" {
		bin := filepath.Join(maiPrBinaryDir, "mai-pr")
		if _, err := os.Stat(bin); err == nil {
			return bin
		}
	}
	// Use a non-test temp dir so it persists across subtests
	dir, err := os.MkdirTemp("", "mai-pr-test-*")
	if err != nil {
		t.Fatal(err)
	}
	maiPrBinaryDir = dir
	bin := filepath.Join(dir, "mai-pr")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/mai-pr/")
	cmd.Dir = projectRoot()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build mai-pr: %v\n%s", err, out)
	}
	return bin
}

func projectRoot() string {
	// Walk up from test/ to find go.mod
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// TestMaiPr_Binary_Builds confirms cmd/mai-pr/ compiles.
func TestMaiPr_Binary_Builds(t *testing.T) {
	bin := buildMaiPr(t)
	if _, err := os.Stat(bin); err != nil {
		t.Fatal("mai-pr binary should exist after build")
	}
}

// TestMaiPr_Binary_Help confirms it runs and shows help.
func TestMaiPr_Binary_Help(t *testing.T) {
	bin := buildMaiPr(t)
	dir := setupRepo(t)

	cmd := exec.Command(bin, "--help")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MAI_REPO_PATH="+dir)
	out, _ := cmd.CombinedOutput()
	output := string(out)
	if !strings.Contains(output, "pr") {
		t.Errorf("mai-pr --help should mention pr commands: %s", output)
	}
}

// TestMaiPr_Binary_CreateAndList creates a PR via mai-pr and lists it.
func TestMaiPr_Binary_CreateAndList(t *testing.T) {
	bin := buildMaiPr(t)
	dir := setupRepo(t)

	// Create a feature branch with a commit
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}

	run("checkout", "-b", "feature/test-pr")
	os.WriteFile(filepath.Join(dir, "new-file.txt"), []byte("hello"), 0644)
	run("add", "-A")
	run("commit", "-m", "add new file")

	// Create PR via mai-pr
	cmd := exec.Command(bin, "Test PR", "--into", "main")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MAI_REPO_PATH="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mai-pr create failed: %v\n%s", err, out)
	}
	output := strings.TrimSpace(string(out))
	// Output format: "<id>  <from> → <to>"
	prID := strings.Fields(output)[0]
	if prID == "" {
		t.Fatalf("expected PR ID in output, got %q", output)
	}

	// List PRs
	cmd = exec.Command(bin)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MAI_REPO_PATH="+dir)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mai-pr list failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), prID) {
		t.Errorf("PR list should contain %s: %s", prID, string(out))
	}
}

// TestMaiPr_DispatchViaMai confirms `mai pr` dispatches to mai-pr.
func TestMaiPr_DispatchViaMai(t *testing.T) {
	bin := buildMaiPr(t)
	dir := setupRepo(t)

	binDir := filepath.Dir(bin)
	maitakeDir := filepath.Join(dir, ".maitake")
	os.MkdirAll(maitakeDir, 0755)
	os.WriteFile(filepath.Join(maitakeDir, "plugins.toml"), []byte(`[plugins]
pr = "mai-pr"
`), 0644)

	// Create branch + commit
	gitCmd := exec.Command("git", "checkout", "-b", "feature/dispatch-test")
	gitCmd.Dir = dir
	gitCmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
	)
	gitCmd.CombinedOutput()
	os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x"), 0644)
	gitCmd = exec.Command("git", "add", "-A")
	gitCmd.Dir = dir
	gitCmd.CombinedOutput()
	gitCmd = exec.Command("git", "commit", "-m", "x")
	gitCmd.Dir = dir
	gitCmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
	)
	gitCmd.CombinedOutput()

	// Use `mai pr` which should dispatch to mai-pr
	cmd := exec.Command(maiBinary, "pr", "Dispatch test", "--into", "main")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+binDir+":"+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mai pr (dispatch) failed: %v\n%s", err, out)
	}
	output := strings.TrimSpace(string(out))
	// Should contain an ID and branch info
	if !strings.Contains(output, "→") {
		t.Errorf("dispatch should create PR with branch info: %s", output)
	}
}
