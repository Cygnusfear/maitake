package notes

import (
	"sort"
	"strings"
	"time"
)

// Index caches note state for fast queries.
// Built by scanning all notes refs, parsing, and folding.
type Index struct {
	// All creation notes keyed by ID
	CreationNotes map[string]*Note

	// Events/comments keyed by the target note ID they reference
	EventsByTarget map[string][]*Note

	// Precomputed states
	States map[string]*State

	// Lookup maps
	ByKind   map[string][]string // kind → note IDs
	ByTarget map[string][]string // file path → note IDs
	ByStatus map[string][]string // status → note IDs
	ByTag    map[string][]string // tag → note IDs

	// Build timestamp
	BuiltAt time.Time
}

// NewIndex creates an empty index.
func NewIndex() *Index {
	return &Index{
		CreationNotes:  make(map[string]*Note),
		EventsByTarget: make(map[string][]*Note),
		States:         make(map[string]*State),
		ByKind:         make(map[string][]string),
		ByTarget:       make(map[string][]string),
		ByStatus:       make(map[string][]string),
		ByTag:          make(map[string][]string),
		BuiltAt:        time.Now(),
	}
}

// Ingest adds a parsed note to the index. Call this for every note read from git.
// After ingesting all notes, call Build() to compute states and lookup maps.
func (idx *Index) Ingest(note *Note) {
	if note.ID != "" {
		// Creation note
		idx.CreationNotes[note.ID] = note
	} else {
		// Event or comment — find which note it targets
		targetID := noteTargetID(note)
		if targetID != "" {
			idx.EventsByTarget[targetID] = append(idx.EventsByTarget[targetID], note)
		}
	}
}

// Build computes all states and lookup maps from ingested notes.
func (idx *Index) Build() {
	idx.States = make(map[string]*State)
	idx.ByKind = make(map[string][]string)
	idx.ByTarget = make(map[string][]string)
	idx.ByStatus = make(map[string][]string)
	idx.ByTag = make(map[string][]string)

	for id, creation := range idx.CreationNotes {
		events := idx.EventsByTarget[id]
		state := FoldEvents(creation, events)
		idx.States[id] = state

		// Populate lookup maps
		idx.ByKind[state.Kind] = append(idx.ByKind[state.Kind], id)
		idx.ByStatus[state.Status] = append(idx.ByStatus[state.Status], id)
		for _, target := range state.Targets {
			idx.ByTarget[target] = append(idx.ByTarget[target], id)
		}
		for _, tag := range state.Tags {
			idx.ByTag[tag] = append(idx.ByTag[tag], id)
		}
	}

	idx.BuiltAt = time.Now()
}

// FindByKind returns note IDs matching the given kind.
func (idx *Index) FindByKind(kind string) []string {
	return idx.ByKind[kind]
}

// FindByStatus returns note IDs matching the given status.
func (idx *Index) FindByStatus(status string) []string {
	return idx.ByStatus[status]
}

// FindByTarget returns note IDs targeting the given path.
func (idx *Index) FindByTarget(target string) []string {
	return idx.ByTarget[target]
}

// FindByTag returns note IDs with the given tag.
func (idx *Index) FindByTag(tag string) []string {
	return idx.ByTag[tag]
}

// Query returns states matching the given filters.
func (idx *Index) Query(opts FindOptions) []*State {
	// Start with all IDs, then intersect with each filter
	candidates := idx.allIDs()

	if opts.Kind != "" {
		candidates = intersect(candidates, idx.ByKind[opts.Kind])
	}
	if opts.Status != "" {
		candidates = intersect(candidates, idx.ByStatus[opts.Status])
	}
	if opts.Tag != "" {
		candidates = intersect(candidates, idx.ByTag[opts.Tag])
	}
	if opts.Target != "" {
		candidates = intersect(candidates, idx.ByTarget[opts.Target])
	}

	var results []*State
	for _, id := range candidates {
		state := idx.States[id]
		if state == nil {
			continue
		}
		if opts.Type != "" && state.Type != opts.Type {
			continue
		}
		if opts.Assignee != "" && state.Assignee != opts.Assignee {
			continue
		}
		results = append(results, state)
	}

	return results
}

