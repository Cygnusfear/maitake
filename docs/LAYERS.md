# Maitake — Layer Architecture

## The full picture

```
┌───────────────────────────────────────────────┐
│ CLI (cmd/mai) │
│ thin: flags → package calls → output │
├───────────────────────────────────────────────┤
│ │
│ pkg/ticket — issues, epics, work items │
│ pkg/review — PRs, findings, verdicts │
│ pkg/doc — documentation on code │
│ pkg/eval — agent behavior measurement │
│ │
│ ─── all build on ─── │
│ │
│ pkg/notes — the substrate │
│ (format, edges, kinds, │
│ slots, branch-scope, index, fold) │
│ │
│ ─── which builds on ─── │
│ │
│ pkg/guard — PII/secret gate │
│ pkg/git — git plumbing │
│ pkg/sync — remote merge │
│ │
└───────────────────────────────────────────────┘
```

## What each layer owns

### pkg/git — plumbing

Talks to git only. No domain concepts.

| Responsibility | How |
|---|---|
| Read/write notes refs | `git notes --ref=<ref> add/show/list` |
| Resolve objects | `git rev-parse` |
| Detect repo state | worktrees, jj, bare, shallow |
| Object iteration | `git notes --ref=<ref> list` |

### pkg/guard — write gate

Every byte that enters a notes ref goes through guard first.

| Check | Source |
|---|---|
| Secrets | gitleaks (if installed) or built-in patterns |
| PII | built-in regex (emails, phones, keys) |
| Format | valid note structure, kind present |
| Size | configurable max per note |

### pkg/notes — the substrate

The core abstraction. Everything above this speaks "notes."

| Concept | What it is |
|---|---|
| Note | headers + body attached to a git object |
| Edge | typed link between notes/objects |
| Kind | open-vocabulary classification |
| Slot | parallel write lane (separate ref) |
| Branch scope | feature-scoped notes namespace |
| Event folding | stale note triage |
| Index | cached fold state for fast queries |
| Supersession | immutable chain of note versions |

### pkg/ticket — issues and work tracking

Event-sourced tickets built on notes.

| Concept | Notes representation |
|---|---|
| Issue/ticket | `kind ticket` creation note |
| Status change | `kind ticket-event` with `field status` |
| Comment | `kind ticket-comment` |
| Label/tag | `kind ticket-event` with `field tags` |
| Milestone | `kind ticket-event` with `field milestone` |
| Assignment | `kind ticket-event` with `field assignee` |
| Dependency | `edge depends-on note:<oid>` |
| Cross-reference | `edge references note:<oid>` |
| Parent/child | `edge child-of note:<oid>` |
| Artifact type | `type artifact` in creation note → born closed |
| Current state | fold(creation + events) |

This replaces: GitHub Issues, Forgejo Issues, Jira tickets, tk shadow-branch tickets.

### pkg/review — PRs and code review

Review graphs built on notes.

| Concept | Notes representation |
|---|---|
| Pull request | `kind review-request` with base/head edges |
| File finding | `kind review` on a file blob with `edge part-of` |
| Acceptance criteria | in the body of a review finding |
| Verdict | `kind review-verdict` (approve / changes-requested) |
| Re-review | new review-request closing the old one |
| Inline comment | `kind review` with line range in body |
| Resolution | close the finding after fix |

This replaces: GitHub PRs, Forgejo MRs, git-appraise.

Agent workflow:
1. Reviewer writes findings directly on files: `mai review find src/auth.ts -m "Fix the race condition. AC: ..."`
2. Implementer runs `mai context src/auth.ts` and sees findings in-place
3. After fixing, implementer closes the finding
4. Reviewer issues verdict

### pkg/doc — documentation on code

Documentation that lives with what it documents.

| Concept | Notes representation |
|---|---|
| File docs | `kind summary` on a file blob |
| Module docs | `kind summary` on a tree (directory) |
| API docs | `kind context` on specific functions (via blob + line edges) |
| Architecture notes | `kind decision` on directories |
| Constraints | `kind constraint` on files/dirs |
| Warnings | `kind warning` on files/dirs |

