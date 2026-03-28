---
name: mai-review
description: Use when performing code review. Leave findings directly on the files that need fixing, with acceptance criteria and rejection criteria. Fix agents see findings in-place via mai context.
---

# Mai Review — File-Level Code Review

## Overview

Review findings go **on the files**, not in a ticket body. The fix agent runs `mai context <file>` and sees exactly what needs attention — no cross-referencing.

## Review workflow

### 1. Coordinator creates the review ticket

```bash
mai ticket "Review: feature-branch changes" --tags review \
  --target src/auth.ts --target src/http.ts \
  -d "Review the auth hardening changes. Use 6-pass review."
```

### 2. Review agent checks existing context

```bash
mai context src/auth.ts     # see what's already known
mai context src/http.ts
```

### 3. Review agent leaves findings on each file

Each finding includes:
- **What's wrong** — the problem
- **AC (Acceptance Criteria)** — what "fixed" looks like
- **REJECT** — what NOT to do

```bash
mai add-note <ticket-id> --file src/auth.ts \
  "Race condition in token refresh.

AC:
- Mutex or single-flight around token refresh
- Concurrent requests block on in-flight refresh
- No request gets a revoked token

REJECT:
- Simple boolean flag instead of proper synchronization
- Retry loop without backoff
- Silencing the error"

mai add-note <ticket-id> --file src/http.ts \
  "Missing backoff on retry.

AC:
- Exponential backoff with jitter
- Max retry count configurable

REJECT:
- Fixed delay between retries
- No jitter"
```

### 4. Review agent leaves a verdict

```bash
mai add-note <ticket-id> "Verdict: changes requested. 2 findings on 2 files."
```

### 5. Fix agent arrives

```bash
mai context src/auth.ts
# → <ticket-id> [ticket] (open) Review: feature-branch changes
#   📌 src/auth.ts: Race condition in token refresh.
#   📌 src/auth.ts: AC: Mutex or single-flight...
```

The fix agent sees the finding, the AC, and the rejection criteria — right there on the file.

### 6. Fix agent works and comments

```bash
mai add-note <ticket-id> --file src/auth.ts "Fixed — added single-flight around refresh"
mai add-note <ticket-id> --file src/http.ts "Fixed — added exponential backoff with jitter"
```

### 7. Re-review and close

```bash
mai context src/auth.ts     # see fix comment alongside finding
mai add-note <ticket-id> "Approved. All findings addressed."
mai close <ticket-id> -m "Review complete, all findings resolved."
```

## Rules

1. **Findings go on files, not in the ticket body.** Use `--file` on every finding.
2. **Every finding has AC and REJECT.** The fix agent needs to know what "done" looks like and what to avoid.
3. **One finding per concern.** Don't lump multiple issues into one comment.
4. **Use line numbers when relevant.** `--line 42` tells the fix agent exactly where to look.
5. **Check context first.** Don't duplicate findings that already exist.
