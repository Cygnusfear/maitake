package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupPRRepo creates a test repo with 'main' and 'feature/auth' branches.
// HEAD is left on 'feature/auth' which has one extra commit (src/auth_v2.go).
// The 'main' branch has the initial commits only.
func setupPRRepo(t *testing.T) string {
	t.Helper()
	dir := setupTestRepo(t)
	gitRun(t, dir, "branch", "-M", "main")
	gitRun(t, dir, "checkout", "-b", "feature/auth")
	os.WriteFile(filepath.Join(dir, "src", "auth_v2.go"), []byte("package src\n\nfunc AuthV2() {}\n"), 0644)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "add auth v2")
	return dir
}

// extractPRID extracts the PR ID from `mai pr` create output.
// Create output format: "<id>  from → into"
func extractPRID(t *testing.T, out string) string {
	t.Helper()
	fields := strings.Fields(out)
	if len(fields) < 1 {
		t.Fatalf("unexpected pr create output: %q", out)
	}
	return fields[0]
}

// === TestPR_Create — happy path 1 ===

func TestPR_Create(t *testing.T) {
	dir := setupPRRepo(t)

	out := mai(t, dir, "pr", "Add auth v2", "--into", "main")

	// Output should have ID and branch arrows
	if !strings.Contains(out, "→") {
		t.Errorf("pr create output should contain →: %q", out)
	}
	if !strings.Contains(out, "feature/auth") {
		t.Errorf("pr create output should contain source branch: %q", out)
	}
	if !strings.Contains(out, "main") {
		t.Errorf("pr create output should contain target branch: %q", out)
	}

	// ID should be parseable
	id := extractPRID(t, out)
	if id == "" {
		t.Fatalf("could not extract PR ID from output: %q", out)
	}

	// Note should exist and be queryable
	show := mai(t, dir, "pr", "show", id)
	if !strings.Contains(show, "Add auth v2") {
		t.Errorf("pr show should display title: %s", show)
	}
}

// === TestPR_List — happy paths 2, 27, 28 ===

func TestPR_List(t *testing.T) {
	t.Run("NoPRs", func(t *testing.T) {
		// Test 28: PR list when no PRs exist — 'No open PRs.'
		dir := setupTestRepo(t)
		out := mai(t, dir, "pr")
		if !strings.Contains(out, "No open PRs.") {
			t.Errorf("expected 'No open PRs.' but got: %q", out)
		}
	})

	t.Run("ListSinglePR", func(t *testing.T) {
		// Test 2: List PRs, verify created PR appears
		dir := setupPRRepo(t)
		createOut := mai(t, dir, "pr", "My feature PR", "--into", "main")
		id := extractPRID(t, createOut)

		listOut := mai(t, dir, "pr")
		if !strings.Contains(listOut, id) {
			t.Errorf("pr list should contain created PR ID %s: %s", id, listOut)
		}
		if !strings.Contains(listOut, "My feature PR") {
			t.Errorf("pr list should contain PR title: %s", listOut)
		}
		if !strings.Contains(listOut, "feature/auth") {
			t.Errorf("pr list should contain source branch: %s", listOut)
		}
		if !strings.Contains(listOut, "main") {
			t.Errorf("pr list should contain target branch: %s", listOut)
		}
	})

	t.Run("MultiplePRs", func(t *testing.T) {
		// Test 27: Multiple PRs listed at once
		dir := setupTestRepo(t)
		gitRun(t, dir, "branch", "-M", "main")

		// Create first feature branch
		gitRun(t, dir, "checkout", "-b", "feature/login")
		os.WriteFile(filepath.Join(dir, "src", "login.go"), []byte("package src\n"), 0644)
		gitRun(t, dir, "add", "-A")
		gitRun(t, dir, "commit", "-m", "add login")
		createOut1 := mai(t, dir, "pr", "Login feature", "--into", "main")
		id1 := extractPRID(t, createOut1)

		// Create second feature branch
		gitRun(t, dir, "checkout", "main")
		gitRun(t, dir, "checkout", "-b", "feature/signup")
		os.WriteFile(filepath.Join(dir, "src", "signup.go"), []byte("package src\n"), 0644)
		gitRun(t, dir, "add", "-A")
		gitRun(t, dir, "commit", "-m", "add signup")
		createOut2 := mai(t, dir, "pr", "Signup feature", "--into", "main")
		id2 := extractPRID(t, createOut2)

		listOut := mai(t, dir, "pr")
		if !strings.Contains(listOut, id1) {
			t.Errorf("pr list should contain first PR %s: %s", id1, listOut)
		}
		if !strings.Contains(listOut, id2) {
			t.Errorf("pr list should contain second PR %s: %s", id2, listOut)
		}
	})
}

