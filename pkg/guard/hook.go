// Package guard runs local maitake hooks before writes.
package guard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultHookTimeout = 10 * time.Second
	hooksDirName       = ".maitake/hooks"
	configPathName     = ".maitake/config"
	preWriteHookName   = "pre-write"
	sharedHooksDirName = "hooks"
)

// RunHook executes .maitake/hooks/<hookName> with content on stdin.
// Returns nil if the hook passes or does not exist.
// Returns an error with the hook's stderr when the hook rejects the write.
func RunHook(maitakeDir string, hookName string, content []byte, env map[string]string) error {
	hookPath := filepath.Join(maitakeDir, hooksDirName, hookName)

	info, err := os.Stat(hookPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat hook %q: %w", hookName, err)
	}

	if !info.Mode().IsRegular() {
		log.Printf("maitake hook %q is not a regular file, skipping", hookName)
		return nil
	}

	if info.Mode().Perm()&0o111 == 0 {
		log.Printf("maitake hook %q is not executable, skipping", hookName)
		return nil
	}

	timeout, err := hookTimeout(maitakeDir)
	if err != nil {
		return fmt.Errorf("load hook timeout: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.Command(hookPath)
	cmd.Dir = maitakeDir
	cmd.Stdin = bytes.NewReader(content)
	cmd.Env = append(os.Environ(), commandEnv(env)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start hook %q: %w", hookName, err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err = <-done:
		if err == nil {
			return nil
		}
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		<-done
		return fmt.Errorf("run hook %q: timed out after %s", hookName, timeout)
	}

	message := strings.TrimSpace(stderr.String())
	if message != "" {
		return fmt.Errorf("run hook %q: %s", hookName, message)
	}

	return fmt.Errorf("run hook %q: %w", hookName, err)
}

// HookExists checks if a hook is installed and executable.
func HookExists(maitakeDir string, hookName string) bool {
	hookPath := filepath.Join(maitakeDir, hooksDirName, hookName)
	info, err := os.Stat(hookPath)
	if err != nil {
		return false
	}

	if !info.Mode().IsRegular() {
		return false
	}

	return info.Mode().Perm()&0o111 != 0
}

// DefaultPreWriteHook returns the default pre-write hook script.
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

// InitHooks creates the hook directory and installs default hooks when missing.
func InitHooks(maitakeDir string) error {
	targetDir := filepath.Join(maitakeDir, hooksDirName)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create hooks directory: %w", err)
	}

	if err := copySharedHooks(maitakeDir, targetDir); err != nil {
		return err
	}

	preWritePath := filepath.Join(targetDir, preWriteHookName)
	if _, err := os.Stat(preWritePath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("stat hook %q: %w", preWriteHookName, err)
		}

		if err := os.WriteFile(preWritePath, DefaultPreWriteHook(), 0o755); err != nil {
			return fmt.Errorf("write default hook %q: %w", preWriteHookName, err)
		}
	}

	if err := os.Chmod(preWritePath, 0o755); err != nil {
		return fmt.Errorf("set executable bit on %q: %w", preWriteHookName, err)
	}

	return nil
}

func copySharedHooks(maitakeDir string, targetDir string) error {
	sharedDir := filepath.Join(maitakeDir, sharedHooksDirName)
	entries, err := os.ReadDir(sharedDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read shared hooks directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		sourcePath := filepath.Join(sharedDir, entry.Name())
		targetPath := filepath.Join(targetDir, entry.Name())

		if _, err := os.Stat(targetPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat installed hook %q: %w", entry.Name(), err)
		}

		content, err := os.ReadFile(sourcePath)
		if err != nil {
			return fmt.Errorf("read shared hook %q: %w", entry.Name(), err)
		}

		if err := os.WriteFile(targetPath, content, 0o755); err != nil {
			return fmt.Errorf("install shared hook %q: %w", entry.Name(), err)
		}
	}

	return nil
}

func hookTimeout(maitakeDir string) (time.Duration, error) {
	configPath := filepath.Join(maitakeDir, configPathName)
	content, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, fs.ErrNotExist) {
			return defaultHookTimeout, nil
		}
		return 0, fmt.Errorf("read %s: %w", configPathName, err)
	}

	return parseHookTimeout(content)
}

func parseHookTimeout(content []byte) (time.Duration, error) {
	section := ""
	for _, rawLine := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			continue
		}

		if section != "hooks" {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		if strings.TrimSpace(key) != "timeout" {
			continue
		}

		seconds, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0, fmt.Errorf("parse hook timeout %q: %w", strings.TrimSpace(value), err)
		}
		if seconds <= 0 {
			return 0, fmt.Errorf("parse hook timeout %q: must be > 0", strings.TrimSpace(value))
		}

		return time.Duration(seconds) * time.Second, nil
	}

	return defaultHookTimeout, nil
}

func commandEnv(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}

	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	values := make([]string, 0, len(keys))
	for _, key := range keys {
		values = append(values, key+"="+env[key])
	}

	return values
}
