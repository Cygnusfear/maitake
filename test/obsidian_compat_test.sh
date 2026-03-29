#!/bin/bash
# Obsidian compatibility integration tests for maitake docs sync.
# Requires: Obsidian running, CLI enabled, mai in PATH.
#
# These tests use the Obsidian CLI to verify that maitake's docs sync
# doesn't break Obsidian's metadata cache, frontmatter, or file handling.
#
# Usage: ./test/obsidian_compat_test.sh [vault_path]

set -euo pipefail

export PATH="$PATH:/Applications/Obsidian.app/Contents/MacOS"

VAULT="${1:-/tmp/mai-obsidian-test}"
PASS=0
FAIL=0
SKIP=0

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m'

pass() { echo -e "  ${GREEN}✓${NC} $1"; PASS=$((PASS + 1)); }
fail() { echo -e "  ${RED}✗${NC} $1"; FAIL=$((FAIL + 1)); }
skip() { echo -e "  ${YELLOW}⊘${NC} $1 (skipped)"; SKIP=$((SKIP + 1)); }

# Check prereqs
if ! command -v Obsidian &>/dev/null; then
    echo "Obsidian CLI not in PATH. Add /Applications/Obsidian.app/Contents/MacOS to PATH."
    exit 1
fi

if ! Obsidian version &>/dev/null; then
    echo "Obsidian not running. Start Obsidian first."
    exit 1
fi

if ! command -v mai &>/dev/null; then
    echo "mai not in PATH."
    exit 1
fi

# Setup: reuse existing vault or create fresh
if [ -d "$VAULT/.git" ]; then
    echo "Using existing test vault at $VAULT"
    cd "$VAULT"
    # Clean leftover doc files from previous runs
    find docs -name '*.md' ! -name 'README.md' -delete 2>/dev/null || true
    # Clear maitake notes from previous runs (nuclear reset)
    git notes --ref=refs/notes/maitake prune 2>/dev/null || true
    # Remove all notes
    for obj in $(git notes --ref=refs/notes/maitake list 2>/dev/null | awk '{print $2}'); do
        git notes --ref=refs/notes/maitake remove "$obj" 2>/dev/null || true
    done
    rm -rf ~/.maitake/cache
else
    echo "Setting up test vault at $VAULT..."
    rm -rf "$VAULT"
    mkdir -p "$VAULT"
    cd "$VAULT"
    git init -q
    git commit -q --allow-empty -m "init"
    mai init 2>/dev/null

    mkdir -p .maitake
    cat > .maitake/config.toml << 'EOF'
[docs]
dir = "docs"
sync = "auto"
watch = false
EOF

    mkdir -p docs
    echo "# Test Vault" > docs/README.md
    git add -A && git commit -q -m "vault setup"

    echo ""
    echo "================================================"
    echo "IMPORTANT: Open $VAULT/docs as a vault in Obsidian"
    echo "Then press Enter to continue..."
    echo "================================================"
    read -r
fi

VAULT_NAME="docs"

echo ""
echo "=== Test Suite: Obsidian Compatibility ==="
echo ""

# ─── Test 1: Basic doc creation and Obsidian visibility ───
echo "1. Doc note → file → Obsidian sees it"

id=$(mai create "Obsidian Test One" -k doc -d "Hello from maitake" 2>&1)
mai docs sync 2>/dev/null
sleep 2  # let Obsidian index the new file

if Obsidian read path="obsidian-test-one.md" vault="$VAULT_NAME" 2>/dev/null | grep -q "Hello from maitake"; then
    pass "Obsidian can read maitake-created doc"
else
    fail "Obsidian cannot read maitake-created doc"
fi

# ─── Test 2: mai-id frontmatter visible to Obsidian ───
echo "2. mai-id in Obsidian's metadata cache"

fm=$(Obsidian property:read name="mai-id" path="obsidian-test-one.md" vault="$VAULT_NAME" 2>/dev/null)
if [ -n "$fm" ] && [ "$fm" = "$id" ]; then
    pass "Obsidian sees mai-id=$id in frontmatter"
else
    fail "Obsidian does not see mai-id (got: '$fm', want: '$id')"
fi

# ─── Test 3: Extra frontmatter preserved after sync ───
echo "3. Obsidian frontmatter survives maitake sync"

Obsidian property:set name="tags" value="test,obsidian" path="obsidian-test-one.md" vault="$VAULT_NAME" 2>/dev/null
sleep 1
Obsidian property:set name="aliases" value="test-alias" path="obsidian-test-one.md" vault="$VAULT_NAME" 2>/dev/null
sleep 1
Obsidian property:set name="cssclasses" value="wide" path="obsidian-test-one.md" vault="$VAULT_NAME" 2>/dev/null
sleep 2  # let Obsidian write all properties to disk

# Verify Obsidian actually wrote the frontmatter before we sync
echo "  (file before sync:)"
head -10 "$VAULT/docs/obsidian-test-one.md" 2>/dev/null | grep -E 'tags|aliases|css' || echo "  WARNING: frontmatter not on disk yet"

# Now run maitake sync — should NOT destroy Obsidian's frontmatter
mai docs sync 2>/dev/null

tags=$(Obsidian property:read name="tags" path="obsidian-test-one.md" vault="$VAULT_NAME" 2>/dev/null)
aliases=$(Obsidian property:read name="aliases" path="obsidian-test-one.md" vault="$VAULT_NAME" 2>/dev/null)
css=$(Obsidian property:read name="cssclasses" path="obsidian-test-one.md" vault="$VAULT_NAME" 2>/dev/null)

