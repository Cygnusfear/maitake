package notes

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cygnusfear/maitake/pkg/git"
)

// cacheDir returns ~/.maitake/cache/.
func cacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".maitake", "cache")
}

// repoHash returns a stable hash of the repo path for cache key namespacing.
func repoHash(repoPath string) string {
	h := sha256.Sum256([]byte(repoPath))
	return fmt.Sprintf("%x", h[:8])
}

// cachedNote is the serializable form of a parsed note for cache storage.
// Includes the git metadata (TargetOID, Ref) that are normally populated on read.
type cachedNote struct {
	Note      *Note  `json:"note"`
	TargetOID string `json:"targetOid"`
	Ref       string `json:"ref"`
}

// cacheEnvelope wraps the cached data with the ref tip for validation.
type cacheEnvelope struct {
	RefTip string       `json:"refTip"`
	Notes  []cachedNote `json:"notes"`
}

type summaryCacheEnvelope struct {
	RefTip     string              `json:"refTip"`
	Entries    []summaryCacheEntry `json:"entries"`
	KindCounts []KindCount         `json:"kindCounts"`
}

type summaryCacheEntry struct {
	Summary   StateSummary `json:"summary"`
	TargetOID string       `json:"targetOid"`
}

func cacheFilePath(repoPath string, tipOID git.OID, suffix string) string {
	dir := cacheDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, repoHash(repoPath), string(tipOID)+suffix)
}

// loadCache tries to load a cached index for the given ref tip.
// Returns the parsed notes if the cache is valid, or nil if miss/stale.
func loadCache(repoPath string, tipOID git.OID) []*Note {
	path := cacheFilePath(repoPath, tipOID, ".json")
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var env cacheEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil
	}

	// Validate tip matches
	if git.OID(env.RefTip) != tipOID {
		return nil
	}

	notes := make([]*Note, 0, len(env.Notes))
	for _, cn := range env.Notes {
		n := cn.Note
		n.TargetOID = cn.TargetOID
		n.Ref = cn.Ref
		// Re-hydrate computed Time from Timestamp
		if n.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339, n.Timestamp); err == nil {
				n.Time = t
			}
		}
		notes = append(notes, n)
	}

	return notes
}

func loadSummaryCache(repoPath string, tipOID git.OID) *summaryCacheEnvelope {
	path := cacheFilePath(repoPath, tipOID, ".summary.json")
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var env summaryCacheEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil
	}
	if git.OID(env.RefTip) != tipOID {
		return nil
	}
	if len(env.Entries) == 0 && len(env.KindCounts) > 0 {
		return nil // old summary-cache schema; force one rebuild to refresh
	}
	return &env
}

// writeCache writes parsed notes to the cache for the given ref tip.
func writeCache(repoPath string, tipOID git.OID, notes []*Note) {
	dir := cacheDir()
	if dir == "" {
		return
	}

	repoDir := filepath.Join(dir, repoHash(repoPath))
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		return
	}

	env := cacheEnvelope{
		RefTip: string(tipOID),
		Notes:  make([]cachedNote, len(notes)),
	}
	for i, n := range notes {
		env.Notes[i] = cachedNote{
			Note:      n,
			TargetOID: n.TargetOID,
			Ref:       n.Ref,
		}
	}

	data, err := json.Marshal(env)
	if err != nil {
		return
	}

	path := cacheFilePath(repoPath, tipOID, ".json")
	os.WriteFile(path, data, 0644)

	// Prune old cache files — keep only the 2 most recent
	pruneCache(repoDir, 2)

	// Opportunistic cross-repo GC (rate-limited via .last-gc marker)
	pruneStaleRepoCaches(dir, pruneStaleRepoCachesMaxAge, pruneStaleRepoCachesCooldown, time.Now())
}

func writeSummaryCache(repoPath string, tipOID git.OID, entries []summaryCacheEntry, kindCounts []KindCount) {
	dir := cacheDir()
	if dir == "" {
		return
	}

	repoDir := filepath.Join(dir, repoHash(repoPath))
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		return
	}

	env := summaryCacheEnvelope{
		RefTip:     string(tipOID),
		Entries:    entries,
		KindCounts: kindCounts,
	}

	data, err := json.Marshal(env)
	if err != nil {
		return
	}

	path := cacheFilePath(repoPath, tipOID, ".summary.json")
	os.WriteFile(path, data, 0644)
	pruneCache(repoDir, 4)
}

// summaryEntriesFromIndex derives summary-cache entries from a built Index.
// Used by Rebuild's cache-hit branch (so a stale or absent summary self-heals)
// and by updateCache (so writes keep the summary in sync with the full cache).
func summaryEntriesFromIndex(idx *Index) []summaryCacheEntry {
	entries := make([]summaryCacheEntry, 0, len(idx.States))
	for _, state := range idx.States {
		creation := idx.CreationNotes[state.ID]
		targetOID := ""
		if creation != nil {
			targetOID = creation.TargetOID
		}
		entries = append(entries, summaryCacheEntry{
			Summary:   ToSummary(state),
			TargetOID: targetOID,
		})
	}
	return entries
}

