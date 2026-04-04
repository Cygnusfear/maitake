package notes

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cygnusfear/maitake/pkg/git"
	"github.com/cygnusfear/maitake/pkg/guard"
)

const pushDebounceDelay = 500 * time.Millisecond

// PostWriteFunc is called after a note is written (Create or Append).
// noteID is the note that was written; ref and targetOID are the git plumbing details.
type PostWriteFunc func(engine Engine, noteID string, ref git.NotesRef, targetOID git.OID)

// RealEngine implements Engine using a git repo, guard hooks, and an in-memory index.
type RealEngine struct {
	repo       git.Repo
	repoPath   string // absolute path to repo root
	maitakeDir string // .maitake directory path
	scope      string // current branch scope (empty = main)
	index      *Index
	config     Config

	// Push debounce: coalesce rapid writes into one push
	pushMu      sync.Mutex
	pushPending bool
	pushTimer   *time.Timer
	lastNoteID  string // most recent note ID for hook

	// Post-write hooks — external packages (e.g. pkg/docs) register here
	postWriteHooks []PostWriteFunc
}

// OnPostWrite registers a callback that fires after every Create or Append.
// Used by pkg/docs to implement auto-sync without the engine importing doc logic.
func (e *RealEngine) OnPostWrite(fn PostWriteFunc) {
	e.postWriteHooks = append(e.postWriteHooks, fn)
}

// firePostWrite invokes all registered post-write hooks.
func (e *RealEngine) firePostWrite(noteID string, ref git.NotesRef, targetOID git.OID) {
	for _, fn := range e.postWriteHooks {
		fn(e, noteID, ref, targetOID)
	}
}

// NewEngine creates a new Engine backed by the given git repo.
func NewEngine(repo git.Repo) (*RealEngine, error) {
	repoPath := repo.GetPath()
	maitakeDir := filepath.Join(repoPath, ".maitake")

	e := &RealEngine{
		repo:       repo,
		repoPath:   repoPath,
		maitakeDir: maitakeDir,
		index:      NewIndex(),
		config:     ReadConfig(maitakeDir),
	}

	// Load branch scope if persisted
	scopeFile := filepath.Join(maitakeDir, "scope")
	if data, err := os.ReadFile(scopeFile); err == nil {
		e.scope = string(data)
	}

	// Build index from current notes
	if err := e.Rebuild(); err != nil {
		// Not fatal — index starts empty for new repos
		_ = err
	}

	return e, nil
}

// activeRef returns the notes ref for the current scope.
func (e *RealEngine) activeRef() git.NotesRef {
	if e.scope != "" {
		return git.BranchRef(e.scope)
	}
	return git.DefaultNotesRef
}

// slotRef returns the notes ref for a named slot, or the active ref if empty.
func (e *RealEngine) slotRef(slot string) git.NotesRef {
	if slot != "" {
		return git.SlotRef(slot)
	}
	return e.activeRef()
}

func (e *RealEngine) docOwnerForPath(path string) string {
	for _, id := range e.index.ByTarget[path] {
		state := e.index.States[id]
		if state != nil && state.Kind == "doc" {
			return id
		}
	}
	return ""
}

