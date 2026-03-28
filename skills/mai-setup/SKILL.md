---
name: mai-setup
description: Use when setting up maitake in a repo for the first time, configuring sync to a remote, or troubleshooting the setup.
---

# Mai Setup — Initialization and Sync

## First-time setup

```bash
mai init --remote forgejo --block github.com
```

This does three things:
1. Creates `.maitake/hooks/pre-write` — scans for secrets before every note write
2. Creates `.maitake/config` — configures auto-push remote and blocked hosts
3. Adds `.maitake/` to `.gitignore` — keeps config local to each machine

### What each flag does

| Flag | What | Example |
|---|---|---|
| `--remote <name>` | Push notes to this git remote after every write | `--remote forgejo` |
| `--block <host>` | Never push notes to this host (repeatable) | `--block github.com` |

### Default behavior

- **No remote** → notes are local only, no auto-push
- **Default blocked** → `github.com` (if no `--block` specified)

## Config file

`.maitake/config`:

```
remote forgejo
blocked-host github.com
blocked-host gitlab.com
```

Edit directly or re-run `mai init`.

## How sync works

### Auto-push (after every write)

When a remote is configured, every `mai create`, `mai close`, `mai add-note`, etc. auto-pushes `refs/notes/maitake` to the remote.

If the push is rejected (remote diverged):
1. Fetch + merge using `cat_sort_uniq` (set-union on note lines)
2. Retry push
3. If still fails, warn to stderr — the write still succeeds locally

### Manual sync

```bash
mai sync
```

Fetch + merge + push in one command. Use after cloning a repo that already has notes on the remote.

### Privacy

Notes refs don't push by default — git ignores them. Only the configured remote gets notes. Blocked hosts are checked before every push.

## After cloning a repo with existing notes

```bash
git clone <url>
cd repo
mai init --remote origin
mai sync                  # pulls existing notes from remote
mai ls                    # see the work queue
```

## Guard hooks

`.maitake/hooks/pre-write` runs before every note write. It receives the JSON note content on stdin. Exit non-zero to reject.

Default hook: tries `gitleaks`, falls back to regex patterns for common secrets (AWS keys, GitHub tokens, private keys, JWTs).

Replace with your own:

```bash
# Custom pre-write hook
cat > .maitake/hooks/pre-write << 'EOF'
#!/bin/bash
set -euo pipefail
# Your custom scanning here
my-scanner --stdin
EOF
chmod +x .maitake/hooks/pre-write
```

## Troubleshooting

```bash
mai doctor                    # graph health, note counts, broken edges
mai ls --status=all           # everything including closed
mai kinds                     # what kinds of notes exist
git notes --ref=refs/notes/maitake list   # raw git notes list
```