// summaryCacheExists reports whether a summary cache file is already present
// for the given ref tip. Used to avoid redundant rewrites when the file is fresh.
func summaryCacheExists(repoPath string, tipOID git.OID) bool {
	path := cacheFilePath(repoPath, tipOID, ".summary.json")
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func querySummaries(summaries []StateSummary, opts ListOptions) []StateSummary {
	filtered := make([]StateSummary, 0, len(summaries))
	for _, s := range summaries {
		if opts.Kind != "" && s.Kind != opts.Kind {
			continue
		}
		if opts.Status != "" && opts.Status != "all" && s.Status != opts.Status {
			continue
		}
		if opts.Type != "" && s.Type != opts.Type {
			continue
		}
		if opts.Assignee != "" && s.Assignee != opts.Assignee {
			continue
		}
		if opts.Tag != "" && !contains(s.Tags, opts.Tag) {
			continue
		}
		if opts.Target != "" && !contains(s.Targets, opts.Target) {
			continue
		}
		filtered = append(filtered, s)
	}

	switch opts.SortBy {
	case "priority":
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].Priority < filtered[j].Priority
		})
	case "updated":
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt)
		})
	default:
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
		})
	}

	if opts.Limit > 0 && len(filtered) > opts.Limit {
		filtered = filtered[:opts.Limit]
	}

	return filtered
}

func summaryEntries(entries []summaryCacheEntry) []StateSummary {
	summaries := make([]StateSummary, len(entries))
	for i, entry := range entries {
		summaries[i] = entry.Summary
	}
	return summaries
}

func resolveSummaryEntry(entries []summaryCacheEntry, partial string) (*summaryCacheEntry, string, error) {
	for i := range entries {
		if entries[i].Summary.ID == partial {
			return &entries[i], partial, nil
		}
	}

	var match *summaryCacheEntry
	var fullID string
	var matches []string
	for i := range entries {
		id := entries[i].Summary.ID
		if id == "" || !strings.Contains(id, partial) {
			continue
		}
		if match == nil {
			match = &entries[i]
			fullID = id
		}
		matches = append(matches, id)
	}

	switch len(matches) {
	case 0:
		return nil, "", nil
	case 1:
		return match, fullID, nil
	default:
		return nil, "", &AmbiguousIDError{Partial: partial, Matches: matches}
	}
}

// pruneCache keeps the N most recent ref tips and removes all cache files
// for older tips. A single tip can produce multiple files (.json for the full
// note cache, .summary.json for the summary fast path, .textindex.json for
// the persisted BM25 index) — pruning by file count alone would silently
// orphan summaries or text indices of the current tip. Pruning by tip keeps
// every companion file for the N most recent tips and drops the rest.
func pruneCache(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	type fileInfo struct {
		name    string
		tip     string
		modTime int64
	}

	var files []fileInfo
	tipMTime := map[string]int64{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		tip := tipFromCacheFileName(e.Name())
		if tip == "" {
			continue
		}
		mt := info.ModTime().UnixNano()
		files = append(files, fileInfo{name: e.Name(), tip: tip, modTime: mt})
		if mt > tipMTime[tip] {
			tipMTime[tip] = mt
		}
	}

	if len(tipMTime) <= keep {
		return
	}

	// Rank tips newest first by their most recent file mtime.
	tips := make([]string, 0, len(tipMTime))
	for t := range tipMTime {
		tips = append(tips, t)
	}
	sort.Slice(tips, func(a, b int) bool {
		return tipMTime[tips[a]] > tipMTime[tips[b]]
	})

	keepSet := make(map[string]struct{}, keep)
	for i := 0; i < keep && i < len(tips); i++ {
		keepSet[tips[i]] = struct{}{}
	}

	for _, f := range files {
		if _, ok := keepSet[f.tip]; ok {
			continue
		}
		os.Remove(filepath.Join(dir, f.name))
	}
}

// tipFromCacheFileName strips the known cache suffixes to recover the ref
// tip OID that keys all three companion files. Returns "" for anything that
// doesn't look like a cache file.
func tipFromCacheFileName(name string) string {
	for _, suffix := range []string{".textindex.gob", ".summary.json", ".json"} {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix)
		}
	}
	return ""
}

// pruneStaleRepoCachesMaxAge is how long a repo cache dir can be untouched
// before the opportunistic GC deletes it.
const pruneStaleRepoCachesMaxAge = 30 * 24 * time.Hour

// pruneStaleRepoCachesCooldown is the minimum interval between cross-repo GC
// runs, tracked via the .last-gc marker file in the cache root.
const pruneStaleRepoCachesCooldown = 24 * time.Hour

// pruneStaleRepoCaches deletes repo-hash directories under the cache root
// whose mtime is older than maxAge. Runs at most once per cooldown interval,
// gated by a .last-gc marker file. Invoked opportunistically from write paths.
// Silent on errors — cache GC is best-effort housekeeping.
func pruneStaleRepoCaches(root string, maxAge, cooldown time.Duration, now time.Time) {
	if root == "" {
		return
	}
	marker := filepath.Join(root, ".last-gc")
	if info, err := os.Stat(marker); err == nil {
		if now.Sub(info.ModTime()) < cooldown {
			return
		}
	}
	// Touch the marker first so concurrent processes don't all scan.
	if err := os.MkdirAll(root, 0755); err != nil {
		return
	}
	_ = os.WriteFile(marker, []byte(now.UTC().Format(time.RFC3339)), 0644)

	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	cutoff := now.Add(-maxAge)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Repo-hash dirs are 16 lowercase hex chars. Skip anything else
		// to avoid clobbering unrelated entries a future refactor might add.
		if !isRepoHashDir(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.RemoveAll(filepath.Join(root, e.Name()))
		}
	}
}

func isRepoHashDir(name string) bool {
	if len(name) != 16 {
		return false
	}
	for _, c := range name {
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
		if !isHex {
			return false
		}
	}
	return true
}

// refTipOID returns the commit OID at the tip of a notes ref.
// Returns empty OID if the ref doesn't exist yet (new repo, no notes).
func refTipOID(repo git.Repo, ref git.NotesRef) git.OID {
	oid, err := repo.GetCommitHash(string(ref))
	if err != nil {
		return ""
	}
	return oid
}