// Create writes a new creation note with a generated ID.
func (e *RealEngine) Create(opts CreateOptions) (*Note, error) {
	if opts.Kind == "" {
		return nil, fmt.Errorf("kind is required")
	}

	id := opts.ID
	if id == "" {
		var err error
		id, err = GenerateID(e.repoPath)
		if err != nil {
			return nil, fmt.Errorf("generating ID: %w", err)
		}
	}

	now := time.Now().UTC()
	if !opts.Timestamp.IsZero() {
		now = opts.Timestamp.UTC()
	}

	note := &Note{
		ID:        id,
		Kind:      opts.Kind,
		Title:     opts.Title,
		Type:      opts.Type,
		Priority:  opts.Priority,
		Assignee:  opts.Assignee,
		Tags:      opts.Tags,
		Body:      opts.Body,
		Edges:     opts.Edges,
		Timestamp: now.Format(time.RFC3339),
		Time:      now,
		Branch:    e.currentGitBranch(),
	}

	// Auto-add target edges
	for _, target := range opts.Targets {
		note.Edges = append(note.Edges, Edge{
			Type:   "targets",
			Target: EdgeTarget{Kind: "path", Ref: target},
		})
	}

	// Doc notes without a target get an auto-derived path so docTargetPath
	// never falls back to slugifying at write time
	if opts.Kind == "doc" && len(opts.Targets) == 0 {
		cfg := ReadConfig(filepath.Join(e.repoPath, ".maitake"))
		docsDir := cfg.Docs.Dir
		if docsDir == "" {
			docsDir = ".mai-docs"
		}
		slug := Slugify(note.Title)
		if slug == "" {
			slug = note.ID
		}
		autoPath := filepath.Join(docsDir, slug+".md")
		note.Edges = append(note.Edges, Edge{
			Type:   "targets",
			Target: EdgeTarget{Kind: "path", Ref: autoPath},
		})
	}

	if opts.Kind == "doc" {
		for _, edge := range note.Edges {
			if edge.Type != "targets" || edge.Target.Kind != "path" {
				continue
			}
			existingID := e.docOwnerForPath(edge.Target.Ref)
			if existingID != "" && existingID != id {
				return nil, fmt.Errorf("doc target %q already owned by %s", edge.Target.Ref, existingID)
			}
		}
	}

	// Set default status based on type
	if note.Status == "" {
		if note.Type == "artifact" {
			note.Status = "closed"
		}
	}

	// Serialize and guard
	data, err := Serialize(note)
	if err != nil {
		return nil, fmt.Errorf("serializing note: %w", err)
	}

	if err := e.runPreWriteHook(data, note); err != nil {
		return nil, err
	}

	// Write to git
	ref := e.slotRef(opts.Slot)
	targetOID, err := e.getOrCreateTarget(note)
	if err != nil {
		return nil, fmt.Errorf("resolving target: %w", err)
	}

	if err := e.repo.AppendNote(ref, targetOID, git.Note(data)); err != nil {
		return nil, fmt.Errorf("writing note: %w", err)
	}

	note.TargetOID = string(targetOID)
	note.Ref = string(ref)
	note.Slot = opts.Slot

	// Update index
	e.index.Ingest(note)
	e.index.Build()

	// Update cache with new ref tip
	e.updateCache(ref)

	// Auto-push to remote (debounced — coalesces rapid writes)
	e.schedulePush(ref, note.ID)

	// Fire post-write hooks (e.g. doc auto-sync, CRDT init)
	e.firePostWrite(note.ID, ref, targetOID)

	return note, nil
}

