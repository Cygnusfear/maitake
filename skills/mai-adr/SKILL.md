---
name: mai-adr
description: Use when recording architecture decisions (ADRs). Combines mai-decision (notes attached to code) with docs materialization and the-archivist's tier system. Decisions are queryable via mai ls, visible in mai context, and optionally materialize as markdown files.
---

# Mai ADR — Architecture Decision Records

ADRs in maitake are `kind: decision` notes. They're stored in the notes ref, attached to the files they affect via `--target`, and visible to any agent that runs `mai context <file>`.

## Quick start

```bash
# Simple decision — attached to a file
mai create "Use mutex for token refresh" -k decision --target src/auth.ts \
  -d "Chose mutex over single-flight. Single-flight propagates the first
caller's error to all waiters, wrong for transient failures."

# Project-wide decision
mai create "JSON notes, not YAML" -k decision \
  -d "JSON for note storage. Standard parsing, cat_sort_uniq merge works
because each note is one self-contained line."
```

## Tiers — match documentation depth to decision weight

| Tier | Scope | Where | How |
|------|-------|-------|-----|
| **Inline** | Single function/file, easily reversed | Code comment | `// @mai: [[dec-xxxx]] — chose X over Y because Z` |
| **Brief** | Multi-file, moderate impact | `mai create -k decision` | Title + 2-3 sentence rationale |
| **Full ADR** | Architectural, hard to undo | `mai create -k decision` + doc file | Full template with alternatives, consequences |

## Creating decisions

### Brief (most common)

```bash
mai create "Use Zustand for client state" -k decision --target src/store/ \
  -d "Minimal API, TypeScript-first, no providers. Rejected Redux (too heavy),
Jotai (atom model less intuitive), Context (prop drilling at scale)."
```

### Full ADR — materializes as a doc file

```bash
mai create "Three-service architecture" -k decision \
  --target docs/decisions/003-three-service-architecture.md \
  -d "# ADR-003: Three-service architecture

## Status
Accepted

## Context
Game needs authoritative physics, persistent state, and AI processing.
Single server can't handle all three at required tick rates.

## Decision
Split into three services: SpacetimeDB (state), Movement Server (physics),
Brain Service (NPC AI). Each owns its domain, communicates via defined contracts.

## Alternatives rejected
- **Monolith**: Can't hit 60Hz physics + AI tick rates on same process
- **Two services**: Physics+AI combined still too heavy for 60Hz

## Consequences
- (+) Each service scales independently
- (+) Clear ownership boundaries
- (-) Cross-service coordination complexity
- (-) Network latency between services"
```

With `docs.sync = "auto"`, this materializes as `docs/decisions/003-three-service-architecture.md`.

### Inline code references

After creating a decision, reference it in code:

```rust
// @mai: [[dec-xxxx]] — three-service split, movement server owns physics
pub struct MovementServer { ... }
```

```typescript
// @mai: [[dec-yyyy]] — Zustand over Redux, see decision for rationale
import { create } from 'zustand';
```

These refs are validated by `mai check` and reverse-lookable via `mai refs <id>`.

## Querying decisions

```bash
# All decisions
mai ls -k decision

# Decisions affecting a specific file
mai context src/auth.ts

# Decisions with a specific tag
mai ls -k decision -l architecture

# All decisions including closed/superseded
mai ls -k decision --status=all

# JSON output for scripts
mai --json ls -k decision
```

## Superseding a decision

Decisions are immutable. When one changes, close the old and create a new:

```bash
mai close dec-1234 -m "Superseded by dec-5678: switched to mutex after error propagation issues"

mai create "Use mutex for token refresh" -k decision --target src/auth.ts \
  -d "Switched from single-flight to mutex. Single-flight propagates first
caller's error to all waiters — wrong for transient failures.
Supersedes: dec-1234"
```

The old decision stays in history. `mai ls -k decision --status=all` shows the full timeline.

## When to record a decision

Ask:
1. **Would another engineer question this choice?** → Document
2. **Are there reasonable alternatives?** → Document why this one
3. **Will forgetting this cause problems later?** → Document

When NOT to document:
- Trivial choices (variable names, indentation)
- Framework conventions (using React patterns in a React app)
- Already documented by an existing decision

## Minimum viable decision

At absolute minimum, a decision needs:

```bash
mai create "TITLE: what was decided" -k decision -d "WHY: rationale. NOT: alternatives rejected."
```

Three pieces: what, why, what-not. Everything else is bonus.

## Rules

1. **Attach to what it affects.** `--target src/auth.ts` or `--target src/data/`.
2. **Include rationale.** Not just "we chose X" but "why X, and why not Y."
3. **Decisions stay open** until superseded. They're reference material, not work items.
4. **Close with reason** when superseded — the chain tells the story.
5. **Inline refs in code** — `// @mai: [[dec-xxxx]]` so `mai context` surfaces them.
