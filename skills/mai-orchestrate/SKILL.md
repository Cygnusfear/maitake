---
name: mai-orchestrate
description: Use when coordinating work across multiple agents via teams. Create tickets, delegate, track deps, monitor progress through mai.
---

# Mai Orchestrate — Teams Coordination

## Creating work

```bash
# Implementation ticket
mai ticket "Fix auth race condition" -p 1 --tags auth --target src/auth.ts \
  -d "Token refresh has a race condition. Add single-flight mutex."

# Review ticket (artifact — born closed until started by worker)
mai review "Review auth changes" --tags review --target src/auth.ts

# Research task
mai create "Investigate token refresh options" -k ticket --tags research
```

## Dependencies

Structure work with dependencies so agents know what to do first:

```bash
mai ticket "Write tests" -p 2 --tags test
# → tre-1111

mai ticket "Implement fix" -p 1 --tags fix
# → tre-2222

mai dep tre-2222 tre-1111     # fix depends on tests being written first

mai dep tree tre-2222         # visualize
# tre-2222 [open] Implement fix
# └── tre-1111 [open] Write tests

mai ready                     # tre-1111 is ready (no deps)
mai blocked                   # tre-2222 is blocked (waiting on tre-1111)
```

## Delegating to teams

```bash
# Create the ticket first, then delegate
mai ticket "Fix auth race condition" -p 1 --tags auth --target src/auth.ts
# → tre-5c4a

teams delegate [{
  "text": "Execute tre-5c4a. Read it with mai show tre-5c4a. Check mai context on target files before working.",
  "assignee": "fix-auth",
  "model": "openai-codex/gpt-5.4"
}]
```

### What the worker does

```bash
# 1. Read the ticket
mai show tre-5c4a

# 2. Check file context (warnings, constraints, existing findings)
mai context src/auth.ts

# 3. Work...

# 4. Comment progress
mai add-note tre-5c4a --file src/auth.ts "Added single-flight mutex around refresh"

# 5. Close
mai close tre-5c4a -m "Fixed — single-flight mutex added, tests pass"
```

### Review delegation

```bash
mai ticket "Review auth changes" --tags review --target src/auth.ts
# → rev-1234

teams delegate [{
  "text": "Execute rev-1234. Use mai-review skill. Leave findings on files with --file. Include AC and REJECT in every finding.",
  "assignee": "reviewer",
  "template": "review"
}]
```

## Monitoring

```bash
mai ls                     # open work queue
mai ls -k ticket           # only tickets
mai ls --tags review       # only review tasks
mai ready                  # what can start next
mai blocked                # what's stuck
mai dep tree <epic-id>     # see the full work breakdown
mai closed                 # recently completed
mai doctor                 # health stats
```

## Linking related work

```bash
mai link tre-5c4a rev-1234            # ticket ↔ review
mai dep tre-impl tre-design           # impl depends on design
mai show tre-5c4a                     # see all links and deps
```

## Rules

1. **Create the ticket before delegating.** The worker reads `mai show`, not the task text.
2. **Tell workers to check `mai context` on target files.** This is how they see warnings, constraints, and review findings.
3. **Use deps to sequence work.** Don't delegate something that depends on unfinished work.
4. **Review workers use `--file` on all findings.** This is how fix agents see findings in-place.