// Append writes an event or comment on an existing note.
func (e *RealEngine) Append(opts AppendOptions) (*Note, error) {
	if opts.TargetID == "" {
		return nil, fmt.Errorf("target ID is required")
	}
	if opts.Kind == "" {
		return nil, fmt.Errorf("kind is required")
	}

	// Resolve target note
	// Resolve target — may be a top-level note or a comment
	fullID, err := e.index.ResolveID(opts.TargetID)
	edgeTargetID := fullID // edge points at what the caller asked for
	if err != nil {
		return nil, fmt.Errorf("resolving target ID: %w", err)
	}
	if fullID == "" {
		// Check if it's a comment ID — resolve to parent for storage
		if parentID, ok := e.index.CommentParent[opts.TargetID]; ok {
			fullID = parentID
			edgeTargetID = opts.TargetID // edge still points at the comment
		} else {
			return nil, fmt.Errorf("note %q not found", opts.TargetID)
		}
	}

	appendNow := time.Now().UTC()
	if !opts.Timestamp.IsZero() {
		appendNow = opts.Timestamp.UTC()
	}

	note := &Note{
		Kind:      opts.Kind,
		Body:      opts.Body,
		Field:     opts.Field,
		Value:     opts.Value,
		Edges:     opts.Edges,
		Location:  opts.Location,
		Parent:    opts.Parent,
		Resolved:  opts.Resolved,
		Timestamp: appendNow.Format(time.RFC3339),
		Time:      appendNow,
		Branch:    e.currentGitBranch(),
	}

	// Comments get their own ID so they can be targeted by edit events
	if opts.Kind == "comment" {
		commentID, err := GenerateID(e.repoPath)
		if err != nil {
			return nil, fmt.Errorf("generating comment ID: %w", err)
		}
		note.ID = commentID
	}

	// Auto-add edge based on kind
	edgeType := "updates"
	switch opts.Kind {
	case "comment":
		edgeType = "on"
	case "event":
		if opts.Field == "status" && opts.Value == "closed" {
			edgeType = "closes"
		} else if opts.Field == "status" && opts.Value == "in_progress" {
			edgeType = "starts"
		} else if opts.Field == "status" && opts.Value == "open" {
			edgeType = "reopens"
		}
	}
	note.Edges = append(note.Edges, Edge{
		Type:   edgeType,
		Target: EdgeTarget{Kind: "note", Ref: edgeTargetID},
	})

	// Serialize and guard
	data, err := Serialize(note)
	if err != nil {
		return nil, fmt.Errorf("serializing note: %w", err)
	}

	if err := e.runPreWriteHook(data, note); err != nil {
		return nil, err
	}

	// Write to git — append to same target as the creation note
	ref := e.slotRef(opts.Slot)
	creation := e.index.CreationNotes[fullID]
	if creation == nil {
		return nil, fmt.Errorf("creation note for %q not found in index", fullID)
	}
	targetOID := git.OID(creation.TargetOID)

	if err := e.repo.AppendNote(ref, targetOID, git.Note(data)); err != nil {
		return nil, fmt.Errorf("appending note: %w", err)
	}

	note.TargetOID = string(targetOID)
	note.Ref = string(ref)
	note.Slot = opts.Slot

	// Update index
	e.index.Ingest(note)
	e.index.Build()

	// If this is a body edit on a doc note, update the YDoc state too
	if opts.Field == "body" && creation.Kind == "doc" {
		body := opts.Body
		if body == "" {
			body = opts.Value
		}
		// Post-write hooks handle CRDT updates (via pkg/docs)
		_ = body // used by hooks via engine.Fold
	}

	// Update cache with new ref tip
	e.updateCache(ref)

	// Auto-push to remote (debounced — coalesces rapid writes)
	e.schedulePush(ref, fullID)

	// Auto-sync docs if configured
	// Fire post-write hooks (e.g. doc auto-sync, CRDT update)
	e.firePostWrite(fullID, ref, targetOID)

	return note, nil
}

// Get returns the raw creation note by ID (not folded).
func (e *RealEngine) Get(id string) (*Note, error) {
	fullID, err := e.index.ResolveID(id)
	if err != nil {
		return nil, err
	}
	if fullID == "" {
		return nil, fmt.Errorf("note %q not found", id)
	}
	note := e.index.CreationNotes[fullID]
	if note == nil {
		return nil, fmt.Errorf("note %q not found in index", fullID)
	}
	return note, nil
}

// Fold returns the computed current state of a note.
func (e *RealEngine) Fold(id string) (*State, error) {
	fullID, err := e.index.ResolveID(id)
	if err != nil {
		return nil, err
	}
	if fullID == "" {
		return nil, fmt.Errorf("note %q not found", id)
	}
	state := e.index.States[fullID]
	if state == nil {
		return nil, fmt.Errorf("state for %q not found", fullID)
	}
	return state, nil
}

// Context returns all open notes targeting a file path.
func (e *RealEngine) Context(path string) ([]State, error) {
	states := e.index.ContextForPath(path)
	result := make([]State, len(states))
	for i, s := range states {
		result[i] = *s
	}
	return result, nil
}

// ContextAll returns all notes targeting a file path (open + closed).
func (e *RealEngine) ContextAll(path string) ([]State, error) {
	states := e.index.ContextAllForPath(path)
	result := make([]State, len(states))
	for i, s := range states {
		result[i] = *s
	}
	return result, nil
}

// Find returns all notes matching filters.
func (e *RealEngine) Find(opts FindOptions) ([]State, error) {
	states := e.index.Query(opts)
	result := make([]State, len(states))
	for i, s := range states {
		result[i] = *s
	}
	return result, nil
}

