package notes

import (
	"sort"
	"strings"
	"time"
)

// FoldEvents computes the current State from a creation note and its events/comments.
// Events are applied in timestamp order. Scalars use last-writer-wins.
// Collections use set operations (+add, -remove) applied in order.
func FoldEvents(creation *Note, events []*Note) *State {
	state := &State{
		ID:        creation.ID,
		Kind:      creation.Kind,
		Status:    "open", // default
		Title:     creation.Title,
		Type:      creation.Type,
		Priority:  creation.Priority,
		Assignee:  creation.Assignee,
		Tags:      copyStrings(creation.Tags),
		Body:      creation.Body,
		CreatedAt: creation.Timestamp,
		UpdatedAt: creation.Timestamp,
		NoteOID:   creation.OID,
	}

	// Extract title from body if not set in headers
	if state.Title == "" && state.Body != "" {
		state.Title = extractTitle(state.Body)
	}

	// Extract initial status from creation note if set
	if creation.Status != "" {
		state.Status = creation.Status
	}

	// Extract targets and deps from creation note edges
	for _, e := range creation.Edges {
		switch e.Type {
		case "targets":
			if e.Target.Kind == "path" {
				state.Targets = append(state.Targets, e.Target.Ref)
			}
		case "depends-on":
			state.Deps = append(state.Deps, e.Target.Ref)
		case "links":
			state.Links = append(state.Links, e.Target.Ref)
		case "part-of":
			state.ParentID = e.Target.Ref
		}
	}

	// Sort events by timestamp
	sorted := make([]*Note, len(events))
	copy(sorted, events)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	// Apply events
	for _, ev := range sorted {
		switch ev.Kind {
		case "event":
			applyEvent(state, ev)
		case "comment":
			state.Comments = append(state.Comments, *ev)
		default:
			// Other kinds treated as comments
			state.Comments = append(state.Comments, *ev)
		}
		state.Events = append(state.Events, *ev)
		if !ev.Timestamp.IsZero() && ev.Timestamp.After(state.UpdatedAt) {
			state.UpdatedAt = ev.Timestamp
		}
	}

	return state
}

func applyEvent(state *State, ev *Note) {
	// Check edges for lifecycle events
	for _, e := range ev.Edges {
		switch e.Type {
		case "closes":
			state.Status = "closed"
		case "reopens":
			state.Status = "open"
		case "starts":
			state.Status = "in_progress"
		}
	}

	// Apply field changes
	if ev.Field != "" {
		applyFieldChange(state, ev.Field, ev.Value)
	}
}

func applyFieldChange(state *State, field, value string) {
	switch field {
	case "status":
		state.Status = value
	case "priority":
		if p, ok := parseInt(value); ok {
			state.Priority = p
		}
	case "assignee":
		state.Assignee = value
	case "type":
		state.Type = value
	case "title":
		state.Title = value
	case "tags":
		applyTagChange(state, value)
	case "deps":
		applySetChange(&state.Deps, value)
	case "links":
		applySetChange(&state.Links, value)
	}
}

func applyTagChange(state *State, value string) {
	if strings.HasPrefix(value, "+") {
		tag := value[1:]
		if !contains(state.Tags, tag) {
			state.Tags = append(state.Tags, tag)
		}
	} else if strings.HasPrefix(value, "-") {
		tag := value[1:]
		state.Tags = remove(state.Tags, tag)
	} else {
		// Bare value = set the entire tags list
		state.Tags = parseTags(value)
	}
}

func applySetChange(set *[]string, value string) {
	if strings.HasPrefix(value, "+") {
		item := value[1:]
		if !contains(*set, item) {
			*set = append(*set, item)
		}
	} else if strings.HasPrefix(value, "-") {
		item := value[1:]
		*set = remove(*set, item)
	}
}

// extractTitle extracts the first markdown heading from the body.
func extractTitle(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	// Fall back to first non-empty line
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			if len(line) > 80 {
				return line[:80]
			}
			return line
		}
	}
	return ""
}

// ToSummary converts a State to a StateSummary.
func ToSummary(s *State) StateSummary {
	return StateSummary{
		ID:        s.ID,
		Kind:      s.Kind,
		Status:    s.Status,
		Type:      s.Type,
		Priority:  s.Priority,
		Title:     s.Title,
		Tags:      s.Tags,
		Targets:   s.Targets,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
}

// helpers

func copyStrings(s []string) []string {
	if s == nil {
		return nil
	}
	out := make([]string, len(s))
	copy(out, s)
	return out
}

func contains(s []string, v string) bool {
	for _, item := range s {
		if item == v {
			return true
		}
	}
	return false
}

func remove(s []string, v string) []string {
	out := make([]string, 0, len(s))
	for _, item := range s {
		if item != v {
			out = append(out, item)
		}
	}
	return out
}

func parseInt(s string) (int, bool) {
	n := 0
	neg := false
	i := 0
	if len(s) > 0 && s[0] == '-' {
		neg = true
		i = 1
	}
	if i >= len(s) {
		return 0, false
	}
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	if neg {
		n = -n
	}
	return n, true
}

// Ensure time import is used
var _ = time.Time{}
