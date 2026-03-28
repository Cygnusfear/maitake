package notes

import (
	"encoding/json"
	"fmt"
	"strings"
)

const separator = "\n"

// Parse parses a single JSON note line.
func Parse(raw []byte) (*Note, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, fmt.Errorf("empty note")
	}

	var note Note
	if err := json.Unmarshal([]byte(trimmed), &note); err != nil {
		return nil, fmt.Errorf("invalid note JSON: %w", err)
	}

	if note.Kind == "" {
		return nil, fmt.Errorf("note missing required 'kind' field")
	}

	return &note, nil
}

// ParseMulti parses multiple JSON note lines from a git note blob.
// Each line is one self-contained JSON note. Empty lines are skipped.
// This is the format produced by git notes append (one JSON object per line)
// and compatible with cat_sort_uniq merge.
func ParseMulti(raw []byte) ([]*Note, error) {
	var notes []*Note
	for i, line := range strings.Split(string(raw), separator) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		note, err := Parse([]byte(line))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		notes = append(notes, note)
	}
	return notes, nil
}

// Serialize converts a Note to a single JSON line (no trailing newline).
func Serialize(note *Note) ([]byte, error) {
	return json.Marshal(note)
}

// SerializePretty converts a Note to indented JSON for human display.
func SerializePretty(note *Note) ([]byte, error) {
	return json.MarshalIndent(note, "", "  ")
}
