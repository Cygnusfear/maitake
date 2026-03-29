package notes

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cygnusfear/maitake/pkg/crdt"
)

// DocFile represents a materialized doc on disk.
type DocFile struct {
	Path    string // relative to repo root
	NoteID  string // from frontmatter, empty if new file
	Content string // markdown content (without frontmatter)
	Hash    string // sha256 of content
}

// DocSyncResult holds the outcome of a docs sync.
type DocSyncResult struct {
	Written   []string // files written from notes
	Imported  []string // new files imported as notes
	Updated   []string // notes updated from file changes
	Conflicts []string // both changed, skipped
	Removed   []string // files removed (note deleted/closed)
}

// DocSyncOptions controls sync behavior.
type DocSyncOptions struct {
	DryRun bool // compute what would happen without writing
}

// SyncDocs performs bidirectional sync between doc notes and markdown files.
func SyncDocs(engine Engine, repoPath string, cfg DocsConfig, opts ...DocSyncOptions) (*DocSyncResult, error) {
	dryRun := len(opts) > 0 && opts[0].DryRun
	if cfg.Dir == "" {
		cfg.Dir = ".mai-docs"
	}
	docsDir := filepath.Join(repoPath, cfg.Dir)
	result := &DocSyncResult{}

	// 1. Load all doc notes
	docNotes := make(map[string]*State) // id → state
	states, _ := engine.Find(FindOptions{Kind: "doc"})
	for i := range states {
		docNotes[states[i].ID] = &states[i]
	}
	// Also include closed docs (might need to remove files)
	closedDocs, _ := engine.Find(FindOptions{Kind: "doc", Status: "closed"})
	closedIDs := make(map[string]bool)
	for _, s := range closedDocs {
		closedIDs[s.ID] = true
	}

	// 2. Scan existing files on disk
	diskFiles := make(map[string]*DocFile) // noteID → file
	newFiles := []*DocFile{}               // files without mai-id

	if err := os.MkdirAll(docsDir, 0755); err != nil {
		return nil, fmt.Errorf("creating docs dir: %w", err)
	}

	filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		rel, _ := filepath.Rel(repoPath, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)
		noteID, body := parseMaiFrontmatter(content)
		hash := contentHash(body)

		df := &DocFile{Path: rel, NoteID: noteID, Content: body, Hash: hash}
		if noteID != "" {
			diskFiles[noteID] = df
		} else {
			newFiles = append(newFiles, df)
		}
		return nil
	})

	// Load tombstones
	tombstones := loadTombstones(filepath.Join(repoPath, ".maitake"))

	// 3. Notes → disk: write or update files
	for id, state := range docNotes {
		if closedIDs[id] {
			continue // handle removals separately
		}
		if tombstones[id] {
			continue // intentionally deleted by user
		}

		targetPath := docTargetPath(state, cfg.Dir)
		noteHash := contentHash(state.Body)

		if df, exists := diskFiles[id]; exists {
			// Both exist — check for conflicts
			if df.Hash == noteHash {
				continue // in sync
			}
			// Hashes differ — merge via CRDT if state available, else file wins
			if !dryRun {
				// Get the creation body as the base for 3-way merge
				creationBody := ""
				if creation, err := engine.Get(id); err == nil && creation != nil {
					creationBody = creation.Body
				}
				merged := mergeViaCRDT(state, df.Content, creationBody)
				engine.Append(AppendOptions{
					TargetID: id,
					Kind:     "event",
					Field:    "body",
					Body:     merged.Body,
				})
				if merged.YDocState != nil {
					engine.Append(AppendOptions{
						TargetID: id,
						Kind:     "event",
						Field:    "ydoc",
						Value:    base64.StdEncoding.EncodeToString(merged.YDocState),
					})
				}
				// Write merged content back to file
				absPath := filepath.Join(repoPath, df.Path)
				writeDocFile(absPath, id, merged.Body)
			}
			result.Updated = append(result.Updated, df.Path)
		} else {
			// Note exists, no file — write it
			if !dryRun {
				absPath := filepath.Join(repoPath, targetPath)
				if err := writeDocFile(absPath, id, state.Body); err != nil {
					continue
				}
			}
			result.Written = append(result.Written, targetPath)
		}
	}

	// 4. Disk → notes: import files without mai-id as doc notes.
	// Preserves the original file path as the target edge.
	for _, df := range newFiles {
		if dryRun {
			result.Imported = append(result.Imported, df.Path)
			continue
		}
		title := titleFromPath(df.Path)
		note, err := engine.Create(CreateOptions{
			Kind:    "doc",
			Title:   title,
			Body:    df.Content,
			Targets: []string{df.Path},
		})
		if err != nil {
			continue
		}
		absPath := filepath.Join(repoPath, df.Path)
		writeDocFile(absPath, note.ID, df.Content)
		result.Imported = append(result.Imported, df.Path)
	}

	// 5. Handle closed doc notes — mark closed in frontmatter, keep the file.
	// We do NOT delete the file; Obsidian/editors would throw a delete modal.
	for id := range closedIDs {
		if df, exists := diskFiles[id]; exists {
			if !dryRun {
				absPath := filepath.Join(repoPath, df.Path)
				markDocFileClosed(absPath, id)
			}
			result.Removed = append(result.Removed, df.Path)
		}
	}

	return result, nil
}