// QueryList returns sorted summaries matching the given filters.
func (idx *Index) QueryList(opts ListOptions) []StateSummary {
	states := idx.Query(opts.FindOptions)

	// Sort
	switch opts.SortBy {
	case "priority":
		sort.Slice(states, func(i, j int) bool {
			return states[i].Priority < states[j].Priority
		})
	case "updated":
		sort.Slice(states, func(i, j int) bool {
			return states[i].UpdatedAt.After(states[j].UpdatedAt)
		})
	default: // "created" or empty
		sort.Slice(states, func(i, j int) bool {
			return states[i].CreatedAt.After(states[j].CreatedAt)
		})
	}

	// Limit
	if opts.Limit > 0 && len(states) > opts.Limit {
		states = states[:opts.Limit]
	}

	summaries := make([]StateSummary, len(states))
	for i, s := range states {
		summaries[i] = ToSummary(s)
	}
	return summaries
}

// ContextForPath returns all open states targeting the given file path.
func (idx *Index) ContextForPath(path string) []*State {
	ids := idx.ByTarget[path]
	var results []*State
	for _, id := range ids {
		state := idx.States[id]
		if state != nil && state.Status != "closed" {
			results = append(results, state)
		}
	}
	return results
}

// ContextAllForPath returns all states targeting the given file path (open + closed).
func (idx *Index) ContextAllForPath(path string) []*State {
	ids := idx.ByTarget[path]
	var results []*State
	for _, id := range ids {
		if state := idx.States[id]; state != nil {
			results = append(results, state)
		}
	}
	return results
}

// ResolveID finds a note by full or partial ID.
// Returns the full ID, or empty string if not found.
// Returns error if ambiguous (multiple matches).
func (idx *Index) ResolveID(partial string) (string, error) {
	// Exact match first
	if _, ok := idx.CreationNotes[partial]; ok {
		return partial, nil
	}

	// Partial match
	var matches []string
	for id := range idx.CreationNotes {
		if strings.Contains(id, partial) {
			matches = append(matches, id)
		}
	}

	switch len(matches) {
	case 0:
		return "", nil
	case 1:
		return matches[0], nil
	default:
		return "", &AmbiguousIDError{Partial: partial, Matches: matches}
	}
}

// AmbiguousIDError is returned when a partial ID matches multiple notes.
type AmbiguousIDError struct {
	Partial string
	Matches []string
}

func (e *AmbiguousIDError) Error() string {
	return "ambiguous ID " + e.Partial + ": matches " + strings.Join(e.Matches, ", ")
}

// KindCounts returns kind usage statistics.
func (idx *Index) KindCounts() []KindCount {
	var counts []KindCount
	for kind, ids := range idx.ByKind {
		counts = append(counts, KindCount{Kind: kind, Count: len(ids)})
	}
	sort.Slice(counts, func(i, j int) bool {
		return counts[i].Count > counts[j].Count
	})
	return counts
}

// helpers

func (idx *Index) allIDs() []string {
	ids := make([]string, 0, len(idx.CreationNotes))
	for id := range idx.CreationNotes {
		ids = append(ids, id)
	}
	return ids
}

func intersect(a, b []string) []string {
	set := make(map[string]bool, len(b))
	for _, s := range b {
		set[s] = true
	}
	var result []string
	for _, s := range a {
		if set[s] {
			result = append(result, s)
		}
	}
	return result
}

// noteTargetID extracts the target note ID from an event/comment's edges.
func noteTargetID(note *Note) string {
	for _, e := range note.Edges {
		switch e.Type {
		case "closes", "reopens", "starts", "updates", "on":
			kind, ref := ParseEdgeTarget(e.Target)
			if kind == "note" {
				return ref
			}
		}
	}
	return ""
}