if echo "$tags" | grep -q "test"; then
    pass "tags survived sync"
else
    fail "tags lost after sync (got: '$tags')"
fi

if echo "$aliases" | grep -q "test-alias"; then
    pass "aliases survived sync"
else
    fail "aliases lost after sync (got: '$aliases')"
fi

if echo "$css" | grep -q "wide"; then
    pass "cssclasses survived sync"
else
    fail "cssclasses lost after sync (got: '$css')"
fi

# ─── Test 4: Body edit via mai updates file, Obsidian sees new content ───
echo "4. mai edit → file updated → Obsidian reads new content"

mai edit "$id" -d "Updated body from agent" 2>/dev/null

sleep 1  # let autoSync + Obsidian catch up

content=$(Obsidian read path="obsidian-test-one.md" vault="$VAULT_NAME" 2>/dev/null)
if echo "$content" | grep -q "Updated body from agent"; then
    pass "Obsidian reads body after mai edit"
else
    fail "Obsidian doesn't see updated body (got: '$content')"
fi

# ─── Test 5: Frontmatter still intact after body edit ───
echo "5. Frontmatter survives body edit"

tags_after=$(Obsidian property:read name="tags" path="obsidian-test-one.md" vault="$VAULT_NAME" 2>/dev/null)
mai_id_after=$(Obsidian property:read name="mai-id" path="obsidian-test-one.md" vault="$VAULT_NAME" 2>/dev/null)

if echo "$tags_after" | grep -q "test"; then
    pass "tags still present after body edit"
else
    fail "tags lost after body edit (got: '$tags_after')"
fi

if [ "$mai_id_after" = "$id" ]; then
    pass "mai-id preserved after body edit"
else
    fail "mai-id changed after body edit (got: '$mai_id_after')"
fi

# ─── Test 6: Obsidian edit → mai sync picks it up ───
echo "6. Obsidian edit → mai docs sync → note updated"

Obsidian append path="obsidian-test-one.md" vault="$VAULT_NAME" content="\n## Obsidian Section\nWritten in Obsidian." 2>/dev/null

sleep 1

mai docs sync 2>/dev/null

body=$(mai --json show "$id" 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin)['body'])" 2>/dev/null)
if echo "$body" | grep -q "Obsidian Section"; then
    pass "mai picked up Obsidian edit"
else
    fail "mai didn't pick up Obsidian edit (body: '$body')"
fi

# ─── Test 7: Closing a doc keeps file with closed:true ───
echo "7. mai close → file stays, closed:true in frontmatter"

close_id=$(mai create "Will Be Closed" -k doc -d "This will be closed" 2>&1)
mai docs sync 2>/dev/null
sleep 1

mai close "$close_id" -m "testing close" 2>/dev/null
sleep 1

# File should still exist
if Obsidian read path="will-be-closed.md" vault="$VAULT_NAME" 2>/dev/null | grep -q "This will be closed"; then
    pass "closed doc file still exists in Obsidian"
else
    fail "closed doc file was deleted"
fi

closed_flag=$(Obsidian property:read name="closed" path="will-be-closed.md" vault="$VAULT_NAME" 2>/dev/null)
if [ "$closed_flag" = "true" ]; then
    pass "closed:true in frontmatter"
else
    fail "closed:true not in frontmatter (got: '$closed_flag')"
fi

# ─── Test 8: Search works across maitake docs ───
echo "8. Obsidian search finds maitake doc content"

results=$(Obsidian search query="Updated body from agent" vault="$VAULT_NAME" total 2>/dev/null)
if [ -n "$results" ] && [ "$results" -gt 0 ] 2>/dev/null; then
    pass "Obsidian search finds maitake content ($results matches)"
else
    fail "Obsidian search doesn't find maitake content (results: '$results')"
fi

# ─── Test 9: Wikilinks between maitake docs resolve ───
echo "9. Wikilinks between docs"

link_id=$(mai create "Link Target" -k doc -d "I am the target" 2>&1)
mai docs sync 2>/dev/null
sleep 1

Obsidian append path="obsidian-test-one.md" vault="$VAULT_NAME" content="\n[[link-target]]" 2>/dev/null
sleep 1

unresolved=$(Obsidian unresolved vault="$VAULT_NAME" 2>/dev/null)
if echo "$unresolved" | grep -q "link-target"; then
    fail "wikilink to link-target is unresolved"
else
    pass "wikilink to link-target resolves"
fi

# ─── Test 10: Unicode content round-trips ───
echo "10. Unicode content round-trip"

uni_id=$(mai create "Unicode Docs" -k doc -d "# Ünïcödé

Héllo wörld 🍄 日本語テスト" 2>&1)
mai docs sync 2>/dev/null
sleep 1

uni_content=$(Obsidian read path="unicode-docs.md" vault="$VAULT_NAME" 2>/dev/null)
if echo "$uni_content" | grep -q "🍄"; then
    pass "unicode content round-trips (emoji)"
else
    fail "unicode content lost"
fi

if echo "$uni_content" | grep -q "日本語"; then
    pass "unicode content round-trips (CJK)"
else
    fail "CJK content lost"
fi

# ─── Summary ───
echo ""
echo "================================================"
echo -e "Results: ${GREEN}${PASS} passed${NC}, ${RED}${FAIL} failed${NC}, ${YELLOW}${SKIP} skipped${NC}"
echo "================================================"

if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