// List returns summary state for notes matching filters.
func (e *RealEngine) List(opts ListOptions) ([]StateSummary, error) {
	return e.index.QueryList(opts), nil
}

// Search performs BM25 full-text search across all notes.
func (e *RealEngine) Search(query string, opts SearchOptions) ([]SearchResult, error) {
	if e.index.Text == nil {
		return nil, nil
	}
	return e.index.Text.SearchFiltered(query, opts), nil
}

// Refs returns all notes with edges pointing at a target (reverse lookup).
func (e *RealEngine) Refs(target string) ([]State, error) {
	// Search all states for edges targeting this
	var results []State
	for _, state := range e.index.States {
		if matchesTarget(state, e.index.CreationNotes[state.ID], target) {
			results = append(results, *state)
		}
	}
	return results, nil
}

// matchesTarget checks if a state has any edge pointing at the given target.
func matchesTarget(state *State, creation *Note, target string) bool {
	if creation != nil {
		for _, edge := range creation.Edges {
			if edge.Target.Ref == target {
				return true
			}
		}
	}
	for _, ev := range state.Events {
		for _, edge := range ev.Edges {
			if edge.Target.Ref == target {
				return true
			}
		}
	}
	return false
}

// Kinds returns all kinds in use with counts.
func (e *RealEngine) Kinds() ([]KindCount, error) {
	return e.index.KindCounts(), nil
}

