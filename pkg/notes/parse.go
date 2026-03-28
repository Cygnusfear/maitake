package notes

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

const separator = "\n---maitake---\n"

// Parse parses raw note bytes into a Note.
// Creation notes start with "id <value>". Events/comments start with "kind <value>".
// Headers come before the first blank line. Body is everything after.
func Parse(raw []byte) (*Note, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("empty note")
	}

	lines := strings.Split(string(raw), "\n")
	note := &Note{
		Headers: make(map[string]string),
	}

	// Parse headers (before first blank line)
	bodyStart := len(lines) // default: no body
	for i, line := range lines {
		if line == "" {
			bodyStart = i + 1
			break
		}

		key, val, ok := parseHeader(line)
		if !ok {
			return nil, fmt.Errorf("line %d: invalid header: %q", i+1, line)
		}

		switch key {
		case "id":
			note.ID = val
		case "kind":
			note.Kind = val
		case "title":
			note.Title = val
		case "type":
			note.Type = val
		case "status":
			note.Status = val
		case "priority":
			p, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("line %d: invalid priority %q: %w", i+1, val, err)
			}
			note.Priority = p
		case "assignee":
			note.Assignee = val
		case "tags":
			note.Tags = parseTags(val)
		case "field":
			note.Field = val
		case "value":
			note.Value = val
		case "edge":
			edge, err := parseEdge(val)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", i+1, err)
			}
			note.Edges = append(note.Edges, edge)
		default:
			note.Headers[key] = val
		}
	}

	// Body is everything after the blank line
	if bodyStart < len(lines) {
		note.Body = strings.Join(lines[bodyStart:], "\n")
		// Trim trailing newline that comes from the split
		note.Body = strings.TrimRight(note.Body, "\n")
	}

	// Validation: must have either id (creation note) or kind (event/comment)
	if note.ID == "" && note.Kind == "" {
		return nil, fmt.Errorf("note must start with 'id' (creation note) or 'kind' (event/comment)")
	}

	// If id is set but kind is not, that's OK — kind may come from another header
	// If kind is set but id is not, that's an event or comment

	return note, nil
}

// ParseMulti parses multiple notes from a concatenated blob, split by separator.
func ParseMulti(raw []byte) ([]*Note, error) {
	parts := bytes.Split(raw, []byte(separator))
	var notes []*Note
	for i, part := range parts {
		trimmed := bytes.TrimSpace(part)
		if len(trimmed) == 0 {
			continue
		}
		note, err := Parse(trimmed)
		if err != nil {
			return nil, fmt.Errorf("note %d: %w", i+1, err)
		}
		notes = append(notes, note)
	}
	return notes, nil
}

// Serialize converts a Note to bytes.
// ID or Kind comes first, then other headers, blank line, body.
func Serialize(note *Note) []byte {
	var b strings.Builder

	// First line: id for creation notes, kind for events/comments
	if note.ID != "" {
		fmt.Fprintf(&b, "id %s\n", note.ID)
	}
	if note.Kind != "" {
		fmt.Fprintf(&b, "kind %s\n", note.Kind)
	}
	if note.Title != "" {
		fmt.Fprintf(&b, "title %s\n", note.Title)
	}
	if note.Type != "" {
		fmt.Fprintf(&b, "type %s\n", note.Type)
	}
	if note.Status != "" {
		fmt.Fprintf(&b, "status %s\n", note.Status)
	}
	if note.Priority != 0 {
		fmt.Fprintf(&b, "priority %d\n", note.Priority)
	}
	if note.Assignee != "" {
		fmt.Fprintf(&b, "assignee %s\n", note.Assignee)
	}
	if len(note.Tags) > 0 {
		fmt.Fprintf(&b, "tags %s\n", strings.Join(note.Tags, ","))
	}
	if note.Field != "" {
		fmt.Fprintf(&b, "field %s\n", note.Field)
	}
	if note.Value != "" {
		fmt.Fprintf(&b, "value %s\n", note.Value)
	}
	for _, edge := range note.Edges {
		fmt.Fprintf(&b, "edge %s %s:%s\n", edge.Type, edge.Target.Kind, edge.Target.Ref)
	}
	// Unknown headers
	for k, v := range note.Headers {
		fmt.Fprintf(&b, "%s %s\n", k, v)
	}

	// Blank line + body
	if note.Body != "" {
		b.WriteString("\n")
		b.WriteString(note.Body)
	}

	return []byte(b.String())
}

// parseHeader splits a header line into key and value.
func parseHeader(line string) (key, val string, ok bool) {
	idx := strings.IndexByte(line, ' ')
	if idx < 0 {
		return "", "", false
	}
	return line[:idx], line[idx+1:], true
}

// parseTags splits comma-separated tags.
func parseTags(s string) []string {
	var tags []string
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

// parseEdge parses an edge value like "targets path:src/auth.ts".
func parseEdge(val string) (Edge, error) {
	parts := strings.SplitN(val, " ", 2)
	if len(parts) != 2 {
		return Edge{}, fmt.Errorf("invalid edge: %q (expected 'type kind:ref')", val)
	}
	edgeType := parts[0]
	targetStr := parts[1]

	colonIdx := strings.IndexByte(targetStr, ':')
	if colonIdx < 0 {
		return Edge{}, fmt.Errorf("invalid edge target: %q (expected 'kind:ref')", targetStr)
	}

	return Edge{
		Type: edgeType,
		Target: EdgeTarget{
			Kind: targetStr[:colonIdx],
			Ref:  targetStr[colonIdx+1:],
		},
	}, nil
}
