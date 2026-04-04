package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPlugin_DispatchToExternal creates a fake mai-test binary,
// registers it in plugins.toml, and verifies mai dispatches to it.
func TestPlugin_DispatchToExternal(t *testing.T) {
	dir := setupRepo(t)

	// Create a fake plugin binary that writes its args to a file
	binDir := t.TempDir()
	markerFile := filepath.Join(t.TempDir(), "plugin-called.txt")

	pluginScript := filepath.Join(binDir, "mai-test")
	os.WriteFile(pluginScript, []byte(`#!/bin/sh
echo "CALLED:$*" > `+markerFile+`
echo "REPO:$MAI_REPO_PATH" >> `+markerFile+`
`), 0755)

	// Write plugins.toml
	maitakeDir := filepath.Join(dir, ".maitake")
	os.MkdirAll(maitakeDir, 0755)
	os.WriteFile(filepath.Join(maitakeDir, "plugins.toml"), []byte(`[plugins]
test = "mai-test"
`), 0644)

	// Run mai test hello world
	cmd := exec.Command(maiBinary, "test", "hello", "world")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+binDir+":"+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mai test failed: %v\n%s", err, out)
	}

	// Verify the plugin was called with the right args
	data, err := os.ReadFile(markerFile)
	if err != nil {
		t.Fatal("plugin marker file not created — dispatch didn't happen")
	}
	content := string(data)
	if !strings.Contains(content, "CALLED:hello world") {
		t.Errorf("plugin should receive args: %s", content)
	}
	// macOS resolves /var → /private/var; accept either
	resolvedDir, _ := filepath.EvalSymlinks(dir)
	if !strings.Contains(content, "REPO:"+dir) && !strings.Contains(content, "REPO:"+resolvedDir) {
		t.Errorf("plugin should receive MAI_REPO_PATH: %s\nwant %s or %s", content, dir, resolvedDir)
	}
}

// TestPlugin_UnknownCommandShowsError verifies that a command not in
// the manifest and not built-in shows a helpful error.
func TestPlugin_UnknownCommandShowsError(t *testing.T) {
	dir := setupRepo(t)

	cmd := exec.Command(maiBinary, "nonexistent-command")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	output := string(out)
	if !strings.Contains(output, "unknown command") {
		t.Errorf("expected 'unknown command' in output: %s", output)
	}
}

// TestPlugin_JsonFlagPassedToPlugin verifies MAI_JSON env is set.
func TestPlugin_JsonFlagPassedToPlugin(t *testing.T) {
	dir := setupRepo(t)

	binDir := t.TempDir()
	markerFile := filepath.Join(t.TempDir(), "json-flag.txt")

	pluginScript := filepath.Join(binDir, "mai-test")
	os.WriteFile(pluginScript, []byte(`#!/bin/sh
echo "JSON:$MAI_JSON" > `+markerFile+`
`), 0755)

	maitakeDir := filepath.Join(dir, ".maitake")
	os.MkdirAll(maitakeDir, 0755)
	os.WriteFile(filepath.Join(maitakeDir, "plugins.toml"), []byte(`[plugins]
test = "mai-test"
`), 0644)

	cmd := exec.Command(maiBinary, "--json", "test")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+binDir+":"+os.Getenv("PATH"))
	cmd.CombinedOutput()

	data, err := os.ReadFile(markerFile)
	if err != nil {
		t.Fatal("plugin marker not created")
	}
	if !strings.Contains(string(data), "JSON:1") {
		t.Errorf("MAI_JSON should be 1 with --json flag: %s", string(data))
	}
}