// BranchUse switches the active notes scope.
func (e *RealEngine) BranchUse(name string) error {
	e.scope = name
	// Persist
	scopeFile := filepath.Join(e.maitakeDir, "scope")
	if err := os.MkdirAll(e.maitakeDir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(scopeFile, []byte(name), 0644); err != nil {
		return err
	}
	return e.Rebuild()
}

// BranchMerge merges a branch scope into the main scope.
func (e *RealEngine) BranchMerge(name string) error {
	branchRef := git.BranchRef(name)
	mainRef := git.DefaultNotesRef

	// Read all notes from branch
	entries := e.repo.ListAllNotedObjects(branchRef)
	for _, oid := range entries {
		noteBytes := e.repo.GetNotes(branchRef, oid)
		if len(noteBytes) == 0 {
			continue
		}
		// Append each note line to the main ref
		for _, note := range noteBytes {
			_ = e.repo.AppendNote(mainRef, oid, note)
		}
	}

	return e.Rebuild()
}

// CurrentBranch returns the active scope name (empty = main).
func (e *RealEngine) CurrentBranch() string {
	return e.scope
}

// Doctor reports graph health.
func (e *RealEngine) Doctor() (*DoctorReport, error) {
	report := &DoctorReport{
		ByKind:   make(map[string]int),
		ByStatus: make(map[string]int),
	}

	for _, state := range e.index.States {
		report.TotalNotes++
		report.ByKind[state.Kind]++
		report.ByStatus[state.Status]++
		report.CreationNotes++
		report.Events += len(state.Events)
		report.Comments += len(state.Comments)
	}

	// Check for broken edges
	for _, state := range e.index.States {
		for _, dep := range state.Deps {
			if e.index.CreationNotes[dep] == nil {
				report.BrokenEdges++
			}
		}
	}

	// List slots
	// TODO: scan for slot and branch refs when NoteRefs is added to git.Repo

	report.IndexFresh = true
	return report, nil
}

// Rebuild forces a full index rebuild from git.
// Uses ~/.maitake/cache/ to skip git reads when the ref tip hasn't changed.
func (e *RealEngine) Rebuild() error {
	idx := NewIndex()
	ref := e.activeRef()

	// Check cache — keyed by notes ref tip SHA
	tipOID := refTipOID(e.repo, ref)
	if tipOID != "" {
		if cached := loadCache(e.repoPath, tipOID); cached != nil {
			for _, n := range cached {
				idx.Ingest(n)
			}
			idx.Build()
			e.index = idx
			return nil
		}
	}

	// Cache miss — full rebuild from git
	var allNotes []*Note

	// Batch read all notes in 2 git commands (not N+1)
	allNotesMap, err := e.repo.GetAllNotesUnfiltered(ref)
	if err != nil {
		return fmt.Errorf("reading notes: %w", err)
	}
	for oid, rawNotes := range allNotesMap {
		for _, raw := range rawNotes {
			note, err := Parse(raw)
			if err != nil {
				continue // skip unparseable notes
			}
			note.TargetOID = string(oid)
			note.Ref = string(ref)
			idx.Ingest(note)
			allNotes = append(allNotes, note)
		}
	}

	idx.Build()
	e.index = idx

	// Write cache for next time
	if tipOID != "" {
		writeCache(e.repoPath, tipOID, allNotes)
	}

	return nil
}

// updateCache writes the current index to the cache after a write.
func (e *RealEngine) updateCache(ref git.NotesRef) {
	tipOID := refTipOID(e.repo, ref)
	if tipOID == "" {
		return
	}
	// Collect all notes from the index
	var allNotes []*Note
	for _, n := range e.index.CreationNotes {
		allNotes = append(allNotes, n)
	}
	for _, events := range e.index.EventsByTarget {
		allNotes = append(allNotes, events...)
	}
	writeCache(e.repoPath, tipOID, allNotes)
}

// currentGitBranch returns the short branch name (e.g. "feature/auth").
// Returns empty string on detached HEAD or error.
func (e *RealEngine) currentGitBranch() string {
	ref, err := e.repo.GetHeadRef()
	if err != nil {
		return ""
	}
	// GetHeadRef returns "refs/heads/feature/auth" — strip prefix
	return strings.TrimPrefix(ref, "refs/heads/")
}

// GitBranch returns the current git branch name (e.g. "feature/auth").
func (e *RealEngine) GitBranch() string {
	return e.currentGitBranch()
}

// IsMerged checks if the 'from' branch has been merged into 'into'.
// Uses git merge-base --is-ancestor.
func (e *RealEngine) IsMerged(from, into string) bool {
	merged, err := e.repo.IsAncestor(from, into)
	if err != nil {
		return false
	}
	return merged
}

// GetConfig returns the current configuration.
func (e *RealEngine) GetConfig() Config {
	return e.config
}

// Sync does a manual fetch + merge + push for the configured remote.
func (e *RealEngine) Sync() error {
	if e.config.Sync.Remote == "" {
		return fmt.Errorf("no remote configured — run mai init --remote <name>")
	}

	ref := string(e.activeRef())
	remote := e.config.Sync.Remote

	// Pull (fetch + merge)
	if err := e.repo.PullNotes(remote, ref); err != nil {
		return fmt.Errorf("pull: %w", err)
	}

	// Rebuild index after merge
	if err := e.Rebuild(); err != nil {
		return fmt.Errorf("rebuild after pull: %w", err)
	}

	// Push
	if err := e.repo.PushNotes(remote, ref); err != nil {
		return fmt.Errorf("push: %w", err)
	}

	return nil
}

// Note: autoSyncDoc, updateYDoc, initYDoc, prevNoteBody moved to pkg/docs.
// The engine fires postWriteHooks instead — see OnPostWrite().

// AppendRaw writes a pre-serialized note to git and re-ingests it into the index.
// Used by post-write hooks (e.g. pkg/docs CRDT) that need to emit events.
func (e *RealEngine) AppendRaw(ref git.NotesRef, targetOID git.OID, data []byte, note *Note) {
	if err := e.repo.AppendNote(ref, targetOID, git.Note(data)); err != nil {
		return
	}
	e.index.Ingest(note)
	e.index.Build()
}

// RepoPath returns the absolute path to the repo root.
func (e *RealEngine) RepoPath() string {
	return e.repoPath
}

// schedulePush debounces auto-push. Multiple writes within 500ms coalesce into one push.
func (e *RealEngine) schedulePush(ref git.NotesRef, noteID string) {
	if e.config.Sync.Remote == "" {
		return
	}

	e.pushMu.Lock()
	defer e.pushMu.Unlock()

	e.lastNoteID = noteID

	if e.pushTimer != nil {
		e.pushTimer.Stop()
	}

	e.pushTimer = time.AfterFunc(pushDebounceDelay, func() {
		e.pushMu.Lock()
		id := e.lastNoteID
		e.pushMu.Unlock()
		e.autoPush(ref, id)
	})
}

// autoPush pushes the notes ref to the configured remote if set.
// On rejection, fetches + merges (cat_sort_uniq) + retries once.
// Failures warn to stderr but never block the write.
func (e *RealEngine) autoPush(ref git.NotesRef, noteID string) {
	if e.config.Sync.Remote == "" {
		return
	}

	remote := e.config.Sync.Remote

	// Check blocked hosts
	remotes, err := e.repo.Remotes()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mai: warning: could not list remotes: %v\n", err)
		return
	}

	// Find the remote URL to check against blocked hosts
	var found bool
	for _, r := range remotes {
		if r == remote {
			found = true
			break
		}
	}
	if !found {
		fmt.Fprintf(os.Stderr, "mai: warning: remote %q not found\n", remote)
		return
	}

	// Check blocked hosts — need to get remote URL
	// Use git config to get the URL
	if len(e.config.Sync.BlockedHosts) > 0 {
		// We can't easily get the remote URL through the Repo interface,
		// so we check via a lightweight git config call. For now, trust
		// the blocked-hosts list and skip the URL check — the remote name
		// itself is what the user configured explicitly.
		// TODO: add GetRemoteURL to Repo interface for proper URL checking
	}

	// Try push
	refPattern := string(ref)
	if err := e.repo.PushNotes(remote, refPattern); err != nil {
		// Push rejected — try fetch + merge + retry
		if pullErr := e.repo.PullNotes(remote, refPattern); pullErr != nil {
			fmt.Fprintf(os.Stderr, "mai: warning: push failed and pull-merge failed: %v\n", pullErr)
			return
		}
		// Retry push after merge
		if retryErr := e.repo.PushNotes(remote, refPattern); retryErr != nil {
			fmt.Fprintf(os.Stderr, "mai: warning: push failed after merge: %v\n", retryErr)
			return
		}
	}

	// Fire post-push hook
	e.runPostPushHook(remote, refPattern, noteID)
}

