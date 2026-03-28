package notes

import (
	"testing"
	"time"
)

func ts(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

func TestFold_CreationOnly(t *testing.T) {
	creation := &Note{
		ID:        "tre-5c4a",
		Kind:      "ticket",
		Title:     "Fix auth",
		Type:      "task",
		Priority:  1,
		Assignee:  "Alice",
		Tags:      []string{"auth"},
		Body:      "Fix the race condition.",
		Timestamp: ts("2026-03-01T10:00:00Z"),
		Edges: []Edge{
			{Type: "targets", Target: EdgeTarget{Kind: "path", Ref: "src/auth.ts"}},
		},
	}

	state := FoldEvents(creation, nil)

	if state.ID != "tre-5c4a" {
		t.Errorf("ID = %q", state.ID)
	}
	if state.Status != "open" {
		t.Errorf("Status = %q, want open", state.Status)
	}
	if state.Priority != 1 {
		t.Errorf("Priority = %d", state.Priority)
	}
	if len(state.Targets) != 1 || state.Targets[0] != "src/auth.ts" {
		t.Errorf("Targets = %v", state.Targets)
	}
	if state.Title != "Fix auth" {
		t.Errorf("Title = %q", state.Title)
	}
}

func TestFold_StatusChanges(t *testing.T) {
	creation := &Note{
		ID:        "tre-5c4a",
		Kind:      "ticket",
		Timestamp: ts("2026-03-01T10:00:00Z"),
	}
	events := []*Note{
		{
			Kind:      "event",
			Edges:     []Edge{{Type: "starts", Target: EdgeTarget{Kind: "note", Ref: "tre-5c4a"}}},
			Timestamp: ts("2026-03-01T11:00:00Z"),
		},
		{
			Kind:      "event",
			Edges:     []Edge{{Type: "closes", Target: EdgeTarget{Kind: "note", Ref: "tre-5c4a"}}},
			Timestamp: ts("2026-03-01T12:00:00Z"),
		},
	}

	state := FoldEvents(creation, events)
	if state.Status != "closed" {
		t.Errorf("Status = %q, want closed", state.Status)
	}
}

func TestFold_ReopenAfterClose(t *testing.T) {
	creation := &Note{
		ID:        "tre-5c4a",
		Kind:      "ticket",
		Timestamp: ts("2026-03-01T10:00:00Z"),
	}
	events := []*Note{
		{
			Kind:      "event",
			Edges:     []Edge{{Type: "closes", Target: EdgeTarget{Kind: "note", Ref: "tre-5c4a"}}},
			Timestamp: ts("2026-03-01T11:00:00Z"),
		},
		{
			Kind:      "event",
			Edges:     []Edge{{Type: "reopens", Target: EdgeTarget{Kind: "note", Ref: "tre-5c4a"}}},
			Timestamp: ts("2026-03-01T12:00:00Z"),
		},
	}

	state := FoldEvents(creation, events)
	if state.Status != "open" {
		t.Errorf("Status = %q, want open", state.Status)
	}
}

func TestFold_FieldChanges(t *testing.T) {
	creation := &Note{
		ID:        "tre-5c4a",
		Kind:      "ticket",
		Priority:  2,
		Assignee:  "Alice",
		Tags:      []string{"auth"},
		Timestamp: ts("2026-03-01T10:00:00Z"),
	}
	events := []*Note{
		{
			Kind:      "event",
			Field:     "priority",
			Value:     "0",
			Timestamp: ts("2026-03-01T11:00:00Z"),
		},
		{
			Kind:      "event",
			Field:     "assignee",
			Value:     "Bob",
			Timestamp: ts("2026-03-01T11:01:00Z"),
		},
		{
			Kind:      "event",
			Field:     "tags",
			Value:     "+critical",
			Timestamp: ts("2026-03-01T11:02:00Z"),
		},
		{
			Kind:      "event",
			Field:     "tags",
			Value:     "-auth",
			Timestamp: ts("2026-03-01T11:03:00Z"),
		},
	}

	state := FoldEvents(creation, events)
	if state.Priority != 0 {
		t.Errorf("Priority = %d, want 0", state.Priority)
	}
	if state.Assignee != "Bob" {
		t.Errorf("Assignee = %q, want Bob", state.Assignee)
	}
	if contains(state.Tags, "auth") {
		t.Errorf("Tags should not contain 'auth': %v", state.Tags)
	}
	if !contains(state.Tags, "critical") {
		t.Errorf("Tags should contain 'critical': %v", state.Tags)
	}
}

func TestFold_Comments(t *testing.T) {
	creation := &Note{
		ID:        "tre-5c4a",
		Kind:      "ticket",
		Timestamp: ts("2026-03-01T10:00:00Z"),
	}
	events := []*Note{
		{
			Kind:      "comment",
			Body:      "First comment",
			Timestamp: ts("2026-03-01T11:00:00Z"),
		},
		{
			Kind:      "comment",
			Body:      "Second comment",
			Timestamp: ts("2026-03-01T12:00:00Z"),
		},
	}

	state := FoldEvents(creation, events)
	if len(state.Comments) != 2 {
		t.Fatalf("Comments = %d, want 2", len(state.Comments))
	}
	if state.Comments[0].Body != "First comment" {
		t.Errorf("Comment[0] = %q", state.Comments[0].Body)
	}
}

func TestFold_DepsAndLinks(t *testing.T) {
	creation := &Note{
		ID:   "tre-5c4a",
		Kind: "ticket",
		Edges: []Edge{
			{Type: "depends-on", Target: EdgeTarget{Kind: "note", Ref: "wrn-1234"}},
			{Type: "links", Target: EdgeTarget{Kind: "note", Ref: "rev-abcd"}},
		},
		Timestamp: ts("2026-03-01T10:00:00Z"),
	}
	events := []*Note{
		{
			Kind:      "event",
			Field:     "deps",
			Value:     "+fix-9999",
			Timestamp: ts("2026-03-01T11:00:00Z"),
		},
	}

	state := FoldEvents(creation, events)
	if len(state.Deps) != 2 {
		t.Fatalf("Deps = %v, want 2", state.Deps)
	}
	if len(state.Links) != 1 || state.Links[0] != "rev-abcd" {
		t.Errorf("Links = %v", state.Links)
	}
}

func TestFold_OutOfOrderTimestamps(t *testing.T) {
	creation := &Note{
		ID:        "tre-5c4a",
		Kind:      "ticket",
		Timestamp: ts("2026-03-01T10:00:00Z"),
	}
	// Events in wrong timestamp order — fold should sort them
	events := []*Note{
		{
			Kind:      "event",
			Edges:     []Edge{{Type: "closes", Target: EdgeTarget{Kind: "note", Ref: "tre-5c4a"}}},
			Timestamp: ts("2026-03-01T12:00:00Z"),
		},
		{
			Kind:      "event",
			Edges:     []Edge{{Type: "starts", Target: EdgeTarget{Kind: "note", Ref: "tre-5c4a"}}},
			Timestamp: ts("2026-03-01T11:00:00Z"),
		},
	}

	state := FoldEvents(creation, events)
	// Starts at 11:00, closes at 12:00 → closed
	if state.Status != "closed" {
		t.Errorf("Status = %q, want closed (events should be sorted by timestamp)", state.Status)
	}
}

func TestFold_TitleFromBody(t *testing.T) {
	creation := &Note{
		ID:        "tre-5c4a",
		Kind:      "ticket",
		Body:      "# Fix the auth race condition\n\nDetails here.",
		Timestamp: ts("2026-03-01T10:00:00Z"),
	}

	state := FoldEvents(creation, nil)
	if state.Title != "Fix the auth race condition" {
		t.Errorf("Title = %q, want extracted from body heading", state.Title)
	}
}

func TestFold_UpdatedAt(t *testing.T) {
	creation := &Note{
		ID:        "tre-5c4a",
		Kind:      "ticket",
		Timestamp: ts("2026-03-01T10:00:00Z"),
	}
	events := []*Note{
		{
			Kind:      "comment",
			Body:      "hello",
			Timestamp: ts("2026-03-05T15:00:00Z"),
		},
	}

	state := FoldEvents(creation, events)
	if !state.UpdatedAt.Equal(ts("2026-03-05T15:00:00Z")) {
		t.Errorf("UpdatedAt = %v, want 2026-03-05T15:00:00Z", state.UpdatedAt)
	}
}

func TestFold_ArtifactBornClosed(t *testing.T) {
	creation := &Note{
		ID:        "rev-1234",
		Kind:      "review",
		Status:    "closed",
		Timestamp: ts("2026-03-01T10:00:00Z"),
	}

	state := FoldEvents(creation, nil)
	if state.Status != "closed" {
		t.Errorf("Status = %q, want closed (artifact born closed)", state.Status)
	}
}
