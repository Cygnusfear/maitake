package notes

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/cygnusfear/maitake/pkg/git"
)

var knownEdgeTargetKinds = map[string]struct{}{
	"commit": {},
	"blob":   {},
	"tree":   {},
	"path":   {},
	"note":   {},
	"change": {},
}

// Parse parses one raw maitake note.
func Parse(raw []byte) (*Note, error) {
	text := normalizeNoteText(raw)
	if strings.TrimSpace(text) == "" {
		return nil, parseError(1, "expected note content")
	}

	lines := strings.Split(text, "\n")
	headerEnd := len(lines)
	for i, line := range lines {
		if line == "" {
			headerEnd = i
			break
		}
	}
	if headerEnd == 0 {
		return nil, parseError(1, "expected 'id <value>' or 'kind <value>'")
	}

	note := &Note{}
	for i := 0; i < headerEnd; i++ {
		lineNo := i + 1
		key, value, err := parseHeaderLine(lines[i], lineNo)
		if err != nil {
			return nil, err
		}

		if lineNo == 1 {
			switch key {
			case "id":
				if value == "" {
					return nil, parseError(lineNo, "expected 'id <value>'")
				}
				note.ID = value
			case "kind":
				if value == "" {
					return nil, parseError(lineNo, "expected 'kind <value>'")
				}
				note.Kind = value
			default:
				return nil, parseError(lineNo, "expected first line to be 'id <value>' or 'kind <value>'")
			}
			continue
		}

		if err := applyHeader(note, key, value, lineNo); err != nil {
			return nil, err
		}
	}

	if note.Kind == "" {
		return nil, parseError(1, "expected kind header")
	}

	if headerEnd < len(lines) {
		note.Body = strings.Join(lines[headerEnd+1:], "\n")
	}

	if len(note.Headers) == 0 {
		note.Headers = nil
	}
	if len(note.Tags) == 0 {
		note.Tags = nil
	}
	if len(note.Edges) == 0 {
		note.Edges = nil
	}

	return note, nil
}

// ParseMulti parses one or more concatenated notes separated by the maitake separator.
func ParseMulti(raw []byte) ([]*Note, error) {
	text := normalizeNoteText(raw)
	if strings.TrimSpace(text) == "" {
		return nil, parseError(1, "expected note content")
	}

	parts := strings.Split(text, git.NotesSeparator)
	notes := make([]*Note, 0, len(parts))
	for i, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		note, err := Parse([]byte(part))
		if err != nil {
			return nil, fmt.Errorf("note %d: %w", i+1, err)
		}
		notes = append(notes, note)
	}
	if len(notes) == 0 {
		return nil, parseError(1, "expected note content")
	}

	return notes, nil
}

// Serialize converts a Note into canonical maitake note bytes.
func Serialize(note *Note) []byte {
	if note == nil {
		return nil
	}

	lines := make([]string, 0, 16)
	if note.ID != "" {
		lines = append(lines, headerLine("id", note.ID))
		if note.Kind != "" {
			lines = append(lines, headerLine("kind", note.Kind))
		}
	} else {
		lines = append(lines, headerLine("kind", note.Kind))
	}

	if note.Title != "" {
		lines = append(lines, headerLine("title", note.Title))
	}
	if note.Type != "" {
		lines = append(lines, headerLine("type", note.Type))
	}
	if note.Status != "" {
		lines = append(lines, headerLine("status", note.Status))
	}
	if note.Priority != 0 {
		lines = append(lines, headerLine("priority", strconv.Itoa(note.Priority)))
	}
	if note.Assignee != "" {
		lines = append(lines, headerLine("assignee", note.Assignee))
	}
	if len(note.Tags) > 0 {
		lines = append(lines, headerLine("tags", strings.Join(note.Tags, ",")))
	}
	if note.Field != "" {
		lines = append(lines, headerLine("field", note.Field))
	}
	if note.Value != "" {
		lines = append(lines, headerLine("value", note.Value))
	}
	for _, edge := range note.Edges {
		lines = append(lines, headerLine("edge", fmt.Sprintf("%s %s:%s", edge.Type, edge.Target.Kind, edge.Target.Ref)))
	}

	if len(note.Headers) > 0 {
		keys := make([]string, 0, len(note.Headers))
		for key := range note.Headers {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			lines = append(lines, headerLine(key, note.Headers[key]))
		}
	}

	lines = append(lines, "")
	if note.Body != "" {
		lines = append(lines, note.Body)
	}

	return []byte(strings.Join(lines, "\n"))
}

func normalizeNoteText(raw []byte) string {
	return strings.ReplaceAll(string(raw), "\r\n", "\n")
}

func parseHeaderLine(line string, lineNo int) (string, string, error) {
	parts := strings.SplitN(line, " ", 2)
	if len(parts) != 2 {
		return "", "", parseError(lineNo, "expected 'key <value>'")
	}

	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	if key == "" || value == "" {
		return "", "", parseError(lineNo, "expected 'key <value>'")
	}

	return key, value, nil
}

func applyHeader(note *Note, key string, value string, lineNo int) error {
	switch key {
	case "id":
		return parseError(lineNo, "unexpected id header after line 1")
	case "kind":
		note.Kind = value
	case "title":
		note.Title = value
	case "type":
		note.Type = value
	case "status":
		note.Status = value
	case "priority":
		priority, err := strconv.Atoi(value)
		if err != nil {
			return parseError(lineNo, fmt.Sprintf("expected integer priority, got %q", value))
		}
		note.Priority = priority
	case "assignee":
		note.Assignee = value
	case "tags":
		note.Tags = parseTags(value)
	case "field":
		note.Field = value
	case "value":
		note.Value = value
	case "edge":
		edge, err := parseEdge(value, lineNo)
		if err != nil {
			return err
		}
		note.Edges = append(note.Edges, edge)
	default:
		if note.Headers == nil {
			note.Headers = make(map[string]string)
		}
		note.Headers[key] = value
	}

	return nil
}

func parseTags(value string) []string {
	parts := strings.Split(value, ",")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	if len(tags) == 0 {
		return nil
	}
	return tags
}

func parseEdge(value string, lineNo int) (Edge, error) {
	parts := strings.SplitN(value, " ", 2)
	if len(parts) != 2 {
		return Edge{}, parseError(lineNo, "expected 'edge <type> <target-kind>:<ref>'")
	}

	edgeType := strings.TrimSpace(parts[0])
	target := strings.TrimSpace(parts[1])
	if edgeType == "" || target == "" {
		return Edge{}, parseError(lineNo, "expected 'edge <type> <target-kind>:<ref>'")
	}

	targetParts := strings.SplitN(target, ":", 2)
	if len(targetParts) != 2 {
		return Edge{}, parseError(lineNo, "expected edge target '<target-kind>:<ref>'")
	}

	targetKind := strings.TrimSpace(targetParts[0])
	targetRef := strings.TrimSpace(targetParts[1])
	if targetKind == "" || targetRef == "" {
		return Edge{}, parseError(lineNo, "expected edge target '<target-kind>:<ref>'")
	}
	if _, ok := knownEdgeTargetKinds[targetKind]; !ok {
		return Edge{}, parseError(lineNo, fmt.Sprintf("unknown edge target kind %q", targetKind))
	}

	return Edge{
		Type: edgeType,
		Target: EdgeTarget{
			Kind: targetKind,
			Ref:  targetRef,
		},
	}, nil
}

func headerLine(key string, value string) string {
	return fmt.Sprintf("%s %s", key, value)
}

func parseError(lineNo int, message string) error {
	return fmt.Errorf("line %d: %s", lineNo, message)
}
