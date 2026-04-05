package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cygnusfear/maitake/pkg/notes"
)

// dispatchPlugin tries to run an external plugin for the given command.
// Returns true if a plugin was found and executed (or failed trying).
// Returns false if no plugin is registered for this command.
func dispatchPlugin(command string, args []string) bool {
	repoPath := findRepoPath()
	if repoPath == "" {
		return false
	}

	maitakeDir := filepath.Join(repoPath, ".maitake")
	globalDir := globalMaitakeDir()

	plugins := notes.LoadPlugins(maitakeDir, globalDir)
	binName, ok := notes.ResolvePlugin(plugins, command)
	if !ok {
		return false
	}

	// Find the binary on PATH
	binPath, err := exec.LookPath(binName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "plugin %q registered but binary %q not found on PATH\n", command, binName)
		fmt.Fprintf(os.Stderr, "install it with: go install github.com/cygnusfear/maitake/cmd/%s@latest\n", binName)
		os.Exit(1)
	}

	// Build environment
	env := os.Environ()
	env = append(env,
		"MAI_REPO_PATH="+repoPath,
		"MAI_MAITAKE_DIR="+maitakeDir,
	)
	if globalJSON {
		env = append(env, "MAI_JSON=1")
	}
	if globalYes {
		env = append(env, "MAI_YES=1")
	}

	// Exec the plugin with remaining args
	cmd := exec.Command(binPath, args...)
	cmd.Dir = repoPath
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
	return true
}

// findRepoPath walks up from cwd (or globalDir) to find a git repo.
func findRepoPath() string {
	dir := globalDir
	if dir == "" {
		dir, _ = os.Getwd()
	}
	if dir == "" {
		return ""
	}

	// Walk up to find .git
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// globalMaitakeDir returns ~/.maitake.
func globalMaitakeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".maitake")
}
