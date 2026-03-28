package notes

import (
	"encoding/json"
	"testing"
)

func TestParse_CreationNote(t *testing.T) {
	raw := []byte(`{"id":"tre-5c4a","kind":"ticket","type":"task","priority":1,"assignee":"Alice","tags":["auth","backend"],"edges":[{"type":"targets","target":{"kind":"path","ref":"src/auth.ts"}},{"type":"depends-on","target":{"kind":"note","ref":"wrn-a4f2"}}],"body":"# Fix auth race condition\n\nThe token refresh has a race condition."}`)

	note, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if note.ID != "tre-5c4a" {
		t.Errorf("ID = %q", note.ID)
	}
	if note.Kind != "ticket" {
		t.Errorf("Kind = %q", note.Kind)
	}
	if note.Type != "task" {
		t.Errorf("Type = %q", note.Type)
	}
	if note.Priority != 1 {
		t.Errorf("Priority = %d", note.Priority)
	}
	if note.Assignee != "Alice" {
		t.Errorf("Assignee = %q", note.Assignee)
	}
	if len(note.Tags) != 2 {
		t.Errorf("Tags = %v", note.Tags)
	}
	if len(note.Edges) != 2 {
		t.Fatalf("Edges = %d", len(note.Edges))
	}
	if note.Edges[0].Type != "targets" || note.Edges[0].Target.Kind != "path" || note.Edges[0].Target.Ref != "src/auth.ts" {
		t.Errorf("Edge[0] = %+v", note.Edges[0])
	}
	if note.Body == "" {
		t.Error("Body is empty")
	}
}

func TestParse_EventNote(t *testing.T) {
	raw := []byte(`{"kind":"event","edges":[{"type":"closes","target":{"kind":"note","ref":"tre-5c4a"}}],"field":"status","value":"closed","body":"Fixed in commit abc123."}`)

	note, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if note.ID != "" {
		t.Errorf("event should have no ID, got %q", note.ID)
	}
	if note.Kind != "event" {
		t.Errorf("Kind = %q", note.Kind)
	}
	if note.Field != "status" {
		t.Errorf("Field = %q", note.Field)
	}
	if note.Value != "closed" {
		t.Errorf("Value = %q", note.Value)
	}
	if len(note.Edges) != 1 || note.Edges[0].Type != "closes" {
		t.Errorf("Edges = %+v", note.Edges)
	}
}

func TestParse_CommentNote(t *testing.T) {
	raw := []byte(`{"kind":"comment","edges":[{"type":"on","target":{"kind":"note","ref":"tre-5c4a"}}],"body":"Found root cause in refresh_token().\nThe mutex was missing."}`)

	note, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if note.Kind != "comment" {
		t.Errorf("Kind = %q", note.Kind)
	}
	if note.Body == "" {
		t.Error("Body is empty")
	}
}

func TestParse_ReviewWithLocation(t *testing.T) {
	raw := []byte(`{"id":"rev-1234","kind":"review","edges":[{"type":"targets","target":{"kind":"path","ref":"src/auth.ts"}}],"location":{"path":"src/auth.ts","range":{"startLine":42,"endLine":58}},"resolved":false,"body":"Race condition here. AC: add mutex."}`)

	note, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if note.Location == nil {
		t.Fatal("Location is nil")
	}
	if note.Location.Path != "src/auth.ts" {
		t.Errorf("Location.Path = %q", note.Location.Path)
	}
	if note.Location.Range.StartLine != 42 {
		t.Errorf("StartLine = %d", note.Location.Range.StartLine)
	}
	if note.Resolved == nil || *note.Resolved != false {
		t.Errorf("Resolved = %v", note.Resolved)
	}
}

func TestParse_ThreadedComment(t *testing.T) {
	raw := []byte(`{"kind":"comment","parent":"comment-abc","edges":[{"type":"on","target":{"kind":"note","ref":"tre-5c4a"}}],"body":"I agree, the mutex approach is correct."}`)

	note, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if note.Parent != "comment-abc" {
		t.Errorf("Parent = %q", note.Parent)
	}
}

