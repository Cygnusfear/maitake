package test

import (
	"strings"
	"testing"
)

func TestSearch_BasicQuery(t *testing.T) {
	dir := setupTestRepo(t)

	// Create tickets with distinct content
	mai(t, dir, "ticket", "fix authentication bug in login page", "-p", "1")
	mai(t, dir, "ticket", "add kubernetes deployment documentation", "-p", "2")
	mai(t, dir, "ticket", "refactor database connection pooling", "-p", "2")

	out := mai(t, dir, "search", "authentication login")
	if !strings.Contains(out, "authentication") {
		t.Errorf("expected search to find 'authentication' ticket, got:\n%s", out)
	}
}

func TestSearch_JSONOutput(t *testing.T) {
	dir := setupTestRepo(t)
	mai(t, dir, "ticket", "kubernetes deployment guide", "-p", "2")

	out := mai(t, dir, "--json", "search", "kubernetes")
	if !strings.HasPrefix(out, "[") {
		t.Errorf("expected JSON array, got:\n%s", out)
	}
	if !strings.Contains(out, "kubernetes") {
		t.Errorf("expected JSON to contain kubernetes match, got:\n%s", out)
	}
}

func TestSearch_FilterByKind(t *testing.T) {
	dir := setupTestRepo(t)

	// Create a ticket and a doc with similar content
	mai(t, dir, "ticket", "auth implementation", "-p", "2")
	mai(t, dir, "create", "auth documentation", "-k", "doc")

	out := mai(t, dir, "search", "auth", "-k", "ticket")
	if !strings.Contains(out, "auth") {
		t.Errorf("expected to find auth ticket, got:\n%s", out)
	}
	// Should NOT contain the doc
	if strings.Contains(out, "doc") && strings.Contains(out, "documentation") {
		// Only fail if it looks like the doc note showed up
		lines := strings.Split(out, "\n")
		for _, l := range lines {
			if strings.Contains(l, "documentation") {
				t.Errorf("expected -k ticket to filter out docs, but found:\n%s", l)
			}
		}
	}
}

func TestSearch_FilterByStatus(t *testing.T) {
	dir := setupTestRepo(t)

	// Create and close one ticket
	out := mai(t, dir, "ticket", "closed auth bug", "-p", "1")
	// Extract ticket ID from output
	id := extractID(t, out)
	mai(t, dir, "close", id)

	// Create an open ticket
	mai(t, dir, "ticket", "open auth feature", "-p", "2")

	searchOut := mai(t, dir, "search", "auth", "--status", "open")
	if strings.Contains(searchOut, "closed auth") {
		t.Errorf("expected --status open to exclude closed ticket, got:\n%s", searchOut)
	}
}

func TestSearch_NoMatches(t *testing.T) {
	dir := setupTestRepo(t)
	mai(t, dir, "ticket", "something unrelated", "-p", "2")

	out := mai(t, dir, "search", "xyznonexistent")
	if !strings.Contains(strings.ToLower(out), "no match") {
		t.Errorf("expected 'No matches' message, got:\n%s", out)
	}
}

func TestSearch_RankedResults(t *testing.T) {
	dir := setupTestRepo(t)

	// Create tickets — one highly relevant, one tangentially
	mai(t, dir, "ticket", "kubernetes cluster deployment automation", "-p", "2")
	mai(t, dir, "ticket", "something else entirely about cooking recipes", "-p", "2")

	out := mai(t, dir, "search", "kubernetes deployment")
	lines := strings.Split(out, "\n")
	if len(lines) == 0 {
		t.Fatal("expected at least one result line")
	}
	// First result should contain kubernetes
	if !strings.Contains(lines[0], "kubernetes") {
		t.Errorf("expected first result to be kubernetes ticket, got:\n%s", lines[0])
	}
}

// extractID pulls the note ID from mai output like "Created mai-xxxx"
func extractID(t *testing.T, output string) string {
	t.Helper()
	// Look for mai-XXXX or similar ID pattern
	for _, word := range strings.Fields(output) {
		if strings.HasPrefix(word, "mai-") {
			return word
		}
	}
	t.Fatalf("could not extract ID from: %s", output)
	return ""
}
