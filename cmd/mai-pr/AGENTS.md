# cmd/mai-pr

Standalone PR binary. Git-native pull requests — no GitHub, no Forgejo, no platform lock-in.

## Dispatched by

`mai pr` → looks up `pr` in `.maitake/plugins.toml` → execs `mai-pr`

## Commands

| Command | What |
|---|---|
| `mai-pr "title" --into main` | Create a PR |
| `mai-pr` (no args) | List PRs (auto-closes merged ones) |
| `mai-pr show <id>` | PR details + diff summary |
| `mai-pr diff <id>` | Full diff |
| `mai-pr accept <id>` | LGTM |
| `mai-pr reject <id> -m reason` | Request changes |
| `mai-pr submit <id>` | Merge + close |
| `mai-pr comment <id> -m msg` | Add comment |

## Imports

- `pkg/notes` — Engine interface (create/fold/find PRs)
- `pkg/git` — Repo interface (diff, merge, branch detection)
- `internal/cli` — shared helpers

Does NOT import `pkg/docs`, `pkg/crdt`, or other `cmd/mai-*` packages.

## Environment

Receives from mai dispatcher:
- `MAI_REPO_PATH` — repo root
- `MAI_MAITAKE_DIR` — .maitake/ path
- `MAI_JSON` — "1" for JSON output

Falls back to git repo discovery from cwd if env not set.
