package notes

import (
	"crypto/rand"
	"fmt"
	"path/filepath"
	"strings"
)

// GenerateID creates a human-readable ID from the directory name + 4 random alphanumeric chars.
// The prefix is built from the first letter of each hyphen/underscore-separated segment.
func GenerateID(dir string) (string, error) {
	dirName := filepath.Base(dir)

	// Extract first letter of each segment
	var prefix strings.Builder
	segments := strings.FieldsFunc(dirName, func(r rune) bool {
		return r == '-' || r == '_'
	})
	for _, seg := range segments {
		if len(seg) > 0 {
			prefix.WriteByte(seg[0])
		}
	}

	p := strings.ToLower(prefix.String())
	if len(p) < 2 {
		if len(dirName) >= 3 {
			p = strings.ToLower(dirName[:3])
		} else {
			p = strings.ToLower(dirName)
		}
	}

	suffix, err := randomAlphanumeric(4)
	if err != nil {
		return "", fmt.Errorf("generating ID suffix: %w", err)
	}

	return p + "-" + suffix, nil
}

func randomAlphanumeric(n int) (string, error) {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b), nil
}
