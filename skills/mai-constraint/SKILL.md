---
name: mai-constraint
description: Use when setting or checking project rules that all agents must follow. Constraints show up in mai context alongside tickets and warnings. They stay open until the rule changes.
---

# Mai Constraint — Project Rules

Constraints are hard rules attached to files, directories, or the project. Every agent sees them when they run `mai context`.

> **Long constraints — use the pipe pattern.** Write to `/tmp` first, pipe in. (See mai-agent skill.)

## Setting constraints

```bash
# On a specific file
mai create "Must be retryable" -k constraint --target src/http.ts \
  -d "All HTTP calls must retry with exponential backoff and jitter."

# On a directory
mai create "No direct DB access" -k constraint --target src/data/ \
  -d "All database access goes through the repository layer. No raw SQL."

# Project-wide (no target)
mai create "All public APIs need auth" -k constraint \
  -d "Every endpoint validates the auth token. No anonymous access except /health."
```

## How agents see them

```bash
mai context src/http.ts
# → con-1234 [constraint] (open) Must be retryable
# →   All HTTP calls must retry with exponential backoff and jitter.
# → tre-5c4a [ticket] (in_progress) Fix retry logic

mai ls -k constraint
# → con-1234 [open] Must be retryable
# → con-5678 [open] No direct DB access
# → con-9012 [open] All public APIs need auth
```

## Updating constraints

Close the old one with a reason, create the new one:

```bash
mai close con-1234 -m "Replaced: retry now handled by HTTP middleware"
mai create "Retry handled by middleware" -k constraint --target src/http.ts \
  -d "Do not add retry in handlers. The middleware does it."
```

## Warnings vs constraints

| | Warning | Constraint |
|---|---|---|
| Purpose | "watch out for this" | "you must do this" |
| Duration | until the fragile thing is fixed | until the rule changes |
| Severity | advisory | mandatory |
| Example | "Token cache has a race condition" | "All HTTP calls must retry" |

```bash
mai warn src/auth.ts "Token cache race condition — hold mutex during refresh"
mai create "Must hold mutex during token operations" -k constraint --target src/auth.ts \
  -d "All token cache reads and writes must hold the refresh mutex."
```

## Rules

1. **Constraints stay open** unless the rule genuinely changes.
2. **Be specific.** Not "write good code." Yes "all HTTP calls must retry with backoff."
3. **Target the right scope.** File for file rules, no target for project rules.
4. **Agents must respect constraints.** If `mai context` shows one, follow it. Disagree? Discuss with the human first.
