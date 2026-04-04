# pkg/docs

Bidirectional sync between doc notes and markdown files on disk. Owns all doc-domain logic that was extracted from `pkg/notes` to enforce the substrate boundary.

## Boundaries

- Imports `pkg/notes` (Engine interface) and `pkg/crdt` (CRDT merge)
- Does NOT import `pkg/git` directly — all git access goes through Engine
- Does NOT import other domain packages (`cmd/mai-*`)
- `boundary_test.go` enforces that `pkg/notes` never imports `pkg/crdt` or `pkg/docs`

## What lives here

| File | What |
|---|---|
| `docs.go` | `SyncDocs` — bidirectional sync, frontmatter parsing, tombstones, CRDT merge |
| `hooks.go` | `RegisterAutoSync` — post-write hooks for CRDT init/update + auto-sync to disk |
| `boundary_test.go` | Compile-time boundary enforcement tests |

## Key types

- `Config` — docs sync config (dir, sync mode, watch)
- `SyncResult` — written/imported/updated/conflicts/removed
- `MergeResult` — CRDT merge output (body + YDoc state)

## How auto-sync works

`RegisterAutoSync(engine)` registers a `PostWriteFunc` that fires after every note write:

1. If the note is `kind: doc` and has no YDoc state → `initYDoc` (create CRDT from body)
2. If the note already has YDoc state and body changed → `updateYDoc` (diff + apply ops)
3. If `docs.sync == "auto"` → materialize the file to disk

The engine calls hooks — it never imports this package.

## CRDT merge strategy

Three-way merge via Yjs (wasm):
1. Load base state (last sync point)
2. Create note-side peer: base + note edits
3. Create file-side peer: base + file edits
4. Merge via `Apply` (CRDT guarantees convergence)

`lastsync` events track the merge base to prevent exponential duplication.
