// Package config defines the application configuration structure and loading logic.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	defaultServerPort    = 8080
	defaultServerWorkers = 4
)

// ProviderCommand specifies the executable and arguments used to invoke an AI provider.
type ProviderCommand struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
}

// Config holds all runtime configuration for an AutoPR instance, loaded from ~/.auto-pr/config.yaml.
type Config struct {
	Provider       string                     `yaml:"provider"`
	GuidelinesFile string                     `yaml:"guidelines_file"`
	StateDirName   string                     `yaml:"state_dir_name"`
	RepositoryDirs []string                   `yaml:"repository_directories"`
	ServerPort     int                        `yaml:"server_port"`
	ServerWorkers  int                        `yaml:"server_workers"`
	CreatePR       bool                       `yaml:"create_pr"`
	MaxFixAttempts int                        `yaml:"max_fix_attempts"`
	BaseBranch     string                     `yaml:"base_branch"`
	CheckCommands  []string                   `yaml:"check_commands"`
	FormatCommands []string                   `yaml:"format_commands"`
	LintCommands   []string                   `yaml:"lint_commands"`
	Providers      map[string]ProviderCommand `yaml:"providers"`
}

// Default returns a Config populated with built-in defaults.
func Default() Config {
	return Config{
		Provider:       "codex",
		StateDirName:   ".auto-pr",
		RepositoryDirs: []string{},
		ServerPort:     defaultServerPort,
		ServerWorkers:  defaultServerWorkers,
		CreatePR:       true,
		MaxFixAttempts: 1,
		CheckCommands:  []string{},
		Providers: map[string]ProviderCommand{
			"gemini":      {Command: "gemini", Args: []string{}},
			"codex":       {Command: "codex", Args: []string{"exec", "-"}},
			"claude-code": {Command: "claude", Args: []string{"--print"}},
		},
	}
}

// Path returns the absolute path to the user-level config file (~/.auto-pr/config.yaml).
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}

	return filepath.Join(home, ".auto-pr", "config.yaml"), nil
}

// PromptsDirPath returns the absolute path to the user-level prompts directory (~/.auto-pr/prompts).
func PromptsDirPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}

	return filepath.Join(home, ".auto-pr", "prompts"), nil
}

// Load reads the config file and merges it over the defaults, returning the result.
// Missing file is not an error; individual unset fields fall back to their defaults.
func Load() (Config, error) {
	cfg := Default()
	path, err := Path()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path) //nolint:gosec // G304: path built from trusted config hierarchy
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}

		return cfg, fmt.Errorf("read config file %s: %w", path, err)
	}
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return cfg, fmt.Errorf("parse config yaml: %w", err)
	}
	if cfg.Provider == "" {
		cfg.Provider = "codex"
	}
	if cfg.StateDirName == "" {
		cfg.StateDirName = ".auto-pr"
	}
	if cfg.ServerPort <= 0 {
		cfg.ServerPort = defaultServerPort
	}
	if cfg.ServerWorkers <= 0 {
		cfg.ServerWorkers = defaultServerWorkers
	}
	if cfg.MaxFixAttempts < 0 {
		cfg.MaxFixAttempts = 0
	}
	if cfg.Providers == nil {
		cfg.Providers = Default().Providers
	}
	if cfg.RepositoryDirs == nil {
		cfg.RepositoryDirs = []string{}
	}

	return cfg, nil
}

// ResolveGuidelinesPath returns the absolute path to the guidelines file, resolving relative
// paths against repoRoot. Returns an empty string if no guidelines file is configured.
func ResolveGuidelinesPath(repoRoot string, cfg Config) string {
	if cfg.GuidelinesFile == "" {
		return ""
	}
	if filepath.IsAbs(cfg.GuidelinesFile) {
		return cfg.GuidelinesFile
	}

	return filepath.Join(repoRoot, cfg.GuidelinesFile)
}
