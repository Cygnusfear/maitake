package docs

import (
	"os/exec"
	"strings"
	"testing"
)

// TestBoundary_NotesDoesNotImportCRDT enforces the layer rule:
// pkg/notes is the substrate. It must not import pkg/crdt (domain logic).
// If this test fails, doc-specific code is leaking into the substrate.
func TestBoundary_NotesDoesNotImportCRDT(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", "github.com/cygnusfear/maitake/pkg/notes").
		CombinedOutput()
	if err != nil {
		t.Skipf("go list failed: %v\n%s", err, out)
	}
	for _, imp := range strings.Split(string(out), "\n") {
		imp = strings.TrimSpace(imp)
		if strings.HasSuffix(imp, "pkg/crdt") {
			t.Errorf("pkg/notes imports pkg/crdt — doc/CRDT logic must live in pkg/docs, not the substrate")
		}
	}
}

// TestBoundary_NotesDoesNotExportDocSync confirms SyncDocs is NOT in pkg/notes.
// After extraction, notes.SyncDocs should not exist — it lives in pkg/docs.
func TestBoundary_NotesDoesNotExportDocSync(t *testing.T) {
	// Check via go doc — if notes.SyncDocs exists, this boundary is violated.
	out, err := exec.Command("go", "doc", "github.com/cygnusfear/maitake/pkg/notes.SyncDocs").
		CombinedOutput()
	if err == nil && !strings.Contains(string(out), "not found") {
		t.Errorf("notes.SyncDocs still exists — should be moved to pkg/docs")
	}
}

// TestBoundary_NotesDoesNotExportDocTypes confirms doc-specific types are NOT in pkg/notes.
func TestBoundary_NotesDoesNotExportDocTypes(t *testing.T) {
	types := []string{"DocFile", "DocSyncResult", "DocSyncOptions"}
	for _, typ := range types {
		out, err := exec.Command("go", "doc", "github.com/cygnusfear/maitake/pkg/notes."+typ).
			CombinedOutput()
		if err == nil && !strings.Contains(string(out), "not found") {
			t.Errorf("notes.%s still exists — should be moved to pkg/docs", typ)
		}
	}
}

// TestBoundary_NotesDoesNotExportTombstone confirms tombstone funcs are NOT in pkg/notes.
func TestBoundary_NotesDoesNotExportTombstone(t *testing.T) {
	funcs := []string{"AddTombstone", "RemoveTombstone"}
	for _, fn := range funcs {
		out, err := exec.Command("go", "doc", "github.com/cygnusfear/maitake/pkg/notes."+fn).
			CombinedOutput()
		if err == nil && !strings.Contains(string(out), "not found") {
			t.Errorf("notes.%s still exists — should be moved to pkg/docs", fn)
		}
	}
}

// TestBoundary_DocsPackageExportsSyncDocs confirms the new home exists.
func TestBoundary_DocsPackageExportsSyncDocs(t *testing.T) {
	// This test passes by compilation: if SyncDocs doesn't exist in this
	// package, the functional tests above won't compile.
	// This is a documentation test — it just confirms the contract.
	var _ func(engine interface{}, repoPath string, cfg Config, opts ...SyncOptions) (*SyncResult, error) = nil
	_ = SyncDocs // must exist in this package
}
