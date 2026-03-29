package notes

import (
	"sort"
	"strings"
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
		Resolved:  creation.Resolved,
		Branch:    creation.Branch,
		CreatedAt: creation.Time,
		UpdatedAt: creation.Time,
		NoteOID:   creation.OID,
	}

	// Extract title from body if not set
	if state.Title == "" && state.Body != "" {
		state.Title = extractTitle(state.Body)
	}

	// Initial status from creation note
	if creation.Status != "" {
		state.Status = creation.Status
	}

	// Extract targets, deps, links from creation note edges
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
		return sorted[i].Time.Before(sorted[j].Time)
	})

	// Apply events
	for _, ev := range sorted {
		switch ev.Kind {
		case "event":
			// Check if this body edit targets a comment rather than the parent
			if ev.Field == "body" && applyCommentEdit(state, ev) {
				// handled — it targeted a comment
			} else {
				applyEvent(state, ev)
				if ev.Field == "body" {
					state.Revisions++
				}
			}
		case "comment":
			state.Comments = append(state.Comments, *ev)
		default:
			state.Comments = append(state.Comments, *ev)
		}
		state.Events = append(state.Events, *ev)
		if !ev.Time.IsZero() && ev.Time.After(state.UpdatedAt) {
			state.UpdatedAt = ev.Time
		}
		// Update resolved from comments
		if ev.Resolved != nil {
			state.Resolved = ev.Resolved
		}
	}

	state.Edited = state.Revisions > 0

	return state
}

// applyCommentEdit checks if a body edit event targets a comment (not the parent).
// Returns true if it found and updated a comment.
func applyCommentEdit(state *State, ev *Note) bool {
	if ev.Field != "body" {
		return false
	}
	// Find which note ID this event targets
	targetID := ""
	for _, e := range ev.Edges {
		if e.Target.Kind == "note" {
			targetID = e.Target.Ref
			break
		}
	}
	if targetID == "" || targetID == state.ID {
		return false // targets the parent note, not a comment
	}
	// Look for a comment with this ID
	for i := range state.Comments {
		if state.Comments[i].ID == targetID {
			body := ev.Value
			if ev.Body != "" {
				body = ev.Body
			}
			state.Comments[i].Body = body
			state.Comments[i].Edited = true
			state.Comments[i].Revisions++
			return true
		}
	}
	return false
}

func applyEvent(state *State, ev *Note) {
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

	if ev.Field != "" {
		value := ev.Value
		if ev.Field == "body" && ev.Body != "" {
			value = ev.Body
		}
		applyFieldChange(state, ev.Field, value)
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
	case "body":
		state.Body = value
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
		state.Tags = remove(state.Tags, value[1:])
	} else {
		state.Tags = splitTags(value)
	}
}

func applySetChange(set *[]string, value string) {
	if strings.HasPrefix(value, "+") {
		item := value[1:]
		if !contains(*set, item) {
			*set = append(*set, item)
		}
	} else if strings.HasPrefix(value, "-") {
		*set = remove(*set, value[1:])
	}
}

func extractTitle(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
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
		Deps:      s.Deps,
		Links:     s.Links,
		Assignee:  s.Assignee,
		Resolved:  s.Resolved,
		Branch:    s.Branch,
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

func splitTags(s string) []string {
	var tags []string
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags
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
