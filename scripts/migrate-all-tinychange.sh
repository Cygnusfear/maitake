#!/bin/bash
# Migrate .tinychange/ entries to mai-changelog artifacts across all sibling
# projects under ../. Dry-run by default; pass --apply to actually write.
#
# Usage:
#   ./scripts/migrate-all-tinychange.sh           # dry-run preview
#   ./scripts/migrate-all-tinychange.sh --apply   # actually migrate
set -euo pipefail

APPLY=0
if [ "${1:-}" = "--apply" ]; then
  APPLY=1
fi

# Find all sibling projects with a .tinychange directory that has entries
PROJECTS_DIR="$(cd "$(dirname "$0")/../.." && pwd)"

echo "Scanning $PROJECTS_DIR for .tinychange/ directories..."
echo

total_projects=0
total_migrated=0
total_skipped=0

for project in "$PROJECTS_DIR"/*/; do
  project="${project%/}"
  name="$(basename "$project")"
  tcdir="$project/.tinychange"

  [ -d "$tcdir" ] || continue

  entry_count=$(find "$tcdir" -maxdepth 1 -name "*.md" -type f 2>/dev/null | wc -l | xargs)
  [ "$entry_count" -gt 0 ] || continue

  # Must be a git repo
  [ -d "$project/.git" ] || {
    echo "⚠  $name: has .tinychange/ but not a git repo, skipping"
    continue
  }

  total_projects=$((total_projects + 1))

  echo "▸ $name ($entry_count entries in .tinychange/)"

  # Ensure .maitake/ exists for mai to work
  if [ ! -d "$project/.maitake" ]; then
    if [ "$APPLY" -eq 1 ]; then
      (cd "$project" && mai init 2>&1 | sed 's/^/    /')
    else
      echo "    (would run: mai init)"
    fi
  fi

  if [ "$APPLY" -eq 1 ]; then
    (cd "$project" && mai-changelog migrate --dir .tinychange 2>&1 | sed 's/^/    /')
    migrated=$(cd "$project" && mai-changelog ls 2>/dev/null | wc -l | xargs)
    total_migrated=$((total_migrated + migrated))
  else
    (cd "$project" && mai-changelog migrate --dir .tinychange --dry-run 2>&1 | sed 's/^/    /')
  fi
  echo
done

if [ "$total_projects" -eq 0 ]; then
  echo "No projects with .tinychange/ entries found."
  exit 0
fi

echo "─────────────────────────────────────"
if [ "$APPLY" -eq 1 ]; then
  echo "Migrated entries across $total_projects project(s)."
  echo "Total changelog entries now: $total_migrated"
  echo
  echo "Next steps:"
  echo "  1. Review with: cd <project> && mai-changelog ls"
  echo "  2. Render markdown: mai-changelog merge"
  echo "  3. When satisfied, remove old dirs: rm -rf <project>/.tinychange/"
else
  echo "Dry-run complete. $total_projects project(s) would be migrated."
  echo
  echo "To apply: $0 --apply"
fi
