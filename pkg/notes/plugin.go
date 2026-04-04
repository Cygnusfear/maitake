package notes

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// pluginsConfig is the TOML structure for plugins.toml.
type pluginsConfig struct {
	Plugins map[string]string `toml:"plugins"`
}

// DefaultPlugins lists the bundled tools that ship with maitake.
var DefaultPlugins = map[string]string{
	"pr":        "mai-pr",
	"docs":      "mai-docs",
	"changelog": "mai-changelog",
}

// LoadPlugins reads plugins.toml from maitakeDir (per-repo), with globalDir
// as fallback. Per-repo entries override global entries; global-only entries
// are preserved.
func LoadPlugins(maitakeDir, globalDir string) map[string]string {
	result := make(map[string]string)

	// Load global first (lower priority)
	if globalDir != "" {
		loadPluginFile(filepath.Join(globalDir, "plugins.toml"), result)
	}

	// Load per-repo (higher priority — overwrites global)
	loadPluginFile(filepath.Join(maitakeDir, "plugins.toml"), result)

	return result
}

func loadPluginFile(path string, into map[string]string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var cfg pluginsConfig
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return
	}
	for k, v := range cfg.Plugins {
		into[k] = v
	}
}

// ResolvePlugin looks up a command name in the plugins map.
// Returns the binary name and true if found, or empty and false if not.
func ResolvePlugin(plugins map[string]string, command string) (string, bool) {
	bin, ok := plugins[command]
	return bin, ok
}

// WriteDefaultPlugins writes plugins.toml with the default bundled tools.
// Does NOT overwrite an existing file.
func WriteDefaultPlugins(maitakeDir string) error {
	path := filepath.Join(maitakeDir, "plugins.toml")
	if _, err := os.Stat(path); err == nil {
		return nil // already exists, don't overwrite
	}

	if err := os.MkdirAll(maitakeDir, 0755); err != nil {
		return err
	}

	cfg := pluginsConfig{Plugins: DefaultPlugins}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}