This replaces: scattered README files, AGENTS.md files (partially), doc comments that go stale.

Key advantage: event folding auto-flags stale docs when the code changes. No more docs that silently rot.

### pkg/eval — agent behavior measurement

Instrument how agents use the system.

| What to measure | How |
|---|---|
| **Read-before-write ratio** | Did the agent run `context` before editing a file? |
| **Note quality** | Did the agent leave a useful note after work? (length, kind, edges present) |
| **Finding→fix rate** | When a review finding is attached to a file, did the next agent fix it? |
| **Stale note generation** | How many notes become stale per session? |
| **Guard rejection rate** | How often do agents try to write PII/secrets? |
| **Ticket hygiene** | Are tickets closed after work? Are comments meaningful? |
| **Context utilization** | When `context` returns warnings/constraints, did the agent respect them? |
| **Event folding behavior** | Do agents close resolved notes after fixing issues? |

Implementation:
- Every mai command logs a structured event to a local eval log
- `mai eval report` aggregates and scores
- `mai eval compare <session-a> <session-b>` diffs agent behavior across sessions
- Eval data stays local (not in notes refs) — it's about the agent, not the repo

Eval log format:
```jsonl
{"ts":"2026-03-28T10:00:00Z","session":"abc","agent":"claude-sonnet","cmd":"context","target":"src/auth.ts","notes_returned":3,"warnings":1}
{"ts":"2026-03-28T10:00:05Z","session":"abc","agent":"claude-sonnet","cmd":"edit","target":"src/auth.ts","context_read":true}
{"ts":"2026-03-28T10:01:00Z","session":"abc","agent":"claude-sonnet","cmd":"note","target":"src/auth.ts","kind":"context","body_len":142,"edges":2}
```

### pkg/sync — remote merge

| Operation | What |
|---|---|
| `sync-init <remote>` | Add push/fetch refspecs for a specific remote |
| `sync-init --remove <remote>` | Remove refspecs |
| `sync` | Push + pull + merge notes ref |
| Merge strategy | Set-union on note blobs. No field-level CRDT. |
| Privacy | No refspecs = no sync. GitHub never sees notes unless configured. |

## What this replaces

| Platform concept | GitHub/Forgejo | Maitake |
|---|---|---|
| Issues | Web UI, API | `mai create`, `mai ls` |
| Labels | Web UI | `tags` in creation note |
| Milestones | Web UI | `milestone` field events |
| PRs | Web UI, branch-based | `mai review request` |
| PR comments | Web UI | `mai review find` (on files) |
| PR approval | Web UI | `mai review verdict` |
| Docs/wiki | Separate wiki or docs/ | `kind summary/context/decision` on code |
| CI status | Webhooks | future: `kind ci-result` on commits |

## What this does NOT replace

- Git hosting (push/pull/clone still needs a remote)
- CI execution (still needs Actions/runners)
- Access control (git SSH/HTTPS auth)
- Web UI for browsing (CLI + agent tooling only, for now)

## Build order

| Phase | Packages | What it unlocks |
|---|---|---|
| 1 | `pkg/git`, `pkg/guard`, `pkg/notes` (core: read, write, list, find, edges) | Basic note operations |
| 2 | `pkg/notes` (slots, branch-scope, index) | Production-grade substrate |
| 3 | `pkg/ticket` (create, events, fold, ls, show, ready, blocked) | Issue tracking |
| 4 | `pkg/review` (request, findings, verdict) | Code review |
| 5 | `pkg/eval` (logging, report, compare) | Agent measurement |
| 6 | `pkg/doc` (summary, context, constraint, warning on files) | Living docs |
| 7 | `pkg/sync` (remote merge, privacy controls) | Multi-machine |
| 8 | jj support, migration from .tickets/ | Ecosystem compat |

Phases 1-3 are the critical path. Phase 5 (eval) should start in parallel with Phase 3 so we're measuring agent behavior from the first real usage.