// docTargetPath determines the file path for a doc note.
// Uses the first target edge if set, otherwise derives from title.
func docTargetPath(state *State, docsDir string) string {
	for _, t := range state.Targets {
		if strings.HasSuffix(t, ".md") {
			return t
		}
	}
	// Derive from title
	slug := slugify(state.Title)
	if slug == "" {
		slug = state.ID
	}
	return filepath.Join(docsDir, slug+".md")
}

// ParseMaiFrontmatterExported is the exported version for testing.
func ParseMaiFrontmatterExported(content string) (string, string) {
	return parseMaiFrontmatter(content)
}

// DocTargetPathExported is the exported version for daemon use.
func DocTargetPathExported(state *State, docsDir string) string {
	return docTargetPath(state, docsDir)
}

// parseMaiFrontmatter extracts mai-id from YAML frontmatter.
// Returns noteID (empty if none) and the body without frontmatter.
func parseMaiFrontmatter(content string) (noteID, body string) {
	if !strings.HasPrefix(content, "---\n") {
		return "", content
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		// Try end of file
		end = strings.Index(content[4:], "\n---")
		if end < 0 {
			return "", content
		}
	}

	fm := content[4 : 4+end]
	body = strings.TrimLeft(content[4+end+4:], "\n")

	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "mai-id:") {
			noteID = strings.TrimSpace(strings.TrimPrefix(line, "mai-id:"))
		}
	}
	return noteID, body
}

// parseAllFrontmatter extracts all frontmatter lines and the body from a YAML
// frontmatter block. Consistent with parseMaiFrontmatter in parsing rules.
func parseAllFrontmatter(content string) (fmLines []string, body string, ok bool) {
	if !strings.HasPrefix(content, "---\n") {
		return nil, content, false
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		end = strings.Index(content[4:], "\n---")
		if end < 0 {
			return nil, content, false
		}
	}
	fmBlock := content[4 : 4+end]
	body = strings.TrimLeft(content[4+end+4:], "\n")
	if fmBlock == "" {
		return []string{}, body, true
	}
	return strings.Split(fmBlock, "\n"), body, true
}

// writeDocFile writes a markdown file preserving all existing Obsidian
// frontmatter fields. Only the mai-id field is added or updated; all other
// fields (tags, aliases, cssclasses, date, etc.) are kept as-is.
// If the file does not exist, writes a minimal frontmatter with just mai-id.
func writeDocFile(absPath, noteID, body string) error {
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return err
	}

	// Try to read and preserve existing frontmatter.
	existing, err := os.ReadFile(absPath)
	if err == nil {
		fmLines, _, ok := parseAllFrontmatter(string(existing))
		if ok {
			maidFound := false
			var newLines []string
			for _, line := range fmLines {
				if strings.HasPrefix(strings.TrimSpace(line), "mai-id:") {
					newLines = append(newLines, "mai-id: "+noteID)
					maidFound = true
				} else {
					newLines = append(newLines, line)
				}
			}
			if !maidFound {
				newLines = append(newLines, "mai-id: "+noteID)
			}
			fm := strings.Join(newLines, "\n")
			content := fmt.Sprintf("---\n%s\n---\n%s\n", fm, body)
			return os.WriteFile(absPath, []byte(content), 0644)
		}
	}

	// File doesn't exist or has no frontmatter — write minimal frontmatter.
	content := fmt.Sprintf("---\nmai-id: %s\n---\n%s\n", noteID, body)
	return os.WriteFile(absPath, []byte(content), 0644)
}

// markDocFileClosed adds closed: true to the frontmatter of a doc file,
// keeping the file and all other frontmatter fields intact.
// If the file does not exist, does nothing.
func markDocFileClosed(absPath, noteID string) error {
	existing, err := os.ReadFile(absPath)
	if err != nil {
		return nil // file already gone, nothing to mark
	}

	fmLines, body, ok := parseAllFrontmatter(string(existing))
	if !ok {
		// No frontmatter — create one with mai-id and closed.
		content := fmt.Sprintf("---\nmai-id: %s\nclosed: true\n---\n%s\n", noteID, body)
		return os.WriteFile(absPath, []byte(content), 0644)
	}

	maidFound, closedFound := false, false
	var newLines []string
	for _, line := range fmLines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "mai-id:") {
			newLines = append(newLines, "mai-id: "+noteID)
			maidFound = true
		} else if strings.HasPrefix(trimmed, "closed:") {
			newLines = append(newLines, "closed: true")
			closedFound = true
		} else {
			newLines = append(newLines, line)
		}
	}
	if !maidFound {
		newLines = append(newLines, "mai-id: "+noteID)
	}
	if !closedFound {
		newLines = append(newLines, "closed: true")
	}

	fm := strings.Join(newLines, "\n")
	content := fmt.Sprintf("---\n%s\n---\n%s\n", fm, body)
	return os.WriteFile(absPath, []byte(content), 0644)
}

