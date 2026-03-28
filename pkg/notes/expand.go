package notes

import (
	"fmt"
	"strings"
)

// Expand replaces [[ref]] wiki links in text with resolved note content.
// Returns the expanded text with a <mai-context> block appended.
func Expand(engine Engine, text string) (string, error) {
	matches := wikiLinkPattern.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text, nil
	}

	type resolved struct {
		target  string
		state   *State
		isFile  bool
		path    string
	}

	var resolutions []resolved
	seen := make(map[string]bool)

	for _, m := range matches {
		// m[2]:m[3] is the capture group (target inside [[]])
		target := text[m[2]:m[3]]
		if seen[target] {
			continue
		}
		seen[target] = true

		// Try to resolve as note ID
		state, err := engine.Fold(target)
		if err == nil && state != nil {
			resolutions = append(resolutions, resolved{target: target, state: state})
			continue
		}

		// Try partial match through Find
		results, _ := engine.Find(FindOptions{})
		for _, s := range results {
			if strings.Contains(s.ID, target) || strings.EqualFold(s.Title, target) {
				stateCopy := s
				resolutions = append(resolutions, resolved{target: target, state: &stateCopy})
				break
			}
		}
	}

	if len(resolutions) == 0 {
		return text, nil
	}

	// Replace [[refs]] inline with resolved IDs
	output := wikiLinkPattern.ReplaceAllStringFunc(text, func(match string) string {
		inner := wikiLinkPattern.FindStringSubmatch(match)
		if len(inner) < 2 {
			return match
		}
		target := inner[1]
		for _, r := range resolutions {
			if r.target == target && r.state != nil {
				return fmt.Sprintf("[[%s]]", r.state.ID)
			}
		}
		return match
	})

	// Append context block
	var ctx strings.Builder
	ctx.WriteString("\n\n<mai-context>\n")
	for _, r := range resolutions {
		if r.state == nil {
			ctx.WriteString(fmt.Sprintf("* `[[%s]]` — not found\n", r.target))
			continue
		}
		ctx.WriteString(fmt.Sprintf("* `[[%s]]` → %s\n", r.target, r.state.ID))
		if r.state.Title != "" {
			ctx.WriteString(fmt.Sprintf("  * %s\n", r.state.Title))
		}
		if r.state.Body != "" {
			// First paragraph only
			body := firstParagraph(r.state.Body)
			if body != "" {
				ctx.WriteString(fmt.Sprintf("  * %s\n", body))
			}
		}
		if len(r.state.Targets) > 0 {
			ctx.WriteString(fmt.Sprintf("  * targets: %s\n", strings.Join(r.state.Targets, ", ")))
		}
	}
	ctx.WriteString("</mai-context>\n")

	return output + ctx.String(), nil
}

// ExpandRefs is a simpler version that just resolves [[ref]] to IDs without context block.
func ExpandRefs(engine Engine, text string) string {
	return wikiLinkPattern.ReplaceAllStringFunc(text, func(match string) string {
		inner := wikiLinkPattern.FindStringSubmatch(match)
		if len(inner) < 2 {
			return match
		}
		target := inner[1]
		state, err := engine.Fold(target)
		if err == nil && state != nil {
			return fmt.Sprintf("[[%s]]", state.ID)
		}
		return match
	})
}

func firstParagraph(body string) string {
	lines := strings.SplitN(body, "\n\n", 2)
	p := strings.TrimSpace(lines[0])
	// Skip markdown headings
	if strings.HasPrefix(p, "#") {
		if len(lines) > 1 {
			parts := strings.SplitN(lines[1], "\n\n", 2)
			p = strings.TrimSpace(parts[0])
		} else {
			return ""
		}
	}
	if len(p) > 250 {
		return p[:250] + "..."
	}
	return p
}

