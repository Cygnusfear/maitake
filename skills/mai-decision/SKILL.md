---
name: mai-decision
description: Use when recording architecture decisions, design choices, or any decision with rationale that should be preserved. Decisions are notes attached to the code they affect.
---

# Mai Decision — Architecture Decision Records

Decisions are notes with `kind: decision` attached to the files or directories they affect. They persist in the notes ref and show up in `mai context` so future agents understand why something was built the way it was.

## Recording a decision

```bash
mai create "Use single-flight for token refresh" -k decision --target src/auth.ts \
  -d "Chose single-flight over mutex because it coalesces concurrent callers
into one refresh call instead of serializing them. Mutex would work but
wastes time — N concurrent callers would do N sequential refreshes.
Single-flight does one refresh and returns the result to all N callers."
```

```bash
mai create "Repository pattern for data access" -k decision --target src/data/ \
  -d "All database access goes through repository interfaces.
Rationale: testability (mock the repo), swappability (change DB without
touching handlers), and enforces the boundary between HTTP and storage."
```

## Project-level decisions

```bash
mai create "JSON notes, not YAML" -k decision \
  -d "Using JSON for note storage instead of YAML.
Rationale: standard parsing, cat_sort_uniq merge works because each
note is one self-contained line, no custom parser to maintain."
```

## How agents see decisions

```bash
mai context src/auth.ts
# → dec-1234 [decision] (open) Use single-flight for token refresh
# →   Chose single-flight over mutex because...

mai ls -k decision
# → dec-1234 [open] Use single-flight for token refresh
# → dec-5678 [open] Repository pattern for data access
# → dec-9012 [open] JSON notes, not YAML
```

## Updating a decision

When a decision changes, close the old one and create a new one:

```bash
mai close dec-1234 -m "Superseded: switched to mutex after discovering single-flight doesn't handle errors well"
mai create "Use mutex for token refresh" -k decision --target src/auth.ts \
  -d "Switched from single-flight to mutex. Single-flight propagates the
first caller's error to all waiters, which is wrong for transient failures."
```

The old decision stays in history (`mai closed`, `mai ls --status=all`).

## When to record decisions

- Choosing between alternatives (mutex vs single-flight vs channel)
- Selecting a library or tool
- Designing an API shape
- Setting a project convention
- Anything where "why did we do it this way?" will be asked later

## Rules

1. **Attach to what it affects.** `--target src/auth.ts` or `--target src/data/`.
2. **Include rationale.** Not just "we chose X" but "we chose X because Y, and not Z because W."
3. **Keep decisions open** until they're superseded.
4. **Close with reason** when superseded — the history tells the story.