func contentHash(s string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(s)))
	return fmt.Sprintf("%x", h[:16])
}

func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	// Collapse multiple dashes
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

// loadTombstones reads .maitake/tombstones — note IDs whose files were intentionally deleted.
func loadTombstones(maitakeDir string) map[string]bool {
	data, err := os.ReadFile(filepath.Join(maitakeDir, "tombstones"))
	if err != nil {
		return nil
	}
	ts := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			ts[line] = true
		}
	}
	return ts
}

// AddTombstone adds a note ID to the tombstone list.
func AddTombstone(repoPath, noteID string) {
	tsFile := filepath.Join(repoPath, ".maitake", "tombstones")
	os.MkdirAll(filepath.Dir(tsFile), 0755)
	data, _ := os.ReadFile(tsFile)
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == noteID {
			return
		}
	}
	f, _ := os.OpenFile(tsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if f != nil {
		f.WriteString(noteID + "\n")
		f.Close()
	}
}

// RemoveTombstone removes a note from the tombstone list (e.g. when re-materializing).
func RemoveTombstone(repoPath, noteID string) {
	tsFile := filepath.Join(repoPath, ".maitake", "tombstones")
	data, err := os.ReadFile(tsFile)
	if err != nil {
		return
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != noteID && strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	os.WriteFile(tsFile, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

// mergeResult holds the output of a CRDT merge.
type mergeResult struct {
	Body      string
	YDocState []byte
}

// lastSyncBody returns the body content as it was when last synced to disk.
// This is the "base" for 3-way merge. We look for the body value at the time
// the file was last written (= the body in the most recent autoSync or docs sync).
// Falls back to creation body if no sync happened.
func lastSyncBody(state *State) string {
	// The file was last written with the note's body at that time.
	// Since we don't track sync timestamps yet, use the CREATION body
	// as the base — it's what the file was first created from.
	// TODO: track last-sync body hash for better 3-way merge base
	return ""
}

// mergeViaCRDT merges a note's body with a file's content using CRDT.
//
// The note's YDoc state is the authority. The file's changes are applied
// as a peer edit on top of the same shared base:
// 1. Load the note's YDoc (contains all note-side edits)
// 2. Get what the YDoc thinks the content is (= note-side view)
// 3. The file was last synced from an OLDER version of the YDoc
// 4. Load that same base state as a "file peer", apply the file diff
// 5. Merge both via full-state Apply
//
// Key insight: the file peer must start from the SAME base state as the
// note doc to share CRDT history. We approximate the base as the initial
// YDoc state (before the most recent note-side edit).
//
// If no YDoc state exists, falls back to file-wins and initializes YDoc.
func mergeViaCRDT(state *State, fileContent string, creationBody string) mergeResult {
	if state.YDocState != nil {
		noteDoc, err := crdt.Load(state.YDocState)
		if err == nil {
			noteContent, _ := noteDoc.Content()
			noteDoc.Close()

			if noteContent == fileContent {
				return mergeResult{Body: state.Body, YDocState: state.YDocState}
			}

			// True 3-way CRDT merge:
			// 1. Create a base YDoc from the original content
			// 2. Create note-side peer from base, apply note edits
			// 3. Create file-side peer from base, apply file edits
			// 4. Merge via full-state Apply
			base := creationBody
			if base == "" {
				base = noteContent
			}

			// Base YDoc
			baseDoc, err := crdt.New()
			if err == nil {
				defer baseDoc.Close()
				baseDoc.Insert(0, base)
				baseState, _ := baseDoc.Save()

				// Note peer: base + note edits
				notePeer, err := crdt.Load(baseState)
				if err == nil {
					defer notePeer.Close()
					noteOps := crdt.Diff(base, noteContent)
					crdt.ApplyOps(notePeer, noteOps)

					// File peer: base + file edits
					filePeer, err := crdt.Load(baseState)
					if err == nil {
						defer filePeer.Close()
						fileOps := crdt.Diff(base, fileContent)
						crdt.ApplyOps(filePeer, fileOps)
						filePeerState, _ := filePeer.Save()

						// Merge: note peer gets file peer's state
						notePeer.Apply(filePeerState)
						merged, _ := notePeer.Content()
						newState, _ := notePeer.Save()
						return mergeResult{Body: merged, YDocState: newState}
					}
				}
			}
		}
	}

	// No YDoc state or CRDT failed — initialize from file content (file wins)
	doc, err := crdt.New()
	if err != nil {
		return mergeResult{Body: fileContent}
	}
	defer doc.Close()
	doc.Insert(0, fileContent)
	newState, _ := doc.Save()
	return mergeResult{Body: fileContent, YDocState: newState}
}

func titleFromPath(path string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".md")
	// Convert dashes/underscores to spaces, title case first letter
	base = strings.ReplaceAll(base, "-", " ")
	base = strings.ReplaceAll(base, "_", " ")
	if len(base) > 0 {
		return strings.ToUpper(base[:1]) + base[1:]
	}
	return base
}
