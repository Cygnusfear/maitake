// Package docs implements bidirectional sync between doc notes and markdown
// files on disk. It consumes the notes.Engine interface — it never touches
// git directly.
package docs

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cygnusfear/maitake/pkg/crdt"
	"github.com/cygnusfear/maitake/pkg/notes"
)

// Config controls doc materialization.
type Config struct {
	Sync  string `toml:"sync"`  // "auto" | "manual" | "off" (default: "manual")
	Dir   string `toml:"dir"`   // docs directory (default: ".mai-docs")
	Watch bool   `toml:"watch"` // daemon watches this repo
}

// DocFile represents a materialized doc on disk.
type DocFile struct {
	Path    string // relative to repo root
	NoteID  string // from frontmatter, empty if new file
	Content string // markdown content (without frontmatter)
	Hash    string // sha256 of content
}

// SyncResult holds the outcome of a docs sync.
type SyncResult struct {
	Written   []string // files written from notes
	Imported  []string // new files imported as notes
	Updated   []string // notes updated from file changes
	Conflicts []string // both changed, skipped
	Removed   []string // files removed (note deleted/closed)
}

// SyncOptions controls sync behavior.
type SyncOptions struct {
	DryRun bool // compute what would happen without writing
}

// SyncDocs performs bidirectional sync between doc notes and markdown files.
func SyncDocs(engine notes.Engine, repoPath string, cfg Config, opts ...SyncOptions) (*SyncResult, error) {
	dryRun := len(opts) > 0 && opts[0].DryRun
	if cfg.Dir == "" {
		cfg.Dir = ".mai-docs"
	}
	docsDir := filepath.Join(repoPath, cfg.Dir)
	result := &SyncResult{}

	// 1. Load all doc notes
	docNotes := make(map[string]*notes.State) // id → state
	states, _ := engine.Find(notes.FindOptions{Kind: "doc"})
	for i := range states {
		docNotes[states[i].ID] = &states[i]
	}
	byPath := make(map[string]*notes.State)   // target path → canonical open doc state
	pathConflicts := make(map[string]bool)     // target path has >1 open doc note
	for i := range states {
		state := &states[i]
		if state.Status == "closed" {
			continue
		}
		path := DocTargetPath(state, cfg.Dir)
		if existing, ok := byPath[path]; ok {
			pathConflicts[path] = true
			_ = existing
			continue
		}
		if pathConflicts[path] {
			continue
		}
		byPath[path] = state
	}
	// Also include closed docs (might need to remove files)
	closedDocs, _ := engine.Find(notes.FindOptions{Kind: "doc", Status: "closed"})
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
		noteID, body := ParseFrontmatter(content)
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
	tombstones := LoadTombstones(filepath.Join(repoPath, ".maitake"))
	newFilesByPath := make(map[string]*DocFile)
	for _, df := range newFiles {
		newFilesByPath[df.Path] = df
	}
	conflictSeen := make(map[string]bool)
	addConflict := func(path string) {
		if !conflictSeen[path] {
			result.Conflicts = append(result.Conflicts, path)
			conflictSeen[path] = true
		}
	}

	// 3. Notes → disk: write or update files
	for id, state := range docNotes {
		if closedIDs[id] {
			continue // handle removals separately
		}
		if tombstones[id] {
			continue // intentionally deleted by user
		}

		targetPath := DocTargetPath(state, cfg.Dir)
		if pathConflicts[targetPath] {
			addConflict(targetPath)
			continue
		}
		noteHash := contentHash(state.Body)

		if df, exists := diskFiles[id]; exists {
			// Both exist — check for conflicts
			if df.Hash == noteHash {
				continue // in sync
			}
			// Hashes differ — merge via CRDT if state available, else file wins
			if !dryRun {
				syncBase := lastSyncBody(state)
				fileChanged := syncBase == "" || contentHash(df.Content) != contentHash(syncBase)
				noteChanged := syncBase == "" || contentHash(state.Body) != contentHash(syncBase)

				if fileChanged && !noteChanged {
					engine.Append(notes.AppendOptions{
						TargetID: id,
						Kind:     "event",
						Field:    "body",
						Body:     df.Content,
					})
					engine.Append(notes.AppendOptions{
						TargetID: id,
						Kind:     "event",
						Field:    "lastsync",
						Body:     df.Content,
					})
				} else if noteChanged && !fileChanged {
					absPath := filepath.Join(repoPath, df.Path)
					WriteDocFile(absPath, id, state.Body)
					engine.Append(notes.AppendOptions{
						TargetID: id,
						Kind:     "event",
						Field:    "lastsync",
						Body:     state.Body,
					})
				} else {
					creationBody := ""
					if creation, err := engine.Get(id); err == nil && creation != nil {
						creationBody = creation.Body
					}
					merged := MergeViaCRDT(state, df.Content, creationBody)
					engine.Append(notes.AppendOptions{
						TargetID: id,
						Kind:     "event",
						Field:    "body",
						Body:     merged.Body,
					})
					if merged.YDocState != nil {
						engine.Append(notes.AppendOptions{
							TargetID: id,
							Kind:     "event",
							Field:    "ydoc",
							Value:    base64.StdEncoding.EncodeToString(merged.YDocState),
						})
					}
					if merged.Body != df.Content {
						absPath := filepath.Join(repoPath, df.Path)
						WriteDocFile(absPath, id, merged.Body)
					}
					engine.Append(notes.AppendOptions{
						TargetID: id,
						Kind:     "event",
						Field:    "lastsync",
						Body:     merged.Body,
					})
				}
			}
			result.Updated = append(result.Updated, df.Path)
		} else {
			if _, pendingImport := newFilesByPath[targetPath]; pendingImport {
				continue
			}
			if !dryRun {
				absPath := filepath.Join(repoPath, targetPath)
				if err := WriteDocFile(absPath, id, state.Body); err != nil {
					continue
				}
				engine.Append(notes.AppendOptions{
					TargetID: id,
					Kind:     "event",
					Field:    "lastsync",
					Body:     state.Body,
				})
			}
			result.Written = append(result.Written, targetPath)
		}
	}

	// 4. Disk → notes: import files without mai-id as doc notes.
	for _, df := range newFiles {
		if pathConflicts[df.Path] {
			addConflict(df.Path)
			continue
		}

		if state, exists := byPath[df.Path]; exists {
			if dryRun {
				result.Updated = append(result.Updated, df.Path)
				continue
			}

			mergedBody := state.Body
			if df.Hash != contentHash(state.Body) {
				creationBody := ""
				if creation, err := engine.Get(state.ID); err == nil && creation != nil {
					creationBody = creation.Body
				}
				merged := MergeViaCRDT(state, df.Content, creationBody)
				mergedBody = merged.Body
				if mergedBody != state.Body {
					engine.Append(notes.AppendOptions{
						TargetID: state.ID,
						Kind:     "event",
						Field:    "body",
						Body:     mergedBody,
					})
				}
				if merged.YDocState != nil {
					engine.Append(notes.AppendOptions{
						TargetID: state.ID,
						Kind:     "event",
						Field:    "ydoc",
						Value:    base64.StdEncoding.EncodeToString(merged.YDocState),
					})
				}
			}

			absPath := filepath.Join(repoPath, df.Path)
			if err := WriteDocFile(absPath, state.ID, mergedBody); err == nil {
				if closedIDs[state.ID] {
					_ = MarkDocFileClosed(absPath, state.ID)
				}
				RemoveTombstone(repoPath, state.ID)
				engine.Append(notes.AppendOptions{
					TargetID: state.ID,
					Kind:     "event",
					Field:    "lastsync",
					Body:     mergedBody,
				})
			}
			result.Updated = append(result.Updated, df.Path)
			continue
		}

		if dryRun {
			result.Imported = append(result.Imported, df.Path)
			continue
		}
		title := TitleFromPath(df.Path)
		note, err := engine.Create(notes.CreateOptions{
			Kind:    "doc",
			Title:   title,
			Body:    df.Content,
			Targets: []string{df.Path},
		})
		if err != nil {
			continue
		}
		absPath := filepath.Join(repoPath, df.Path)
		WriteDocFile(absPath, note.ID, df.Content)
		engine.Append(notes.AppendOptions{
			TargetID: note.ID,
			Kind:     "event",
			Field:    "lastsync",
			Body:     df.Content,
		})
		result.Imported = append(result.Imported, df.Path)
	}

	// 5. Handle closed doc notes — mark closed in frontmatter, keep the file.
	for id := range closedIDs {
		if df, exists := diskFiles[id]; exists {
			if pathConflicts[df.Path] {
				addConflict(df.Path)
				continue
			}
			if !dryRun {
				absPath := filepath.Join(repoPath, df.Path)
				if data, err := os.ReadFile(absPath); err == nil {
					currentID, _ := ParseFrontmatter(string(data))
					if currentID != id {
						continue
					}
				}
				MarkDocFileClosed(absPath, id)
			}
			result.Removed = append(result.Removed, df.Path)
		}
	}

	return result, nil
}