func TestParse_RejectEmpty(t *testing.T) {
	_, err := Parse([]byte(""))
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParse_RejectMissingKind(t *testing.T) {
	_, err := Parse([]byte(`{"id":"test","body":"no kind"}`))
	if err == nil {
		t.Fatal("expected error for missing kind")
	}
}

func TestParse_RejectInvalidJSON(t *testing.T) {
	_, err := Parse([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestRoundTrip(t *testing.T) {
	original := &Note{
		ID:       "tre-5c4a",
		Kind:     "ticket",
		Type:     "task",
		Title:    "Fix auth race condition",
		Priority: 1,
		Assignee: "Alice",
		Tags:     []string{"auth", "backend"},
		Edges: []Edge{
			{Type: "targets", Target: EdgeTarget{Kind: "path", Ref: "src/auth.ts"}},
		},
		Body: "The token refresh has a race condition.",
	}

	serialized, err := Serialize(original)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := Parse(serialized)
	if err != nil {
		t.Fatalf("round-trip parse failed: %v\nSerialized: %s", err, serialized)
	}

	if parsed.ID != original.ID {
		t.Errorf("ID: %q != %q", parsed.ID, original.ID)
	}
	if parsed.Kind != original.Kind {
		t.Errorf("Kind: %q != %q", parsed.Kind, original.Kind)
	}
	if parsed.Priority != original.Priority {
		t.Errorf("Priority: %d != %d", parsed.Priority, original.Priority)
	}
	if parsed.Body != original.Body {
		t.Errorf("Body: %q != %q", parsed.Body, original.Body)
	}
	if len(parsed.Tags) != len(original.Tags) {
		t.Errorf("Tags: %v != %v", parsed.Tags, original.Tags)
	}
	if len(parsed.Edges) != len(original.Edges) {
		t.Errorf("Edges: %d != %d", len(parsed.Edges), len(original.Edges))
	}
}

func TestRoundTrip_Event(t *testing.T) {
	original := &Note{
		Kind:  "event",
		Field: "status",
		Value: "in_progress",
		Edges: []Edge{
			{Type: "starts", Target: EdgeTarget{Kind: "note", Ref: "tre-5c4a"}},
		},
	}

	serialized, err := Serialize(original)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(serialized)
	if err != nil {
		t.Fatal(err)
	}

	if parsed.Kind != "event" {
		t.Errorf("Kind = %q", parsed.Kind)
	}
	if parsed.Field != "status" {
		t.Errorf("Field = %q", parsed.Field)
	}
}

func TestParseMulti(t *testing.T) {
	line1, _ := json.Marshal(&Note{ID: "note-1", Kind: "ticket", Body: "First"})
	line2, _ := json.Marshal(&Note{Kind: "event", Edges: []Edge{{Type: "closes", Target: EdgeTarget{Kind: "note", Ref: "note-1"}}}})
	raw := append(line1, '\n')
	raw = append(raw, line2...)

	notes, err := ParseMulti(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 {
		t.Fatalf("got %d notes, want 2", len(notes))
	}
	if notes[0].ID != "note-1" {
		t.Errorf("note 0 ID = %q", notes[0].ID)
	}
	if notes[1].Kind != "event" {
		t.Errorf("note 1 Kind = %q", notes[1].Kind)
	}
}

func TestParseMulti_SingleNote(t *testing.T) {
	raw, _ := json.Marshal(&Note{ID: "only", Kind: "warning", Body: "Just one"})
	notes, err := ParseMulti(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 {
		t.Fatalf("got %d, want 1", len(notes))
	}
}

func TestParseMulti_EmptyLines(t *testing.T) {
	line, _ := json.Marshal(&Note{ID: "x", Kind: "ticket"})
	raw := append([]byte("\n\n"), line...)
	raw = append(raw, '\n', '\n')

	notes, err := ParseMulti(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 {
		t.Fatalf("got %d, want 1", len(notes))
	}
}


