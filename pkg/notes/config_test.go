package notes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfig_ReadWrite(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{
		Remote:       "forgejo",
		BlockedHosts: []string{"github.com", "gitlab.com"},
	}

	if err := WriteConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}

	got := ReadConfig(dir)
	if got.Remote != "forgejo" {
		t.Errorf("Remote = %q, want forgejo", got.Remote)
	}
	if len(got.BlockedHosts) != 2 {
		t.Fatalf("BlockedHosts = %v, want 2", got.BlockedHosts)
	}
	if got.BlockedHosts[0] != "github.com" {
		t.Errorf("BlockedHosts[0] = %q", got.BlockedHosts[0])
	}
	if got.BlockedHosts[1] != "gitlab.com" {
		t.Errorf("BlockedHosts[1] = %q", got.BlockedHosts[1])
	}
}

func TestConfig_ReadMissing(t *testing.T) {
	cfg := ReadConfig("/nonexistent/dir")
	if cfg.Remote != "" {
		t.Errorf("Remote = %q, want empty", cfg.Remote)
	}
	if len(cfg.BlockedHosts) != 0 {
		t.Errorf("BlockedHosts = %v, want empty", cfg.BlockedHosts)
	}
}

func TestConfig_Comments(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "config"), []byte("# comment line\nremote origin\n# another\nblocked-host github.com\n"), 0644)

	cfg := ReadConfig(dir)
	if cfg.Remote != "origin" {
		t.Errorf("Remote = %q, want origin", cfg.Remote)
	}
	if len(cfg.BlockedHosts) != 1 || cfg.BlockedHosts[0] != "github.com" {
		t.Errorf("BlockedHosts = %v", cfg.BlockedHosts)
	}
}

func TestConfig_EmptyRemote(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{BlockedHosts: []string{"github.com"}}
	WriteConfig(dir, cfg)

	got := ReadConfig(dir)
	if got.Remote != "" {
		t.Errorf("Remote = %q, want empty", got.Remote)
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
	if IsBlockedHost("https://gitlab.com/user/repo.git", blocked) {
		t.Error("should not block gitlab.com")
	}
}

func TestIsBlockedHost_HTTP(t *testing.T) {
	blocked := []string{"git.example.com"}
	if !IsBlockedHost("http://git.example.com/repo.git", blocked) {
		t.Error("should block HTTP URL")
	}
}
