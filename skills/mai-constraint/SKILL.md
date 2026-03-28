---
name: mai-constraint
description: Use when setting or checking project rules that all agents must follow. Constraints are notes that never close (until the rule changes).
---

# Mai Constraint — Project Rules

## What constraints are

Constraints are hard rules attached to files or the project root. They show up in `mai context` alongside tickets and warnings. Every agent sees them before touching the file.

## Setting constraints

```bash
# File-level constraint
mai create "Must be retryable" -k constraint --target src/http.ts \
  -d "All HTTP calls must have retry with exponential backoff and jitter."

# Directory-level constraint (targets a path prefix)
mai create "No direct DB access" -k constraint --target src/data/ \
  -d "All database access goes through the repository layer. No raw SQL in handlers."

# Project-level constraint (no target)
mai create "All public APIs need auth" -k constraint \
  -d "Every endpoint must validate the auth token. No anonymous access except /health."
```

## Checking constraints

Agents see constraints when they run context:

```bash
mai context src/http.ts
# → con-1234 [constraint] (open) Must be retryable
# →   All HTTP calls must have retry with exponential backoff and jitter.
# → tre-5c4a [ticket] (in_progress) Fix retry logic
```

To see all constraints:

```bash
mai ls -k constraint
```

## Updating constraints

Constraints are notes — update them by closing the old one and creating a new one:

```bash
mai close con-1234 -m "Replaced: retry is now handled by the HTTP middleware"
mai create "Retry handled by middleware" -k constraint --target src/http.ts \
  -d "Do not add retry logic in individual handlers. The middleware handles it."
```

## Rules

1. **Constraints never close** unless the rule genuinely changes.
2. **Be specific.** "Write good code" is not a constraint. "All HTTP calls must retry with backoff" is.
3. **Target the right scope.** File-level for file-specific rules, no target for project-wide rules.
4. **Agents must respect constraints.** If `mai context` shows a constraint, follow it. If you disagree, discuss with the human — don't silently violate it.
