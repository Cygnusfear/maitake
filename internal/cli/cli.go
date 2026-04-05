// Package cli provides shared helpers for mai and mai-* plugin binaries.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// assumeYes is set when -y/--yes was parsed by a binary, or propagated via MAI_YES=1.
var assumeYes bool

// SetAssumeYes flips the process-wide auto-confirm flag. Binaries call this
// after parsing their own -y/--yes flag. The env var MAI_YES=1 is also honored.
func SetAssumeYes(v bool) { assumeYes = v }

// AssumeYes reports whether prompts should be auto-confirmed — either because
// -y/--yes was set locally or MAI_YES=1 was propagated from the parent mai process.
func AssumeYes() bool {
	return assumeYes || os.Getenv("MAI_YES") == "1"
}

// Confirm prints prompt and reads a y/N answer from stdin. Returns true if the
// user answers y/Y, or immediately true if AssumeYes() is set. Use for any
// destructive or bulk operation.
func Confirm(prompt string) bool {
	if AssumeYes() {
		return true
	}
	fmt.Print(prompt)
	var answer string
	fmt.Scanln(&answer)
	return answer == "y" || answer == "Y"
}

// Fatal prints an error message and exits.
func Fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "mai: "+format+"\n", args...)
	os.Exit(1)
}

// PrintJSON writes JSON to stdout.
func PrintJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

// FlagSet holds common parsed CLI flags.
type FlagSet struct {
	Kind     string
	Typ      string
	Title    string
	Priority int
	Assignee string
	Tags     []string
	Targets  []string
	Body     string
	Status   string
	Message  string
	Help     bool
}

// ParseFlags parses common mai flags from args.
func ParseFlags(args []string) (FlagSet, []string) {
	var f FlagSet
	var positional []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			f.Help = true
		case "-k", "--kind":
			i++; if i < len(args) { f.Kind = args[i] }
		case "-t", "--title":
			i++; if i < len(args) { f.Title = args[i] }
		case "--type":
			i++; if i < len(args) { f.Typ = args[i] }
		case "-p", "--priority":
			i++; if i < len(args) { fmt.Sscanf(args[i], "%d", &f.Priority) }
		case "-a", "--assignee":
			i++; if i < len(args) { f.Assignee = args[i] }
		case "-l", "--tags":
			i++; if i < len(args) { f.Tags = strings.Split(args[i], ",") }
		case "--target":
			i++; if i < len(args) { f.Targets = append(f.Targets, args[i]) }
		case "-d", "--description", "-m", "--message":
			i++; if i < len(args) { f.Body = args[i] }
		case "--status":
			i++; if i < len(args) { f.Status = args[i] }
		default:
			if strings.HasPrefix(args[i], "--status=") {
				f.Status = strings.TrimPrefix(args[i], "--status=")
			} else if strings.HasPrefix(args[i], "-k=") {
				f.Kind = strings.TrimPrefix(args[i], "-k=")
			} else if strings.HasPrefix(args[i], "-") {
				Fatal("unknown flag: %s", args[i])
			} else {
				positional = append(positional, args[i])
			}
		}
	}
	return f, positional
}
