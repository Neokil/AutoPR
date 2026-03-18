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

func renderPromptTemplate(promptsDir, name string, data interface{}) (string, error) {
	defaultTemplate, err := defaultPromptTemplate(name)
	if err != nil {
		return "", err
	}
	templatePath := filepath.Join(promptsDir, name)
	templateContent, err := readOrCreatePromptTemplate(templatePath, defaultTemplate)
	if err != nil {
		return "", err
	}
	tpl, err := template.New(name).Parse(templateContent)
	if err != nil {
		return "", fmt.Errorf("parse prompt template %s: %w", templatePath, err)
	}
	var out bytes.Buffer
	if err := tpl.Execute(&out, data); err != nil {
		return "", fmt.Errorf("execute prompt template %s: %w", templatePath, err)
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

func readOrCreatePromptTemplate(path, fallback string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create prompts dir: %w", err)
	}
	data, err := os.ReadFile(path)
	if err == nil {
		return string(data), nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("read prompt template %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(fallback), 0o644); err != nil {
		return "", fmt.Errorf("write default prompt template %s: %w", path, err)
	}
	return fallback, nil
}