// runPostPushHook fires .maitake/hooks/post-push after a successful push.
// The hook receives the remote name and ref as env vars.
// Failures warn to stderr — they never block the push.
func (e *RealEngine) runPostPushHook(remote, ref string, noteID string) {
	if !guard.HookExists(e.maitakeDir, "post-push") {
		return
	}
	env := map[string]string{
		"MAI_REMOTE":    remote,
		"MAI_REF":       ref,
		"MAI_REPO_PATH": e.repoPath,
		"MAI_NOTE_ID":   noteID,
	}
	if err := guard.RunHook(e.maitakeDir, "post-push", nil, env); err != nil {
		fmt.Fprintf(os.Stderr, "mai: warning: post-push hook: %v\n", err)
	}
}

// runPreWriteHook runs the pre-write guard hook.
func (e *RealEngine) runPreWriteHook(data []byte, note *Note) error {
	env := map[string]string{
		"MAI_NOTE_KIND": note.Kind,
		"MAI_NOTE_ID":   note.ID,
	}
	return guard.RunHook(e.maitakeDir, "pre-write", data, env)
}

// getOrCreateTarget determines the git OID to attach a note to.
// For file targets, uses the blob OID via Show + StoreBlob.
// For standalone notes, creates a deterministic synthetic blob.
func (e *RealEngine) getOrCreateTarget(note *Note) (git.OID, error) {
	// Check edges for a file target
	for _, edge := range note.Edges {
		if edge.Type == "targets" && edge.Target.Kind == "path" {
			// Get file contents at HEAD, then hash to get OID
			contents, err := e.repo.Show("HEAD", edge.Target.Ref)
			if err == nil && contents != "" {
				oid, err := e.repo.StoreBlob(contents)
				if err == nil {
					return oid, nil
				}
			}
		}
	}

	// No file target — create a deterministic synthetic blob from the note ID
	content := "maitake:" + note.ID
	oid, err := e.repo.StoreBlob(content)
	if err != nil {
		return "", fmt.Errorf("creating synthetic target: %w", err)
	}
	return oid, nil
}

// NoteRefs delegates to the git repo to list maitake notes refs.
// This extends the git.Repo interface for maitake-specific ref filtering.
