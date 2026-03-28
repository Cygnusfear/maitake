# AGENTS.md

## What is hongos

Git-native notes engine + ticket system + code review. One Go binary. Built on `refs/notes/*` — no shadow branches, no worktrees, no external dependencies beyond git.

Mycelium-inspired substrate (openprose/mycelium) reimplemented in Go. Ticket and review semantics layered on top. Event-sourced model — no mutable blobs.

## Architecture

Two layers, one binary:

```
┌──────────────────────────────────┐
│  CLI + commands                   │
│  (ticket, review, note, ls, ...) │
├──────────────────────────────────┤
│  pkg/notes   — notes engine       │
│  pkg/ticket  — ticket semantics   │
│  pkg/review  — review semantics   │
│  pkg/sync    — remote sync        │
│  pkg/guard   — PII/secret gate    │
├──────────────────────────────────┤
│  pkg/git     — git plumbing       │
│  (notes refs, objects, worktrees) │
└──────────────────────────────────┘
```

### Layer rules

- `pkg/git` talks to git only. No domain concepts.
- `pkg/notes` knows about note format, edges, kinds, composting, slots, branch-scope. Does NOT know about tickets or reviews.
- `pkg/ticket` and `pkg/review` build on `pkg/notes`. They never touch git directly.
- `pkg/guard` scans content before any write. Every write path goes through it.
- `pkg/sync` handles remote push/pull and note merge.
- `cmd/` is thin — flag parsing, then calls into packages.

### Hard rules

1. **No god files.** 500 lines is the soft limit. 800 is the hard limit. Split before it grows.
2. **Interfaces between layers.** `pkg/notes` exposes an interface, `pkg/ticket` consumes it. No reaching through.
3. **Every public function has a test.** No exceptions.
4. **Errors are values, not panics.** Return `error`. Wrap with context. Never `log.Fatal` in library code.
5. **No `interface{}` or `any` in public APIs.** Type everything.
6. **Guard every write.** All content passes through `pkg/guard` before hitting git. PII/secrets rejected at write time.
7. **Event-sourced tickets.** Tickets are immutable creation notes + event stream. No mutable blobs. No merge conflicts.
8. **Notes are append-only.** Supersession via `supersedes` header, not mutation.
9. **Performance budget.** Listing/filtering 10,000 notes must complete in <200ms. Build caching/indexing from day 0.
10. **jj support from day 0.** Detect `.jj/`, use change_id edges, handle rewritten commit OIDs.

### Testing

- Unit tests: `*_test.go` next to the code
- Integration tests: `test/` directory, require git binary
- Benchmarks: `*_bench_test.go` for any operation that touches thousands of notes
- Run: `go test ./...`

### Naming

- Package names are short nouns: `notes`, `ticket`, `review`, `git`, `guard`, `sync`
- Types are exported, specific: `NoteRef`, `TicketEvent`, `ReviewFinding`
- Functions say what they do: `ReadNote`, `FoldEvents`, `AttachToFile`
- No abbreviations in public APIs except `ID` and `OID`

### Dependencies

- Standard library preferred
- `git` CLI for plumbing (not libgit2 — too heavy, not needed)
- gitleaks for secret scanning (optional, graceful degradation if missing)
- That's it. No frameworks. No ORMs. No YAML libraries (notes format is simpler than YAML).

### Documentation

- `docs/` for design docs and specs
- `AGENTS.md` in each `pkg/` directory describing purpose and boundaries
- README.md for users
- Go doc comments on every exported symbol