// === TestPR_Show — happy paths 3, 4, 5 ===

func TestPR_Show(t *testing.T) {
	t.Run("BasicShow", func(t *testing.T) {
		// Test 3: Show PR — verify title, branches, status, review pending
		dir := setupPRRepo(t)
		createOut := mai(t, dir, "pr", "Auth v2 implementation", "--into", "main")
		id := extractPRID(t, createOut)

		show := mai(t, dir, "pr", "show", id)

		if !strings.Contains(show, "Auth v2 implementation") {
			t.Errorf("pr show missing title: %s", show)
		}
		if !strings.Contains(show, "feature/auth") {
			t.Errorf("pr show missing source branch: %s", show)
		}
		if !strings.Contains(show, "main") {
			t.Errorf("pr show missing target branch: %s", show)
		}
		if !strings.Contains(show, "→") {
			t.Errorf("pr show missing branch arrow: %s", show)
		}
		if !strings.Contains(show, "open") {
			t.Errorf("pr show missing open status: %s", show)
		}
		// Review should be pending (no comments yet)
		if !strings.Contains(show, "pending") {
			t.Errorf("pr show missing 'pending' review status: %s", show)
		}
	})

	t.Run("ShowWithDiff", func(t *testing.T) {
		// Test 4: Show PR with --diff — verify diff output appears
		dir := setupPRRepo(t)
		createOut := mai(t, dir, "pr", "Auth v2 diff test", "--into", "main")
		id := extractPRID(t, createOut)

		show := mai(t, dir, "pr", "show", id, "--diff")

		// The full diff section should appear
		if !strings.Contains(show, "Full Diff") {
			t.Errorf("pr show --diff should contain 'Full Diff' section: %s", show)
		}
		// Should also show the added file
		if !strings.Contains(show, "auth_v2") {
			t.Errorf("pr show --diff should show the added file: %s", show)
		}
	})

	t.Run("ShowJSON", func(t *testing.T) {
		// Test 5: Show PR with --json — verify JSON format
		dir := setupPRRepo(t)
		createOut := mai(t, dir, "pr", "Auth v2 json test", "--into", "main")
		id := extractPRID(t, createOut)

		show := mai(t, dir, "--json", "pr", "show", id)

		if !strings.Contains(show, `"id"`) {
			t.Errorf("--json pr show should have 'id' field: %s", show)
		}
		if !strings.Contains(show, `"pr"`) {
			t.Errorf("--json pr show should have kind 'pr': %s", show)
		}
		if !strings.Contains(show, `"open"`) {
			t.Errorf("--json pr show should have status 'open': %s", show)
		}
		if !strings.Contains(show, `"title"`) {
			t.Errorf("--json pr show should have 'title' field: %s", show)
		}
		if !strings.Contains(show, "Auth v2 json test") {
			t.Errorf("--json pr show should have the PR title in output: %s", show)
		}
	})
}

// === TestPR_AcceptReject — happy paths 6, 7, 8 ===

