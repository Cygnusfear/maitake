package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cygnusfear/maitake/pkg/git"
	"github.com/cygnusfear/maitake/pkg/migrate"
	"github.com/cygnusfear/maitake/pkg/notes"
)

func setupMigrateRepo(t *testing.T) (string, notes.Engine) {
	t.Helper()
	dir := setupTestRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)
	return dir, engine
}

func writeTicketFile(t *testing.T, dir, filename, content string) {
	t.Helper()
	ticketsDir := filepath.Join(dir, ".tickets")
	os.MkdirAll(ticketsDir, 0755)
	os.WriteFile(filepath.Join(ticketsDir, filename), []byte(content), 0644)
}

// === BASIC MIGRATION ===

func TestMigrate_SimpleTicket(t *testing.T) {
	dir, engine := setupMigrateRepo(t)
	writeTicketFile(t, dir, "tre-5c4a.md", `---
id: tre-5c4a
status: open
deps: []
links: []
created: 2026-03-01T10:00:00Z
type: task
priority: 1
assignee: Alice
tags: [auth, backend]
---
# Fix auth race condition

The token refresh has a race condition between concurrent requests.
`)

	report, err := migrate.Run(engine, migrate.Options{
		TicketsDir: filepath.Join(dir, ".tickets"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Migrated != 1 {
		t.Fatalf("Migrated = %d, want 1", report.Migrated)
	}
	if report.Errors != 0 {
		t.Fatalf("Errors = %d", report.Errors)
	}

	// Verify through engine — ID should be the original tk ID
	state, err := engine.Fold("tre-5c4a")
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "open" {
		t.Errorf("Status = %q, want open", state.Status)
	}
	if state.Priority != 1 {
		t.Errorf("Priority = %d", state.Priority)
	}
	if state.Assignee != "Alice" {
		t.Errorf("Assignee = %q", state.Assignee)
	}
	if !containsStr(state.Tags, "auth") || !containsStr(state.Tags, "backend") {
		t.Errorf("Tags = %v", state.Tags)
	}
}

func TestMigrate_ClosedTicket(t *testing.T) {
	dir, engine := setupMigrateRepo(t)
	writeTicketFile(t, dir, "tre-1234.md", `---
id: tre-1234
status: closed
deps: []
links: []
created: 2026-02-15T10:00:00Z
type: bug
priority: 2
tags: [fix]
---
# Fixed bug

Was broken, now fixed.
`)

	report, _ := migrate.Run(engine, migrate.Options{TicketsDir: filepath.Join(dir, ".tickets")})
	if report.Migrated != 1 {
		t.Fatal("migration failed")
	}

	state, _ := engine.Fold(report.Results[0].ID)
	if state.Status != "closed" {
		t.Errorf("Status = %q, want closed", state.Status)
	}
	if state.Type != "bug" {
		t.Errorf("Type = %q, want bug", state.Type)
	}
}

func TestMigrate_InProgressTicket(t *testing.T) {
	dir, engine := setupMigrateRepo(t)
	writeTicketFile(t, dir, "tre-abcd.md", `---
id: tre-abcd
status: in_progress
deps: []
links: []
created: 2026-03-01T10:00:00Z
type: feature
priority: 0
assignee: Bob
tags: [wip]
---
# Feature in progress

Working on it.
`)

	report, _ := migrate.Run(engine, migrate.Options{TicketsDir: filepath.Join(dir, ".tickets")})
	state, _ := engine.Fold(report.Results[0].ID)
	if state.Status != "in_progress" {
		t.Errorf("Status = %q, want in_progress", state.Status)
	}
}

// === COMMENTS ===

func TestMigrate_WithComments(t *testing.T) {
	dir, engine := setupMigrateRepo(t)
	writeTicketFile(t, dir, "tre-note.md", `---
id: tre-note
status: open
deps: []
links: []
created: 2026-03-01T10:00:00Z
type: task
priority: 2
---
# Ticket with comments

Description here.

## Notes

**2026-03-01T11:00:00Z**

First comment — found the root cause.

**2026-03-01T12:00:00Z**

Second comment — fix deployed.
`)

	report, _ := migrate.Run(engine, migrate.Options{TicketsDir: filepath.Join(dir, ".tickets")})
	if report.Results[0].Comments != 2 {
		t.Fatalf("Comments = %d, want 2", report.Results[0].Comments)
	}

	state, _ := engine.Fold(report.Results[0].ID)
	if len(state.Comments) != 2 {
		t.Fatalf("State.Comments = %d, want 2", len(state.Comments))
	}
	if !strings.Contains(state.Comments[0].Body, "root cause") {
		t.Errorf("Comment[0] = %q", state.Comments[0].Body)
	}
	if !strings.Contains(state.Comments[1].Body, "fix deployed") {
		t.Errorf("Comment[1] = %q", state.Comments[1].Body)
	}
}

// === DEPS AND LINKS ===

func TestMigrate_WithDepsAndLinks(t *testing.T) {
	dir, engine := setupMigrateRepo(t)
	writeTicketFile(t, dir, "tre-parent.md", `---
id: tre-parent
status: open
deps: [tre-child]
links: [tre-related]
created: 2026-03-01T10:00:00Z
type: epic
priority: 1
---
# Parent epic

Has deps and links.
`)

	report, _ := migrate.Run(engine, migrate.Options{TicketsDir: filepath.Join(dir, ".tickets")})
	state, _ := engine.Fold(report.Results[0].ID)

	if len(state.Deps) != 1 || state.Deps[0] != "tre-child" {
		t.Errorf("Deps = %v, want [tre-child]", state.Deps)
	}
	if len(state.Links) != 1 || state.Links[0] != "tre-related" {
		t.Errorf("Links = %v, want [tre-related]", state.Links)
	}
}

func TestMigrate_WithParent(t *testing.T) {
	dir, engine := setupMigrateRepo(t)
	writeTicketFile(t, dir, "tre-sub.md", `---
id: tre-sub
status: open
deps: []
links: []
parent: tre-epic
created: 2026-03-01T10:00:00Z
type: task
priority: 2
---
# Subtask

Child of an epic.
`)

	report, _ := migrate.Run(engine, migrate.Options{TicketsDir: filepath.Join(dir, ".tickets")})
	state, _ := engine.Fold(report.Results[0].ID)
	if state.ParentID != "tre-epic" {
		t.Errorf("ParentID = %q, want tre-epic", state.ParentID)
	}
}

// === EXTERNAL REFS ===

func TestMigrate_WithForgejoIssue(t *testing.T) {
	dir, engine := setupMigrateRepo(t)
	writeTicketFile(t, dir, "tre-ext.md", `---
id: tre-ext
status: closed
deps: []
links: []
created: 2026-02-15T10:00:00Z
type: bug
priority: 2
forgejo-issue: 5740
---
# Bug with forgejo ref

External reference preserved.
`)

	report, _ := migrate.Run(engine, migrate.Options{TicketsDir: filepath.Join(dir, ".tickets")})
	note, _ := engine.Get(report.Results[0].ID)

	hasExtRef := false
	for _, edge := range note.Edges {
		if edge.Type == "external-ref" && edge.Target.Kind == "forgejo" && edge.Target.Ref == "5740" {
			hasExtRef = true
		}
	}
	if !hasExtRef {
		t.Errorf("Missing forgejo external ref edge. Edges: %+v", note.Edges)
	}
}

// === BODY SECTIONS ===

func TestMigrate_PreservesGoalAndAC(t *testing.T) {
	dir, engine := setupMigrateRepo(t)
	writeTicketFile(t, dir, "tre-rich.md", `---
id: tre-rich
status: open
deps: []
links: []
created: 2026-03-01T10:00:00Z
type: task
priority: 2
---
# Rich ticket

Description text.

## Goal
Build the thing properly.

## Acceptance Criteria
- [ ] Tests pass
- [ ] No regressions

## Verification
- [ ] Run tests
`)

	report, _ := migrate.Run(engine, migrate.Options{TicketsDir: filepath.Join(dir, ".tickets")})
	state, _ := engine.Fold(report.Results[0].ID)

	if !strings.Contains(state.Body, "## Goal") {
		t.Errorf("Body should contain Goal section:\n%s", state.Body)
	}
	if !strings.Contains(state.Body, "## Acceptance Criteria") {
		t.Errorf("Body should contain AC section:\n%s", state.Body)
	}
}

// === MULTI-LINE TAGS ===

func TestMigrate_MultiLineTags(t *testing.T) {
	dir, engine := setupMigrateRepo(t)
	writeTicketFile(t, dir, "tre-yaml.md", `---
id: tre-yaml
status: closed
deps: []
links: []
created: 2026-03-01T10:00:00Z
type: task
priority: 2
tags:
  - research
  - oracle
  - architecture
---
# Multi-line tags

Tags in YAML list format.
`)

	report, _ := migrate.Run(engine, migrate.Options{TicketsDir: filepath.Join(dir, ".tickets")})
	state, _ := engine.Fold(report.Results[0].ID)

	if !containsStr(state.Tags, "research") || !containsStr(state.Tags, "oracle") || !containsStr(state.Tags, "architecture") {
		t.Errorf("Tags = %v, want [research, oracle, architecture]", state.Tags)
	}
}

// === DRY RUN ===

func TestMigrate_DryRun(t *testing.T) {
	dir, engine := setupMigrateRepo(t)
	writeTicketFile(t, dir, "tre-dry.md", `---
id: tre-dry
status: open
deps: []
links: []
created: 2026-03-01T10:00:00Z
type: task
priority: 2
---
# Dry run test

Should not be written.

## Notes

**2026-03-01T11:00:00Z**

A comment.
`)

	report, _ := migrate.Run(engine, migrate.Options{
		TicketsDir: filepath.Join(dir, ".tickets"),
		DryRun:     true,
	})
	if report.Migrated != 1 {
		t.Fatalf("Migrated = %d", report.Migrated)
	}
	if report.Results[0].Comments != 1 {
		t.Errorf("DryRun should count comments: %d", report.Results[0].Comments)
	}

	// Engine should have nothing
	summaries, _ := engine.List(notes.ListOptions{})
	if len(summaries) != 0 {
		t.Errorf("DryRun should not write: %d notes found", len(summaries))
	}
}

// === MULTIPLE FILES ===

func TestMigrate_MultipleFiles(t *testing.T) {
	dir, engine := setupMigrateRepo(t)
	for i := 0; i < 5; i++ {
		status := "open"
		if i%2 == 0 {
			status = "closed"
		}
		writeTicketFile(t, dir, fmt.Sprintf("tre-%04d.md", i), fmt.Sprintf(`---
id: tre-%04d
status: %s
deps: []
links: []
created: 2026-03-01T10:00:00Z
type: task
priority: %d
---
# Ticket %d

Body %d.
`, i, status, i, i, i))
	}

	report, _ := migrate.Run(engine, migrate.Options{TicketsDir: filepath.Join(dir, ".tickets")})
	if report.Migrated != 5 {
		t.Fatalf("Migrated = %d, want 5", report.Migrated)
	}
	if report.Errors != 0 {
		t.Fatalf("Errors = %d", report.Errors)
	}

	summaries, _ := engine.List(notes.ListOptions{})
	if len(summaries) != 5 {
		t.Errorf("List = %d, want 5", len(summaries))
	}
}

// === REAL RAMBOARD DATA ===

func TestMigrate_RamboardTickets(t *testing.T) {
	ticketsDir := "/Users/alexander/Projects/ramboard/.tickets"
	if _, err := os.Stat(ticketsDir); err != nil {
		t.Skip("ramboard .tickets/ not available")
	}

	dir := setupTestRepo(t)
	repo, _ := git.NewGitRepo(dir)
	engine, _ := notes.NewEngine(repo)

	report, err := migrate.Run(engine, migrate.Options{TicketsDir: ticketsDir})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Ramboard migration: %d total, %d migrated, %d skipped, %d errors",
		report.Total, report.Migrated, report.Skipped, report.Errors)

	for _, r := range report.Results {
		if r.Error != nil {
			t.Logf("  ERROR %s: %v", r.ID, r.Error)
		}
		if r.Skipped {
			t.Logf("  SKIP: %s (no YAML frontmatter)", r.Title)
		}
	}

	if report.Errors > 0 {
		t.Errorf("%d migration errors (skips are OK)", report.Errors)
	}

	// Verify we can list and fold everything
	summaries, _ := engine.List(notes.ListOptions{})
	t.Logf("  %d notes in engine after migration", len(summaries))

	if len(summaries) < report.Migrated {
		t.Errorf("List = %d, Migrated = %d", len(summaries), report.Migrated)
	}

	// Spot check: fold each one
	var foldErrors int
	for _, s := range summaries {
		_, err := engine.Fold(s.ID)
		if err != nil {
			foldErrors++
			t.Logf("  FOLD ERROR %s: %v", s.ID, err)
		}
	}
	if foldErrors > 0 {
		t.Errorf("%d fold errors", foldErrors)
	}
}

// === TIMESTAMP AND BRANCH REGRESSION ===

func TestMigrate_TimestampsPopulated(t *testing.T) {
	dir, engine := setupMigrateRepo(t)
	writeTicketFile(t, dir, "tre-ts.md", `---
id: tre-ts
status: open
deps: []
links: []
created: 2026-03-15T14:30:00Z
type: task
priority: 2
---
# Timestamp test

Should have real timestamps.
`)

	migrate.Run(engine, migrate.Options{TicketsDir: filepath.Join(dir, ".tickets")})

	state, err := engine.Fold("tre-ts")
	if err != nil {
		t.Fatal(err)
	}

	// CreatedAt must be the ORIGINAL timestamp, not the migration time
	if state.CreatedAt.IsZero() {
		t.Fatal("CreatedAt is zero — timestamp not hydrated during migration")
	}
	want, _ := time.Parse(time.RFC3339, "2026-03-15T14:30:00Z")
	if !state.CreatedAt.Equal(want) {
		t.Errorf("CreatedAt = %v, want %v (original tk timestamp)", state.CreatedAt, want)
	}
	if state.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero — timestamp not hydrated during migration")
	}
}

func TestMigrate_ClosedTicket_TimestampsPopulated(t *testing.T) {
	dir, engine := setupMigrateRepo(t)
	writeTicketFile(t, dir, "tre-cts.md", `---
id: tre-cts
status: closed
deps: []
links: []
created: 2026-02-01T10:00:00Z
type: bug
priority: 1
---
# Closed with timestamps

Should have real timestamps even when closed.
`)

	migrate.Run(engine, migrate.Options{TicketsDir: filepath.Join(dir, ".tickets")})

	state, err := engine.Fold("tre-cts")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := time.Parse(time.RFC3339, "2026-02-01T10:00:00Z")
	if !state.CreatedAt.Equal(want) {
		t.Errorf("CreatedAt = %v, want %v (original tk timestamp)", state.CreatedAt, want)
	}
	if state.Status != "closed" {
		t.Errorf("Status = %q, want closed", state.Status)
	}
}

func TestMigrate_WithComments_TimestampsPopulated(t *testing.T) {
	dir, engine := setupMigrateRepo(t)
	writeTicketFile(t, dir, "tre-cmt.md", `---
id: tre-cmt
status: open
deps: []
links: []
created: 2026-03-01T10:00:00Z
type: task
priority: 2
---
# Comments timestamp test

## Notes

**2026-03-05T12:00:00Z**

First comment.

**2026-03-10T15:00:00Z**

Second comment.
`)

	migrate.Run(engine, migrate.Options{TicketsDir: filepath.Join(dir, ".tickets")})

	state, err := engine.Fold("tre-cmt")
	if err != nil {
		t.Fatal(err)
	}
	wantCreated, _ := time.Parse(time.RFC3339, "2026-03-01T10:00:00Z")
	if !state.CreatedAt.Equal(wantCreated) {
		t.Errorf("CreatedAt = %v, want %v", state.CreatedAt, wantCreated)
	}

	// UpdatedAt should be the last comment's timestamp
	wantUpdated, _ := time.Parse(time.RFC3339, "2026-03-10T15:00:00Z")
	if !state.UpdatedAt.Equal(wantUpdated) {
		t.Errorf("UpdatedAt = %v, want %v (last comment timestamp)", state.UpdatedAt, wantUpdated)
	}

	// Verify individual comment timestamps
	if len(state.Comments) != 2 {
		t.Fatalf("Comments = %d, want 2", len(state.Comments))
	}
	wantComment1, _ := time.Parse(time.RFC3339, "2026-03-05T12:00:00Z")
	if !state.Comments[0].Time.Equal(wantComment1) {
		t.Errorf("Comment[0].Time = %v, want %v", state.Comments[0].Time, wantComment1)
	}
	wantComment2, _ := time.Parse(time.RFC3339, "2026-03-10T15:00:00Z")
	if !state.Comments[1].Time.Equal(wantComment2) {
		t.Errorf("Comment[1].Time = %v, want %v", state.Comments[1].Time, wantComment2)
	}
}

func TestMigrate_BranchStamped(t *testing.T) {
	dir, engine := setupMigrateRepo(t)
	writeTicketFile(t, dir, "tre-br.md", `---
id: tre-br
status: open
deps: []
links: []
created: 2026-03-01T10:00:00Z
type: task
priority: 2
---
# Branch stamp test

Should have the current branch stamped.
`)

	migrate.Run(engine, migrate.Options{TicketsDir: filepath.Join(dir, ".tickets")})

	// The note should have the current branch stamped
	note, err := engine.Get("tre-br")
	if err != nil {
		t.Fatal(err)
	}
	// In a test repo the branch is usually "main" or "master"
	// The important thing is it's not empty
	if note.Branch == "" {
		t.Error("Branch should be stamped on migrated note, got empty")
	}
}

func TestMigrate_DepsLinksInSummary(t *testing.T) {
	dir, engine := setupMigrateRepo(t)
	writeTicketFile(t, dir, "tre-a.md", `---
id: tre-a
status: open
deps: [tre-b]
links: [tre-c]
created: 2026-03-01T10:00:00Z
type: task
priority: 1
---
# Has deps and links
`)
	writeTicketFile(t, dir, "tre-b.md", `---
id: tre-b
status: open
deps: []
links: []
created: 2026-03-01T10:00:00Z
type: task
priority: 2
---
# Dep target
`)
	writeTicketFile(t, dir, "tre-c.md", `---
id: tre-c
status: open
deps: []
links: []
created: 2026-03-01T10:00:00Z
type: task
priority: 2
---
# Link target
`)

	migrate.Run(engine, migrate.Options{TicketsDir: filepath.Join(dir, ".tickets")})

	summaries, _ := engine.List(notes.ListOptions{})
	var found bool
	for _, s := range summaries {
		if s.ID == "tre-a" {
			found = true
			if len(s.Deps) != 1 || s.Deps[0] != "tre-b" {
				t.Errorf("summary.Deps = %v, want [tre-b]", s.Deps)
			}
			if len(s.Links) != 1 || s.Links[0] != "tre-c" {
				t.Errorf("summary.Links = %v, want [tre-c]", s.Links)
			}
		}
	}
	if !found {
		t.Error("tre-a not found in list")
	}
}

// === HELPERS ===

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}


