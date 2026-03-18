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

var promptTemplateNames = []string{
	tplTicket,
	tplInvestigate,
	tplImplement,
	tplPR,
}

//go:embed prompts/*.md.tmpl
var embeddedPromptFS embed.FS

func initializePromptTemplates(promptsDir string) error {
	for _, name := range promptTemplateNames {
		source, err := ensurePromptTemplate(promptsDir, name)
		if err != nil {
			return err
		}
		fmt.Printf("prompt template %s: %s\n", name, source)
	}
	return nil
}

func renderPromptTemplate(promptsDir, name string, data interface{}) (string, error) {
	templatePath := filepath.Join(promptsDir, name)
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		if os.IsNotExist(err) {
			if _, ensureErr := ensurePromptTemplate(promptsDir, name); ensureErr != nil {
				return "", ensureErr
			}
			templateContent, err = os.ReadFile(templatePath)
			if err != nil {
				return "", fmt.Errorf("read prompt template %s: %w", templatePath, err)
			}
		} else {
			return "", fmt.Errorf("read prompt template %s: %w", templatePath, err)
		}
	}
	tpl, err := template.New(name).Parse(string(templateContent))
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

func ensurePromptTemplate(promptsDir, name string) (string, error) {
	path := filepath.Join(promptsDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create prompts dir: %w", err)
	}
	if _, err := os.Stat(path); err == nil {
		return "loaded from file", nil
	}
	fallback, err := defaultPromptTemplate(name)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(fallback), 0o644); err != nil {
		return "", fmt.Errorf("write default prompt template %s: %w", path, err)
	}
	return "loaded from embedded template", nil
}

func readOrCreatePromptTemplate(path, fallback string) (string, error) {
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
