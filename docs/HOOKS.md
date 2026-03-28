---
mai-id: mai-epky
---
# Maitake — Hook System

## Overview

Maitake uses executable hooks for extensible behavior at key lifecycle points. Same contract as git hooks: executable files in `.maitake/hooks/`, receive context on stdin, exit code controls behavior.

## Hook directory

```
.maitake/hooks/
├── pre-write        ← before a note enters the notes ref
├── pre-push         ← before notes ref is pushed to a remote
├── post-write       ← after a note is written (logging, notifications)
└── post-close       ← after a note is closed
```

`.maitake/` is gitignored. Hooks are local to each machine. Teams share hooks via a `hooks/` directory in the repo root that `mai init` copies from (same pattern as `.githooks/`).

## Contract

| Property | Value |
|---|---|
| Location | `.maitake/hooks/<name>` |
| Executable | any language — bash, python, go binary, whatever |
| Stdin | note content (for `pre-write`) or context-specific data |
| Stdout | ignored (use stderr for messages) |
| Stderr | shown to the user on rejection |
| Exit 0 | allow / success |
| Exit non-zero | reject / abort |
| Missing hook | treated as exit 0 (allow) |
| Non-executable hook | skipped with warning |
| Timeout | 10s default, configurable in `.maitake/config` |

## pre-write

Runs before every note write. Receives the full note content on stdin. If it exits non-zero, the write is rejected and the stderr message is shown to the user.

**Use cases:**
- Secret/PII scanning
- Content policy enforcement
- Size limits
- Format validation

**Environment variables:**

```
MAI_TARGET_OID=<oid>        # git object the note will attach to
MAI_TARGET_PATH=<path>      # file path (if target is a file, empty otherwise)
MAI_NOTE_KIND=<kind>        # the note's kind header
MAI_NOTE_SLOT=<slot>        # slot name (empty = default)
MAI_NOTE_REF=<ref>          # notes ref being written to
```

**Default hook (shipped with `mai init`):**

```bash
#!/usr/bin/env bash
set -euo pipefail

# Gitleaks if available
if command -v gitleaks &>/dev/null; then
    gitleaks detect --pipe --no-banner 2>&1
    exit $?
fi

# Fallback: catch the obvious stuff
content=$(cat)
patterns=(
    'AKIA[0-9A-Z]{16}'
    '-----BEGIN.*PRIVATE KEY-----'
    'ghp_[A-Za-z0-9]{36}'
    'gho_[A-Za-z0-9]{36}'
    'sk-[A-Za-z0-9]{48}'
    'eyJ[A-Za-z0-9_-]*\.eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*'
)

for pattern in "${patterns[@]}"; do
    if echo "$content" | grep -qE "$pattern"; then
        echo "maitake pre-write: possible secret detected (pattern: $pattern)" >&2
        echo "Use --skip-hooks to bypass (not recommended)" >&2
        exit 1
    fi
done

exit 0
```

## pre-push

Runs before notes refs are pushed to a remote. Receives the remote name and URL on stdin. Use for last-chance scanning of all outbound note content.

**Environment variables:**

```
MAI_REMOTE_NAME=<name>      # e.g., "origin", "forgejo"
MAI_REMOTE_URL=<url>        # remote URL
MAI_REFS=<refs>             # space-separated notes refs being pushed
```

## post-write

Runs after a note is successfully written. Non-blocking — exit code is logged but does not affect the write.

**Use cases:**
- Eval logging (record what the agent wrote)
- Notifications
- External integrations

**Environment variables:**

Same as `pre-write` plus:

```
MAI_NOTE_OID=<oid>          # the newly created note blob OID
MAI_AUTHOR=<name>           # git user.name
```

## post-close

Runs after a note is closed. Non-blocking.

**Environment variables:**

```
MAI_CLOSED_OID=<oid>        # the closed note's OID
MAI_TARGET_PATH=<path>      # file path (if applicable)
MAI_NOTE_KIND=<kind>        # the closed note's kind
```

## Bypass

```bash
mai note src/auth.ts -k warning -m "..." --skip-hooks
```

Skipping hooks is logged as a `kind observation` note on the repo root:

```
kind observation
title Hooks bypassed

pre-write hook skipped by <user> at <timestamp>.
```

This means bypasses are auditable.

## Sharing hooks across a team

Put hook scripts in a `hooks/` directory in the repo root (tracked in git). Run `mai init` to copy them into `.maitake/hooks/`:

```
repo-root/
├── hooks/
│   ├── pre-write      ← tracked, shared via git
│   └── pre-push
└── .maitake/
    └── hooks/
        ├── pre-write  ← local copy, executable
        └── pre-push
```

`mai init` copies from `hooks/` → `.maitake/hooks/` and sets executable bits.

## Configuration

`.maitake/config` (gitignored):

```
[hooks]
timeout = 10        # seconds, per hook
skip-missing = true # don't warn about missing hooks (default: true)
```

## pkg/guard implementation

With the hook system, `pkg/guard` is thin:

```go
package guard

// RunHook executes a hook by name with content on stdin.
// Returns nil if hook passes or doesn't exist.
// Returns error with stderr message if hook rejects.
func RunHook(maitakeDir string, hookName string, content []byte, env map[string]string) error

// HookExists checks if a hook is installed and executable.
func HookExists(maitakeDir string, hookName string) bool
```

That's the entire package.

