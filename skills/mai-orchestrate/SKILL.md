---
name: mai-orchestrate
description: Use when coordinating work across multiple agents via teams. Create tickets, delegate, track deps, monitor progress, run reviews — all through mai.
---

# Mai Orchestrate — Teams Coordination

## Creating work

### Tickets

```bash
mai ticket "Fix auth race condition" -p 1 --tags auth --target src/auth.ts \
  -d "Token refresh has a race condition. Add single-flight mutex."
# → tre-5c4a
```

### With dependencies

```bash
mai ticket "Write auth tests" -p 2 --tags test
# → tre-1111

mai ticket "Implement auth fix" -p 1 --tags fix
# → tre-2222

mai dep tre-2222 tre-1111       # fix depends on tests first

mai dep tree tre-2222
# tre-2222 [open] Implement auth fix
# └── tre-1111 [open] Write auth tests
```

### Review tickets

```bash
mai ticket "Review auth changes" -p 1 --tags review \
  --target src/auth.ts --target src/http.ts
# → rev-1234
```

## Delegating

### Implementation worker

```bash
teams delegate [{
  "text": "Execute tre-5c4a. Run: mai show tre-5c4a to read the ticket. Run mai context on target files before working. Comment progress with mai add-note. Close with mai close when done.",
  "assignee": "fix-auth",
  "model": "openai-codex/gpt-5.4"
}]
```

### Review worker

```bash
teams delegate [{
  "text": "Review rev-1234. Use mai-review skill. Leave findings on files with mai add-note --file. Include AC and REJECT in every finding.",
  "assignee": "reviewer",
  "template": "review"
}]
```

### Fix worker (after review)

```bash
teams delegate [{
  "text": "Fix the review findings on rev-1234. Run mai context on each target file to see findings. Fix each one. Comment your fixes with mai add-note --file.",
  "assignee": "fixer",
  "template": "fix"
}]
```

### Research / oracle

```bash
mai create "Investigate token refresh options" -k ticket --tags research \
  -d "Research mutex vs single-flight vs channel-based approaches."
# → res-4567

teams delegate [{
  "text": "Execute res-4567. Research and post findings with mai add-note.",
  "assignee": "oracle",
  "template": "oracle"
}]
```

## What workers do

Every worker follows the same pattern:

```bash
# 1. Read the ticket
mai show <ticket-id>

# 2. Check file context
mai context <target-file>       # see warnings, constraints, review findings

# 3. Work...

# 4. Comment progress (file-specific when relevant)
mai add-note <ticket-id> "progress update"
mai add-note <ticket-id> --file src/auth.ts "added mutex here"

# 5. Close when done
mai close <ticket-id> -m "summary of what was done"
```

## Monitoring

```bash
mai ls                          # open work queue (sorted by priority)
mai ls -k ticket                # only tickets
mai ls --tags review            # review tasks
mai ready                       # unblocked — can start now
mai blocked                     # waiting on deps
mai dep tree <epic-id>          # full work breakdown
mai closed                      # recently completed
mai doctor                      # health stats
```

## Review cycle

The full review-fix-approve cycle:

```bash
# 1. Create review ticket
mai ticket "Review: feature X" --tags review --target src/foo.ts --target src/bar.ts

# 2. Delegate to reviewer
teams delegate [{"text": "Review ...", "template": "review"}]
# → reviewer leaves findings on files with AC/REJECT

# 3. Delegate to fixer
teams delegate [{"text": "Fix review findings on ...", "template": "fix"}]
# → fixer reads mai context, fixes, comments

# 4. Delegate re-review
teams delegate [{"text": "Re-review ...", "template": "review"}]
# → re-reviewer verifies, approves, closes
```

## Linking related work

```bash
mai link tre-5c4a rev-1234      # ticket ↔ review
mai dep tre-impl tre-design     # impl depends on design
mai show tre-5c4a               # see all links and deps
```

## Rules

1. **Create the ticket before delegating.** Workers read `mai show`, not the task text.
2. **Tell workers to check `mai context` on target files.** This is how they see warnings, constraints, and findings.
3. **Use deps to sequence work.** Don't delegate work that depends on unfinished work.
4. **Review workers use `--file` on all findings.** Fix agents see findings in-place.
5. **Link related tickets.** Reviews ↔ implementations, epics ↔ children.
