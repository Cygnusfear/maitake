# Mixed-Truth / Cutover Repos

A specialization of the mai harness for repos where different directories have different trust levels — active vs. legacy, current vs. reference, shared vs. retired.

## When to apply this

Use when any of these are true:

- the repo has **legacy** and **target** code side by side
- different directories have different trust levels
- some directories are active, some frozen, some placeholders
- subagents are likely to grep or read files before getting the right context
- architecture decisions must stay attached to the code they affect
- the human wants the repo itself to teach current truth, not the chat session

If the repo has a single code path and no mixed reality, skip this document. Stick to the base harness.

## The failure mode this guards against

An agent greps the repo, reads a legacy directory, and treats its architecture as target direction. The error propagates into design docs, explainers, plans, and eventually implementation. The contamination survives chat loss because the agent wrote it down.

The harness blocks this at five layers: boundary map, constraints, banners, delegation briefing, and pre-write hook.

## Step 1 — Classify the repo first

Do not start by writing constraints. First answer:

> Which paths are safe to derive direction from, and which are not?

Create a small taxonomy. Keep categories explicit and stable. A default:

| Category | Meaning |
|---|---|
| **ACTIVE** / **CLEANROOM** | current implementation direction |
| **SHARED** | valid support surface used by active code |
| **LEGACY** | frozen reference only; never direction |
| **RESERVED** | placeholder for future implementation |
| **TEMPORARY** | throwaway scaffolding; production must not depend on it |
| **REFERENCE** | docs / spec / source corpus only |

Only invent a category if it changes behavior. Do not create categories for aesthetics.

## Step 2 — Investigate before tagging

If a path's status is uncertain, do not guess. Investigate:

- read the nearest `AGENTS.md`
- read the governing spec / architecture docs
- check the actual dependency graph (`Cargo.toml`, package manifests, imports)
- verify the file or directory **exists**

A taxonomy tag must satisfy all three:

1. **cited** from the governing docs / architecture
2. **verified** against actual dependency reality
3. **exists** on disk

If any of those fail, mark the path `unclear — needs grounding` until resolved.

## Step 3 — Create the boundary-map artifact

One authoritative artifact records the taxonomy and its evidence:

```bash
mai artifact "Boundary map — active vs shared vs legacy" \
  -l architecture,harness \
  -d "<category table + evidence pointers + open flags>"
```

Then constraints, banners, and briefings cite this artifact. The artifact is the seed from which the rest grows.

## Step 4 — Per-directory constraints

Create one constraint per meaningful directory or root. Examples:

```bash
mai create "LEGACY — frozen reference, never direction" -k constraint \
  --target src/old-system/ \
  -d "Read for historical intent only. Never cite as implementation direction without an explicit 'reference only, not direction' label."

mai create "ACTIVE — target architecture" -k constraint \
  --target src/new-system/ \
  -d "Current implementation direction. Evidence here is valid for design and implementation."

mai create "SHARED — support surface used by active code" -k constraint \
  --target tools/runtime-support/ \
  -d "Valid implementation surface. Update with care — multiple active systems depend on it."
```

Add project-wide constraints for the discipline rules:

```bash
mai create "Never derive architecture from legacy paths" -k constraint \
  -d "Active-vs-legacy boundary is canonical. See the boundary-map artifact."

mai create "Mandatory arrival protocol" -k constraint \
  -d "At session start: mai ls -k constraint, read the boundary map, then mai context <file> before touching any file."
```

## Step 5 — ADR the decisions

Constraints say **what**. ADRs say **why**.

Create at least two ADRs:

```bash
mai adr "Repo cutover — active code replaces legacy" --target docs/architecture/ \
  -d "This repo is being rebuilt in new directories. Old directories remain readable as source corpus only."

mai adr "Legacy reference-only discipline for subagents" --target src/ \
  -d "Subagents arrive without the full conversation. They must not derive architecture from legacy/reference code."
```

