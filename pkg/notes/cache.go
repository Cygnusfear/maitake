package notes

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

// loadCache tries to load a cached index for the given ref tip.
// Returns the parsed notes if the cache is valid, or nil if miss/stale.
func loadCache(repoPath string, tipOID git.OID) []*Note {
	dir := cacheDir()
	if dir == "" {
		return nil
	}

	path := filepath.Join(dir, repoHash(repoPath), string(tipOID)+".json")
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

	path := filepath.Join(repoDir, string(tipOID)+".json")
	os.WriteFile(path, data, 0644)

	// Prune old cache files — keep only the 2 most recent
	pruneCache(repoDir, 2)
}

// pruneCache removes old cache files, keeping only the N most recent.
func pruneCache(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) <= keep {
		return
	}

	type fileInfo struct {
		name    string
		modTime int64
	}

	var files []fileInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{name: e.Name(), modTime: info.ModTime().UnixNano()})
	}

	// Sort newest first
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime > files[j].modTime
	})

	// Remove old ones
	for i := keep; i < len(files); i++ {
		os.Remove(filepath.Join(dir, files[i].name))
	}
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
