# pkg/notes

The substrate. Creates notes with IDs, appends events and comments, folds current state, queries by kind/status/target/tag. Built on `pkg/git` and `pkg/guard`.

## Boundaries

- Knows about note format, edges, kinds, event folding, slots, branch-scope
- Does NOT know about tickets, reviews, docs, CRDTs, or changelogs — those are just kinds
- Consumers see the `Engine` interface (19 methods)
- **Must NOT import** `pkg/crdt`, `pkg/docs`, or any domain package
- Boundary enforced by `pkg/docs/boundary_test.go` and `scripts/check-boundary.sh`

## Key types

- `Engine` — interface consumed by all domain tools
- `RealEngine` — implementation with git repo, guard hooks, in-memory index
- `Note` — single JSON line in a git note
- `State` — computed by folding a note's event stream
- `PostWriteFunc` — callback hook for external packages (e.g. pkg/docs CRDT)
- `Config` — loaded from `.maitake/config.toml`

## Plugin support

- `LoadPlugins` / `ResolvePlugin` / `WriteDefaultPlugins` — plugin manifest system
- Manifest lives at `.maitake/plugins.toml`
- `mai init` writes defaults; users add third-party tools

## Engine hooks

Domain packages register via `engine.OnPostWrite(fn)`. Hooks fire after every `Create` or `Append`. The engine never calls domain code directly — it just fires callbacks.

```go
engine.OnPostWrite(func(e Engine, noteID string, ref git.NotesRef, oid git.OID) {
    // e.g. CRDT state init, auto-sync to disk
})
```

`AppendRaw(ref, oid, data, note)` lets hooks write additional notes (e.g. ydoc events) without going through the full Create/Append flow.
