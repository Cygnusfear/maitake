---
name: mai-harness
description: Use when a repo needs durable mai-backed context so every agent sees rules, rationale, and hazards before editing code. Covers promoting chat knowledge into tickets, constraints, warnings, ADRs, artifacts, and reviews; mirroring critical items into AGENTS.md; and enforcing mai context usage through arrival rituals, delegation templates, review gates, and verification.
---

# Mai Harness — Durable Repo Knowledge

Turn transient chat context into durable repo knowledge. Make every agent consume it before editing code.

## Why

Chat context dies. Sessions end. New agents arrive blind. They rediscover decisions, miss warnings, violate constraints, and treat stale docs as truth.

The harness fixes one problem: an agent edits a file before seeing what the repo already knows about it.

## The contract

1. Durable knowledge lives in `mai`, not chat.
2. Critical rules mirror into `AGENTS.md` as physical tripwires.
3. Every agent runs `mai context <file>` before touching that file.
4. Long notes use the pipe pattern — write to `/tmp`, pipe into `mai add-note`. (See mai-agent skill.)
4. Every worker prompt carries the arrival ritual.
5. Verification keeps the harness from rotting.

## Knowledge surfaces

One kind per concern. Pick the narrowest correct one.

| Kind | When to use | Example |
|---|---|---|
| `ticket` | work to do | Fix auth race condition |
| `constraint` | hard rule, must obey | All HTTP calls retry with backoff |
| `warning` | fragile area, workaround exists | Token cache not thread-safe |
| `decision` (ADR) | why X, not Y | Mutex over single-flight |
| `artifact` | research, audit, plan, post-mortem | Token refresh perf analysis |
| `review` | findings with acceptance/rejection criteria | Auth hardening review |
| `AGENTS.md` banner | physical tripwire before any tool runs | Top of directory README |

**Targeting rule.** Attach each note to the narrowest scope that stays correct:

- file → `--target src/auth.ts`
- directory → `--target src/data/`
- project → no `--target`

## The promotion rule

Promote chat knowledge into the repo when any of these hold:

- another agent will need it
- it changes implementation behavior
- it explains a non-obvious choice
- it marks a fragile area
- it constrains future work
- it distinguishes current truth from historical truth
- the human asked for it, or would if they saw it

If none hold, do not create a note. Density matters — one strong note beats five vague ones.

## Kind-selection guide

| The chat said... | Use |
|---|---|
| "everyone must..." | constraint |
| "watch out, this breaks when..." | warning |
| "we chose X because Y, rejected Z" | ADR |
| "here is the full analysis" | artifact |
| "do this work" | ticket |
| "this code is wrong because..." | review finding |
| "this directory is frozen / reference-only / active" | AGENTS.md banner + constraint |

## Encoding workflow

Apply in order.

### 1. Identify durable knowledge

Walk the current conversation and list what survives chat loss. Focus on rules, rationale, hazards, and truth markers.

### 2. Choose the right kind

Use the kind-selection guide. If two kinds seem right, pick the stronger one and link from the weaker.

### 3. Attach to the right scope

Target narrowly. Widen only when the rule genuinely applies wider.

### 4. Mirror critical items into AGENTS.md

Agents grep and read files before they run any `mai` command. Put constraints, directory categories, and arrival rituals at the top of the relevant `AGENTS.md`.

### 5. Wire the arrival ritual

Root `AGENTS.md` must tell every arriving agent to run `mai ls -k constraint`, `mai ls -k warning`, and `mai context <file>` before editing.

### 6. Build a delegation briefing

Every `teams delegate` task begins with the arrival ritual block. Subagents do not inherit the rules — they must be told.

### 7. Verify

A script checks that banners, constraints, rituals, and hooks exist and behave correctly.

### 8. Maintain

When the rule changes, update the note first, then the banner, then the delegation template, then the verification.

## Enforcement ladder

No single layer catches every agent. Stack them.

### Layer 1 — Arrival ritual in root AGENTS.md

Put it on the first screen. Not buried.

```
## Arrival — before touching any file

1. mai ls -k constraint
2. mai ls -k warning
3. mai show <ticket-id>   # if working a ticket
4. mai context <file>     # before reading or editing
```

### Layer 2 — Project-wide constraints

Create them once:

```bash
mai create "Run mai context before editing" -k constraint \
  -d "Every agent runs mai context <file> before editing. Warnings, constraints, and review findings exist for a reason."

mai create "Leave a note after meaningful work" -k constraint \
  -d "Close the ticket. Warn on newly discovered hazards. ADR non-obvious decisions."
```

These surface in `mai ls -k constraint` and in `mai context` for every file.

### Layer 3 — Directory banners

Every meaningful directory gets an `AGENTS.md` banner on the first screen. State what the directory is, whether it is current truth, and what rules apply locally.

### Layer 4 — Delegation template

Every worker prompt starts with the arrival ritual block plus any project-specific discipline. Keep the template in root `AGENTS.md` so coordinators copy-paste rather than improvise.

### Layer 5 — Review gate

Reject work that:

- edited important files without surfacing `mai context`
- made non-obvious choices without an ADR
- discovered hazards without leaving warnings
- landed behavior changes without closing or commenting the ticket

Some behavior resists mechanical enforcement. Review enforcement catches it.

### Layer 6 — Verification script

One script checks:

- arrival ritual exists in root `AGENTS.md`
- delegation template exists
- required project-wide constraints exist
- directory banners exist where expected
- pre-write hook accepts safe notes and rejects unsafe ones

Run it in CI or on demand.

### Layer 7 — Pre-write hook

`.maitake/hooks/pre-write` can reject notes that violate repo rules. Give it:

- the specific danger it guards against
- a clear error message pointing to docs
- an escape hatch (`MAI_NOTE_KIND`, env override) for deliberate use

See `references/templates.md` for a template hook.

## Anti-patterns

| Anti-pattern | Fix |
|---|---|
| Constraint buried only in chat | `mai create -k constraint` |
| ADR with no rationale | Add what, why, and what-not |
| AGENTS.md banner with no matching mai note | Create the note or delete the banner |
| Worker prompt that skips the arrival ritual | Use the delegation template |
| Targetless rule that should be file-scoped | Add `--target` |
| Verification that only checks file existence | Check behavior too |
| Five vague notes where one strong note fits | Delete four, strengthen one |
| Note density so high nobody reads mai context | Prune aggressively |
| Hook in `.maitake/` but no installer in VCS | Ship the installer, not the live dir |

## Maintenance order

When something changes, update in this order:

1. the mai note (constraint / warning / ADR / artifact)
2. the AGENTS.md banner
3. the delegation template, if delegation-visible
4. the verification script
5. the hook installer

## Quick-start checklist

- [ ] list what the chat knows that the repo does not
- [ ] create project-wide constraints for discipline rules
- [ ] ADR every non-obvious choice
- [ ] warn every fragile area
- [ ] artifact every research / audit / plan
- [ ] add arrival ritual to root `AGENTS.md`
- [ ] add delegation template to root `AGENTS.md`
- [ ] banner each meaningful directory's `AGENTS.md`
- [ ] install pre-write hook (with installer)
- [ ] write verification script
- [ ] run verification

## Specializations

- **Templates:** `references/templates.md` — arrival rituals, banners, constraints, ADR commands, delegation briefing, pre-write hook, verification script.
- **Mixed-truth / cutover repos:** `references/mixed-truth-boundaries.md` — active-vs-legacy taxonomy, boundary maps, anti-example preservation, subagent contamination prevention.

## Final rule

If the repo has knowledge worth keeping, do not trust agents to infer it.
Make the repo teach them.