func TestPR_AcceptReject(t *testing.T) {
	t.Run("Accept", func(t *testing.T) {
		// Test 6: Accept PR — verify output, then show confirms accepted status
		dir := setupPRRepo(t)
		createOut := mai(t, dir, "pr", "Accept test PR", "--into", "main")
		id := extractPRID(t, createOut)

		acceptOut := mai(t, dir, "pr", "accept", id)
		if !strings.Contains(acceptOut, "accepted") {
			t.Errorf("pr accept output should contain 'accepted': %s", acceptOut)
		}
		if !strings.Contains(acceptOut, id) {
			t.Errorf("pr accept output should contain PR ID: %s", acceptOut)
		}

		// Show should confirm accepted status
		show := mai(t, dir, "pr", "show", id)
		if !strings.Contains(show, "accepted") {
			t.Errorf("pr show after accept should contain 'accepted': %s", show)
		}
	})

	t.Run("Reject", func(t *testing.T) {
		// Test 7: Reject PR — verify output, then show confirms rejected + message visible
		dir := setupPRRepo(t)
		createOut := mai(t, dir, "pr", "Reject test PR", "--into", "main")
		id := extractPRID(t, createOut)

		rejectOut := mai(t, dir, "pr", "reject", id, "-m", "Need more tests")
		if !strings.Contains(rejectOut, "Changes requested") {
			t.Errorf("pr reject output should contain 'Changes requested': %s", rejectOut)
		}
		if !strings.Contains(rejectOut, "Need more tests") {
			t.Errorf("pr reject output should contain the reason: %s", rejectOut)
		}

		// Show should confirm rejected status with message visible
		show := mai(t, dir, "pr", "show", id)
		if !strings.Contains(show, "changes requested") {
			t.Errorf("pr show after reject should contain 'changes requested': %s", show)
		}
		if !strings.Contains(show, "Need more tests") {
			t.Errorf("pr show after reject should contain reject message: %s", show)
		}
	})

	t.Run("AcceptAfterReject", func(t *testing.T) {
		// Test 8: Accept after reject — verify review flips back to accepted
		dir := setupPRRepo(t)
		createOut := mai(t, dir, "pr", "Flip test PR", "--into", "main")
		id := extractPRID(t, createOut)

		// First reject it
		mai(t, dir, "pr", "reject", id, "-m", "Not ready")

		show := mai(t, dir, "pr", "show", id)
		if !strings.Contains(show, "changes requested") {
			t.Errorf("show after reject should say 'changes requested': %s", show)
		}

		// Then accept it — should flip
		acceptOut := mai(t, dir, "pr", "accept", id, "-m", "LGTM now")
		if !strings.Contains(acceptOut, "accepted") {
			t.Errorf("accept after reject should succeed: %s", acceptOut)
		}

		show = mai(t, dir, "pr", "show", id)
		if !strings.Contains(show, "accepted") {
			t.Errorf("show after accept-after-reject should show 'accepted': %s", show)
		}
		// Should NOT still show rejected state
		if strings.Contains(show, "changes requested") {
			t.Errorf("show after accept should NOT show 'changes requested': %s", show)
		}
	})
}

// === TestPR_Submit — happy path 9 ===

func TestPR_Submit(t *testing.T) {
	dir := setupPRRepo(t)
	createOut := mai(t, dir, "pr", "Submit test PR", "--into", "main")
	id := extractPRID(t, createOut)

	// Submit the PR — should merge feature/auth into main
	submitOut := mai(t, dir, "pr", "submit", id)
	if !strings.Contains(submitOut, "closed") {
		t.Errorf("pr submit should say 'closed': %s", submitOut)
	}
	if !strings.Contains(submitOut, "feature/auth") {
		t.Errorf("pr submit should mention source branch: %s", submitOut)
	}
	if !strings.Contains(submitOut, "main") {
		t.Errorf("pr submit should mention target branch: %s", submitOut)
	}

	// Verify the PR note is now closed
	show := mai(t, dir, "pr", "show", id)
	if !strings.Contains(show, "closed") {
		t.Errorf("pr show after submit should show 'closed': %s", show)
	}

	// Verify the git merge actually happened: main should now have auth_v2.go
	// (submit switches to main, so we can check directly)
	gitRun(t, dir, "checkout", "main")
	if _, err := os.Stat(filepath.Join(dir, "src", "auth_v2.go")); err != nil {
		t.Errorf("auth_v2.go should exist on main after merge: %v", err)
	}
}

// === TestPR_Diff — happy paths 10, 11 ===

