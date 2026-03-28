// Package guard runs hooks before and after note writes.
// The actual scanning logic lives in the hook scripts, not in Go code.
package guard

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const defaultTimeout = 10 * time.Second

// RunHook executes a hook by name. Checks repo-local first (.maitake/hooks/),
// then falls back to global (~/.maitake/hooks/).
// Returns nil if the hook passes (exit 0) or doesn't exist anywhere.
// Returns an error with stderr content if the hook rejects (non-zero exit).
func RunHook(maitakeDir, hookName string, content []byte, env map[string]string) error {
	hookPath := resolveHookPath(maitakeDir, hookName)
	if hookPath == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, hookPath)
	cmd.Stdin = bytes.NewReader(content)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Build environment: inherit current env + add custom vars
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	if err := cmd.Run(); err != nil {
		stderrStr := stderr.String()
		if stderrStr != "" {
			return fmt.Errorf("hook %s rejected: %s", hookName, stderrStr)
		}
		return fmt.Errorf("hook %s rejected (exit %s)", hookName, err)
	}

	return nil
}

// HookExists checks if a hook is installed and executable (repo-local or global).
func HookExists(maitakeDir, hookName string) bool {
	return resolveHookPath(maitakeDir, hookName) != ""
}

// resolveHookPath returns the path to a hook, checking repo-local first, then global.
// Returns empty string if the hook doesn't exist anywhere.
func resolveHookPath(maitakeDir, hookName string) string {
	// 1. Repo-local: .maitake/hooks/<name>
	local := filepath.Join(maitakeDir, "hooks", hookName)
	if isExecutableFile(local) {
		return local
	}

	// 2. Global: ~/.maitake/hooks/<name>
	if globalDir := globalHooksDir(); globalDir != "" {
		global := filepath.Join(globalDir, hookName)
		if isExecutableFile(global) {
			return global
		}
	}

	return ""
}

// globalHooksDir returns ~/.maitake/hooks/ or empty string if home can't be resolved.
func globalHooksDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".maitake", "hooks")
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir() && info.Mode()&0111 != 0
}

// DefaultPreWriteHook returns the default pre-write hook script content.
func DefaultPreWriteHook() []byte {
	return []byte(`#!/usr/bin/env bash
set -euo pipefail

# Gitleaks if available
if command -v gitleaks &>/dev/null; then
    gitleaks detect --pipe --no-banner 2>&1
    exit $?
fi

# Fallback: catch the obvious stuff
content=$(cat)
patterns=(
    'AKIA[0-9A-Z]{16}'
    '-----BEGIN.*PRIVATE KEY-----'
    'ghp_[A-Za-z0-9]{36}'
    'gho_[A-Za-z0-9]{36}'
    'sk-[A-Za-z0-9]{48}'
    'eyJ[A-Za-z0-9_-]*\.eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*'
)

for pattern in "${patterns[@]}"; do
    if echo "$content" | grep -qE "$pattern"; then
        echo "maitake pre-write: possible secret detected (pattern: $pattern)" >&2
        echo "Use --skip-hooks to bypass (not recommended)" >&2
        exit 1
    fi
done

exit 0
`)
}

// InitHooks creates .maitake/hooks/ and writes default hooks if not present.
// Sets executable bits on created hooks.
func InitHooks(maitakeDir string) error {
	hooksDir := filepath.Join(maitakeDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("creating hooks directory: %w", err)
	}

	preWrite := filepath.Join(hooksDir, "pre-write")
	if _, err := os.Stat(preWrite); os.IsNotExist(err) {
		if err := os.WriteFile(preWrite, DefaultPreWriteHook(), 0755); err != nil {
			return fmt.Errorf("writing pre-write hook: %w", err)
		}
	}

	return nil
}
