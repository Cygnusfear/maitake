---
name: mai-agent
description: Use when working in any git repo with maitake. Check for notes before touching files, leave notes after meaningful work. This is the base agent contract.
---

# Mai Agent — Arrival and Departure

## On arrival

Before touching any file, check what's known about it:

```bash
mai context <file-you-will-touch>
```

This shows open tickets, warnings, constraints, and review findings targeting that file — including file-located comments from other agents.

Also check project-level rules:

```bash
mai ls -k constraint          # hard rules you must follow
mai ls -k warning             # known fragile areas
```

## During work

Comment on your ticket as you work:

```bash
mai add-note <ticket-id> "what you're doing and why"
```

If your comment is about a specific file:

```bash
mai add-note <ticket-id> --file src/auth.ts "Add mutex around token refresh"
mai add-note <ticket-id> --file src/auth.ts --line 42 "Race condition on this line"
```

## On departure

Close your ticket with a summary:

```bash
mai close <ticket-id> -m "What was done and the result"
```

If you found something fragile that future agents should know:

```bash
mai warn src/auth.ts "Token cache is not thread-safe — must hold mutex during refresh"
```

If you made a decision worth recording:

```bash
mai create "Use single-flight for token refresh" -k decision --target src/auth.ts \
  -d "Chose single-flight over mutex because it coalesces concurrent callers."
```

## Querying

```bash
mai show <id>              # full state of any note
mai ls                     # open work queue (default: open + in_progress)
mai ls -k warning          # all open warnings
mai ready                  # notes with all deps resolved
mai blocked                # notes with unresolved deps
mai dep tree <id>          # dependency tree
mai kinds                  # what kinds of notes exist
mai doctor                 # graph health
```

## Rules

1. **Always check context before editing a file.** Don't ignore warnings, constraints, or review findings.
2. **Always leave a note after meaningful work.** The next agent needs context.
3. **Close things you resolve.** Don't leave the queue dirty.
4. **Use file-located comments** when your comment is about a specific file, not the ticket in general.
