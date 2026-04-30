package markdown

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// AppendSection appends a markdown H2 section with a timestamp and body to the file at path,
// creating the file if it does not exist.
func AppendSection(path, title, body string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer func() { _ = f.Close() }()
	_, err = fmt.Fprintf(f, "\n## %s (%s)\n\n%s\n", title, time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(body))
	if err != nil {
		return fmt.Errorf("write log section: %w", err)
	}

	return nil
}

// Write writes content to path as a markdown file, trimming leading/trailing whitespace.
func Write(path string, content string) error {
	err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644)
	if err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// Tail returns the last maxLines lines of the file at path, or the full content if shorter.
// Returns an empty string if the file cannot be read.
func Tail(path string, maxLines int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) <= maxLines {
		return string(data)
	}

	return strings.Join(lines[len(lines)-maxLines:], "\n")
}
