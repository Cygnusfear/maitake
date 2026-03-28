package notes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMaiFrontmatter(t *testing.T) {
	content := "---\nmai-id: doc-123\n---\n# Hello\n\nWorld.\n"
	id, body := parseMaiFrontmatter(content)
	if id != "doc-123" {
		t.Errorf("id = %q, want doc-123", id)
	}
	if !strings.HasPrefix(body, "# Hello") {
		t.Errorf("body = %q", body)
	}
}

func TestParseMaiFrontmatter_NoFrontmatter(t *testing.T) {
	content := "# Just markdown\n\nNo frontmatter here.\n"
	id, body := parseMaiFrontmatter(content)
	if id != "" {
		t.Errorf("id = %q, want empty", id)
	}
	if body != content {
		t.Errorf("body should be unchanged")
	}
}

func TestParseMaiFrontmatter_OtherFields(t *testing.T) {
	content := "---\ntitle: My Doc\nmai-id: abc-xyz\ntags: [a, b]\n---\n# Content\n"
	id, body := parseMaiFrontmatter(content)
	if id != "abc-xyz" {
		t.Errorf("id = %q", id)
	}
	if !strings.HasPrefix(body, "# Content") {
		t.Errorf("body = %q", body)
	}
}

func TestWriteDocFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")

	err := writeDocFile(path, "doc-123", "# Hello\n\nWorld.")
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.Contains(content, "mai-id: doc-123") {
		t.Errorf("missing mai-id: %s", content)
	}
	if !strings.Contains(content, "# Hello") {
		t.Errorf("missing body: %s", content)
	}

	// Round-trip
	id, body := parseMaiFrontmatter(content)
	if id != "doc-123" {
		t.Errorf("round-trip id = %q", id)
	}
	if !strings.HasPrefix(body, "# Hello") {
		t.Errorf("round-trip body = %q", body)
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Architecture Overview", "architecture-overview"},
		{"My Cool Doc!", "my-cool-doc"},
		{"  spaces  ", "spaces"},
		{"already-slugged", "already-slugged"},
		{"UPPER_CASE", "upper-case"},
	}
	for _, tt := range tests {
		got := slugify(tt.in)
		if got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTitleFromPath(t *testing.T) {
	tests := []struct{ in, want string }{
		{"docs/architecture-overview.md", "Architecture overview"},
		{"docs/my_doc.md", "My doc"},
		{"simple.md", "Simple"},
	}
	for _, tt := range tests {
		got := titleFromPath(tt.in)
		if got != tt.want {
			t.Errorf("titleFromPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestContentHash_Deterministic(t *testing.T) {
	h1 := contentHash("hello world")
	h2 := contentHash("hello world")
	h3 := contentHash("different")

	if h1 != h2 {
		t.Error("same content should produce same hash")
	}
	if h1 == h3 {
		t.Error("different content should produce different hash")
	}
}

func TestContentHash_IgnoresTrailingWhitespace(t *testing.T) {
	h1 := contentHash("hello")
	h2 := contentHash("hello  \n\n")
	if h1 != h2 {
		t.Error("should ignore trailing whitespace")
	}
}
