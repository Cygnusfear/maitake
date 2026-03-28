package guard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeHook(t *testing.T, dir, name, script string) {
	t.Helper()
	hooksDir := filepath.Join(dir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(hooksDir, name)
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestRunHook_Passes(t *testing.T) {
	dir := t.TempDir()
	writeHook(t, dir, "pre-write", "#!/bin/bash\nexit 0\n")

	err := RunHook(dir, "pre-write", []byte("hello"), nil)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestRunHook_Rejects(t *testing.T) {
	dir := t.TempDir()
	writeHook(t, dir, "pre-write", "#!/bin/bash\necho 'bad content' >&2\nexit 1\n")

	err := RunHook(dir, "pre-write", []byte("hello"), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "bad content") {
		t.Fatalf("expected stderr in error, got %v", err)
	}
}

func TestRunHook_Missing(t *testing.T) {
	dir := t.TempDir()
	err := RunHook(dir, "pre-write", []byte("hello"), nil)
	if err != nil {
		t.Fatalf("missing hook should return nil, got %v", err)
	}
}

func TestRunHook_NotExecutable(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, "hooks")
	os.MkdirAll(hooksDir, 0755)
	os.WriteFile(filepath.Join(hooksDir, "pre-write"), []byte("#!/bin/bash\nexit 1"), 0644) // not executable

	err := RunHook(dir, "pre-write", []byte("hello"), nil)
	if err != nil {
		t.Fatalf("non-executable hook should return nil, got %v", err)
	}
}

func TestRunHook_StdinContent(t *testing.T) {
	dir := t.TempDir()
	// Hook reads stdin and checks for magic string
	writeHook(t, dir, "pre-write", "#!/bin/bash\ngrep -q 'magic-string' && exit 1 || exit 0\n")

	// Without magic string — passes
	if err := RunHook(dir, "pre-write", []byte("safe content"), nil); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	// With magic string — rejects
	if err := RunHook(dir, "pre-write", []byte("contains magic-string here"), nil); err == nil {
		t.Fatal("expected rejection, got nil")
	}
}

func TestRunHook_EnvVars(t *testing.T) {
	dir := t.TempDir()
	writeHook(t, dir, "pre-write", "#!/bin/bash\nif [ \"$MAI_NOTE_KIND\" = \"ticket\" ]; then exit 0; fi; exit 1\n")

	env := map[string]string{"MAI_NOTE_KIND": "ticket"}
	if err := RunHook(dir, "pre-write", []byte("hello"), env); err != nil {
		t.Fatalf("expected nil with correct env, got %v", err)
	}

	env2 := map[string]string{"MAI_NOTE_KIND": "wrong"}
	if err := RunHook(dir, "pre-write", []byte("hello"), env2); err == nil {
		t.Fatal("expected rejection with wrong env, got nil")
	}
}

func TestHookExists(t *testing.T) {
	dir := t.TempDir()

	// Use a hook name that doesn't exist globally
	if HookExists(dir, "test-nonexistent-hook") {
		t.Fatal("should not exist yet")
	}

	writeHook(t, dir, "pre-write", "#!/bin/bash\nexit 0\n")

	if !HookExists(dir, "pre-write") {
		t.Fatal("should exist now")
	}
}

func TestInitHooks(t *testing.T) {
	dir := t.TempDir()

	if err := InitHooks(dir); err != nil {
		t.Fatal(err)
	}

	if !HookExists(dir, "pre-write") {
		t.Fatal("pre-write hook should exist after init")
	}

	// Calling again should not overwrite
	if err := InitHooks(dir); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultPreWriteHook_ValidBash(t *testing.T) {
	content := DefaultPreWriteHook()
	if !strings.HasPrefix(string(content), "#!/usr/bin/env bash") {
		t.Fatal("default hook should start with shebang")
	}
}
