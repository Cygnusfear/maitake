package notes

import (
	"strings"
	"testing"
)

func TestParse_CreationNote(t *testing.T) {
	raw := []byte(`id tre-5c4a
kind ticket
title Fix auth race condition
type task
status open
priority 1
assignee Alice
tags auth,backend
edge targets path:src/auth.ts
edge depends-on note:wrn-a4f2

The token refresh has a race condition.`)

	note, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if note.ID != "tre-5c4a" {
		t.Errorf("ID = %q, want tre-5c4a", note.ID)
	}
	if note.Kind != "ticket" {
		t.Errorf("Kind = %q, want ticket", note.Kind)
	}
	if note.Title != "Fix auth race condition" {
		t.Errorf("Title = %q", note.Title)
	}
	if note.Type != "task" {
		t.Errorf("Type = %q", note.Type)
	}
	if note.Status != "open" {
		t.Errorf("Status = %q", note.Status)
	}
	if note.Priority != 1 {
		t.Errorf("Priority = %d", note.Priority)
	}
	if note.Assignee != "Alice" {
		t.Errorf("Assignee = %q", note.Assignee)
	}
	if len(note.Tags) != 2 || note.Tags[0] != "auth" || note.Tags[1] != "backend" {
		t.Errorf("Tags = %v", note.Tags)
	}
	if len(note.Edges) != 2 {
		t.Fatalf("Edges = %d, want 2", len(note.Edges))
	}
	if note.Edges[0].Type != "targets" || note.Edges[0].Target.Kind != "path" || note.Edges[0].Target.Ref != "src/auth.ts" {
		t.Errorf("Edge[0] = %+v", note.Edges[0])
	}
	if note.Edges[1].Type != "depends-on" || note.Edges[1].Target.Kind != "note" || note.Edges[1].Target.Ref != "wrn-a4f2" {
		t.Errorf("Edge[1] = %+v", note.Edges[1])
	}
	if note.Body != "The token refresh has a race condition." {
		t.Errorf("Body = %q", note.Body)
	}
}

func TestParse_EventNote(t *testing.T) {
	raw := []byte(`kind event
edge closes note:tre-5c4a
field status
value closed

Fixed in commit abc123.`)

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
	if note.Body != "Fixed in commit abc123." {
		t.Errorf("Body = %q", note.Body)
	}
}

func TestParse_CommentNote(t *testing.T) {
	raw := []byte(`kind comment
edge on note:tre-5c4a

Found root cause in refresh_token().
The mutex was missing.`)

	note, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if note.Kind != "comment" {
		t.Errorf("Kind = %q", note.Kind)
	}
	if !strings.Contains(note.Body, "Found root cause") {
		t.Errorf("Body = %q", note.Body)
	}
	if !strings.Contains(note.Body, "mutex was missing") {
		t.Errorf("Body should be multi-line, got %q", note.Body)
	}
}

func TestParse_UnknownHeaders(t *testing.T) {
	raw := []byte(`id test-1234
kind warning
custom-field some value
another-field 42

Body text.`)

	note, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if note.Headers["custom-field"] != "some value" {
		t.Errorf("custom-field = %q", note.Headers["custom-field"])
	}
	if note.Headers["another-field"] != "42" {
		t.Errorf("another-field = %q", note.Headers["another-field"])
	}
}

func TestParse_EmptyBody(t *testing.T) {
	raw := []byte(`id test-0001
kind ticket
title No body here`)

	note, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if note.Body != "" {
		t.Errorf("Body = %q, want empty", note.Body)
	}
}

func TestParse_RejectEmpty(t *testing.T) {
	_, err := Parse([]byte(""))
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParse_RejectMissingIDAndKind(t *testing.T) {
	_, err := Parse([]byte("title Just a title\n\nBody"))
	if err == nil {
		t.Fatal("expected error for note without id or kind")
	}
}

func TestParse_EdgeTargetKinds(t *testing.T) {
	raw := []byte(`id test-edge
kind context
edge targets commit:abc123def456
edge targets blob:fedcba654321
edge targets path:src/auth.ts
edge targets note:wrn-1234
edge targets change:jj-change-id
edge targets tree:aabbccdd`)

	note, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	expected := []struct{ kind, ref string }{
		{"commit", "abc123def456"},
		{"blob", "fedcba654321"},
		{"path", "src/auth.ts"},
		{"note", "wrn-1234"},
		{"change", "jj-change-id"},
		{"tree", "aabbccdd"},
	}
	if len(note.Edges) != len(expected) {
		t.Fatalf("got %d edges, want %d", len(note.Edges), len(expected))
	}
	for i, e := range expected {
		if note.Edges[i].Target.Kind != e.kind || note.Edges[i].Target.Ref != e.ref {
			t.Errorf("edge %d: got %+v, want kind=%s ref=%s", i, note.Edges[i].Target, e.kind, e.ref)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	original := &Note{
		ID:       "tre-5c4a",
		Kind:     "ticket",
		Title:    "Fix auth race condition",
		Type:     "task",
		Status:   "open",
		Priority: 1,
		Assignee: "Alice",
		Tags:     []string{"auth", "backend"},
		Edges: []Edge{
			{Type: "targets", Target: EdgeTarget{Kind: "path", Ref: "src/auth.ts"}},
		},
		Body: "The token refresh has a race condition.",
	}

	serialized := Serialize(original)
	parsed, err := Parse(serialized)
	if err != nil {
		t.Fatalf("round-trip parse failed: %v\nSerialized:\n%s", err, serialized)
	}

	if parsed.ID != original.ID {
		t.Errorf("ID: %q != %q", parsed.ID, original.ID)
	}
	if parsed.Kind != original.Kind {
		t.Errorf("Kind: %q != %q", parsed.Kind, original.Kind)
	}
	if parsed.Title != original.Title {
		t.Errorf("Title: %q != %q", parsed.Title, original.Title)
	}
	if parsed.Type != original.Type {
		t.Errorf("Type: %q != %q", parsed.Type, original.Type)
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

	serialized := Serialize(original)
	parsed, err := Parse(serialized)
	if err != nil {
		t.Fatalf("round-trip parse failed: %v\nSerialized:\n%s", err, serialized)
	}

	if parsed.Kind != "event" {
		t.Errorf("Kind = %q", parsed.Kind)
	}
	if parsed.Field != "status" {
		t.Errorf("Field = %q", parsed.Field)
	}
	if parsed.Value != "in_progress" {
		t.Errorf("Value = %q", parsed.Value)
	}
}

func TestParseMulti(t *testing.T) {
	raw := []byte("id note-1\nkind ticket\n\nFirst note\n---maitake---\nkind event\nedge closes note:note-1\n\nClosed it")
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
	raw := []byte("id only-one\nkind warning\n\nJust one note")
	notes, err := ParseMulti(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 {
		t.Fatalf("got %d notes, want 1", len(notes))
	}
}
