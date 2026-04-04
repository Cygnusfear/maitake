package notes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPlugins_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	plugins := LoadPlugins(dir, "")
	if len(plugins) != 0 {
		t.Errorf("empty dir should return no plugins, got %d", len(plugins))
	}
}

func TestLoadPlugins_RepoConfig(t *testing.T) {
	dir := t.TempDir()
	maitakeDir := filepath.Join(dir, ".maitake")
	os.MkdirAll(maitakeDir, 0755)
	os.WriteFile(filepath.Join(maitakeDir, "plugins.toml"), []byte(`[plugins]
pr = "mai-pr"
docs = "mai-docs"
changelog = "mai-changelog"
`), 0644)

	plugins := LoadPlugins(maitakeDir, "")
	if len(plugins) != 3 {
		t.Fatalf("expected 3 plugins, got %d", len(plugins))
	}
	if plugins["pr"] != "mai-pr" {
		t.Errorf("pr = %q, want mai-pr", plugins["pr"])
	}
	if plugins["docs"] != "mai-docs" {
		t.Errorf("docs = %q, want mai-docs", plugins["docs"])
	}
	if plugins["changelog"] != "mai-changelog" {
		t.Errorf("changelog = %q, want mai-changelog", plugins["changelog"])
	}
}

func TestLoadPlugins_GlobalFallback(t *testing.T) {
	repoDir := t.TempDir()
	globalDir := t.TempDir()

	os.MkdirAll(repoDir, 0755)
	os.WriteFile(filepath.Join(globalDir, "plugins.toml"), []byte(`[plugins]
pr = "mai-pr"
`), 0644)

	// No repo-level config, should fall back to global
	plugins := LoadPlugins(repoDir, globalDir)
	if plugins["pr"] != "mai-pr" {
		t.Errorf("global fallback: pr = %q, want mai-pr", plugins["pr"])
	}
}

func TestLoadPlugins_RepoOverridesGlobal(t *testing.T) {
	repoDir := t.TempDir()
	globalDir := t.TempDir()

	os.WriteFile(filepath.Join(repoDir, "plugins.toml"), []byte(`[plugins]
pr = "my-custom-pr"
`), 0644)
	os.WriteFile(filepath.Join(globalDir, "plugins.toml"), []byte(`[plugins]
pr = "mai-pr"
docs = "mai-docs"
`), 0644)

	plugins := LoadPlugins(repoDir, globalDir)
	if plugins["pr"] != "my-custom-pr" {
		t.Errorf("repo should override global: pr = %q, want my-custom-pr", plugins["pr"])
	}
	// Global-only entries still visible
	if plugins["docs"] != "mai-docs" {
		t.Errorf("global-only plugin should be present: docs = %q", plugins["docs"])
	}
}

func TestResolvePlugin_Found(t *testing.T) {
	plugins := map[string]string{"pr": "mai-pr", "docs": "mai-docs"}
	bin, ok := ResolvePlugin(plugins, "pr")
	if !ok || bin != "mai-pr" {
		t.Errorf("ResolvePlugin(pr) = %q, %v; want mai-pr, true", bin, ok)
	}
}

func TestResolvePlugin_NotFound(t *testing.T) {
	plugins := map[string]string{"pr": "mai-pr"}
	_, ok := ResolvePlugin(plugins, "bogus")
	if ok {
		t.Error("ResolvePlugin(bogus) should return false")
	}
}

func TestWriteDefaultPlugins(t *testing.T) {
	dir := t.TempDir()
	err := WriteDefaultPlugins(dir)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "plugins.toml"))
	if err != nil {
		t.Fatal("plugins.toml should exist after WriteDefaultPlugins")
	}
	content := string(data)
	for _, name := range []string{"pr", "docs", "changelog"} {
		if !pluginContains(content, name+" = ") {
			t.Errorf("default plugins.toml should contain %q", name)
		}
	}
}

func TestWriteDefaultPlugins_DoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	existing := `[plugins]
pr = "my-custom-pr"
`
	os.WriteFile(filepath.Join(dir, "plugins.toml"), []byte(existing), 0644)

	err := WriteDefaultPlugins(dir)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "plugins.toml"))
	if string(data) != existing {
		t.Error("WriteDefaultPlugins should not overwrite existing config")
	}
}

func pluginContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
