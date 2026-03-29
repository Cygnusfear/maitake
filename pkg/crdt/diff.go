package crdt

import (
	"unicode/utf8"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// Op represents a single text edit operation for a YDoc.
type Op struct {
	// Position in the current document (UTF-8 character index, not byte).
	Pos uint32
	// For inserts: the text to insert. Empty for deletes.
	Text string
	// For deletes: number of characters to remove. 0 for inserts.
	Len uint32
}

// IsInsert returns true if this is an insert operation.
func (o Op) IsInsert() bool { return o.Text != "" }

// IsDelete returns true if this is a delete operation.
func (o Op) IsDelete() bool { return o.Len > 0 }

// Diff computes the minimal sequence of Insert/Delete operations to
// transform oldText into newText. Operations are in apply order —
// apply them sequentially to a YDoc starting from oldText.
//
// Uses Myers diff (character-level) via sergi/go-diff.
func Diff(oldText, newText string) []Op {
	if oldText == newText {
		return nil
	}

	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(oldText, newText, true)
	diffs = dmp.DiffCleanupEfficiency(diffs)

	var ops []Op
	pos := uint32(0) // current position in the evolving document

	for _, d := range diffs {
		charLen := uint32(utf8.RuneCountInString(d.Text))
		switch d.Type {
		case diffmatchpatch.DiffEqual:
			pos += charLen
		case diffmatchpatch.DiffDelete:
			ops = append(ops, Op{Pos: pos, Len: charLen})
			// Don't advance pos — deleted chars are gone
		case diffmatchpatch.DiffInsert:
			ops = append(ops, Op{Pos: pos, Text: d.Text})
			pos += charLen // advance past inserted text
		}
	}

	return ops
}

// ApplyOps applies a sequence of Ops to a TextDoc.
func ApplyOps(doc *TextDoc, ops []Op) error {
	for _, op := range ops {
		if op.IsDelete() {
			if err := doc.Delete(op.Pos, op.Len); err != nil {
				return err
			}
		}
		if op.IsInsert() {
			if err := doc.Insert(op.Pos, op.Text); err != nil {
				return err
			}
		}
	}
	return nil
}