func TestPR_Diff(t *testing.T) {
	t.Run("FullDiff", func(t *testing.T) {
		// Test 10: Diff PR — verify actual diff content between branches
		dir := setupPRRepo(t)
		createOut := mai(t, dir, "pr", "Diff test PR", "--into", "main")
		id := extractPRID(t, createOut)

		diffOut := mai(t, dir, "pr", "diff", id)
		// Should contain the new file added on feature/auth
		if !strings.Contains(diffOut, "auth_v2") {
			t.Errorf("pr diff should show the new file: %s", diffOut)
		}
		// Diff output should contain + signs (additions)
		if !strings.Contains(diffOut, "+") {
			t.Errorf("pr diff should contain additions: %s", diffOut)
		}
	})

	t.Run("DiffStat", func(t *testing.T) {
		// Test 11: Diff PR --stat — verify stat summary
		dir := setupPRRepo(t)
		createOut := mai(t, dir, "pr", "Diff stat PR", "--into", "main")
		id := extractPRID(t, createOut)

		statOut := mai(t, dir, "pr", "diff", id, "--stat")
		// Stat output should contain file names and insertion counts
		if !strings.Contains(statOut, "auth_v2") {
			t.Errorf("pr diff --stat should mention the new file: %s", statOut)
		}
		// Stat format typically includes "insertions"
		if !strings.Contains(statOut, "insertion") {
			t.Errorf("pr diff --stat should show insertions: %s", statOut)
		}
	})
}

// === TestPR_Comment — happy paths 12, 13; edge case 29 ===

func TestPR_Comment(t *testing.T) {
	t.Run("BasicComment", func(t *testing.T) {
		// Test 12: Comment on PR — verify comment appears in show
		dir := setupPRRepo(t)
		createOut := mai(t, dir, "pr", "Comment test PR", "--into", "main")
		id := extractPRID(t, createOut)

		mai(t, dir, "pr", "comment", id, "-m", "This looks good overall")

		show := mai(t, dir, "pr", "show", id)
		if !strings.Contains(show, "This looks good overall") {
			t.Errorf("pr show should contain the comment: %s", show)
		}
	})

	t.Run("FileAndLineComment", func(t *testing.T) {
		// Test 13: Comment with --file and --line — verify file-located comment
		dir := setupPRRepo(t)
		createOut := mai(t, dir, "pr", "File comment PR", "--into", "main")
		id := extractPRID(t, createOut)

		mai(t, dir, "pr", "comment", id, "-m", "Fix this function", "--file", "src/auth_v2.go", "--line", "3")

		show := mai(t, dir, "pr", "show", id)
		if !strings.Contains(show, "Fix this function") {
			t.Errorf("pr show should contain file-located comment body: %s", show)
		}
		if !strings.Contains(show, "auth_v2.go") {
			t.Errorf("pr show should contain the file path: %s", show)
		}
		if !strings.Contains(show, ":3") {
			t.Errorf("pr show should contain the line number: %s", show)
		}
	})

	t.Run("MessageNotPrefixedWithFlag", func(t *testing.T) {
		// Test 29: PR comment uses -m flag correctly (message NOT prefixed with '-m')
		dir := setupPRRepo(t)
		createOut := mai(t, dir, "pr", "Flag test PR", "--into", "main")
		id := extractPRID(t, createOut)

		mai(t, dir, "pr", "comment", id, "-m", "The actual message")

		show := mai(t, dir, "pr", "show", id)
		if !strings.Contains(show, "The actual message") {
			t.Errorf("comment body should be 'The actual message' (not '-m The actual message'): %s", show)
		}
		// The literal string "-m" should not appear as the comment body
		if strings.Contains(show, "\n-m\n") || strings.Contains(show, "\n-m The actual") {
			t.Errorf("comment should NOT be prefixed with '-m': %s", show)
		}
	})
}

// === TestPR_AutoClose — auto-close 14, 15 ===

