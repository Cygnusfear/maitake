package guard

import (
	"bytes"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunHookPasses(t *testing.T) {
	maitakeDir := t.TempDir()
	writeHook(t, maitakeDir, "pre-write", "#!/usr/bin/env bash\nset -euo pipefail\ncat >/dev/null\n")

	err := RunHook(maitakeDir, "pre-write", []byte("hello"), nil)
	if err != nil {
		t.Fatalf("RunHook() error = %v, want nil", err)
	}
}

func TestRunHookRejectsWithStderr(t *testing.T) {
	maitakeDir := t.TempDir()
	writeHook(t, maitakeDir, "pre-write", "#!/usr/bin/env bash\nset -euo pipefail\necho 'nope' >&2\nexit 1\n")

	err := RunHook(maitakeDir, "pre-write", []byte("hello"), nil)
	if err == nil {
		t.Fatal("RunHook() error = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Fatalf("RunHook() error = %v, want stderr message", err)
	}
}

func TestRunHookMissingReturnsNil(t *testing.T) {
	maitakeDir := t.TempDir()

	err := RunHook(maitakeDir, "pre-write", []byte("hello"), nil)
	if err != nil {
		t.Fatalf("RunHook() error = %v, want nil", err)
	}
}

func TestRunHookNonExecutableSkipsWithWarning(t *testing.T) {
	maitakeDir := t.TempDir()
	path := writeHookFile(t, maitakeDir, "pre-write", "#!/usr/bin/env bash\nexit 1\n", 0o644)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat hook: %v", err)
	}

	var logs bytes.Buffer
	originalWriter := log.Writer()
	originalFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	defer log.SetOutput(originalWriter)
	defer log.SetFlags(originalFlags)

	err := RunHook(maitakeDir, "pre-write", []byte("hello"), nil)
	if err != nil {
		t.Fatalf("RunHook() error = %v, want nil", err)
	}
	if !strings.Contains(logs.String(), "not executable") {
		t.Fatalf("log output = %q, want non-executable warning", logs.String())
	}
}

func TestRunHookPassesEnvVars(t *testing.T) {
	maitakeDir := t.TempDir()
	writeHook(t, maitakeDir, "pre-write", "#!/usr/bin/env bash\nset -euo pipefail\nif [ \"${MAI_NOTE_KIND:-}\" != \"ticket\" ]; then\n  echo 'missing env' >&2\n  exit 1\nfi\n")

	err := RunHook(maitakeDir, "pre-write", []byte("hello"), map[string]string{"MAI_NOTE_KIND": "ticket"})
	if err != nil {
		t.Fatalf("RunHook() error = %v, want nil", err)
	}
}

func TestRunHookPassesContentOnStdin(t *testing.T) {
	maitakeDir := t.TempDir()
	writeHook(t, maitakeDir, "pre-write", "#!/usr/bin/env bash\nset -euo pipefail\ncontent=$(cat)\nif [ \"$content\" != \"hello from stdin\" ]; then\n  echo \"got:$content\" >&2\n  exit 1\nfi\n")

	err := RunHook(maitakeDir, "pre-write", []byte("hello from stdin"), nil)
	if err != nil {
		t.Fatalf("RunHook() error = %v, want nil", err)
	}
}

func TestRunHookTimesOut(t *testing.T) {
	maitakeDir := t.TempDir()
	writeHook(t, maitakeDir, "pre-write", "#!/usr/bin/env bash\nset -euo pipefail\nsleep 5\n")
	writeConfig(t, maitakeDir, "[hooks]\ntimeout = 1\n")

	start := time.Now()
	err := RunHook(maitakeDir, "pre-write", []byte("hello"), nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("RunHook() error = nil, want timeout")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("RunHook() error = %v, want timeout message", err)
	}
	if elapsed >= 4*time.Second {
		t.Fatalf("RunHook() elapsed = %s, want timeout before 4s", elapsed)
	}
}

func TestHookExists(t *testing.T) {
	maitakeDir := t.TempDir()
	if HookExists(maitakeDir, "pre-write") {
		t.Fatal("HookExists() = true for missing hook, want false")
	}

	writeHookFile(t, maitakeDir, "pre-write", "#!/usr/bin/env bash\nexit 0\n", 0o644)
	if HookExists(maitakeDir, "pre-write") {
		t.Fatal("HookExists() = true for non-executable hook, want false")
	}

	if err := os.Chmod(filepath.Join(maitakeDir, ".maitake", "hooks", "pre-write"), 0o755); err != nil {
		t.Fatalf("chmod hook: %v", err)
	}
	if !HookExists(maitakeDir, "pre-write") {
		t.Fatal("HookExists() = false for executable hook, want true")
	}
}

func TestDefaultPreWriteHookIsValidBash(t *testing.T) {
	maitakeDir := t.TempDir()
	scriptPath := filepath.Join(maitakeDir, "pre-write")
	if err := os.WriteFile(scriptPath, DefaultPreWriteHook(), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	out, err := exec.Command("bash", "-n", scriptPath).CombinedOutput()
	if err != nil {
		t.Fatalf("bash -n failed: %v\n%s", err, out)
	}

	content := string(DefaultPreWriteHook())
	if !strings.Contains(content, "gitleaks detect --pipe --no-banner") {
		t.Fatalf("DefaultPreWriteHook() missing gitleaks call: %q", content)
	}
	if !strings.HasPrefix(content, "#!/usr/bin/env bash\n") {
		t.Fatalf("DefaultPreWriteHook() missing bash shebang: %q", content)
	}
}

func TestInitHooksCreatesDirectoryStructure(t *testing.T) {
	maitakeDir := t.TempDir()

	if err := InitHooks(maitakeDir); err != nil {
		t.Fatalf("InitHooks() error = %v", err)
	}

	hooksDir := filepath.Join(maitakeDir, ".maitake", "hooks")
	if info, err := os.Stat(hooksDir); err != nil {
		t.Fatalf("stat hooks dir: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("hooks path is not a directory")
	}

	preWritePath := filepath.Join(hooksDir, "pre-write")
	info, err := os.Stat(preWritePath)
	if err != nil {
		t.Fatalf("stat pre-write hook: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("pre-write hook mode = %v, want executable", info.Mode().Perm())
	}
	if stringMustRead(t, preWritePath) != string(DefaultPreWriteHook()) {
		t.Fatal("InitHooks() wrote unexpected pre-write content")
	}
}

func TestInitHooksCopiesSharedHooksAndPreservesExistingHook(t *testing.T) {
	maitakeDir := t.TempDir()
	sharedDir := filepath.Join(maitakeDir, "hooks")
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		t.Fatalf("mkdir shared hooks: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sharedDir, "post-write"), []byte("#!/usr/bin/env bash\nexit 0\n"), 0o644); err != nil {
		t.Fatalf("write shared hook: %v", err)
	}

	existing := writeHookFile(t, maitakeDir, "pre-write", "#!/usr/bin/env bash\necho existing\n", 0o700)

	if err := InitHooks(maitakeDir); err != nil {
		t.Fatalf("InitHooks() error = %v", err)
	}

	if got := stringMustRead(t, existing); got != "#!/usr/bin/env bash\necho existing\n" {
		t.Fatalf("InitHooks() overwrote existing pre-write hook: %q", got)
	}

	postWritePath := filepath.Join(maitakeDir, ".maitake", "hooks", "post-write")
	if got := stringMustRead(t, postWritePath); got != "#!/usr/bin/env bash\nexit 0\n" {
		t.Fatalf("InitHooks() copied wrong shared hook content: %q", got)
	}
	info, err := os.Stat(postWritePath)
	if err != nil {
		t.Fatalf("stat copied hook: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("copied hook mode = %v, want executable", info.Mode().Perm())
	}
}

func writeHook(t *testing.T, maitakeDir string, hookName string, script string) {
	t.Helper()
	writeHookFile(t, maitakeDir, hookName, script, 0o755)
}

func writeHookFile(t *testing.T, maitakeDir string, hookName string, script string, mode os.FileMode) string {
	t.Helper()
	hooksDir := filepath.Join(maitakeDir, ".maitake", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatalf("mkdir hooks dir: %v", err)
	}
	path := filepath.Join(hooksDir, hookName)
	if err := os.WriteFile(path, []byte(script), mode); err != nil {
		t.Fatalf("write hook: %v", err)
	}
	return path
}

func writeConfig(t *testing.T, maitakeDir string, content string) {
	t.Helper()
	configDir := filepath.Join(maitakeDir, ".maitake")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config"), []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func stringMustRead(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}
