# internal/cli

Shared CLI helpers for `mai` and all `mai-*` plugin binaries. Internal package — not importable by external consumers.

## What lives here

- `Fatal(format, args...)` — print error and exit
- `PrintJSON(v)` — write indented JSON to stdout
- `ParseFlags(args)` → `(FlagSet, positional)` — common mai flag parsing
- `FlagSet` — parsed flags struct (kind, title, priority, assignee, tags, targets, body, status)

## Why internal

Every plugin binary needs the same flag parsing and output helpers. Duplicating them would drift. `internal/cli` keeps them in one place while preventing external import (Go's `internal` visibility rule).

## Adding helpers

If a helper is needed by 2+ binaries, add it here. If it's specific to one binary, keep it in that binary's package.
