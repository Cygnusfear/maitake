package notes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanCodeRefs(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)

	os.WriteFile(filepath.Join(srcDir, "auth.ts"), []byte(`
// @mai: [[tre-5c4a]]
export function refreshToken() {}

// @mai: [[wrn-a4f2]]
export function validate() {}
`), 0644)

	os.WriteFile(filepath.Join(srcDir, "server.py"), []byte(`
# @mai: [[tre-5c4a]]
def handle_request():
    pass
`), 0644)

	refs, err := ScanCodeRefs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 3 {
		t.Fatalf("got %d refs, want 3", len(refs))
	}

	// Check first ref
	found := false
	for _, r := range refs {
		if r.Target == "tre-5c4a" && r.File == "src/auth.ts" && r.Line == 2 {
			found = true
		}
	}
	if !found {
		t.Errorf("missing tre-5c4a ref in auth.ts:2, got: %+v", refs)
	}
}

func TestScanCodeRefs_SkipsNodeModules(t *testing.T) {
	dir := t.TempDir()
	nmDir := filepath.Join(dir, "node_modules", "pkg")
	os.MkdirAll(nmDir, 0755)
	os.WriteFile(filepath.Join(nmDir, "index.ts"), []byte(`// @mai: [[should-skip]]`), 0644)

	refs, err := ScanCodeRefs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 {
		t.Errorf("should skip node_modules, got: %+v", refs)
	}
}

func TestScanCodeRefs_MultiplePerLine(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`
// @mai: [[note-a]] see also @mai: [[note-b]]
func main() {}
`), 0644)

	refs, _ := ScanCodeRefs(dir)
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2", len(refs))
	}
}

func TestExtractWikiRefs(t *testing.T) {
	refs := ExtractWikiRefs("tre-5c4a", "See [[wrn-a4f2]] and also [[docs/auth#OAuth Flow]] for details.")
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2", len(refs))
	}
	if refs[0].Target != "wrn-a4f2" {
		t.Errorf("ref[0].Target = %q", refs[0].Target)
	}
	if refs[1].Target != "docs/auth#OAuth Flow" {
		t.Errorf("ref[1].Target = %q", refs[1].Target)
	}
}

func TestExtractWikiRefs_WithAlias(t *testing.T) {
	refs := ExtractWikiRefs("note-1", "See [[tre-5c4a|the auth ticket]] for context.")
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1", len(refs))
	}
	if refs[0].Target != "tre-5c4a" {
		t.Errorf("Target = %q, want tre-5c4a (alias should be stripped)", refs[0].Target)
	}
}

func TestExtractWikiRefs_NoRefs(t *testing.T) {
	refs := ExtractWikiRefs("note-1", "No wiki links here.")
	if len(refs) != 0 {
		t.Errorf("got %d refs, want 0", len(refs))
	}
}
