package notes

import (
	"bytes"
	"encoding/gob"
	"os"
	"path/filepath"
	"time"

	"github.com/cygnusfear/maitake/pkg/git"
)

// textIndexEnvelope is the on-disk form of a built TextIndex. Keyed by ref
// tip OID in the same ~/.maitake/cache/<repohash>/ directory as the full and
// summary caches. Written by the first search after a rebuild; reloaded on
// subsequent CLI invocations until a note write bumps the tip.
//
// Encoded with encoding/gob rather than JSON — profiling on a 5000-note
// corpus showed json.Unmarshal of the nested term-frequency maps dominated
// search latency (~650ms of 1.1s total). gob roundtrips 3-5x faster because
// it skips the reflection-heavy map[string]T rehashing that JSON does.
type textIndexEnvelope struct {
	RefTip   string
	DocCount int
	AvgDL    float64
	DF       map[string]int
	Docs     []persistedDoc
}

// persistedDoc is the dehydrated per-document state. States are rehydrated
// on load via the summary cache — the text index never holds full note
// bodies, only the scoring shape.
type persistedDoc struct {
	ID     string
	DocLen float64
	Terms  map[string]float64
}

// loadTextIndexCache tries to load a persisted text index for the given ref
// tip. Returns nil on miss, corruption, or schema mismatch.
func loadTextIndexCache(repoPath string, tipOID git.OID) *textIndexEnvelope {
	path := cacheFilePath(repoPath, tipOID, ".textindex.gob")
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var env textIndexEnvelope
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&env); err != nil {
		return nil
	}
	if git.OID(env.RefTip) != tipOID {
		return nil
	}
	return &env
}

// writeTextIndexCache serializes a built TextIndex to disk. Runs after the
// first search in a process; subsequent searches in the same process reuse
// the in-memory index, and subsequent CLI invocations reuse the on-disk one.
func writeTextIndexCache(repoPath string, tipOID git.OID, ti *TextIndex) {
	if ti == nil || ti.docCount == 0 {
		return
	}
	dir := cacheDir()
	if dir == "" {
		return
	}
	repoDir := filepath.Join(dir, repoHash(repoPath))
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		return
	}
	docs := make([]persistedDoc, ti.docCount)
	for i := 0; i < ti.docCount; i++ {
		docs[i] = persistedDoc{
			ID:     ti.docIDs[i],
			DocLen: ti.docLens[i],
			Terms:  ti.docTerms[i],
		}
	}
	env := textIndexEnvelope{
		RefTip:   string(tipOID),
		DocCount: ti.docCount,
		AvgDL:    ti.avgDL,
		DF:       ti.df,
		Docs:     docs,
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(env); err != nil {
		return
	}
	path := cacheFilePath(repoPath, tipOID, ".textindex.gob")
	_ = os.WriteFile(path, buf.Bytes(), 0644)
	// Opportunistic cross-repo GC — same rate-limit as writeCache.
	pruneStaleRepoCaches(dir, pruneStaleRepoCachesMaxAge, pruneStaleRepoCachesCooldown, time.Now())
}

// textIndexCacheExists reports whether a persisted text index is present for
// the given ref tip.
func textIndexCacheExists(repoPath string, tipOID git.OID) bool {
	path := cacheFilePath(repoPath, tipOID, ".textindex.gob")
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// hydrateFromCache reconstructs a TextIndex from a persisted envelope, pairing
// each doc with the supplied state-resolver. Missing states are skipped — a
// subsequent rebuild will regenerate the on-disk copy.
func (ti *TextIndex) hydrateFromCache(env *textIndexEnvelope, resolveState func(id string) *State) {
	ti.docIDs = make([]string, 0, env.DocCount)
	ti.docLens = make([]float64, 0, env.DocCount)
	ti.docTerms = make([]map[string]float64, 0, env.DocCount)
	ti.states = make([]*State, 0, env.DocCount)
	ti.df = env.DF
	ti.avgDL = env.AvgDL

	for _, d := range env.Docs {
		state := resolveState(d.ID)
		if state == nil {
			continue
		}
		ti.docIDs = append(ti.docIDs, d.ID)
		ti.docLens = append(ti.docLens, d.DocLen)
		ti.docTerms = append(ti.docTerms, d.Terms)
		ti.states = append(ti.states, state)
	}
	ti.docCount = len(ti.docIDs)
}

// stateFromSummary builds a lightweight *State from a StateSummary for use
// in search results. Only the fields consumed by matchesFind and the CLI
// renderer are populated — body, events, and comments are intentionally
// empty because search scoring comes from the persisted term frequencies,
// not from re-tokenizing the full note on every query.
func stateFromSummary(s StateSummary) *State {
	return &State{
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
