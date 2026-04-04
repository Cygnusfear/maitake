# Mai Harness Templates

Copy-paste blocks for the seven enforcement layers. Adjust paths, wording, and category names to the repo.

---

## 1. Root AGENTS.md arrival ritual

Put this at the very top of the repo's root `AGENTS.md`. Before anything else.

```markdown
## Arrival — before touching any file

1. `mai ls -k constraint`           # hard rules you must follow
2. `mai ls -k warning`              # known fragile areas
3. `mai show <ticket-id>`           # if you are working a ticket
4. `mai context <file>`             # before reading or editing any source file

If you are delegating to a worker, prepend the **Delegation briefing** block
(below) to the worker's task.
```

---

## 2. Directory AGENTS.md banner

Every meaningful directory gets a banner on the first screen of its `AGENTS.md`. Describe what the directory IS, not what it does.

```markdown
<!-- harness banner — do not bury -->
# <path> — <one-line role>

**Category:** ACTIVE | SHARED | LEGACY | RESERVED | TEMPORARY | REFERENCE
**Implementation direction?** yes | no | reference only
**Authoritative map:** see <link to boundary-map ticket or architecture doc>
**Local rules:** <one line, or link to constraint IDs>

Never cite this path as direction if the category says otherwise.
```

If the repo does not need a full taxonomy, use a minimal variant:

```markdown
# <path>

**Role:** <one line>
**Status:** active
**Local rules:** see `mai context <this dir>`
```

---

## 3. Project-wide constraints

Create these once at the start. They surface in `mai ls -k constraint` and in every `mai context <file>`.

```bash
# Mandatory arrival protocol
mai create "Run mai context before editing" -k constraint \
  -d "Every agent runs 'mai context <file>' before reading or editing. Warnings, constraints, and review findings exist for a reason. Skipping this is a review-gate failure."

# Leave knowledge behind
mai create "Leave a note after meaningful work" -k constraint \
  -d "Close the ticket. Add warnings for newly discovered hazards. ADR non-obvious decisions. Artifact research. No silent completions."

# Honor the boundary
mai create "Never derive architecture from legacy paths" -k constraint \
  -d "Active-vs-legacy boundary is canonical. See the boundary-map artifact. If you must cite a legacy path, label it 'reference only, not direction'."

# Delegation discipline
mai create "Subagents arrive with the briefing block" -k constraint \
  -d "Every teams delegate task starts with the arrival ritual and the path taxonomy. Subagents do not inherit session context."
```

Adjust wording to match the repo's voice. Delete the boundary constraint if the repo has no mixed-truth problem.

---

## 4. ADR commands

Attach each decision to the narrowest scope that stays correct.

```bash
# File-level
mai adr "Use mutex for token refresh" --target src/auth.ts \
  -d "Mutex over single-flight. SF propagates first caller's error to all waiters, wrong for transient failures."

# Directory-level
mai adr "Repository layer owns all DB access" --target src/data/ \
  -d "No raw SQL outside src/data/. Testability and migration safety."

# Project-level
mai adr "JSON notes, not YAML" \
  -d "JSON parses in one line. cat_sort_uniq merge works because each note is one self-contained line."
```

The `-d` block should always answer three questions: what, why, and what-not.

---

## 5. Delegation briefing

Prepend this to every `teams delegate` task. Keep the template in root `AGENTS.md` so coordinators copy it, not invent it.

```text
BEFORE TOUCHING ANY FILE:
- mai ls -k constraint
- mai ls -k warning
- mai context <file>   # before reading or editing each file
- if on a ticket: mai show <ticket-id>

LEAVE KNOWLEDGE BEHIND:
- close the ticket with a summary
- mai warn <path> for newly discovered fragility
- mai adr for non-obvious decisions
- mai add-note --file <path> for file-specific findings

PATH TAXONOMY (adjust or delete if the repo has no mixed-truth split):
- ACTIVE / CLEANROOM: current implementation direction
- SHARED: support surface used by active code
- LEGACY: frozen reference only, never direction
- RESERVED: placeholder, no implementation yet
- TEMPORARY: throwaway scaffolding

If you must cite a LEGACY or REFERENCE path, label it explicitly
'reference only, not direction'.

[actual task here]
```

---

## 6. Pre-write hook

Guards against the most common repo-specific contamination at note-write time.

`.maitake/hooks/pre-write`:

