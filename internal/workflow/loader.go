package workflow

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// userHomeDir is a variable so tests can override it.
var userHomeDir = os.UserHomeDir //nolint:gochecknoglobals

// Load returns a Config using the three-level hierarchy:
//  1. <repoRoot>/.auto-pr/workflow.yaml
//  2. ~/.auto-pr/workflow.yaml
//  3. Embedded binary default
func Load(repoRoot string) (Config, error) {
	projectPath := filepath.Join(repoRoot, ".auto-pr", "workflow.yaml")
	cfg, ok, err := loadFromFile(projectPath)
	if err != nil {
		return Config{}, fmt.Errorf("load project workflow config: %w", err)
	}
	if ok {
		return cfg, nil
	}

	home, err := userHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("resolve home dir: %w", err)
	}
	globalPath := filepath.Join(home, ".auto-pr", "workflow.yaml")
	cfg, ok, err = loadFromFile(globalPath)
	if err != nil {
		return Config{}, fmt.Errorf("load global workflow config: %w", err)
	}
	if ok {
		return cfg, nil
	}

	return loadEmbeddedDefault()
}

// ReadPrompt returns the content of a prompt file using the same three-level hierarchy:
//  1. <repoRoot>/.auto-pr/<promptRelPath>
//  2. ~/.auto-pr/<promptRelPath>
//  3. Embedded binary default
func ReadPrompt(repoRoot, promptRelPath string) ([]byte, error) {
	projectPath := filepath.Join(repoRoot, ".auto-pr", promptRelPath)
	data, err := os.ReadFile(projectPath)
	if err == nil {
		return data, nil
	}

	home, err := userHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	globalPath := filepath.Join(home, ".auto-pr", promptRelPath)
	data, err = os.ReadFile(globalPath)
	if err == nil {
		return data, nil
	}

	data, err = fs.ReadFile(embeddedPromptsFS, promptRelPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("prompt %q: %w", promptRelPath, ErrPromptNotFound)
		}

		return nil, fmt.Errorf("read embedded prompt %q: %w", promptRelPath, err)
	}

	return data, nil
}

func loadFromFile(path string) (Config, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, false, nil
		}

		return Config{}, false, fmt.Errorf("read workflow file: %w", err)
	}
	cfg, err := parse(data)
	if err != nil {
		return Config{}, false, err
	}

	return cfg, true, nil
}

func loadEmbeddedDefault() (Config, error) {
	cfg, err := parse(embeddedWorkflowYAML)
	if err != nil {
		return Config{}, fmt.Errorf("embedded default workflow is invalid: %w", err)
	}

	return cfg, nil
}

func parse(data []byte) (Config, error) {
	var cfg Config
	err := yaml.Unmarshal(data, &cfg)
	if err != nil {
		return Config{}, fmt.Errorf("parse workflow yaml: %w", err)
	}
	err = cfg.Validate()
	if err != nil {
		return Config{}, fmt.Errorf("invalid workflow config: %w", err)
	}

	return cfg, nil
}
