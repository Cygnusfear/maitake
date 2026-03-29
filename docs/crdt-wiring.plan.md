---
mai-id: mai-k1gv
---
# CRDT Docs Layer — Wiring Plan

## Status: Planning

## What exists
- `pkg/crdt`: TextDoc backed by yrs via WASM/wazero
- API: New, Load, Insert, Delete, Content, Save, StateVector, Diff, Apply
- Tests pass (concurrent merge works, one edge case skipped)
- SyncDocs uses plain text content hash + last-writer-wins for conflicts
- Body edit events are append-only (field=body)

## What we're building
Replace the content hash comparison in SyncDocs with CRDT-backed merge. When both note body AND file diverge, instead of "file wins", merge character-level.

## Architecture

```
Note body (plain text) ←→ YDoc state (binary, stored as body-edit event)
                              ↕
                     Materialized .md file
```

## Storage model

Each doc note gets a **YDoc state** stored as a base64-encoded body-edit event:

```json
{"kind":"event", "field":"ydoc", "value":"<base64 YDoc binary>", "edges":[...]}
```

The `body` field stays plain text (human-readable in `mai show`). The `ydoc` field is the CRDT state for merge. They're kept in sync:
- New doc: body = content, ydoc = YDoc initialized with that content
- Edit via `mai edit`: body updated via event, ydoc updated with insert/delete ops
- Edit via file: diff file vs current body → compute ops → update ydoc + body

## Slices

### Slice A: YDoc state on doc notes
- [ ] On `mai create -k doc`, initialize YDoc with body content, store state as ydoc event
- [ ] `fold.go`: new `case "ydoc"` that stores latest YDoc binary on State
- [ ] State gets `YDocState []byte` field (not serialized to JSON, internal only)
- [ ] On `mai edit <doc-id>`, compute diff vs current body, apply as YDoc ops, emit both body + ydoc events

### Slice B: SyncDocs uses CRDT for conflict resolution
- [ ] When file hash ≠ note hash AND both changed since last sync:
  - Load YDoc from note's ydoc state
  - Diff file content vs YDoc content → insert/delete ops
  - Apply ops to YDoc
  - New body = YDoc.Content()
  - Emit body + ydoc events
- [ ] When only file changed: same as above (unidirectional merge)
- [ ] When only note changed: materialize YDoc.Content() to file

### Slice C: Multi-peer merge
- [ ] When two repos push concurrent edits to the same doc:
  - git notes merge via cat_sort_uniq already works (append-only)
  - Both body events survive
  - Fold applies them in timestamp order
  - BUT: body last-writer-wins loses one edit
  - Fix: fold replays body edits as YDoc operations (not plain overwrites)
  - The ydoc event from each peer has a state vector
  - Merge: Load state1, Apply diff from state2 → merged content
- [ ] `mai docs sync` after pull: detect diverged ydoc states, merge via Apply

### Slice D: Diff engine (text → insert/delete ops)
- [ ] Given old text and new text, produce a minimal sequence of Insert/Delete operations
- [ ] Options: Myers diff (line-level) → character-level, or use Go's `github.com/sergi/go-diff`
- [ ] This is the bridge between "file changed on disk" and "YDoc operations"

## Risks / Pre-mortem

### 1. YDoc state grows unboundedly
Every edit adds to the YATA history. Large documents with many edits → large YDoc state.
**Mitigation**: Periodic compaction — snapshot YDoc state and discard old ydoc events. The note events still have the full history; the YDoc just needs the latest state.

### 2. Diff → ops conversion is lossy for non-trivial edits
Moving a paragraph looks like delete + insert, not a move. The CRDT handles this fine (it's just ops) but the merge result might have duplicates if two peers move the same text.
**Mitigation**: Accept this for v1. Character-level merge is already better than last-writer-wins.

### 3. WASM instance per merge — solved
yrs-wasi uses a global `static DOC` — one doc per instance. `doc_new()` resets it.
So: compile WASM once, create one instance, reuse it across all docs sequentially.
`doc_load(state)` switches to a specific doc, operate, `doc_save()`, move to next.
Only instantiate for docs where hashes actually diverge (most will be in sync).

### 4. The skipped test — concurrent append at end-of-doc
Two separate WASM instances appending at the same position don't merge correctly.
**Mitigation**: Investigate whether this is a yrs-wasi bug or a fundamental YATA limitation. For v1, accept that concurrent end-of-doc appends may duplicate.

### 5. Base64 YDoc state bloats the notes ref
Each ydoc event is a base64-encoded binary blob in a JSON line. For a 10KB document, the YDoc state might be 20-50KB encoded.
**Mitigation**: Only store ydoc state on doc notes that actually need merging. Or compress before base64.

### 6. Breaking change — old notes won't have ydoc state
Existing doc notes have no ydoc field. First sync needs to initialize YDoc from current body.
**Mitigation**: `SyncDocs` initializes YDoc lazily — if no ydoc state exists, create from body. This is just the "new doc" path.

## Order of work
1. Slice D first (diff engine) — pure Go, no CRDT dependency, testable independently
2. Slice A (YDoc on notes) — wire crdt into engine
3. Slice B (SyncDocs merge) — the payoff
4. Slice C (multi-peer) — only matters when two people/agents edit concurrently

## Obsidian compatibility

Obsidian watches files via fsnotify. Critical rules:
1. **Never overwrite a file Obsidian has unsaved changes in** — file mtime check before write, or just: file always wins so we never overwrite diverged files (current behavior, keep it)
2. **Deleted files** — don't delete materialized files when note closes. Instead add `closed: true` to frontmatter. Obsidian won't show a scary "file deleted" modal.
3. **Frontmatter** — only touch `mai-id` field. Preserve all other frontmatter Obsidian adds (tags, aliases, cssclasses, etc)
4. **Write during edit race** — daemon debounces writes (500ms). Obsidian autosaves every 2s. Worst case: Obsidian's save overwrites our write, next sync picks it up. File always wins.

## WASM reuse strategy

yrs-wasi has a global `static DOC: Mutex<Option<Doc>>`. One doc at a time per instance.
- **Compile once**: `wazero.CompileModule` (~100ms) — do this at engine init
- **One instance**: reuse across all docs — `doc_load()` switches, operate, `doc_save()`, next
- **Only instantiate when needed**: most docs will be hash-match (in sync), skip CRDT entirely
- TextDoc wrapper needs a `Pool` or `Shared` variant that doesn't own the runtime

## What this DOESN'T do
- Real-time collaboration (no live sync, no WebSocket)
- Undo/redo (just edit again)
- Rich text CRDT (we're text-only, markdown is plain text)