Attach ADRs to the broadest path they govern so `mai context` finds them early.

## Step 6 — Physical banners in AGENTS.md

`mai` is not enough. Agents often read files before running `mai context`. Put the taxonomy into the filesystem.

Every meaningful directory states its category on the first screen of its `AGENTS.md`:

```markdown
# src/old-system — LEGACY

**Category:** LEGACY
**Implementation direction?** no — reference only
**Authoritative map:** see boundary-map artifact
**Replacement:** src/new-system/

This directory is frozen. Read for historical intent. Never cite as
implementation direction without an explicit "reference only" label.
Active code lives in src/new-system/.
```

A good banner answers:

- what category this path is in
- whether it is valid implementation direction
- where to look for the authoritative boundary map
- what the replacement path is, if any

## Step 7 — Subagent briefing block

This is the most important operational layer. Subagents do not know the taxonomy unless you tell them.

Every `teams delegate` task starts with:

```text
BEFORE TOUCHING ANY FILE:
- mai ls -k constraint
- read the boundary-map artifact
- mai context <file> before reading or editing each file

PATH TAXONOMY:
- ACTIVE/CLEANROOM: <list paths>
- SHARED:           <list paths>
- LEGACY:           <list paths>
- RESERVED:         <list paths>
- TEMPORARY:        <list paths>

LEGACY paths are reference only. Never cite them as implementation direction.
If you must mention one, label it explicitly: "reference only, not direction".

[actual task here]
```

## Step 8 — Pre-write hook for contamination

`.maitake/hooks/pre-write` scans new note content for legacy-path citations without a safe label. See the hook template in `references/templates.md`.

The hook rejects unlabeled citations and prints where to look. It gives a bypass (`MAI_LEGACY_OVERRIDE=1`) for deliberate use.

## Step 9 — Verification

The verification script checks:

- every classified directory has the right banner in its `AGENTS.md`
- every classified directory has a matching constraint
- project-wide constraints exist
- the pre-write hook accepts safe labeled citations
- the pre-write hook rejects unlabeled citations

Run it in CI. That is what keeps the harness from rotting.

## Step 10 — Keep structural hazards visible

If the investigation surfaces dangerous mismatches, do **not** hide them. Flag them.

Common hazards:

- a shared crate living in a legacy folder
- stale path dependencies in manifests
- docs that describe retired architecture as live
- empty placeholder directories that look active
- generated or broken paths that no longer exist

The response:

1. flag
2. investigate
3. update docs / taxonomy
4. flag again if still a structural risk

Do not let the harness paper over a real repo problem.

## Step 11 — Preserve rejected work as anti-examples

If an artifact or explainer got contaminated, do not silently delete it. Keep it only if clearly labeled:

- `REJECTED / CONTAMINATED`
- `DO NOT USE AS TRUTH`
- `anti-example`

This preserves the failure mode without letting it resurface as truth.

## Maintenance

Update whenever:

- a directory changes category
- a replacement path becomes real
- a temporary path graduates or is deleted
- a new subagent failure class appears
- the arrival ritual needs another tripwire

Update in this order:

1. boundary-map artifact
2. root `AGENTS.md` taxonomy
3. directory banners
4. constraints / ADRs
5. delegation template
6. verification script

## Common mistakes

### 1. Relying on `mai context` alone

Agents often grep or open files before running it. Always add physical banners.

### 2. Flat-classifying a mixed directory

If one subtree is active/shared and its siblings are legacy, split the classification per subtree. Do not flatten because it is simpler.

### 3. Citing docs without verifying the dependency graph

A taxonomy tag must ground in both docs **and** actual dependency reality.

### 4. Forgetting file existence

Stale references to moved or deleted directories are common. Verify the path exists before tagging.

### 5. Building the harness but not the installer

If `.maitake/` is gitignored, hooks do not propagate. Ship the installer.

### 6. Unstructured subagent prompts

That is how contamination re-enters.
