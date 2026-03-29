package crdt

import (
	"testing"
)

func TestDiff_NoChange(t *testing.T) {
	ops := Diff("hello", "hello")
	if len(ops) != 0 {
		t.Errorf("expected no ops, got %d", len(ops))
	}
}

func TestDiff_Insert(t *testing.T) {
	ops := Diff("AB", "AXB")
	// Should be: insert "X" at position 1
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d: %+v", len(ops), ops)
	}
	if !ops[0].IsInsert() || ops[0].Pos != 1 || ops[0].Text != "X" {
		t.Errorf("unexpected op: %+v", ops[0])
	}
}

func TestDiff_Delete(t *testing.T) {
	ops := Diff("ABCD", "AD")
	// Should be: delete 2 chars at position 1
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d: %+v", len(ops), ops)
	}
	if !ops[0].IsDelete() || ops[0].Pos != 1 || ops[0].Len != 2 {
		t.Errorf("unexpected op: %+v", ops[0])
	}
}

func TestDiff_Replace(t *testing.T) {
	ops := Diff("Hello world", "Hello CRDT")
	// Delete "world" (5 chars at pos 6), insert "CRDT" at pos 6
	t.Logf("ops: %+v", ops)
	if len(ops) < 2 {
		t.Fatalf("expected at least 2 ops, got %d", len(ops))
	}
}

func TestDiff_Append(t *testing.T) {
	ops := Diff("AB", "ABCD")
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d: %+v", len(ops), ops)
	}
	if !ops[0].IsInsert() || ops[0].Pos != 2 || ops[0].Text != "CD" {
		t.Errorf("unexpected op: %+v", ops[0])
	}
}

func TestDiff_Prepend(t *testing.T) {
	ops := Diff("CD", "ABCD")
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d: %+v", len(ops), ops)
	}
	if !ops[0].IsInsert() || ops[0].Pos != 0 || ops[0].Text != "AB" {
		t.Errorf("unexpected op: %+v", ops[0])
	}
}

func TestDiff_Unicode(t *testing.T) {
	ops := Diff("héllo", "héllo wörld")
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d: %+v", len(ops), ops)
	}
	// "héllo" is 5 characters (not 6 bytes)
	if ops[0].Pos != 5 || ops[0].Text != " wörld" {
		t.Errorf("unexpected op: pos=%d text=%q", ops[0].Pos, ops[0].Text)
	}
}

func TestApplyOps_RoundTrip(t *testing.T) {
	old := "# Architecture\n\nThe system uses services.\n"
	new := "# Architecture\n\nThe system uses microservices.\n\n## Details\nMore info here.\n"

	ops := Diff(old, new)
	t.Logf("%d ops", len(ops))

	// Create a YDoc with old content, apply ops, check result
	doc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	doc.Insert(0, old)
	if err := ApplyOps(doc, ops); err != nil {
		t.Fatal(err)
	}

	content, _ := doc.Content()
	if content != new {
		t.Errorf("after ApplyOps:\n  got:  %q\n  want: %q", content, new)
	}
}

func TestApplyOps_LargeEdit(t *testing.T) {
	old := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\n"
	new := "Line 1\nNew Line\nLine 3\nLine 4\nLine 5\nLine 6\n"

	ops := Diff(old, new)
	t.Logf("%d ops: %+v", len(ops), ops)

	doc, _ := New()
	defer doc.Close()
	doc.Insert(0, old)

	if err := ApplyOps(doc, ops); err != nil {
		t.Fatal(err)
	}

	content, _ := doc.Content()
	if content != new {
		t.Errorf("after ApplyOps:\n  got:  %q\n  want: %q", content, new)
	}
}
