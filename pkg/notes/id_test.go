package notes

import (
	"regexp"
	"testing"
)

func TestGenerateID_Format(t *testing.T) {
	id, err := GenerateID("/home/user/my-project")
	if err != nil {
		t.Fatal(err)
	}
	// Should be prefix-XXXX where prefix is from dir name segments
	matched, _ := regexp.MatchString(`^[a-z]+-[a-z0-9]{4}$`, id)
	if !matched {
		t.Errorf("ID %q doesn't match expected format", id)
	}
}

func TestGenerateID_PrefixExtraction(t *testing.T) {
	tests := []struct {
		dir    string
		prefix string // expected prefix (before the dash)
	}{
		{"/home/user/my-project", "mp"},
		{"/home/user/trek", "tre"},
		{"/home/user/pi-extensions", "pe"},
		{"/home/user/a-b-c", "abc"},
		{"/home/user/single", "sin"},
	}

	for _, tt := range tests {
		id, err := GenerateID(tt.dir)
		if err != nil {
			t.Errorf("GenerateID(%q): %v", tt.dir, err)
			continue
		}
		parts := splitID(id)
		if parts[0] != tt.prefix {
			t.Errorf("GenerateID(%q) prefix = %q, want %q (full id: %q)", tt.dir, parts[0], tt.prefix, id)
		}
	}
}

func TestGenerateID_Uniqueness(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := GenerateID("/tmp/test-project")
		if err != nil {
			t.Fatal(err)
		}
		if ids[id] {
			t.Fatalf("duplicate ID after %d generations: %s", i, id)
		}
		ids[id] = true
	}
}

func splitID(id string) []string {
	for i := len(id) - 1; i >= 0; i-- {
		if id[i] == '-' {
			return []string{id[:i], id[i+1:]}
		}
	}
	return []string{id}
}
