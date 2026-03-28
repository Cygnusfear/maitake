package notes

import (
	"os"
	"path/filepath"
	"strings"
)

// Config holds maitake configuration, stored in .maitake/config.
type Config struct {
	Remote       string   // remote to auto-push to (empty = no push)
	BlockedHosts []string // hosts that must never receive notes
	DocsDir      string   // docs materialization directory (default: "docs")
}

// ReadConfig reads .maitake/config. Returns zero config if file doesn't exist.
func ReadConfig(maitakeDir string) Config {
	data, err := os.ReadFile(filepath.Join(maitakeDir, "config"))
	if err != nil {
		return Config{}
	}

	cfg := Config{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := parseConfigLine(line)
		if !ok {
			continue
		}
		switch key {
		case "remote":
			cfg.Remote = val
		case "blocked-host":
			cfg.BlockedHosts = append(cfg.BlockedHosts, val)
		case "docs-dir":
			cfg.DocsDir = val
		}
	}
	return cfg
}

// WriteConfig writes .maitake/config.
func WriteConfig(maitakeDir string, cfg Config) error {
	if err := os.MkdirAll(maitakeDir, 0755); err != nil {
		return err
	}

	var lines []string
	if cfg.Remote != "" {
		lines = append(lines, "remote "+cfg.Remote)
	}
	for _, host := range cfg.BlockedHosts {
		lines = append(lines, "blocked-host "+host)
	}
	if cfg.DocsDir != "" {
		lines = append(lines, "docs-dir "+cfg.DocsDir)
	}

	return os.WriteFile(filepath.Join(maitakeDir, "config"), []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

func parseConfigLine(line string) (key, val string, ok bool) {
	idx := strings.IndexByte(line, ' ')
	if idx < 0 {
		return "", "", false
	}
	return line[:idx], strings.TrimSpace(line[idx+1:]), true
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
	if strings.Contains(url, "@") {
		parts := strings.SplitN(url, "@", 2)
		if len(parts) == 2 {
			host := strings.SplitN(parts[1], ":", 2)[0]
			return host
		}
	}
	// HTTPS: https://github.com/user/repo.git
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	parts := strings.SplitN(url, "/", 2)
	return parts[0]
}
