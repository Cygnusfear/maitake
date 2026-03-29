package crdt

import (
	"testing"
)

func TestNew(t *testing.T) {
	doc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	content, err := doc.Content()
	if err != nil {
		t.Fatal(err)
	}
	if content != "" {
		t.Errorf("new doc content = %q, want empty", content)
	}
}

func TestInsertAndContent(t *testing.T) {
	doc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	if err := doc.Insert(0, "Hello, world!"); err != nil {
		t.Fatal(err)
	}

	content, err := doc.Content()
	if err != nil {
		t.Fatal(err)
	}
	if content != "Hello, world!" {
		t.Errorf("content = %q, want Hello, world!", content)
	}

	length, err := doc.Length()
	if err != nil {
		t.Fatal(err)
	}
	if length != 13 {
		t.Errorf("length = %d, want 13", length)
	}
}

func TestDelete(t *testing.T) {
	doc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	doc.Insert(0, "Hello, world!")
	doc.Delete(5, 8) // remove ", world!"

	content, _ := doc.Content()
	if content != "Hello" {
		t.Errorf("after delete = %q, want Hello", content)
	}
}

func TestSaveAndLoad(t *testing.T) {
	// Create and populate a doc
	doc1, _ := New()
	doc1.Insert(0, "Hello from doc1")
	state, err := doc1.Save()
	if err != nil {
		t.Fatal(err)
	}
	doc1.Close()

	// Load into a new doc
	doc2, err := Load(state)
	if err != nil {
		t.Fatal(err)
	}
	defer doc2.Close()

	content, _ := doc2.Content()
	if content != "Hello from doc1" {
		t.Errorf("loaded content = %q", content)
	}
}

func TestConcurrentMerge(t *testing.T) {
	// Two docs start from the same state, edit concurrently, merge via full state
	doc1, _ := New()
	doc1.Insert(0, "Hello World")
	state, _ := doc1.Save()

	doc2, _ := Load(state)

	// Doc1 inserts " beautiful" at position 5
	doc1.Insert(5, " beautiful")

	// Doc2 inserts " amazing" at position 5 (concurrent edit!)
	doc2.Insert(5, " amazing")

	// Merge via full state (the path maitake uses)
	state1, _ := doc1.Save()
	state2, _ := doc2.Save()

	merged1, _ := Load(state1)
	merged1.Apply(state2)
	merged2, _ := Load(state2)
	merged2.Apply(state1)

	content1, _ := merged1.Content()
	content2, _ := merged2.Content()

	if content1 != content2 {
		t.Errorf("docs diverged!\n  merged1: %q\n  merged2: %q", content1, content2)
	}

	// Both edits should be present
	if !containsStr(content1, "beautiful") || !containsStr(content1, "amazing") {
		t.Errorf("missing edits: %q", content1)
	}

	t.Logf("Merged result: %q", content1)

	doc1.Close()
	doc2.Close()
	merged1.Close()
	merged2.Close()
}

func TestDiffApply_Sequential(t *testing.T) {
	// Diff+Apply works for sequential (non-concurrent) edits
	base, _ := New()
	base.Insert(0, "AB")
	state, _ := base.Save()
	base.Close()

	doc1, _ := Load(state)
	doc2, _ := Load(state)

	// Doc1 appends C (only one side edits)
	doc1.Insert(2, "C")

	sv2, _ := doc2.StateVector()
	diff, _ := doc1.Diff(sv2)
	t.Logf("diff len: %d bytes", len(diff))

	err := doc2.Apply(diff)
	if err != nil {
		t.Fatal(err)
	}

	c1, _ := doc1.Content()
	c2, _ := doc2.Content()
	t.Logf("doc1: %q, doc2: %q", c1, c2)
	if c1 != c2 {
		t.Errorf("sequential diff/apply diverged")
	}

	doc1.Close()
	doc2.Close()
}

func TestFullStateMerge_EndOfDoc(t *testing.T) {
	// Full state merge: both peers append at end of document.
	// This is the merge path maitake uses (not diff+apply).
	base, _ := New()
	base.Insert(0, "AB")
	state, _ := base.Save()
	base.Close()

	doc1, _ := Load(state)
	doc2, _ := Load(state)

	doc1.Insert(2, "C")
	doc2.Insert(2, "D")

	state1, _ := doc1.Save()
	state2, _ := doc2.Save()

	// Merge both ways — must converge
	merged1, _ := Load(state1)
	merged1.Apply(state2)
	merged2, _ := Load(state2)
	merged2.Apply(state1)

	c1, _ := merged1.Content()
	c2, _ := merged2.Content()

	if c1 != c2 {
		t.Errorf("full state merge diverged: %q vs %q", c1, c2)
	}
	if !containsStr(c1, "C") || !containsStr(c1, "D") {
		t.Errorf("missing edits: %q", c1)
	}
	t.Logf("converged: %q", c1)

	merged1.Close()
	merged2.Close()
	doc1.Close()
	doc2.Close()
}

func TestMultipleEditsAndMerge(t *testing.T) {
	// Two peers concurrently append sections, then merge via full state
	base, _ := New()
	base.Insert(0, "# Architecture\n\nThe system uses services.\n")
	state, _ := base.Save()
	base.Close()

	doc1, _ := Load(state)
	doc2, _ := Load(state)

	// Doc1 appends a line
	len1, _ := doc1.Length()
	doc1.Insert(uint32(len1), "\n## Microservices\nWe use gRPC.\n")

	// Doc2 appends a different line (concurrent!)
	len2, _ := doc2.Length()
	doc2.Insert(uint32(len2), "\n## Monolith\nOne big binary.\n")

	// Merge via full state (not diff+apply — see spike findings)
	state1, _ := doc1.Save()
	state2, _ := doc2.Save()

	merged1, _ := Load(state1)
	merged1.Apply(state2)

	merged2, _ := Load(state2)
	merged2.Apply(state1)

	content1, _ := merged1.Content()
	content2, _ := merged2.Content()

	if content1 != content2 {
		t.Errorf("diverged:\n  merged1: %q\n  merged2: %q", content1, content2)
	}

	// Both sections should be present
	if !containsStr(content1, "Microservices") || !containsStr(content1, "Monolith") {
		t.Errorf("missing edits in merged: %q", content1)
	}

	t.Logf("Merged:\n%s", content1)

	doc1.Close()
	doc2.Close()
	merged1.Close()
	merged2.Close()
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
