package notes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfig_WriteAndReadTOML(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{
		Sync: SyncConfig{
			Remote:       "forgejo",
			BlockedHosts: []string{"github.com", "gitlab.com"},
		},
		Docs: DocsConfig{
			Sync: "auto",
			Dir:  "wiki",
		},
	}

	if err := WriteConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}

	got := ReadConfig(dir)
	if got.Sync.Remote != "forgejo" {
		t.Errorf("Remote = %q, want forgejo", got.Sync.Remote)
	}
	if len(got.Sync.BlockedHosts) != 2 {
		t.Fatalf("BlockedHosts = %v, want 2", got.Sync.BlockedHosts)
	}
	if got.Docs.Sync != "auto" {
		t.Errorf("Docs.Sync = %q, want auto", got.Docs.Sync)
	}
	if got.Docs.Dir != "wiki" {
		t.Errorf("Docs.Dir = %q, want wiki", got.Docs.Dir)
	}
}

func TestConfig_Defaults(t *testing.T) {
	cfg := defaultConfig()
	if cfg.Sync.Remote != "" {
		t.Errorf("Remote = %q, want empty", cfg.Sync.Remote)
	}
	if cfg.Docs.Dir != ".mai-docs" {
		t.Errorf("Docs.Dir = %q, want .mai-docs (default)", cfg.Docs.Dir)
	}
	if cfg.Docs.Sync != "auto" {
		t.Errorf("Docs.Sync = %q, want auto (default)", cfg.Docs.Sync)
	}
}

func TestConfig_LegacyCompat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config"), []byte("remote origin\nblocked-host github.com\ndocs-dir .mushroom\n"), 0644)

	cfg := ReadConfig(dir)
	if cfg.Sync.Remote != "origin" {
		t.Errorf("Remote = %q, want origin", cfg.Sync.Remote)
	}
	if len(cfg.Sync.BlockedHosts) != 1 || cfg.Sync.BlockedHosts[0] != "github.com" {
		t.Errorf("BlockedHosts = %v", cfg.Sync.BlockedHosts)
	}
	if cfg.Docs.Dir != ".mushroom" {
		t.Errorf("Docs.Dir = %q, want .mushroom", cfg.Docs.Dir)
	}
}

func TestConfig_TOMLOverridesLegacy(t *testing.T) {
	dir := t.TempDir()
	// Legacy says remote=origin
	os.WriteFile(filepath.Join(dir, "config"), []byte("remote origin\n"), 0644)
	// TOML says remote=forgejo
	os.WriteFile(filepath.Join(dir, "config.toml"), []byte("[sync]\nremote = \"forgejo\"\n"), 0644)

	cfg := ReadConfig(dir)
	if cfg.Sync.Remote != "forgejo" {
		t.Errorf("Remote = %q, want forgejo (TOML should override legacy)", cfg.Sync.Remote)
	}
}

func TestIsBlockedHost_SSH(t *testing.T) {
	blocked := []string{"github.com"}
	if !IsBlockedHost("git@github.com:user/repo.git", blocked) {
		t.Error("should block github.com SSH URL")
	}
	if IsBlockedHost("git@gitlab.com:user/repo.git", blocked) {
		t.Error("should not block gitlab.com")
	}
}

func TestIsBlockedHost_HTTPS(t *testing.T) {
	blocked := []string{"github.com"}
	if !IsBlockedHost("https://github.com/user/repo.git", blocked) {
		t.Error("should block github.com HTTPS URL")
	}
}
