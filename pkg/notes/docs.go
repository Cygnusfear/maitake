package notes

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DocsConfig controls doc materialization.
type DocsConfig struct {
	Dir string // relative path from repo root (default: "docs")
}

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

// SyncDocs performs bidirectional sync between doc notes and markdown files.
func SyncDocs(engine Engine, repoPath string, cfg DocsConfig) (*DocSyncResult, error) {
	if cfg.Dir == "" {
		cfg.Dir = "docs"
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

	// 3. Notes → disk: write or update files
	for id, state := range docNotes {
		if closedIDs[id] {
			continue // handle removals separately
		}

		targetPath := docTargetPath(state, cfg.Dir)
		noteHash := contentHash(state.Body)

		if df, exists := diskFiles[id]; exists {
			// Both exist — check for conflicts
			if df.Hash == noteHash {
				continue // in sync
			}
			// File changed — check if note also changed
			// For now: file wins (Obsidian edits are the common case)
			_, err := engine.Append(AppendOptions{
				TargetID: id,
				Kind:     "event",
				Field:    "body",
				Value:    df.Content,
			})
			if err == nil {
				result.Updated = append(result.Updated, df.Path)
			}
		} else {
			// Note exists, no file — write it
			absPath := filepath.Join(repoPath, targetPath)
			if err := writeDocFile(absPath, id, state.Body); err != nil {
				continue
			}
			result.Written = append(result.Written, targetPath)
		}
	}

	// 4. Disk → notes: import new files (no mai-id)
	for _, df := range newFiles {
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
		// Rewrite the file with frontmatter
		absPath := filepath.Join(repoPath, df.Path)
		writeDocFile(absPath, note.ID, df.Content)
		result.Imported = append(result.Imported, df.Path)
	}

	// 5. Handle closed/deleted doc notes — remove materialized files
	for id := range closedIDs {
		if df, exists := diskFiles[id]; exists {
			absPath := filepath.Join(repoPath, df.Path)
			os.Remove(absPath)
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

// writeDocFile writes a markdown file with mai-id frontmatter.
func writeDocFile(absPath, noteID, body string) error {
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return err
	}
	content := fmt.Sprintf("---\nmai-id: %s\n---\n%s\n", noteID, body)
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
