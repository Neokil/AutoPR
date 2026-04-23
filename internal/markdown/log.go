package markdown

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func AppendSection(path, title, body string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = fmt.Fprintf(f, "\n## %s (%s)\n\n%s\n", title, time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(body))
	return err
}

func Write(path string, content string) error {
	return os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644)
}

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
