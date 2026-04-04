# cmd/mai-docs

Standalone docs binary. Bidirectional sync between doc notes and markdown files.

## Dispatched by

`mai docs` → looks up `docs` in `.maitake/plugins.toml` → execs `mai-docs`

Also handles: `mai check`, `mai refs`, `mai expand` (dispatched as `mai-docs check`, etc.)

## Commands

| Command | What |
|---|---|
| `mai-docs sync` | Sync doc notes ↔ markdown files |
| `mai-docs check` | Validate code refs and wiki links |
| `mai-docs refs <id>` | Find references to a note |
| `mai-docs expand <text>` | Expand [[wiki refs]] |
| `mai-docs daemon` | Watch for file changes (stub — delegates to mai daemon for now) |

## Imports

- `pkg/notes` — Engine interface
- `pkg/docs` — SyncDocs, RegisterAutoSync, Config
- `pkg/git` — Repo interface
- `internal/cli` — shared helpers

## Hook registration

Calls `docs.RegisterAutoSync(engine)` after creating the engine — this wires up CRDT init/update and auto-sync to disk.
