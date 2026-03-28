// Package migrate converts tk shadow-branch tickets (.tickets/*.md with YAML frontmatter)
// to maitake git notes (JSON event streams).
package migrate

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cygnusfear/maitake/pkg/notes"
)

// Result describes the outcome of migrating one ticket.
type Result struct {
	ID       string
	Title    string
	Status   string
	Comments int
	Skipped  bool   // true if the file was skipped (not an error, just incompatible format)
	Error    error
}

// Report describes the outcome of a full migration run.
type Report struct {
	Total     int
	Migrated  int
	Skipped   int
	Errors    int
	Results   []Result
}

// Options controls migration behavior.
type Options struct {
	TicketsDir string // path to .tickets/ directory
	DryRun     bool   // if true, parse but don't write
}

// Run migrates all .tickets/*.md files into the given notes engine.
func Run(engine notes.Engine, opts Options) (*Report, error) {
	files, err := filepath.Glob(filepath.Join(opts.TicketsDir, "*.md"))
	if err != nil {
		return nil, fmt.Errorf("listing tickets: %w", err)
	}

	report := &Report{Total: len(files)}

	for _, file := range files {
		result := migrateOne(engine, file, opts.DryRun)
		report.Results = append(report.Results, result)
		if result.Skipped {
			report.Skipped++
		} else if result.Error != nil {
			report.Errors++
		} else {
			report.Migrated++
		}
	}

	return report, nil
}

func migrateOne(engine notes.Engine, file string, dryRun bool) Result {
	data, err := os.ReadFile(file)
	if err != nil {
		return Result{Error: fmt.Errorf("reading %s: %w", file, err)}
	}

	ticket, err := parseTkTicket(data)
	if err != nil {
		// Files without YAML frontmatter are old-format — skip, don't error
		if strings.Contains(err.Error(), "frontmatter") {
			return Result{Skipped: true, Title: filepath.Base(file)}
		}
		return Result{Error: fmt.Errorf("parsing %s: %w", file, err)}
	}

	result := Result{
		ID:     ticket.ID,
		Title:  ticket.Title,
		Status: ticket.Status,
	}

	if dryRun {
		result.Comments = len(ticket.Comments)
		return result
	}

	// Create the note — preserve the original tk ID and timestamp
	createOpts := notes.CreateOptions{
		ID:        ticket.ID,
		Kind:      "ticket",
		Title:     ticket.Title,
		Type:      ticket.Type,
		Priority:  ticket.Priority,
		Assignee:  ticket.Assignee,
		Tags:      ticket.Tags,
		Body:      ticket.Body,
		Timestamp: ticket.Created,
	}

	// Add edges for deps, links, parent
	for _, dep := range ticket.Deps {
		createOpts.Edges = append(createOpts.Edges, notes.Edge{
			Type:   "depends-on",
			Target: notes.EdgeTarget{Kind: "note", Ref: dep},
		})
	}
	for _, link := range ticket.Links {
		createOpts.Edges = append(createOpts.Edges, notes.Edge{
			Type:   "links",
			Target: notes.EdgeTarget{Kind: "note", Ref: link},
		})
	}
	if ticket.Parent != "" {
		createOpts.Edges = append(createOpts.Edges, notes.Edge{
			Type:   "part-of",
			Target: notes.EdgeTarget{Kind: "note", Ref: ticket.Parent},
		})
	}

	// Preserve extra fields
	if ticket.ExternalRef != "" {
		createOpts.Edges = append(createOpts.Edges, notes.Edge{
			Type:   "external-ref",
			Target: notes.EdgeTarget{Kind: "forgejo", Ref: ticket.ExternalRef},
		})
	}

	note, err := engine.Create(createOpts)
	if err != nil {
		result.Error = fmt.Errorf("creating note for %s: %w", ticket.ID, err)
		return result
	}
	result.ID = note.ID // same as ticket.ID — preserved from tk

	// Set status if not default open
	if ticket.Status == "in_progress" {
		engine.Append(notes.AppendOptions{
			TargetID: note.ID,
			Kind:     "event",
			Field:    "status",
			Value:    "in_progress",
		})
	} else if ticket.Status == "closed" {
		engine.Append(notes.AppendOptions{
			TargetID: note.ID,
			Kind:     "event",
			Field:    "status",
			Value:    "closed",
		})
	}

	// Migrate comments — preserve original timestamps
	for _, comment := range ticket.Comments {
		appendOpts := notes.AppendOptions{
			TargetID: note.ID,
			Kind:     "comment",
			Body:     comment.Body,
		}
		if comment.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339, comment.Timestamp); err == nil {
				appendOpts.Timestamp = t
			}
		}
		engine.Append(appendOpts)
		result.Comments++
	}

	return result
}

// tkTicket is the parsed form of a .tickets/*.md file.
type tkTicket struct {
	ID          string
	Status      string
	Type        string
	Priority    int
	Assignee    string
	Tags        []string
	Deps        []string
	Links       []string
	Parent      string
	ExternalRef string
	Created     time.Time
	Title       string
	Body        string
	Comments    []tkComment
}

