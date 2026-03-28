# pkg/git

Thin wrapper around git CLI plumbing. This package talks to git and returns structured Go types.

## Boundaries

- NO domain concepts (no notes, no tickets, no reviews)
- NO note format knowledge
- Consumers use the `Repo` interface — never shell out to git directly

## Key types

- `OID` — 40-char hex SHA
- `NotesRef` — fully qualified notes reference (e.g. `refs/notes/maitake`)
- `Repo` — interface for all git operations
- `RealRepo` — implementation that shells out to git CLI
- `MockRepo` — in-memory implementation for testing consumers

## Testing

- Unit tests mock the command runner
- Integration tests create real temp git repos
