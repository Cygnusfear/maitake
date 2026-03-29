package notes

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds maitake configuration. Loaded from .maitake/config.toml (per-repo)
// with ~/.maitake/config.toml as global defaults. Per-repo overrides global field-by-field.
type Config struct {
	Sync  SyncConfig  `toml:"sync"`
	Docs  DocsConfig  `toml:"docs"`
	Hooks HooksConfig `toml:"hooks"`
}

// SyncConfig controls remote push/pull.
type SyncConfig struct {
	Remote       string   `toml:"remote"`
	BlockedHosts []string `toml:"blocked-hosts"`
}

// DocsConfig controls doc materialization.
type DocsConfig struct {
	Sync  string `toml:"sync"`  // "auto" | "manual" | "off" (default: "manual")
	Dir   string `toml:"dir"`   // docs directory (default: ".mai-docs")
	Watch bool   `toml:"watch"` // daemon watches this repo
}

// HooksConfig controls hook execution.
type HooksConfig struct {
	PreWrite bool `toml:"pre-write"`
	PostPush bool `toml:"post-push"`
}

// globalConfigPath returns ~/.maitake/config.toml.
func globalConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".maitake", "config.toml")
}

// repoConfigPath returns .maitake/config.toml for a repo.
func repoConfigPath(maitakeDir string) string {
	return filepath.Join(maitakeDir, "config.toml")
}

// ReadConfig loads config with global defaults merged with per-repo overrides.
// Also reads the legacy flat format for backwards compatibility.
func ReadConfig(maitakeDir string) Config {
	cfg := defaultConfig()

	// Load global
	if path := globalConfigPath(); path != "" {
		loadTOML(path, &cfg)
	}

	// Load legacy flat format (backwards compat)
	legacyPath := filepath.Join(maitakeDir, "config")
	if _, err := os.Stat(legacyPath); err == nil {
		loadLegacy(legacyPath, &cfg)
	}

	// Load per-repo TOML (overrides everything)
	loadTOML(repoConfigPath(maitakeDir), &cfg)

	return cfg
}

// WriteConfig writes .maitake/config.toml.
func WriteConfig(maitakeDir string, cfg Config) error {
	if err := os.MkdirAll(maitakeDir, 0755); err != nil {
		return err
	}
	f, err := os.Create(repoConfigPath(maitakeDir))
	if err != nil {
		return err
	}
	defer f.Close()
	enc := toml.NewEncoder(f)
	return enc.Encode(cfg)
}

func defaultConfig() Config {
	return Config{
		Docs: DocsConfig{
			Sync:  "auto",
			Dir:   ".mai-docs",
			Watch: false, // opt-in: set docs.watch = true in repo or global config
		},
		Hooks: HooksConfig{
			PreWrite: true,
			PostPush: true,
		},
	}
}

func loadTOML(path string, cfg *Config) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	// Decode into a separate struct, then merge non-zero fields
	var overlay Config
	if _, err := toml.Decode(string(data), &overlay); err != nil {
		return
	}
	mergeConfig(cfg, &overlay)
}

// mergeConfig applies non-zero overlay fields onto base.
func mergeConfig(base, overlay *Config) {
	if overlay.Sync.Remote != "" {
		base.Sync.Remote = overlay.Sync.Remote
	}
	if len(overlay.Sync.BlockedHosts) > 0 {
		base.Sync.BlockedHosts = overlay.Sync.BlockedHosts
	}
	if overlay.Docs.Sync != "" {
		base.Docs.Sync = overlay.Docs.Sync
	}
	if overlay.Docs.Dir != "" {
		base.Docs.Dir = overlay.Docs.Dir
	}
	// Watch is a bool — overlay always wins if the TOML key was present
	// (toml decoder sets false explicitly, so we can't distinguish absent from false
	// without raw parsing. For now, overlay wins.)
	base.Docs.Watch = overlay.Docs.Watch
	base.Hooks.PreWrite = overlay.Hooks.PreWrite
	base.Hooks.PostPush = overlay.Hooks.PostPush
}

// loadLegacy reads the old flat key-value config format for backwards compat.
func loadLegacy(path string, cfg *Config) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range splitLines(string(data)) {
		line = trimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		key, val := splitKeyVal(line)
		switch key {
		case "remote":
			cfg.Sync.Remote = val
		case "blocked-host":
			cfg.Sync.BlockedHosts = append(cfg.Sync.BlockedHosts, val)
		case "docs-dir":
			cfg.Docs.Dir = val
		}
	}
}



// helpers — avoid importing strings for trivial ops
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimSpace(s string) string {
	i, j := 0, len(s)
	for i < j && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r') {
		i++
	}
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t' || s[j-1] == '\r') {
		j--
	}
	return s[i:j]
}

func splitKeyVal(line string) (string, string) {
	for i := 0; i < len(line); i++ {
		if line[i] == ' ' {
			return line[:i], trimSpace(line[i+1:])
		}
	}
	return line, ""
}

// IsBlockedHost checks if a remote URL points to a blocked host.
func IsBlockedHost(remoteURL string, blocked []string) bool {
	host := extractHost(remoteURL)
	for _, b := range blocked {
		if host == b {
			return true
		}
	}
	return false
}

func extractHost(url string) string {
	// SSH: git@github.com:user/repo.git
	for i := 0; i < len(url); i++ {
		if url[i] == '@' {
			rest := url[i+1:]
			for j := 0; j < len(rest); j++ {
				if rest[j] == ':' {
					return rest[:j]
				}
			}
			return rest
		}
	}
	// HTTPS
	s := url
	for _, prefix := range []string{"https://", "http://"} {
		if len(s) > len(prefix) && s[:len(prefix)] == prefix {
			s = s[len(prefix):]
			break
		}
	}
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return s[:i]
		}
	}
	return s
}
