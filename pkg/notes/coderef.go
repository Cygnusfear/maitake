package notes

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// CodeRef represents a // @mai: [[target]] reference found in source code.
type CodeRef struct {
	File   string // relative path from repo root
	Line   int    // 1-indexed line number
	Target string // the [[target]] content (note ID, file#section, etc.)
	Raw    string // the full matched line
}

// @mai: [[target]] pattern — matches in comments across languages
// Supports: // @mai: [[x]], # @mai: [[x]], -- @mai: [[x]], /* @mai: [[x]] */
var codeRefPattern = regexp.MustCompile(`@mai:\s*\[\[([^\]]+)\]\]`)

// WikiRef represents a [[target]] wiki link found in a note body.
type WikiRef struct {
	NoteID string // the note containing the link
	Target string // the [[target]] content
}

// wikiLinkPattern matches [[target]] or [[target|alias]] in note bodies
var wikiLinkPattern = regexp.MustCompile(`\[\[([^\]|]+)(?:\|[^\]]+)?\]\]`)

// Source file extensions to scan for @mai: refs
var sourceExtensions = map[string]bool{
	".go": true, ".rs": true, ".ts": true, ".tsx": true,
	".js": true, ".jsx": true, ".py": true, ".rb": true,
	".c": true, ".h": true, ".cpp": true, ".hpp": true,
	".java": true, ".kt": true, ".swift": true, ".zig": true,
	".lua": true, ".sh": true, ".bash": true, ".zsh": true,
	".toml": true, ".yaml": true, ".yml": true,
}

// Directories to skip when scanning
var skipDirs = map[string]bool{
	".git": true, ".maitake": true, "node_modules": true,
	"vendor": true, "target": true, "dist": true, "build": true,
	".next": true, "__pycache__": true, ".worktrees": true,
}

// ScanCodeRefs walks the repo and finds all // @mai: [[target]] references.
func ScanCodeRefs(repoPath string) ([]CodeRef, error) {
	var refs []CodeRef

	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(info.Name())
		if !sourceExtensions[ext] {
			return nil
		}

		// Skip large files (>1MB)
		if info.Size() > 1<<20 {
			return nil
		}

		rel, _ := filepath.Rel(repoPath, path)
		fileRefs, err := scanFileForCodeRefs(path, rel)
		if err != nil {
			return nil // skip unreadable files
		}
		refs = append(refs, fileRefs...)
		return nil
	})

	return refs, err
}

func scanFileForCodeRefs(absPath, relPath string) ([]CodeRef, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var refs []CodeRef
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		matches := codeRefPattern.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			refs = append(refs, CodeRef{
				File:   relPath,
				Line:   lineNum,
				Target: m[1],
				Raw:    strings.TrimSpace(line),
			})
		}
	}
	return refs, scanner.Err()
}

// ExtractWikiRefs extracts all [[target]] links from a note's body.
func ExtractWikiRefs(noteID, body string) []WikiRef {
	var refs []WikiRef
	matches := wikiLinkPattern.FindAllStringSubmatch(body, -1)
	for _, m := range matches {
		refs = append(refs, WikiRef{
			NoteID: noteID,
			Target: m[1],
		})
	}
	return refs
}

// CheckResult holds the output of mai check.
type CheckResult struct {
	CodeRefs    []CodeRef
	WikiRefs    []WikiRef
	BrokenCode  []RefError
	BrokenWiki  []RefError
	OrphanNotes []string // notes with no code refs or wiki refs pointing at them
}

// RefError is a single validation failure for a code ref or wiki link.
type RefError struct {
	File    string // source file (for code refs) or empty
	NoteID  string // note ID (for wiki refs) or empty
	Line    int
	Target  string
	Message string
}

// Check validates all [[refs]] in note bodies and // @mai: refs in source code.
func Check(engine Engine, repoPath string) (*CheckResult, error) {
	result := &CheckResult{}

	// 1. Scan source code for @mai: refs
	codeRefs, err := ScanCodeRefs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("scanning code refs: %w", err)
	}
	result.CodeRefs = codeRefs

	// 2. Extract wiki refs from all note bodies
	states, err := engine.Find(FindOptions{})
	if err != nil {
		return nil, fmt.Errorf("loading notes: %w", err)
	}
	// Also check closed notes
	closedStates, _ := engine.Find(FindOptions{Status: "closed"})
	states = append(states, closedStates...)

	for _, state := range states {
		bodyRefs := ExtractWikiRefs(state.ID, state.Body)
		// Also scan comments
		for _, c := range state.Comments {
			bodyRefs = append(bodyRefs, ExtractWikiRefs(state.ID, c.Body)...)
		}
		result.WikiRefs = append(result.WikiRefs, bodyRefs...)
	}

	// 3. Build lookup sets for validation
	noteIDs := make(map[string]bool)
	for _, s := range states {
		noteIDs[s.ID] = true
	}

	// 4. Validate code refs — each [[target]] must resolve to a note ID
	for _, ref := range result.CodeRefs {
		if !resolveTarget(ref.Target, noteIDs, repoPath) {
			result.BrokenCode = append(result.BrokenCode, RefError{
				File:   ref.File,
				Line:   ref.Line,
				Target: ref.Target,
				Message: fmt.Sprintf("broken code ref @mai: [[%s]] — no matching note", ref.Target),
			})
		}
	}

	// 5. Validate wiki refs in note bodies — [[target]] must resolve to a note or file
	for _, ref := range result.WikiRefs {
		if !resolveTarget(ref.Target, noteIDs, repoPath) {
			result.BrokenWiki = append(result.BrokenWiki, RefError{
				NoteID: ref.NoteID,
				Target: ref.Target,
				Message: fmt.Sprintf("broken wiki link [[%s]] in note %s", ref.Target, ref.NoteID),
			})
		}
	}

	return result, nil
}

// resolveTarget checks if a [[target]] resolves to something real.
// Tries: exact note ID, partial note ID, file path.
func resolveTarget(target string, noteIDs map[string]bool, repoPath string) bool {
	// Exact note ID match
	if noteIDs[target] {
		return true
	}

	// Partial note ID match (e.g. "5c4a" matches "tre-5c4a")
	for id := range noteIDs {
		if strings.Contains(id, target) {
			return true
		}
	}

	// File path match (e.g. "src/auth.ts" or "docs/architecture.md")
	absPath := filepath.Join(repoPath, target)
	// Strip #section suffix for file refs
	if idx := strings.Index(absPath, "#"); idx >= 0 {
		absPath = absPath[:idx]
	}
	if _, err := os.Stat(absPath); err == nil {
		return true
	}

	return false
}

func (e *RefError) String() string {
	if e.File != "" {
		return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Message)
	}
	return e.Message
}