// DocTargetPath determines the file path for a doc note.
func DocTargetPath(state *notes.State, docsDir string) string {
	for _, t := range state.Targets {
		if strings.HasSuffix(t, ".md") {
			return t
		}
	}
	slug := Slugify(state.Title)
	if slug == "" {
		slug = state.ID
	}
	return filepath.Join(docsDir, slug+".md")
}

// ParseFrontmatter extracts mai-id from YAML frontmatter.
func ParseFrontmatter(content string) (noteID, body string) {
	if !strings.HasPrefix(content, "---\n") {
		return "", content
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
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

// ParseAllFrontmatter extracts all frontmatter lines and the body.
func ParseAllFrontmatter(content string) (fmLines []string, body string, ok bool) {
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

// WriteDocFile writes a markdown file preserving existing Obsidian frontmatter.
func WriteDocFile(absPath, noteID, body string) error {
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return err
	}

	existing, err := os.ReadFile(absPath)
	if err == nil {
		fmLines, _, ok := ParseAllFrontmatter(string(existing))
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
			if string(existing) == content {
				return nil
			}
			return os.WriteFile(absPath, []byte(content), 0644)
		}
	}

	content := fmt.Sprintf("---\nmai-id: %s\n---\n%s\n", noteID, body)
	if string(existing) == content {
		return nil
	}
	return os.WriteFile(absPath, []byte(content), 0644)
}

// MarkDocFileClosed adds closed: true to frontmatter.
func MarkDocFileClosed(absPath, noteID string) error {
	existing, err := os.ReadFile(absPath)
	if err != nil {
		return nil
	}

	fmLines, body, ok := ParseAllFrontmatter(string(existing))
	if !ok {
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

// Slugify converts a title to a filename-safe slug.
func Slugify(s string) string {
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
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

// LoadTombstones reads .maitake/tombstones.
func LoadTombstones(maitakeDir string) map[string]bool {
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

// RemoveTombstone removes a note from the tombstone list.
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

// MergeResult holds the output of a CRDT merge.
type MergeResult struct {
	Body      string
	YDocState []byte
}

func lastSyncBody(state *notes.State) string {
	return state.LastSyncBody
}

// MergeViaCRDT merges a note's body with a file's content using CRDT.
func MergeViaCRDT(state *notes.State, fileContent string, creationBody string) MergeResult {
	if state.YDocState != nil {
		noteDoc, err := crdt.Load(state.YDocState)
		if err == nil {
			noteContent, _ := noteDoc.Content()
			noteDoc.Close()

			if noteContent == fileContent {
				return MergeResult{Body: state.Body, YDocState: state.YDocState}
			}

			base := lastSyncBody(state)
			if base == "" && state.YDocState != nil {
				base = noteContent
			}
			if base == "" {
				base = creationBody
			}
			if base == "" {
				base = noteContent
			}

			baseDoc, err := crdt.New()
			if err == nil {
				defer baseDoc.Close()
				baseDoc.Insert(0, base)
				baseState, _ := baseDoc.Save()

				notePeer, err := crdt.Load(baseState)
				if err == nil {
					defer notePeer.Close()
					noteOps := crdt.Diff(base, noteContent)
					crdt.ApplyOps(notePeer, noteOps)

					filePeer, err := crdt.Load(baseState)
					if err == nil {
						defer filePeer.Close()
						fileOps := crdt.Diff(base, fileContent)
						crdt.ApplyOps(filePeer, fileOps)
						filePeerState, _ := filePeer.Save()

						notePeer.Apply(filePeerState)
						merged, _ := notePeer.Content()
						newState, _ := notePeer.Save()
						return MergeResult{Body: merged, YDocState: newState}
					}
				}
			}
		}
	}

	doc, err := crdt.New()
	if err != nil {
		return MergeResult{Body: fileContent}
	}
	defer doc.Close()
	doc.Insert(0, fileContent)
	newState, _ := doc.Save()
	return MergeResult{Body: fileContent, YDocState: newState}
}

// TitleFromPath derives a title from a file path.
func TitleFromPath(path string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".md")
	base = strings.ReplaceAll(base, "-", " ")
	base = strings.ReplaceAll(base, "_", " ")
	if len(base) > 0 {
		return strings.ToUpper(base[:1]) + base[1:]
	}
	return base
}
