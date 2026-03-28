package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/cygnusfear/maitake/pkg/notes"
)

func runDocsSync(e notes.Engine, args []string) {
	cwd, _ := os.Getwd()
	dir := globalDir
	if dir == "" {
		dir = cwd
	}

	cfg := notes.DocsConfig{}

	for i := 0; i < len(args); i++ {
		if args[i] == "--dir" && i+1 < len(args) {
			i++
			cfg.Dir = args[i]
		}
	}

	// Read docs dir from .maitake/config if not specified
	if cfg.Dir == "" {
		maiCfg := e.GetConfig()
		if maiCfg.DocsDir != "" {
			cfg.Dir = maiCfg.DocsDir
		}
	}

	result, err := notes.SyncDocs(e, dir, cfg)
	if err != nil {
		fatal("docs sync: %v", err)
	}

	if globalJSON {
		printJSON(result)
		return
	}

	total := len(result.Written) + len(result.Imported) + len(result.Updated) + len(result.Removed)
	if total == 0 {
		fmt.Println("Everything in sync.")
		return
	}

	for _, f := range result.Written {
		fmt.Printf("  → %s (written from note)\n", f)
	}
	for _, f := range result.Imported {
		fmt.Printf("  ← %s (imported as note)\n", f)
	}
	for _, f := range result.Updated {
		fmt.Printf("  ↔ %s (note updated from file)\n", f)
	}
	for _, f := range result.Removed {
		fmt.Printf("  ✗ %s (removed, note closed)\n", f)
	}

	fmt.Printf("\n%d written, %d imported, %d updated, %d removed\n",
		len(result.Written), len(result.Imported), len(result.Updated), len(result.Removed))
}

func runCheck(e notes.Engine, args []string) {
	cwd, _ := os.Getwd()
	dir := globalDir
	if dir == "" {
		dir = cwd
	}

	result, err := notes.Check(e, dir)
	if err != nil {
		fatal("check: %v", err)
	}

	if globalJSON {
		printJSON(result)
		return
	}

	errors := len(result.BrokenCode) + len(result.BrokenWiki)

	// Summary
	fmt.Printf("Scanned: %d code refs, %d wiki links\n", len(result.CodeRefs), len(result.WikiRefs))

	if errors == 0 {
		fmt.Println("✓ All refs resolve.")
		return
	}

	fmt.Printf("\n%d broken ref(s):\n\n", errors)

	for _, e := range result.BrokenCode {
		fmt.Printf("  %s:%d  %s\n", e.File, e.Line, e.Message)
	}
	for _, e := range result.BrokenWiki {
		if e.NoteID != "" {
			fmt.Printf("  [%s]  %s\n", e.NoteID, e.Message)
		} else {
			fmt.Printf("  %s\n", e.Message)
		}
	}
	fmt.Println()
	os.Exit(1)
}

func runRefs(e notes.Engine, args []string) {
	if len(args) < 1 {
		fatal("usage: mai refs <id>")
	}
	target := args[0]

	cwd, _ := os.Getwd()
	dir := globalDir
	if dir == "" {
		dir = cwd
	}

	// Find code refs pointing at this target
	codeRefs, err := notes.ScanCodeRefs(dir)
	if err != nil {
		fatal("refs: %v", err)
	}

	// Find wiki refs in notes pointing at this target
	states, _ := e.Find(notes.FindOptions{})
	closedStates, _ := e.Find(notes.FindOptions{Status: "closed"})
	states = append(states, closedStates...)

	var codeMatches []notes.CodeRef
	var wikiMatches []notes.WikiRef

	for _, ref := range codeRefs {
		if matchesRef(ref.Target, target) {
			codeMatches = append(codeMatches, ref)
		}
	}

	for _, state := range states {
		for _, ref := range notes.ExtractWikiRefs(state.ID, state.Body) {
			if matchesRef(ref.Target, target) {
				wikiMatches = append(wikiMatches, ref)
			}
		}
		for _, c := range state.Comments {
			for _, ref := range notes.ExtractWikiRefs(state.ID, c.Body) {
				if matchesRef(ref.Target, target) {
					wikiMatches = append(wikiMatches, ref)
				}
			}
		}
	}

	if globalJSON {
		printJSON(map[string]interface{}{
			"target":   target,
			"code":     codeMatches,
			"wiki":     wikiMatches,
		})
		return
	}

	if len(codeMatches) == 0 && len(wikiMatches) == 0 {
		fmt.Printf("No references to %q found.\n", target)
		return
	}

	fmt.Printf("References to %q:\n\n", target)

	if len(codeMatches) > 0 {
		fmt.Println("Code refs:")
		for _, r := range codeMatches {
			fmt.Printf("  %s:%d  %s\n", r.File, r.Line, r.Raw)
		}
	}

	if len(wikiMatches) > 0 {
		if len(codeMatches) > 0 {
			fmt.Println()
		}
		fmt.Println("Note refs:")
		for _, r := range wikiMatches {
			fmt.Printf("  [%s]  [[%s]]\n", r.NoteID, r.Target)
		}
	}
}

func runExpand(e notes.Engine, args []string) {
	text := strings.Join(args, " ")
	if text == "" {
		fatal("usage: mai expand <text with [[refs]]>")
	}

	result, err := notes.Expand(e, text)
	if err != nil {
		fatal("expand: %v", err)
	}

	fmt.Print(result)
}

func matchesRef(refTarget, query string) bool {
	if refTarget == query {
		return true
	}
	if strings.Contains(refTarget, query) {
		return true
	}
	// Partial ID match
	if strings.Contains(query, refTarget) {
		return true
	}
	return false
}