func TestPR_AutoClose(t *testing.T) {
	t.Run("AutoCloseOnList", func(t *testing.T) {
		// Test 14: Create PR, manually merge branch, then `mai pr` list — verify auto-close
		dir := setupPRRepo(t)
		createOut := mai(t, dir, "pr", "Auto-close test", "--into", "main")
		id := extractPRID(t, createOut)

		// Verify PR is open initially
		listOut := mai(t, dir, "pr")
		if !strings.Contains(listOut, id) {
			t.Fatalf("PR %s should appear in list: %s", id, listOut)
		}

		// Manually merge feature/auth into main using git
		gitRun(t, dir, "checkout", "main")
		gitRun(t, dir, "merge", "feature/auth", "--no-ff", "-m", "Manual merge")

		// Go back to feature/auth (doesn't matter, but let's be consistent)
		gitRun(t, dir, "checkout", "feature/auth")

		// Now listing PRs should detect the merge and auto-close
		listOut = mai(t, dir, "pr")
		// The output should either show 'closed' status or contain the PR with merged marker
		if strings.Contains(listOut, "No open PRs.") {
			// PR was auto-closed and no longer shows in default list — that's also fine
			// depending on implementation — let's check the PR show instead
		} else {
			if !strings.Contains(listOut, "closed") && !strings.Contains(listOut, "merged") {
				t.Errorf("PR should show as closed or merged after branch merge: %s", listOut)
			}
		}

		// After auto-close, show should confirm closed status
		show := mai(t, dir, "pr", "show", id)
		if !strings.Contains(show, "closed") {
			t.Errorf("pr show after auto-close should show 'closed': %s", show)
		}
	})

	t.Run("SubmitAlreadyMerged", func(t *testing.T) {
		// Test 15: Submit already-merged PR — should just close without merge error
		dir := setupPRRepo(t)
		createOut := mai(t, dir, "pr", "Pre-merged PR", "--into", "main")
		id := extractPRID(t, createOut)

		// Manually merge first
		gitRun(t, dir, "checkout", "main")
		gitRun(t, dir, "merge", "feature/auth", "--no-ff", "-m", "Already merged")
		gitRun(t, dir, "checkout", "feature/auth")

		// Submit should detect already-merged and just close it
		submitOut := mai(t, dir, "pr", "submit", id)
		if !strings.Contains(submitOut, "Already merged") && !strings.Contains(submitOut, "closed") {
			t.Errorf("pr submit on already-merged PR should say 'Already merged': %s", submitOut)
		}

		// PR should be closed
		show := mai(t, dir, "pr", "show", id)
		if !strings.Contains(show, "closed") {
			t.Errorf("PR should be closed after submit: %s", show)
		}
	})
}

// === TestPR_Errors — error cases 16–25 ===

