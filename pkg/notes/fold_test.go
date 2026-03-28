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
		Time: ts("2026-03-01T10:00:00Z"),
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
		Time: ts("2026-03-01T10:00:00Z"),
	}
	events := []*Note{
		{
			Kind:      "event",
			Edges:     []Edge{{Type: "starts", Target: EdgeTarget{Kind: "note", Ref: "tre-5c4a"}}},
			Time: ts("2026-03-01T11:00:00Z"),
		},
		{
			Kind:      "event",
			Edges:     []Edge{{Type: "closes", Target: EdgeTarget{Kind: "note", Ref: "tre-5c4a"}}},
			Time: ts("2026-03-01T12:00:00Z"),
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
		Time: ts("2026-03-01T10:00:00Z"),
	}
	events := []*Note{
		{
			Kind:      "event",
			Edges:     []Edge{{Type: "closes", Target: EdgeTarget{Kind: "note", Ref: "tre-5c4a"}}},
			Time: ts("2026-03-01T11:00:00Z"),
		},
		{
			Kind:      "event",
			Edges:     []Edge{{Type: "reopens", Target: EdgeTarget{Kind: "note", Ref: "tre-5c4a"}}},
			Time: ts("2026-03-01T12:00:00Z"),
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
		Time: ts("2026-03-01T10:00:00Z"),
	}
	events := []*Note{
		{
			Kind:      "event",
			Field:     "priority",
			Value:     "0",
			Time: ts("2026-03-01T11:00:00Z"),
		},
		{
			Kind:      "event",
			Field:     "assignee",
			Value:     "Bob",
			Time: ts("2026-03-01T11:01:00Z"),
		},
		{
			Kind:      "event",
			Field:     "tags",
			Value:     "+critical",
			Time: ts("2026-03-01T11:02:00Z"),
		},
		{
			Kind:      "event",
			Field:     "tags",
			Value:     "-auth",
			Time: ts("2026-03-01T11:03:00Z"),
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
		Time: ts("2026-03-01T10:00:00Z"),
	}
	events := []*Note{
		{
			Kind:      "comment",
			Body:      "First comment",
			Time: ts("2026-03-01T11:00:00Z"),
		},
		{
			Kind:      "comment",
			Body:      "Second comment",
			Time: ts("2026-03-01T12:00:00Z"),
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
		Time: ts("2026-03-01T10:00:00Z"),
	}
	events := []*Note{
		{
			Kind:      "event",
			Field:     "deps",
			Value:     "+fix-9999",
			Time: ts("2026-03-01T11:00:00Z"),
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
		Time: ts("2026-03-01T10:00:00Z"),
	}
	// Events in wrong timestamp order — fold should sort them
	events := []*Note{
		{
			Kind:      "event",
			Edges:     []Edge{{Type: "closes", Target: EdgeTarget{Kind: "note", Ref: "tre-5c4a"}}},
			Time: ts("2026-03-01T12:00:00Z"),
		},
		{
			Kind:      "event",
			Edges:     []Edge{{Type: "starts", Target: EdgeTarget{Kind: "note", Ref: "tre-5c4a"}}},
			Time: ts("2026-03-01T11:00:00Z"),
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
		Time: ts("2026-03-01T10:00:00Z"),
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
		Time: ts("2026-03-01T10:00:00Z"),
	}
	events := []*Note{
		{
			Kind:      "comment",
			Body:      "hello",
			Time: ts("2026-03-05T15:00:00Z"),
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
		Time: ts("2026-03-01T10:00:00Z"),
	}

	state := FoldEvents(creation, nil)
	if state.Status != "closed" {
		t.Errorf("Status = %q, want closed (artifact born closed)", state.Status)
	}
}

// === TIMESTAMP REGRESSION TESTS ===

func TestFold_CreatedAtFromTimestamp(t *testing.T) {
	creation := &Note{
		ID:        "t-ts",
		Kind:      "ticket",
		Timestamp: "2026-03-15T14:30:00Z",
		Time:      ts("2026-03-15T14:30:00Z"),
	}

	state := FoldEvents(creation, nil)
	want := ts("2026-03-15T14:30:00Z")
	if !state.CreatedAt.Equal(want) {
		t.Errorf("CreatedAt = %v, want %v", state.CreatedAt, want)
	}
	if !state.UpdatedAt.Equal(want) {
		t.Errorf("UpdatedAt = %v, want %v (no events, should match created)", state.UpdatedAt, want)
	}
}

func TestFold_UpdatedAtFromEvents(t *testing.T) {
	creation := &Note{
		ID:   "t-upd",
		Kind: "ticket",
		Time: ts("2026-03-01T10:00:00Z"),
	}
	events := []*Note{
		{
			Kind: "comment",
			Body: "first",
			Time: ts("2026-03-05T12:00:00Z"),
		},
		{
			Kind: "comment",
			Body: "second",
			Time: ts("2026-03-10T15:00:00Z"),
		},
	}

	state := FoldEvents(creation, events)
	if !state.CreatedAt.Equal(ts("2026-03-01T10:00:00Z")) {
		t.Errorf("CreatedAt = %v, want 2026-03-01", state.CreatedAt)
	}
	if !state.UpdatedAt.Equal(ts("2026-03-10T15:00:00Z")) {
		t.Errorf("UpdatedAt = %v, want 2026-03-10 (latest event)", state.UpdatedAt)
	}
}

func TestFold_ZeroTimeCreation(t *testing.T) {
	// Old notes without timestamps — Time is zero
	creation := &Note{
		ID:   "t-zero",
		Kind: "ticket",
		// Time is zero value
	}

	state := FoldEvents(creation, nil)
	if !state.CreatedAt.IsZero() {
		t.Errorf("CreatedAt should be zero for notes without timestamp, got %v", state.CreatedAt)
	}
}

// === BRANCH REGRESSION TESTS ===

func TestFold_BranchFromCreation(t *testing.T) {
	creation := &Note{
		ID:     "t-branch",
		Kind:   "ticket",
		Branch: "feature/auth",
		Time:   ts("2026-03-01T10:00:00Z"),
	}

	state := FoldEvents(creation, nil)
	if state.Branch != "feature/auth" {
		t.Errorf("Branch = %q, want feature/auth", state.Branch)
	}
}

func TestFold_NoBranch(t *testing.T) {
	creation := &Note{
		ID:   "t-nobr",
		Kind: "ticket",
		Time: ts("2026-03-01T10:00:00Z"),
	}

	state := FoldEvents(creation, nil)
	if state.Branch != "" {
		t.Errorf("Branch = %q, want empty", state.Branch)
	}
}

// === TOSUMMARY REGRESSION TESTS ===

func TestToSummary_IncludesDepsLinksAssigneeBranch(t *testing.T) {
	state := &State{
		ID:       "t-sum",
		Kind:     "ticket",
		Status:   "open",
		Priority: 1,
		Title:    "Test",
		Tags:     []string{"auth"},
		Targets:  []string{"src/auth.ts"},
		Deps:     []string{"dep-1", "dep-2"},
		Links:    []string{"link-1"},
		Assignee: "Alice",
		Branch:   "feature/auth",
		CreatedAt: ts("2026-03-01T10:00:00Z"),
		UpdatedAt: ts("2026-03-05T15:00:00Z"),
	}

	summary := ToSummary(state)
	if len(summary.Deps) != 2 {
		t.Errorf("summary.Deps = %v, want [dep-1 dep-2]", summary.Deps)
	}
	if len(summary.Links) != 1 {
		t.Errorf("summary.Links = %v, want [link-1]", summary.Links)
	}
	if summary.Assignee != "Alice" {
		t.Errorf("summary.Assignee = %q, want Alice", summary.Assignee)
	}
	if summary.Branch != "feature/auth" {
		t.Errorf("summary.Branch = %q, want feature/auth", summary.Branch)
	}
}
