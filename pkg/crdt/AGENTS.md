# pkg/crdt

Text CRDT via Yjs (ywasm compiled to wasm, run through wazero). Provides character-level merge for doc sync.

## Boundaries

- Pure CRDT operations — no git, no notes, no domain concepts
- Consumed by `pkg/docs` only — `pkg/notes` must NOT import this package
- Boundary enforced by `pkg/docs/boundary_test.go` and `scripts/check-boundary.sh`

## Key types

- `TextDoc` — a Yjs text document (wraps wasm instance)
- `Diff(old, new)` → `[]Op` — compute edit operations
- `ApplyOps(doc, ops)` — apply operations to a document
- `New()` / `Load(state)` — create or restore from saved state
- `doc.Save()` → `[]byte` — serialize for storage
- `doc.Content()` → `string` — read current text
- `doc.Apply(remoteState)` — merge remote peer's state
- `doc.Close()` — release wasm resources (always defer this)

## How doc sync uses it

1. Load note's YDoc state (`crdt.Load`)
2. Create base doc from last sync point
3. Fork two peers: note-side edits + file-side edits
4. Merge via `Apply` — CRDT guarantees convergence
5. Save merged state back to note as `ydoc` event
