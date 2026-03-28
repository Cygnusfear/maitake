package notes

import (
	"crypto/rand"
	"fmt"
	"path/filepath"
	"strings"
)

const randomIDAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// GenerateID creates a human-readable note ID from the directory name and a random suffix.
func GenerateID(dir string) (string, error) {
	dirName := filepath.Base(filepath.Clean(dir))
	prefix := idPrefix(dirName)
	suffix, err := randomAlphanumeric(4)
	if err != nil {
		return "", fmt.Errorf("generate random suffix: %w", err)
	}

	return fmt.Sprintf("%s-%s", prefix, suffix), nil
}

func idPrefix(dirName string) string {
	segments := strings.FieldsFunc(dirName, func(r rune) bool {
		return r == '-' || r == '_'
	})

	var prefix strings.Builder
	for _, segment := range segments {
		if segment == "" {
			continue
		}
		prefix.WriteByte(byte(strings.ToLower(segment)[0]))
	}

	value := prefix.String()
	if len(value) >= 2 {
		return value
	}
	if len(dirName) >= 3 {
		return strings.ToLower(dirName[:3])
	}
	return strings.ToLower(dirName)
}

func randomAlphanumeric(length int) (string, error) {
	buf := make([]byte, length)
	random := make([]byte, length)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}

	for i := range buf {
		buf[i] = randomIDAlphabet[int(random[i])%len(randomIDAlphabet)]
	}

	return string(buf), nil
}