```bash
#!/usr/bin/env bash
set -euo pipefail

# The pre-write hook receives note JSON on stdin and metadata via env:
#   MAI_NOTE_KIND      — ticket | constraint | warning | decision | artifact | review
#   MAI_NOTE_TARGETS   — newline-separated target paths (may be empty)

body=$(cat)

# Escape hatch: deliberate override
if [[ "${MAI_LEGACY_OVERRIDE:-0}" == "1" ]]; then
  exit 0
fi

# Example 1: reject unlabeled citations of legacy paths.
# Adjust path prefixes and safe-label list to the repo.
legacy_paths='(^|[/"`'\''` ])(src/old-system/|legacy/|archive/)'
safe_labels='(reference only|not direction|anti-example|historical intent|LEGACY|do not derive)'

if grep -Eq "$legacy_paths" <<<"$body"; then
  if ! grep -Eqi "$safe_labels" <<<"$body"; then
    cat >&2 <<EOF
maitake pre-write: note cites a legacy path without a safe label.

  Dangerous class: legacy/reference paths treated as implementation direction.
  Required: label the citation explicitly, e.g.
    "(reference only, not direction) src/old-system/foo.ts"

  See:
    - root AGENTS.md — path taxonomy
    - mai ls -k constraint | grep -i legacy
    - boundary-map artifact

  Bypass (deliberate only):
    MAI_LEGACY_OVERRIDE=1 mai ...
EOF
    exit 1
  fi
fi

# Example 2: require constraints to have a rationale.
if [[ "${MAI_NOTE_KIND:-}" == "constraint" ]]; then
  if [[ $(wc -c <<<"$body") -lt 80 ]]; then
    echo "maitake pre-write: constraint body too short — explain the rule." >&2
    exit 1
  fi
fi

exit 0
```

Make it executable:

```bash
chmod +x .maitake/hooks/pre-write
```

**Important.** `.maitake/` is gitignored. Ship an **installer** in VCS (`scripts/install-harness-hook`) rather than the live hook, or the hook does not propagate.

---

## 7. Verification script

Run on demand or in CI. The exact checks depend on the repo — treat this as a skeleton.

`scripts/verify-mai-harness`:

```bash
#!/usr/bin/env bash
set -euo pipefail

fail=0
warn() { echo "FAIL: $*" >&2; fail=1; }
ok()   { echo "ok:   $*"; }

# 1. Arrival ritual present in root AGENTS.md
if grep -q "Arrival — before touching any file" AGENTS.md; then
  ok "arrival ritual present"
else
  warn "arrival ritual missing from root AGENTS.md"
fi

# 2. Delegation briefing present
if grep -q "BEFORE TOUCHING ANY FILE" AGENTS.md; then
  ok "delegation briefing present"
else
  warn "delegation briefing missing from root AGENTS.md"
fi

# 3. Required project-wide constraints exist
required_constraints=(
  "Run mai context before editing"
  "Leave a note after meaningful work"
)
for c in "${required_constraints[@]}"; do
  if mai ls -k constraint --status=all 2>/dev/null | grep -qF "$c"; then
    ok "constraint: $c"
  else
    warn "missing constraint: $c"
  fi
done

# 4. Pre-write hook exists and is executable
hook=".maitake/hooks/pre-write"
if [[ -x "$hook" ]]; then
  ok "pre-write hook installed"
else
  warn "pre-write hook missing or not executable: $hook"
fi

# 5. Pre-write hook behavior — safe note accepted
tmp=$(mktemp)
echo '{"kind":"ticket","title":"ok","body":"safe content"}' >"$tmp"
if MAI_NOTE_KIND=ticket "$hook" <"$tmp" >/dev/null 2>&1; then
  ok "hook accepts safe note"
else
  warn "hook rejects safe note"
fi
rm -f "$tmp"

# 6. Directory banners exist where expected (customize list)
expected_banners=(
  "src/active/AGENTS.md"
  "src/legacy/AGENTS.md"
)
for f in "${expected_banners[@]}"; do
  if [[ -f "$f" ]] && grep -q "Category:" "$f"; then
    ok "banner present: $f"
  else
    warn "missing banner: $f"
  fi
done

exit "$fail"
```

---

## 8. Installer for the hook

`.maitake/` is typically gitignored, so ship the hook via an installer.

`scripts/install-harness-hook`:

```bash
#!/usr/bin/env bash
set -euo pipefail

hooks_dir=".maitake/hooks"
src="scripts/harness-pre-write.sh"
dst="$hooks_dir/pre-write"

if [[ ! -d .maitake ]]; then
  echo "run 'mai init' first" >&2
  exit 1
fi

mkdir -p "$hooks_dir"
cp "$src" "$dst"
chmod +x "$dst"
echo "installed $dst"
```

Keep the actual hook body at `scripts/harness-pre-write.sh` under version control.

---

## 9. Boundary map artifact (optional)

For repos with mixed truth, create one authoritative map as an artifact.

```bash
mai artifact "Boundary map — active vs shared vs legacy" \
  -l architecture,harness \
  -d "Classifies every relevant repo path with evidence. Blocks further work until agreed.

Category table:
- ACTIVE:    src/active/, src/new/
- SHARED:    src/shared/, tools/runtime-support/
- LEGACY:    src/old-system/, archive/
- RESERVED:  src/future/
- TEMPORARY: scratch/

Evidence: <links to architecture doc, Cargo.toml / package.json, dep graph>.
Open flags: <any structural hazards found during investigation>."
```

See `references/mixed-truth-boundaries.md` for full procedure.
