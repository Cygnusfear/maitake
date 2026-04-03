---
name: mai-setup
description: Use when setting up maitake in a repo for the first time, configuring sync to a remote, or troubleshooting the setup.
---

# Mai Setup — Initialization and Sync

## First-time setup

```bash
mai init                            # local only — no remote, no push
mai init --remote forgejo           # enable auto-push to a remote
mai init --remote forgejo --block github.com  # push to forgejo, block github
```

This creates:
1. `.maitake/hooks/pre-write` — scans for secrets before every note write
2. `.maitake/config.toml` — sync remote + blocked hosts
3. `.gitignore` entry — keeps `.maitake/` out of the repo

### What each flag does

| Flag | What | Example |
|---|---|---|
| `--remote <name>` | Push notes to this git remote after every write | `--remote forgejo` |
| `--block <host>` | Never push notes to this host (repeatable) | `--block github.com` |

### Default behavior

- **No `--remote`** → notes are local only, nothing pushes anywhere
- **Default blocked** → `github.com` (even when a remote is configured)
- **No flags at all** → fully local, private, zero network activity

## Config file

`.maitake/config.toml`:

```toml
[sync]
remote = "forgejo"
blocked-hosts = ["github.com", "gitlab.com"]

[docs]
sync = "auto"      # "auto" | "manual" | "off"
dir = ".mai-docs"

[hooks]
pre-write = true
post-push = true
```

Edit directly or re-run `mai init`. Legacy flat-format config files are still read for backwards compatibility.

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

Git notes don't push with `git push` — git ignores `refs/notes/*` by default.
maitake only pushes to the remote you configure in `.maitake/config.toml`.
Blocked hosts are checked before every push. No remote configured = nothing
leaves your machine.

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
