---
name: mai-adr
description: Use when recording architecture decisions (ADRs). Decisions are kind:decision notes attached to code via --target, queryable via mai ls/context, and optionally materialize as markdown files. Covers detection triggers, tier system, comment templates, and anti-patterns.
---

# Mai ADR — Architecture Decision Records

ADRs in maitake are `kind: decision` notes. They're stored in the notes ref, attached to the files they affect via `--target`, and visible to any agent that runs `mai context <file>`.

> **Long ADRs — use the pipe pattern.** Write to `/tmp` first, pipe into `mai create` or `mai add-note`. (See mai-agent skill.)

## Quick start

```bash
# Simple decision — attached to a file
mai adr "Use mutex for token refresh" --target src/auth.ts \
  -d "Chose mutex over single-flight. Single-flight propagates the first
caller's error to all waiters, wrong for transient failures."

# Project-wide decision (no --target)
mai adr "JSON notes, not YAML" \
  -d "JSON for note storage. Standard parsing, cat_sort_uniq merge works
because each note is one self-contained line."
```

`mai adr` is a shortcut for `mai create -k decision`. Both work.

## Tiers — match documentation depth to decision weight

| Tier | Scope | Where | How |
|------|-------|-------|-----|
| **Inline** | Single function/file, easily reversed | Code comment | `// @mai: [[dec-xxxx]] — chose X over Y because Z` |
| **Brief** | Multi-file, moderate impact | `mai create -k decision` | Title + 2-3 sentence rationale |
| **Full ADR** | Architectural, hard to undo | `mai create -k decision` + doc file | Full template with alternatives, consequences |

## Creating decisions

### Brief (most common)

```bash
mai adr "Use Zustand for client state" --target src/store/ \
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

# Search decisions by topic
mai search "token refresh" -k decision

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

## Detection triggers — when to reach for `mai adr`

| Trigger | Example | Tier |
|---------|---------|------|
| Technology/library selection | "PostgreSQL instead of MongoDB" | Brief or Full |
| Architecture pattern choice | "Event sourcing for audit logs" | Full |
| Breaking an existing pattern | "Not using the repository pattern here because..." | Brief |
| Security-related trade-off | "httpOnly cookies vs localStorage for tokens" | Full |
| Performance trade-off | "Denormalizing this table for read perf" | Brief |
| API design choice | "PUT vs PATCH for this endpoint" | Brief |
| Error handling strategy | "Fail fast here instead of retry" | Inline |
| Data structure selection | "Map instead of Object because..." | Inline |

**Quick test:** Would another engineer question this choice? Are there reasonable alternatives? Will forgetting this cause problems? If any → document.

**When NOT to document:** trivial choices (variable names), framework conventions (React patterns in a React app), already covered by an existing decision.

## Comment templates

### Inline (tier 1)

```
// Uses [X] because [reason] (vs [Y]: [why rejected])
```

```rust
// Uses BTreeMap because we need sorted iteration (vs HashMap: faster lookup but unsorted)
```

```typescript
// Parses dates manually because date-fns adds 70KB for 3 operations
// @mai: [[dec-xxxx]]
```

### Y-statement (rapid capture for brief ADRs)

When a full description feels heavy but inline is too light:

> **In the context of** [situation],
> **facing** [concern],
> **we decided** [outcome]
> **and rejected** [alternatives],
> **to achieve** [benefits],
> **accepting that** [trade-offs].

Example:
> **In the context of** session management,
> **facing** horizontal scalability needs,
> **we decided** Redis for session storage
> **and rejected** in-memory and database sessions,
> **to achieve** stateless servers and sub-ms lookups,
> **accepting that** we add a failure dependency.

## Anti-patterns

| Anti-pattern | Fix |
|---|---|
| **"I'll document later"** | Document before implementing. You won't come back. |
| **Missing alternatives** | "We chose X" without saying what else was considered is useless. |
| **Pure description** | "This sorts the array" — explain WHY this sorting approach. |
| **Retroactive rationalization** | Writing ADRs after the fact to justify what's already built. |
| **ADR graveyard** | Decisions exist but nothing references them. Use `// @mai:` in code. |
| **Over-documentation** | Not every variable name needs an ADR. Use the tier system. |

## Review checklist

When reviewing code, check:

- [ ] New dependency or technology? ADR exists?
- [ ] Non-obvious approach? Comment explains why?
- [ ] Breaking an existing pattern? Justification documented?
- [ ] Trade-off made? Both sides documented?

## Minimum viable decision

Three pieces: **what**, **why**, **what-not**.

```bash
mai adr "Use mutex for token refresh" --target src/auth.ts \
  -d "Mutex over single-flight. SF propagates first caller's error to all waiters."
```

## Rules

1. **Attach to what it affects.** `--target src/auth.ts` or `--target src/data/`.
2. **Include rationale.** Not just "we chose X" but "why X, and why not Y."
3. **Decisions stay open** until superseded. They're reference material, not work items.
4. **Close with reason** when superseded — the chain tells the story.
5. **Inline refs in code** — `// @mai: [[dec-xxxx]]` so `mai context` surfaces them.
