# mai pr full workflow — implementation plan

Ticket: mai-hymz

## Context
- `commands.go` is 1029 lines (over 800 hard limit) — PR code MUST move to its own file
- git-appraise patterns: accept=resolved comment, reject=unresolved comment, submit=merge+close
- Engine hides git.Repo — submit needs both Engine (close) + Repo (git merge)

## Architecture decisions
- **Split:** All PR commands go in `cmd/mai/pr.go`
- **Repo access:** Add `withEngineAndRepo(fn func(Engine, git.Repo))` helper in main.go — no need to expose Repo through Engine
- **accept/reject** are comments with `resolved=true/false` (same pattern as git-appraise)
- **submit** = checkout target + merge source + close note
- **auto-close** = during `pr list`, if is-ancestor detected, append close event

## Files to change

### 1. cmd/mai/pr.go (NEW — ~350 lines)
Move from commands.go:
- `runPRCreate`
- `runPRList` (with auto-close)
- `prBranches`

Add:
- `runPRShow(e Engine, args []string)` — fold PR, show details + diff summary + comments + review verdict
- `runPRAccept(e Engine, args []string)` — append comment resolved=true
- `runPRReject(e Engine, args []string)` — append comment resolved=false, require message
- `runPRSubmit(e Engine, repo git.Repo, args []string)` — merge + close
- `runPRDiff(e Engine, repo git.Repo, args []string)` — git diff target...source
- `runPRComment(e Engine, args []string)` — alias for add-note on PR

### 2. cmd/mai/main.go
- Add `withEngineAndRepo` helper
- Replace flat pr dispatch with subcommand routing:
  - `pr` (no args) → `runPRList`
  - `pr show <id>` → `runPRShow`
  - `pr accept <id>` → `runPRAccept`
  - `pr reject <id>` → `runPRReject`
  - `pr submit <id>` → `runPRSubmit`
  - `pr diff <id>` → `runPRDiff`
  - `pr comment <id>` → `runPRComment`
  - anything else → `runPRCreate` (title text)

### 3. cmd/mai/render.go
- Add `printPRState(s *State, from, into string, merged bool, diffStat string)` for PR-specific display

### 4. cmd/mai/commands.go
- Remove `runPRCreate`, `runPRList`, `prBranches` (moved to pr.go)

## Commit plan
1. Extract PR code to pr.go + update dispatch (structural, no behavior change)
2. Add pr show + pr diff
3. Add pr accept + pr reject
4. Add pr submit + auto-close
5. Add pr comment

## Test plan
- All existing tests must pass after each commit
- Manual test: create PR, show it, accept, submit, verify auto-close
