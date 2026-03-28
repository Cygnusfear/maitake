package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second

// runner executes git commands with timeout and stderr capture.
type runner struct {
	dir     string
	timeout time.Duration
}

func newRunner(dir string) *runner {
	return &runner{dir: dir, timeout: defaultTimeout}
}

// run executes `git <args>` and returns stdout. On failure, returns a *GitError
// with the stderr content.
func (r *runner) run(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	return r.runCtx(ctx, args...)
}

// runCtx executes `git <args>` with an explicit context.
func (r *runner) runCtx(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		subcmd := ""
		if len(args) > 0 {
			subcmd = args[0]
		}
		return "", &GitError{
			Cmd:    subcmd,
			Args:   args,
			Stderr: stderr.String(),
			Err:    err,
		}
	}

	return strings.TrimRight(stdout.String(), "\n"), nil
}

// runWithStdin executes `git <args>` with data piped to stdin.
func (r *runner) runWithStdin(input []byte, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.dir
	cmd.Stdin = bytes.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		subcmd := ""
		if len(args) > 0 {
			subcmd = args[0]
		}
		return "", &GitError{
			Cmd:    subcmd,
			Args:   args,
			Stderr: stderr.String(),
			Err:    err,
		}
	}

	return strings.TrimRight(stdout.String(), "\n"), nil
}

// runRaw returns raw stdout bytes without trimming.
func (r *runner) runRaw(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		subcmd := ""
		if len(args) > 0 {
			subcmd = args[0]
		}
		return nil, &GitError{
			Cmd:    subcmd,
			Args:   args,
			Stderr: stderr.String(),
			Err:    err,
		}
	}

	return stdout.Bytes(), nil
}

// gitAvailable checks that git is installed and reachable.
func gitAvailable() error {
	_, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("git not found in PATH: %w", err)
	}
	return nil
}
