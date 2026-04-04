# cmd/mai-changelog

Standalone changelog binary. Replaces tinychange — stores changelog entries as mai artifacts with zero files in the working tree.

## Dispatched by

`mai changelog` → looks up `changelog` in `.maitake/plugins.toml` → execs `mai-changelog`

## Commands

| Command | What |
|---|---|
| `mai-changelog new "desc" -k fix` | Create a changelog entry (artifact + tags) |
| `mai-changelog ls` | List unreleased entries |
| `mai-changelog merge` | Render markdown changelog to stdout |
| `mai-changelog merge --output FILE` | Write changelog to file |

## How entries are stored

Each entry is a `kind: artifact` note tagged `changelog,<category>`. Born closed — doesn't appear in `mai ls`.

Categories: fix, feat, chore, security, docs, refactor, perf, deploy, test

## Imports

- `pkg/notes` — Engine interface (create artifacts, query by tag)
- `pkg/git` — Repo interface
- `internal/cli` — shared helpers

Does NOT import `pkg/docs`, `pkg/crdt`, or other `cmd/mai-*` packages.

## vs tinychange

| | tinychange | mai-changelog |
|---|---|---|
| Storage | .tinychange/*.md files in working tree | git notes (invisible) |
| Author | frontmatter field | automatic (git user) |
| Query | grep files | `mai ls -k artifact -l changelog` |
| Merge | `tinychange merge` → changelog.md | `mai-changelog merge` → stdout or file |
