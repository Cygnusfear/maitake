---
name: mai-pr
description: Use when creating, reviewing, or merging pull requests through mai. Git-native PRs stored as notes — no GitHub, no Forgejo, no platform lock-in. Covers creation, review (accept/reject), submission (merge), auto-close, and diff inspection.
---

# Mai PR — Git-Native Pull Requests

PRs in maitake are `kind: pr` notes with source and target branches in `targets[]`. Everything lives in `refs/notes/maitake` — syncs with `git push/pull`, merges via set-union.

## Quick start

```bash
# On a feature branch:
git checkout -b feature/auth
# ... make changes, commit ...

mai pr "Add auth middleware" --into main
# → mai-5c4a  feature/auth → main

mai pr                          # list open PRs
mai pr show mai-5c4a            # details + diff summary + review status
mai pr diff mai-5c4a            # full diff
mai pr accept mai-5c4a          # LGTM
mai pr submit mai-5c4a          # merge + close
```

## Creating a PR

```bash
mai pr "title" --into <target>    # target defaults to main
mai pr "title" -d "description"   # with body
mai pr "title" -a reviewer        # assign reviewer
mai pr "title" -l auth,backend    # add tags
```

The source branch is auto-detected from HEAD. Fails on detached HEAD or if source equals target.

If no title is given, auto-generates `feature/auth → main`.

## Listing PRs

```bash
mai pr              # all PRs (open, closed, merged)
mai pr --json       # JSON output
```

**Auto-close:** When listing, any PR whose source branch is already merged into the target gets automatically closed with a comment.

## Showing a PR

```bash
mai pr show <id>            # details + diff summary + review verdict + comments
mai pr show <id> --diff     # include full inline diff
mai pr show <id> --json     # JSON output
```

The show output includes:
- Title, status, branches
- Merge status (✓ merged / open)
- Review verdict (✅ accepted / ❌ changes requested / ⏳ pending)
- Diff summary (`--stat` style)
- All comments with timestamps, authors, and resolved markers

## Reviewing a PR

### Accept

```bash
mai pr accept <id>                    # default message: "LGTM"
mai pr accept <id> -m "Ship it"       # custom message
```

Appends a comment with `resolved: true`.

### Reject

```bash
mai pr reject <id> -m "Race condition in token refresh"
```

Appends a comment with `resolved: false`. Message is **required**.

### Multiple reviewers

Accept and reject stack — the latest resolved comment determines the verdict. Multiple reviewers can all leave accept/reject comments; `pr show` displays all of them.

```bash
mai pr accept <id> -m "LGTM from auth team"
mai pr reject <id> -m "Needs perf testing"    # overrides to rejected
mai pr accept <id> -m "Perf tested, all good" # back to accepted
```

## Commenting on a PR

```bash
mai pr comment <id> -m "Looks good overall"
mai pr comment <id> -m "Race here" --file src/auth.ts --line 42
```

Same as `mai add-note` but scoped to the PR. Supports file-located and line-located comments.

## Diff

```bash
mai pr diff <id>            # full diff (target...source)
mai pr diff <id> --stat     # summary only
```

## Submitting (merging) a PR

```bash
mai pr submit <id>          # merge source into target, close the PR
mai pr submit <id> --force  # skip unresolved comment check
```

Submit does:
1. Checks for unresolved comments (blocks unless `--force`)
2. If already merged → just closes the note
3. Otherwise → `git checkout <target> && git merge <source>` (no-ff)
4. Closes the PR note with a merge message

### Already merged

If the source branch was merged outside of mai (via `git merge`, GitHub, etc.), submit detects it and just closes the note.

## Workflow: coordinator + agents

### 1. Create the PR

```bash
git checkout -b feature/auth
# implement...
git commit -m "Add auth middleware"
mai pr "Add auth middleware" --into main -a reviewer
```

### 2. Review

```bash
mai pr show <id>                # reviewer reads the PR
mai pr diff <id>                # reviewer checks the diff
mai pr comment <id> -m "Missing null check" --file src/auth.ts --line 15
mai pr reject <id> -m "See inline comments"
```

### 3. Fix and re-review

```bash
# implementer fixes...
git commit -m "Fix null check"
mai pr accept <id> -m "Fixed, LGTM"
```

### 4. Merge

```bash
mai pr submit <id>
# → Merged feature/auth into main. mai-5c4a → closed.
```

## Data model

A PR is a note with:
- `kind: "pr"`
- `targets: [sourceBranch, targetBranch]`
- Events for status changes (open → closed)
- Comments for review verdicts (`resolved: true/false`) and discussion

Accept = comment with `resolved: true`
Reject = comment with `resolved: false`
Submit = git merge + close event
Auto-close = close event triggered by merge detection

## Error handling

| Scenario | Result |
|---|---|
| Detached HEAD | `fatal: pr: not on a branch` |
| Source = target | `fatal: pr: source and target are the same branch` |
| `pr show` on a ticket | `fatal: pr show: <id> is not a PR (kind: ticket)` |
| `pr reject` without `-m` | `fatal: pr reject: reason required` |
| `pr submit` with unresolved comments | `fatal: pr submit: unresolved comments exist (use --force)` |
| `pr submit` on closed PR | `fatal: pr submit: <id> is already closed` |
| Merge conflict during submit | `fatal: pr submit: merge failed: <git error>` |

## Key files

- `cmd/mai/pr.go` — all PR subcommands
- `cmd/mai/main.go` — `dispatchPR` routing + `withEngineAndRepo` helper
- `test/pr_test.go` — 29 regression tests
- `pkg/notes/engine.go` — `IsMerged`, `GitBranch`
- `pkg/git/git.go` — `IsAncestor`, `Diff`, `MergeBase`, `MergeRef`
