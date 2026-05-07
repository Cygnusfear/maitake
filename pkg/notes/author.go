package notes

import "os"

func (e *RealEngine) stampAuthor(note *Note) {
	name, email := e.resolveAuthor()
	note.Author = name
	note.AuthorEmail = email
}

func (e *RealEngine) resolveAuthor() (string, string) {
	name := firstEnv("MAI_AUTHOR_NAME", "GIT_AUTHOR_NAME")
	email := firstEnv("MAI_AUTHOR_EMAIL", "GIT_AUTHOR_EMAIL")

	if name == "" {
		if configuredName, err := e.repo.GetUserName(); err == nil {
			name = configuredName
		}
	}
	if email == "" {
		if configuredEmail, err := e.repo.GetUserEmail(); err == nil {
			email = configuredEmail
		}
	}
	if name == "" {
		name = email
	}
	return name, email
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}
