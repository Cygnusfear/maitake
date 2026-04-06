---
name: mai-review
description: Use when performing code review or requesting changes. Leave findings directly on files with acceptance criteria and rejection criteria. Fix agents see findings in-place via mai context.
---

# Mai Review — File-Level Code Review

Review findings go **on the files**, not in a ticket body. The fix agent runs `mai context <file>` and sees exactly what needs fixing — no cross-referencing.

## Opening a review

### Coordinator creates the review ticket

```bash
mai ticket "Review: auth hardening" -p 1 -l review \
  --target src/auth.ts --target src/http.ts \
  -d "Review the auth changes. 6-pass review."
# → tre-5c4a
```

Then delegates to a review agent:

```bash
teams delegate [{
  "text": "Review tre-5c4a. Use mai-review skill. Read mai show tre-5c4a, then check mai context on each target file. Leave findings with --file, include AC and REJECT.",
  "assignee": "reviewer",
  "template": "review"
}]
```

## Reviewing

### 1. Check existing context

```bash
mai show tre-5c4a             # read the review ticket
mai context src/auth.ts       # see existing notes
mai context src/http.ts
mai search "auth" -k decision # find prior decisions about this area
```

### 2. Leave findings on each file

Every finding has three parts:
- **What's wrong** — the problem
- **AC** — what "fixed" looks like (acceptance criteria)
- **REJECT** — what NOT to do (rejection criteria)

> **Long findings — use the pipe pattern.** Write to `/tmp` first, then pipe in.
> Never burn tokens rewriting a finding that failed. (See mai-agent skill for full pattern.)

```bash
mai add-note tre-5c4a --file src/auth.ts \
  "Race condition in token refresh.

AC:
- Mutex or single-flight around token refresh
- Concurrent requests block on in-flight refresh
- No request gets a revoked token

REJECT:
- Simple boolean flag instead of proper synchronization
- Retry loop without backoff
- Silencing the error"
```

```bash
mai add-note tre-5c4a --file src/http.ts \
  "Missing backoff on retry.

AC:
- Exponential backoff with jitter
- Max retry count configurable

REJECT:
- Fixed delay between retries
- No jitter"
```

Use `--line` when you can point to a specific location:

```bash
mai add-note tre-5c4a --file src/auth.ts --line 42 \
  "Race condition starts here — two concurrent callers both see expired token."
```

### 3. Leave a verdict

```bash
mai add-note tre-5c4a "Verdict: changes requested. 2 findings on 2 files."
```

## Fixing

### Fix agent arrives

```bash
mai show tre-5c4a             # read the review
mai context src/auth.ts
# → tre-5c4a [ticket] (open) Review: auth hardening
#   📌 src/auth.ts: Race condition in token refresh.
#   📌 src/auth.ts:42: Race condition starts here...
```

The findings are right there — what's wrong, how to fix it, what not to do.

### Fix agent works and comments

```bash
mai add-note tre-5c4a --file src/auth.ts "Fixed — added single-flight mutex around refresh"
mai add-note tre-5c4a --file src/http.ts "Fixed — exponential backoff with jitter, max 3 retries"
```

## Re-review

### Re-reviewer checks

```bash
mai context src/auth.ts       # see finding + fix comment side by side
mai context src/http.ts
```

### Approve and close

```bash
mai add-note tre-5c4a "Approved. All findings addressed."
mai close tre-5c4a -m "Review complete."
```

## Standalone review artifacts

For reviews that are records (not active work):

```bash
mai review "Architecture review findings" --target src/ \
  -d "Findings from the Q1 architecture review."
```

This creates a `type: artifact` ticket — born closed. Use `mai show` to read it, it won't clutter `mai ls`.

## Rules

1. **Findings go on files with `--file`.** Not in the ticket body.
2. **Every finding has AC and REJECT.** The fix agent needs to know what "done" looks like and what to avoid.
3. **One finding per concern.** Don't lump multiple issues into one comment.
4. **Use `--line` when relevant.** Point to the exact location.
5. **Check context first.** Don't duplicate findings that already exist.
6. **Fix agents comment on the same file.** `--file src/auth.ts "Fixed — added mutex"` so the re-reviewer sees finding + fix together.