type tkComment struct {
	Timestamp string
	Body      string
}

// parseTkTicket parses a tk-style markdown ticket with YAML frontmatter.
func parseTkTicket(data []byte) (*tkTicket, error) {
	lines := strings.Split(string(data), "\n")

	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return nil, fmt.Errorf("missing YAML frontmatter")
	}

	ticket := &tkTicket{Status: "open", Type: "task", Priority: 2}

	// Parse frontmatter
	fmEnd := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			fmEnd = i
			break
		}
	}
	if fmEnd < 0 {
		return nil, fmt.Errorf("unclosed YAML frontmatter")
	}

	// Collect frontmatter lines, handling multi-line YAML lists
	fmLines := lines[1:fmEnd]
	fields := collectYAMLFields(fmLines)

	for key, val := range fields {
		switch key {
		case "id":
			ticket.ID = val
		case "status":
			ticket.Status = val
		case "type":
			ticket.Type = val
		case "priority":
			fmt.Sscanf(val, "%d", &ticket.Priority)
		case "assignee":
			ticket.Assignee = val
		case "tags":
			ticket.Tags = parseYAMLListOrInline(val)
		case "deps":
			ticket.Deps = parseYAMLListOrInline(val)
		case "links":
			ticket.Links = parseYAMLListOrInline(val)
		case "parent":
			ticket.Parent = val
		case "forgejo-issue":
			ticket.ExternalRef = val
		case "external-ref":
			ticket.ExternalRef = val
		case "created":
			ticket.Created, _ = time.Parse(time.RFC3339, val)
		}
	}

	// Parse body after frontmatter
	bodyLines := lines[fmEnd+1:]
	ticket.Title, ticket.Body, ticket.Comments = parseBody(bodyLines)

	return ticket, nil
}

// collectYAMLFields handles both single-line and multi-line YAML values.
// Multi-line lists look like:
//
//	tags:
//	  - research
//	  - oracle
//
// Single-line lists look like:
//
//	tags: [research, oracle]
func collectYAMLFields(lines []string) map[string]string {
	fields := make(map[string]string)
	var currentKey string

	for _, line := range lines {
		// Indented list item: "  - value"
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") && currentKey != "" {
			item := strings.TrimPrefix(trimmed, "- ")
			existing := fields[currentKey]
			if existing == "" {
				fields[currentKey] = item
			} else {
				fields[currentKey] = existing + "," + item
			}
			continue
		}

		// Key-value line
		idx := strings.Index(line, ": ")
		if idx < 0 {
			// Key with no value on same line (multi-line list follows)
			if strings.HasSuffix(strings.TrimSpace(line), ":") {
				currentKey = strings.TrimSuffix(strings.TrimSpace(line), ":")
				fields[currentKey] = ""
			}
			continue
		}

		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+2:])
		currentKey = key
		fields[key] = val
	}

	return fields
}

// parseYAMLListOrInline parses both inline [a, b] and comma-joined "a,b" formats.
func parseYAMLListOrInline(val string) []string {
	val = strings.TrimSpace(val)
	if val == "" || val == "[]" {
		return nil
	}

	// Inline format: [a, b, c]
	if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
		val = val[1 : len(val)-1]
	}

	var items []string
	for _, item := range strings.Split(val, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}

func parseBody(lines []string) (title, body string, comments []tkComment) {
	// Find title (first # heading)
	bodyStart := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			title = strings.TrimPrefix(trimmed, "# ")
			bodyStart = i + 1
			break
		}
		if trimmed != "" {
			bodyStart = i
			break
		}
	}

	// Find ## Notes section
	notesStart := -1
	for i := bodyStart; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "## Notes" {
			notesStart = i
			break
		}
	}

	// Body is everything between title and ## Notes (or end)
	bodyEnd := len(lines)
	if notesStart >= 0 {
		bodyEnd = notesStart
	}
	if bodyStart < bodyEnd {
		body = strings.TrimSpace(strings.Join(lines[bodyStart:bodyEnd], "\n"))
	}

	// Parse comments from ## Notes section
	if notesStart >= 0 {
		comments = parseComments(lines[notesStart+1:])
	}

	return
}

func parseComments(lines []string) []tkComment {
	var comments []tkComment
	var current *tkComment

	scanner := bufio.NewScanner(strings.NewReader(strings.Join(lines, "\n")))
	for scanner.Scan() {
		line := scanner.Text()

		// Timestamp line: **2026-03-27T19:20:29Z**
		if strings.HasPrefix(line, "**") && strings.HasSuffix(line, "**") {
			if current != nil && strings.TrimSpace(current.Body) != "" {
				comments = append(comments, *current)
			}
			ts := strings.Trim(line, "*")
			current = &tkComment{Timestamp: strings.TrimSpace(ts)}
			continue
		}

		if current != nil {
			if current.Body != "" {
				current.Body += "\n"
			}
			current.Body += line
		}
	}

	if current != nil && strings.TrimSpace(current.Body) != "" {
		current.Body = strings.TrimSpace(current.Body)
		comments = append(comments, *current)
	}

	return comments
}
