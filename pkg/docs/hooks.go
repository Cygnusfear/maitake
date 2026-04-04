package docs

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"time"

	"github.com/cygnusfear/maitake/pkg/crdt"
	"github.com/cygnusfear/maitake/pkg/git"
	"github.com/cygnusfear/maitake/pkg/notes"
)

// RegisterAutoSync registers post-write hooks on the engine for:
// - CRDT YDoc initialization on doc creation
// - CRDT YDoc updates on doc body edits
// - Auto-sync to disk when docs.sync is "auto"
//
// This is the bridge between the substrate (pkg/notes) and the doc domain (pkg/docs).
func RegisterAutoSync(engine *notes.RealEngine) {
	engine.OnPostWrite(func(e notes.Engine, noteID string, ref git.NotesRef, targetOID git.OID) {
		re := engine // need concrete engine for repo access + index

		state, err := e.Fold(noteID)
		if err != nil || state == nil || state.Kind != "doc" {
			return
		}

		cfg := e.GetConfig()

		// CRDT: initialize or update YDoc
		handleCRDT(e, re, noteID, state, ref, targetOID)

		// Auto-sync to disk
		if cfg.Docs.Sync == "auto" {
			autoSyncDoc(e, re, noteID, state, cfg)
		}
	})
}

// handleCRDT initializes or updates the YDoc for a doc note.
func handleCRDT(e notes.Engine, re *notes.RealEngine, noteID string, state *notes.State, ref git.NotesRef, targetOID git.OID) {
	// Check if this note already has YDoc state
	if len(state.YDocState) > 0 {
		// YDoc exists — update it with the latest body
		updateYDoc(e, re, noteID, state, ref, targetOID)
	} else if state.Body != "" {
		// No YDoc yet — initialize from body
		initYDoc(re, noteID, state.Body, ref, targetOID)
	}
}

// initYDoc creates a YDoc from body text and stores its state as a ydoc event.
func initYDoc(re *notes.RealEngine, noteID, body string, ref git.NotesRef, targetOID git.OID) {
	doc, err := crdt.New()
	if err != nil {
		return
	}
	defer doc.Close()

	if err := doc.Insert(0, body); err != nil {
		return
	}
	crdtState, err := doc.Save()
	if err != nil {
		return
	}

	encoded := base64.StdEncoding.EncodeToString(crdtState)
	ydocNote := &notes.Note{
		Kind:      "event",
		Field:     "ydoc",
		Value:     encoded,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Edges: []notes.Edge{{
			Type:   "updates",
			Target: notes.EdgeTarget{Kind: "note", Ref: noteID},
		}},
	}
	data, err := notes.Serialize(ydocNote)
	if err != nil {
		return
	}
	re.AppendRaw(ref, targetOID, data, ydocNote)
}

// updateYDoc loads the existing YDoc state, diffs old vs new body, applies ops, saves.
func updateYDoc(e notes.Engine, re *notes.RealEngine, noteID string, state *notes.State, ref git.NotesRef, targetOID git.OID) {
	// Only update if the latest event was a body edit
	if len(state.Events) == 0 {
		return
	}
	lastEvent := state.Events[len(state.Events)-1]
	if lastEvent.Field != "body" {
		return
	}

	newBody := state.Body

	var doc *crdt.TextDoc
	var err error
	if state.YDocState != nil {
		doc, err = crdt.Load(state.YDocState)
	} else {
		doc, err = crdt.New()
		if err == nil {
			// Get old body before this edit
			creation, _ := e.Get(noteID)
			oldBody := ""
			if creation != nil {
				oldBody = creation.Body
			}
			for _, ev := range state.Events {
				if ev.Field == "body" {
					evBody := ev.Body
					if evBody == "" {
						evBody = ev.Value
					}
					if evBody == newBody {
						break
					}
					oldBody = evBody
				}
			}
			doc.Insert(0, oldBody)
		}
	}
	if err != nil {
		return
	}
	defer doc.Close()

	ydocContent, _ := doc.Content()
	ops := crdt.Diff(ydocContent, newBody)
	if err := crdt.ApplyOps(doc, ops); err != nil {
		return
	}

	newState, err := doc.Save()
	if err != nil {
		return
	}

	encoded := base64.StdEncoding.EncodeToString(newState)
	ydocNote := &notes.Note{
		Kind:      "event",
		Field:     "ydoc",
		Value:     encoded,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Edges: []notes.Edge{{
			Type:   "updates",
			Target: notes.EdgeTarget{Kind: "note", Ref: noteID},
		}},
	}
	data, err := notes.Serialize(ydocNote)
	if err != nil {
		return
	}
	re.AppendRaw(ref, targetOID, data, ydocNote)
}

// prevNoteBody returns the note body as it was BEFORE the most recent body event.
func prevNoteBody(e notes.Engine, noteID string) string {
	state, err := e.Fold(noteID)
	if err != nil || state == nil {
		return ""
	}
	creation, _ := e.Get(noteID)
	if creation == nil {
		return ""
	}
	cur := creation.Body
	prev := cur
	for _, ev := range state.Events {
		if ev.Kind == "event" && ev.Field == "body" {
			prev = cur
			b := ev.Value
			if ev.Body != "" {
				b = ev.Body
			}
			cur = b
		}
	}
	return prev
}

// autoSyncDoc materializes a doc note to disk when docs.sync is "auto".
func autoSyncDoc(e notes.Engine, re *notes.RealEngine, noteID string, state *notes.State, cfg notes.Config) {
	docsDir := cfg.Docs.Dir
	if docsDir == "" {
		docsDir = ".mai-docs"
	}

	targetPath := DocTargetPath(state, docsDir)
	absPath := filepath.Join(re.RepoPath(), targetPath)

	if state.Status == "closed" {
		MarkDocFileClosed(absPath, state.ID)
		return
	}

	if data, err := os.ReadFile(absPath); err == nil {
		_, existingBody := ParseFrontmatter(string(data))
		prev := prevNoteBody(e, noteID)
		if contentHash(existingBody) != contentHash(prev) {
			return // file was independently edited
		}
	}

	WriteDocFile(absPath, state.ID, state.Body)
}

// ContentHash exported for tests that need it
func ContentHash(s string) string {
	return contentHash(s)
}


