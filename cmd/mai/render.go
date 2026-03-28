package main

import (
	"fmt"
	"strings"

	"github.com/cygnusfear/maitake/pkg/notes"
)

func printState(s *notes.State) {
	fmt.Printf("%-8s [%s] %s\n", s.ID, s.Status, s.Title)
	fmt.Printf("kind: %s", s.Kind)
	if s.Type != "" {
		fmt.Printf("  type: %s", s.Type)
	}
	if s.Priority != 0 {
		fmt.Printf("  priority: %d", s.Priority)
	}
	if s.Assignee != "" {
		fmt.Printf("  assignee: %s", s.Assignee)
	}
	fmt.Println()

	if len(s.Tags) > 0 {
		fmt.Printf("tags: %s\n", strings.Join(s.Tags, ", "))
	}
	if len(s.Targets) > 0 {
		fmt.Printf("targets: %s\n", strings.Join(s.Targets, ", "))
	}
	if len(s.Deps) > 0 {
		fmt.Printf("deps: %s\n", strings.Join(s.Deps, ", "))
	}
	if len(s.Links) > 0 {
		fmt.Printf("links: %s\n", strings.Join(s.Links, ", "))
	}
	if s.ParentID != "" {
		fmt.Printf("parent: %s\n", s.ParentID)
	}
	if s.Resolved != nil {
		if *s.Resolved {
			fmt.Println("resolved: yes")
		} else {
			fmt.Println("resolved: no")
		}
	}

	if !s.CreatedAt.IsZero() {
		fmt.Printf("created: %s\n", s.CreatedAt.Format("2006-01-02 15:04"))
	}
	if !s.UpdatedAt.IsZero() && s.UpdatedAt != s.CreatedAt {
		fmt.Printf("updated: %s\n", s.UpdatedAt.Format("2006-01-02 15:04"))
	}

	if s.Body != "" {
		fmt.Println()
		fmt.Println(s.Body)
	}

	if len(s.Comments) > 0 {
		fmt.Println()
		fmt.Println("## Comments")
		for _, c := range s.Comments {
			ts := ""
			if c.Timestamp != "" {
				ts = c.Timestamp
			}
			author := c.Author
			if author == "" {
				author = "agent"
			}
			fmt.Printf("\n**%s** (%s)\n\n%s\n", ts, author, c.Body)
		}
	}
}

func printSummaryLine(s notes.StateSummary) {
	tags := ""
	if len(s.Tags) > 0 {
		tags = " [" + strings.Join(s.Tags, ",") + "]"
	}
	status := s.Status
	if s.Priority > 0 {
		status = fmt.Sprintf("P%d|%s", s.Priority, s.Status)
	}
	fmt.Printf("%-8s [%-12s] %s%s\n", s.ID, status, s.Title, tags)
}

func printSummaryFromState(s *notes.State) {
	summary := notes.ToSummary(s)
	printSummaryLine(summary)
}

func printContextLine(s *notes.State) {
	resolved := ""
	if s.Resolved != nil {
		if *s.Resolved {
			resolved = " ✓"
		} else {
			resolved = " ✗"
		}
	}
	fmt.Printf("%s [%s] (%s) %s%s\n", s.ID, s.Kind, s.Status, s.Title, resolved)
	if s.Body != "" {
		// Show first 2 lines of body indented
		lines := strings.Split(s.Body, "\n")
		max := 2
		if len(lines) < max {
			max = len(lines)
		}
		for _, line := range lines[:max] {
			fmt.Printf("  %s\n", line)
		}
	}
	fmt.Println()
}
