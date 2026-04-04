#!/bin/bash
# Enforces the substrate boundary: pkg/notes must not import domain packages.
# Run in CI or locally: ./scripts/check-boundary.sh
set -euo pipefail

FAIL=0

check_no_import() {
  local pkg="$1"
  local forbidden="$2"
  local label="$3"

  imports=$(go list -f '{{join .Imports "\n"}}' "$pkg" 2>/dev/null | grep -E "$forbidden" || true)
  if [ -n "$imports" ]; then
    echo "FAIL: $label"
    echo "  $pkg imports:"
    echo "$imports" | sed 's/^/    /'
    FAIL=1
  fi
}

echo "Checking substrate boundaries..."

# pkg/notes must not import domain packages
check_no_import "./pkg/notes/" "pkg/(crdt|docs|pr|changelog)" \
  "pkg/notes imports domain packages — doc/CRDT/PR logic must not live in the substrate"

# cmd/mai must not import pkg/crdt directly
check_no_import "./cmd/mai/" "pkg/crdt" \
  "cmd/mai imports pkg/crdt directly — domain logic should go through pkg/docs"

if [ "$FAIL" -eq 0 ]; then
  echo "✓ All boundaries clean."
else
  echo ""
  echo "Boundary violations found. Fix before merging."
  exit 1
fi
