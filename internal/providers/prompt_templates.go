package providers

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"text/template"
)

const (
	tplTicket      = "ticket.md.tmpl"
	tplInvestigate = "investigate.md.tmpl"
	tplImplement   = "implement.md.tmpl"
	tplPR          = "pr.md.tmpl"
)

//go:embed prompts/*.md.tmpl
var embeddedPromptFS embed.FS

func renderPromptTemplate(promptsDir, providerName, name string, data interface{}) (string, error) {
	// Check for a provider-specific override first.
	if providerName != "" {
		providerPath := filepath.Join(promptsDir, providerName, name)
		if content, err := os.ReadFile(providerPath); err == nil {
			tpl, err := template.New(name).Parse(string(content))
			if err != nil {
				return "", fmt.Errorf("parse prompt template %s: %w", providerPath, err)
			}
			var out bytes.Buffer
			if err := tpl.Execute(&out, data); err != nil {
				return "", fmt.Errorf("execute prompt template %s: %w", providerPath, err)
			}
			return out.String(), nil
		}
	}

	// Fall back to the built-in embedded template.
	content, err := defaultPromptTemplate(name)
	if err != nil {
		return "", err
	}
	tpl, err := template.New(name).Parse(content)
	if err != nil {
		return "", fmt.Errorf("parse prompt template %s: %w", name, err)
	}
	var out bytes.Buffer
	if err := tpl.Execute(&out, data); err != nil {
		return "", fmt.Errorf("execute prompt template %s: %w", name, err)
	}
	return out.String(), nil
}

func defaultPromptTemplate(name string) (string, error) {
	path := filepath.Join("prompts", name)
	data, err := fs.ReadFile(embeddedPromptFS, path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("unknown prompt template: %s", name)
		}
		return "", fmt.Errorf("read embedded prompt template %s: %w", name, err)
	}
	return string(data), nil
}