func TestPR_Errors(t *testing.T) {
	t.Run("DetachedHEAD", func(t *testing.T) {
		// Test 16: PR on detached HEAD — should fail
		dir := setupTestRepo(t)
		gitRun(t, dir, "branch", "-M", "main")

		// Detach HEAD at current commit
		head := gitRun(t, dir, "rev-parse", "HEAD")
		gitRun(t, dir, "checkout", head)

		out := maiFail(t, dir, "pr", "My PR")
		if !strings.Contains(out, "detached HEAD") && !strings.Contains(out, "not on a branch") {
			t.Errorf("expected 'detached HEAD' error: %s", out)
		}
	})

	t.Run("SameSourceAndTarget", func(t *testing.T) {
		// Test 17: PR with same source and target branch — should fail
		dir := setupTestRepo(t)
		gitRun(t, dir, "branch", "-M", "main")

		// We're on main; try to PR main → main
		out := maiFail(t, dir, "pr", "Same branch PR", "--into", "main")
		if !strings.Contains(out, "same branch") {
			t.Errorf("expected 'same branch' error: %s", out)
		}
	})

	t.Run("ShowNonExistentPR", func(t *testing.T) {
		// Test 18: Show non-existent PR — should fail
		dir := setupTestRepo(t)
		out := maiFail(t, dir, "pr", "show", "nonexistent-pr-id")
		if !strings.Contains(out, "not found") && !strings.Contains(out, "no note") {
			t.Errorf("expected 'not found' error: %s", out)
		}
	})

	t.Run("ShowTicketAsPR", func(t *testing.T) {
		// Test 19: Show a ticket (not a PR) via `pr show` — should fail with 'not a PR'
		dir := setupTestRepo(t)
		ticketID := mai(t, dir, "ticket", "Regular ticket")

		out := maiFail(t, dir, "pr", "show", ticketID)
		if !strings.Contains(out, "not a PR") {
			t.Errorf("expected 'not a PR' error: %s", out)
		}
	})

	t.Run("RejectWithoutMessage", func(t *testing.T) {
		// Test 20: Reject without message — should fail
		dir := setupPRRepo(t)
		createOut := mai(t, dir, "pr", "Reject no msg PR", "--into", "main")
		id := extractPRID(t, createOut)

		out := maiFail(t, dir, "pr", "reject", id)
		if !strings.Contains(out, "reason required") && !strings.Contains(out, "required") {
			t.Errorf("expected 'reason required' error: %s", out)
		}
	})

	t.Run("SubmitWithUnresolvedComments", func(t *testing.T) {
		// Test 21: Submit with unresolved comments — should fail
		dir := setupPRRepo(t)
		createOut := mai(t, dir, "pr", "Unresolved PR", "--into", "main")
		id := extractPRID(t, createOut)

		// Reject adds an unresolved comment
		mai(t, dir, "pr", "reject", id, "-m", "Needs work")

		out := maiFail(t, dir, "pr", "submit", id)
		if !strings.Contains(out, "unresolved") {
			t.Errorf("expected 'unresolved' error: %s", out)
		}
	})

	t.Run("SubmitForceOverridesUnresolved", func(t *testing.T) {
		// Test 22: Submit with --force overrides unresolved check
		dir := setupPRRepo(t)
		createOut := mai(t, dir, "pr", "Force submit PR", "--into", "main")
		id := extractPRID(t, createOut)

		// Reject adds an unresolved comment
		mai(t, dir, "pr", "reject", id, "-m", "Needs work but merging anyway")

		// --force should override
		submitOut := mai(t, dir, "pr", "submit", id, "--force")
		if !strings.Contains(submitOut, "closed") && !strings.Contains(submitOut, "Merged") {
			t.Errorf("pr submit --force should succeed: %s", submitOut)
		}

		// PR should be closed
		show := mai(t, dir, "pr", "show", id)
		if !strings.Contains(show, "closed") {
			t.Errorf("PR should be closed after force submit: %s", show)
		}
	})

	t.Run("SubmitAlreadyClosedPR", func(t *testing.T) {
		// Test 23: Submit already-closed PR — should fail
		dir := setupPRRepo(t)
		createOut := mai(t, dir, "pr", "Close and submit PR", "--into", "main")
		id := extractPRID(t, createOut)

		// Submit once (closes it)
		mai(t, dir, "pr", "submit", id)

		// Try to submit again
		out := maiFail(t, dir, "pr", "submit", id)
		if !strings.Contains(out, "already closed") {
			t.Errorf("expected 'already closed' error: %s", out)
		}
	})

	t.Run("DiffOnNonPR", func(t *testing.T) {
		// Test 24: Diff on non-PR — should fail
		dir := setupTestRepo(t)
		ticketID := mai(t, dir, "ticket", "A regular ticket")

		out := maiFail(t, dir, "pr", "diff", ticketID)
		if !strings.Contains(out, "not a PR") {
			t.Errorf("expected 'not a PR' error: %s", out)
		}
	})

	t.Run("AcceptNonPR", func(t *testing.T) {
		// Test 25: Accept non-PR — should fail
		dir := setupTestRepo(t)
		ticketID := mai(t, dir, "ticket", "A ticket not a PR")

		out := maiFail(t, dir, "pr", "accept", ticketID)
		if !strings.Contains(out, "not a PR") {
			t.Errorf("expected 'not a PR' error: %s", out)
		}
	})
}

// === TestPR_EdgeCases — edge cases 26–29 ===

func TestPR_EdgeCases(t *testing.T) {
	t.Run("NoTitleAutoGenerated", func(t *testing.T) {
		// Test 26: PR with no title auto-generates 'branch → target'
		dir := setupPRRepo(t)

		// Create PR with no title
		createOut := mai(t, dir, "pr", "--into", "main")
		id := extractPRID(t, createOut)

		// Show should contain the auto-generated title "feature/auth → main"
		show := mai(t, dir, "pr", "show", id)
		if !strings.Contains(show, "feature/auth") {
			t.Errorf("auto-generated title should contain source branch: %s", show)
		}
		if !strings.Contains(show, "→") {
			t.Errorf("auto-generated title should contain arrow: %s", show)
		}
	})
}
