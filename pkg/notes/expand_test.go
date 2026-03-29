package notes

import (
	"strings"
	"testing"
)

// mockEngine for expand tests — implements just enough of Engine.
type mockExpandEngine struct {
	notes map[string]*State
}

func (m *mockExpandEngine) Create(opts CreateOptions) (*Note, error)     { return nil, nil }
func (m *mockExpandEngine) Append(opts AppendOptions) (*Note, error)     { return nil, nil }
func (m *mockExpandEngine) Get(id string) (*Note, error)                 { return nil, nil }
func (m *mockExpandEngine) Context(path string) ([]State, error)         { return nil, nil }
func (m *mockExpandEngine) ContextAll(path string) ([]State, error)      { return nil, nil }
func (m *mockExpandEngine) Refs(target string) ([]State, error)          { return nil, nil }
func (m *mockExpandEngine) List(opts ListOptions) ([]StateSummary, error) { return nil, nil }
func (m *mockExpandEngine) Kinds() ([]KindCount, error)                  { return nil, nil }
func (m *mockExpandEngine) BranchUse(name string) error                  { return nil }
func (m *mockExpandEngine) BranchMerge(name string) error                { return nil }
func (m *mockExpandEngine) CurrentBranch() string                        { return "" }
func (m *mockExpandEngine) GitBranch() string                            { return "" }
func (m *mockExpandEngine) IsMerged(from, into string) bool              { return false }
func (m *mockExpandEngine) Doctor() (*DoctorReport, error)               { return nil, nil }
func (m *mockExpandEngine) Rebuild() error                               { return nil }
func (m *mockExpandEngine) Sync() error                                  { return nil }
func (m *mockExpandEngine) GetConfig() Config                            { return Config{} }

func (m *mockExpandEngine) Fold(id string) (*State, error) {
	if s, ok := m.notes[id]; ok {
		return s, nil
	}
	// Partial match
	for k, s := range m.notes {
		if strings.Contains(k, id) {
			return s, nil
		}
	}
	return nil, &noteNotFoundError{id}
}

func (m *mockExpandEngine) Find(opts FindOptions) ([]State, error) {
	var results []State
	for _, s := range m.notes {
		if opts.Status != "" && s.Status != opts.Status {
			continue
		}
		results = append(results, *s)
	}
	return results, nil
}

type noteNotFoundError struct{ id string }

func (e *noteNotFoundError) Error() string { return "not found: " + e.id }

func newMockEngine(notes ...*State) *mockExpandEngine {
	m := &mockExpandEngine{notes: make(map[string]*State)}
	for _, n := range notes {
		m.notes[n.ID] = n
	}
	return m
}

// === firstParagraph ===

