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
var userHomeDir = os.UserHomeDir

// Load returns a WorkflowConfig using the three-level hierarchy:
//  1. <repoRoot>/.auto-pr/workflow.yaml
//  2. ~/.auto-pr/workflow.yaml
//  3. Embedded binary default
func Load(repoRoot string) (WorkflowConfig, error) {
	projectPath := filepath.Join(repoRoot, ".auto-pr", "workflow.yaml")
	if cfg, ok, err := loadFromFile(projectPath); err != nil {
		return WorkflowConfig{}, fmt.Errorf("load project workflow config: %w", err)
	} else if ok {
		return cfg, nil
	}

	home, err := userHomeDir()
	if err != nil {
		return WorkflowConfig{}, fmt.Errorf("resolve home dir: %w", err)
	}
	globalPath := filepath.Join(home, ".auto-pr", "workflow.yaml")
	if cfg, ok, err := loadFromFile(globalPath); err != nil {
		return WorkflowConfig{}, fmt.Errorf("load global workflow config: %w", err)
	} else if ok {
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
	if data, err := os.ReadFile(projectPath); err == nil {
		return data, nil
	}

	home, err := userHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	globalPath := filepath.Join(home, ".auto-pr", promptRelPath)
	if data, err := os.ReadFile(globalPath); err == nil {
		return data, nil
	}

	data, err := fs.ReadFile(embeddedPromptsFS, promptRelPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("prompt %q not found in project, global config, or embedded defaults", promptRelPath)
		}
		return nil, fmt.Errorf("read embedded prompt %q: %w", promptRelPath, err)
	}
	return data, nil
}

func loadFromFile(path string) (WorkflowConfig, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return WorkflowConfig{}, false, nil
		}
		return WorkflowConfig{}, false, err
	}
	cfg, err := parse(data)
	if err != nil {
		return WorkflowConfig{}, false, err
	}
	return cfg, true, nil
}

func loadEmbeddedDefault() (WorkflowConfig, error) {
	cfg, err := parse(embeddedWorkflowYAML)
	if err != nil {
		return WorkflowConfig{}, fmt.Errorf("embedded default workflow is invalid: %w", err)
	}
	return cfg, nil
}

func parse(data []byte) (WorkflowConfig, error) {
	var cfg WorkflowConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return WorkflowConfig{}, fmt.Errorf("parse workflow yaml: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return WorkflowConfig{}, fmt.Errorf("invalid workflow config: %w", err)
	}
	return cfg, nil
}