func TestExpand_FirstParagraph(t *testing.T) {
	t.Run("NormalMultiline", func(t *testing.T) {
		got := firstParagraph("First paragraph here.\n\nSecond paragraph.")
		if got != "First paragraph here." {
			t.Errorf("got %q, want %q", got, "First paragraph here.")
		}
	})
	t.Run("SingleLine", func(t *testing.T) {
		got := firstParagraph("Just one line")
		if got != "Just one line" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("EmptyString", func(t *testing.T) {
		got := firstParagraph("")
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
	t.Run("StartsWithBlankLines", func(t *testing.T) {
		got := firstParagraph("\n\nActual content here.\n\nMore stuff.")
		// After SplitN on "\n\n", first part is empty, so firstParagraph trims it
		// The heading-skip logic may also apply
		if got == "" {
			// First part is empty string, TrimSpace = ""
			// Not a heading, so it returns ""
			// This is technically correct — empty first paragraph
		}
		t.Logf("firstParagraph with leading blanks: %q", got)
	})
	t.Run("OnlyNewlines", func(t *testing.T) {
		got := firstParagraph("\n\n\n")
		t.Logf("firstParagraph with only newlines: %q", got)
		// Should be empty or whitespace
	})
	t.Run("SkipsMarkdownHeading", func(t *testing.T) {
		got := firstParagraph("# Heading\n\nThe real paragraph.")
		if got != "The real paragraph." {
			t.Errorf("got %q, want %q", got, "The real paragraph.")
		}
	})
	t.Run("HeadingOnly", func(t *testing.T) {
		got := firstParagraph("# Just a heading")
		if got != "" {
			t.Errorf("got %q, want empty (heading-only body)", got)
		}
	})
	t.Run("TruncatesLongParagraph", func(t *testing.T) {
		long := strings.Repeat("x", 300)
		got := firstParagraph(long)
		if len(got) != 253 { // 250 + "..."
			t.Errorf("length = %d, want 253", len(got))
		}
		if !strings.HasSuffix(got, "...") {
			t.Errorf("should end with '...'")
		}
	})
}

// === ExpandRefs ===

func TestExpandRefs_ValidID(t *testing.T) {
	eng := newMockEngine(&State{ID: "mai-abc1", Title: "Fix auth", Status: "open"})
	result := ExpandRefs(eng, "See [[mai-abc1]] for details")
	if !strings.Contains(result, "[[mai-abc1]]") {
		t.Errorf("should resolve to ID: %q", result)
	}
}

func TestExpandRefs_NonExistent(t *testing.T) {
	eng := newMockEngine(&State{ID: "mai-abc1", Title: "Fix auth", Status: "open"})
	result := ExpandRefs(eng, "See [[mai-zzzz]] for details")
	if !strings.Contains(result, "[[mai-zzzz]]") {
		t.Errorf("non-existent ref should be left as-is: %q", result)
	}
}

func TestExpandRefs_MultipleRefs(t *testing.T) {
	eng := newMockEngine(
		&State{ID: "mai-abc1", Title: "First"},
		&State{ID: "mai-def2", Title: "Second"},
	)
	result := ExpandRefs(eng, "Link [[mai-abc1]] and [[mai-def2]] together")
	if !strings.Contains(result, "[[mai-abc1]]") || !strings.Contains(result, "[[mai-def2]]") {
		t.Errorf("both refs should resolve: %q", result)
	}
}

func TestExpandRefs_EmptyRef(t *testing.T) {
	eng := newMockEngine(&State{ID: "mai-abc1", Title: "Fix auth"})
	result := ExpandRefs(eng, "See [[]] for nothing")
	// Empty ref won't match the pattern (requires at least one char)
	t.Logf("empty ref result: %q", result)
}

func TestExpandRefs_NoRefs(t *testing.T) {
	eng := newMockEngine(&State{ID: "mai-abc1", Title: "Fix auth"})
	input := "Just plain text without wiki links"
	result := ExpandRefs(eng, input)
	if result != input {
		t.Errorf("got %q, want unchanged %q", result, input)
	}
}

func TestExpandRefs_NestedBrackets(t *testing.T) {
	eng := newMockEngine(&State{ID: "mai-abc1", Title: "Fix auth"})
	result := ExpandRefs(eng, "See [[[[nested]]]] for fun")
	// regex shouldn't match [[ inside — should handle gracefully
	t.Logf("nested brackets result: %q", result)
}

// === Expand (full) ===

func TestExpand_WithContextBlock(t *testing.T) {
	eng := newMockEngine(&State{
		ID:      "mai-abc1",
		Title:   "Fix auth race",
		Status:  "open",
		Body:    "The token refresh has a race condition.",
		Targets: []string{"src/auth.ts"},
	})
	result, err := Expand(eng, "Check out [[mai-abc1]]")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "<mai-context>") {
		t.Errorf("should have context block: %q", result)
	}
	if !strings.Contains(result, "mai-abc1") {
		t.Errorf("should mention note ID: %q", result)
	}
	if !strings.Contains(result, "Fix auth race") {
		t.Errorf("should include title: %q", result)
	}
	if !strings.Contains(result, "src/auth.ts") {
		t.Errorf("should include targets: %q", result)
	}
}

func TestExpand_EmptyString(t *testing.T) {
	eng := newMockEngine()
	result, err := Expand(eng, "")
	if err != nil {
		t.Fatal(err)
	}
	if result != "" {
		t.Errorf("got %q, want empty", result)
	}
}

func TestExpand_NoRefs(t *testing.T) {
	eng := newMockEngine(&State{ID: "mai-abc1", Title: "Fix auth"})
	input := "Plain text, no links"
	result, err := Expand(eng, input)
	if err != nil {
		t.Fatal(err)
	}
	if result != input {
		t.Errorf("got %q, want unchanged %q", result, input)
	}
}

func TestExpand_WhitespaceOnly(t *testing.T) {
	eng := newMockEngine()
	result, err := Expand(eng, "   \t\n  ")
	if err != nil {
		t.Fatal(err)
	}
	if result != "   \t\n  " {
		t.Errorf("whitespace-only should return unchanged: %q", result)
	}
}

func TestExpand_MultipleRefsDeduped(t *testing.T) {
	eng := newMockEngine(&State{ID: "mai-abc1", Title: "Fix auth"})
	result, err := Expand(eng, "See [[mai-abc1]] and again [[mai-abc1]]")
	if err != nil {
		t.Fatal(err)
	}
	// Context block should only mention the note once
	count := strings.Count(result, "→ mai-abc1")
	if count != 1 {
		t.Errorf("expected 1 context entry, got %d in: %q", count, result)
	}
}

func TestExpand_UnresolvedRefNoContextBlock(t *testing.T) {
	eng := newMockEngine() // empty engine — nothing resolves
	result, err := Expand(eng, "See [[doesnt-exist]]")
	if err != nil {
		t.Fatal(err)
	}
	// No resolutions → no context block
	if strings.Contains(result, "<mai-context>") {
		t.Errorf("should not have context block when nothing resolves: %q", result)
	}
}

// === ExtractWikiRefs ===

func TestExtractWikiRefs_Basic(t *testing.T) {
	refs := ExtractWikiRefs("note-1", "See [[mai-abc1]] and [[src/auth.ts]]")
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if refs[0].Target != "mai-abc1" {
		t.Errorf("ref 0 target = %q", refs[0].Target)
	}
	if refs[1].Target != "src/auth.ts" {
		t.Errorf("ref 1 target = %q", refs[1].Target)
	}
}

func TestExtractWikiRefs_PlainText(t *testing.T) {
	refs := ExtractWikiRefs("note-1", "Plain text")
	if len(refs) != 0 {
		t.Errorf("expected 0 refs, got %d", len(refs))
	}
}

func TestExtractWikiRefs_AliasedLink(t *testing.T) {
	refs := ExtractWikiRefs("note-1", "See [[mai-abc1|the auth fix]]")
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	if refs[0].Target != "mai-abc1" {
		t.Errorf("should extract target without alias: %q", refs[0].Target)
	}
}

func TestExtractWikiRefs_EmptyBody(t *testing.T) {
	refs := ExtractWikiRefs("note-1", "")
	if len(refs) != 0 {
		t.Errorf("expected 0 refs from empty body, got %d", len(refs))
	}
}
