package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type ProviderCommand struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
}

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

func Default() Config {
	return Config{
		Provider:       "codex",
		StateDirName:   ".auto-pr",
		RepositoryDirs: []string{},
		ServerPort:     8080,
		ServerWorkers:  4,
		CreatePR:       true,
		MaxFixAttempts: 1,
		CheckCommands:  []string{},
		Providers: map[string]ProviderCommand{
			"gemini": {Command: "gemini", Args: []string{}},
			"codex":  {Command: "codex", Args: []string{"exec", "-"}},
		},
	}
}

func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".auto-pr", "config.yaml"), nil
}

func Load() (Config, error) {
	cfg := Default()
	path, err := ConfigPath()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		} else {
			return cfg, fmt.Errorf("read config file %s: %w", path, err)
		}
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config yaml: %w", err)
	}
	if cfg.Provider == "" {
		cfg.Provider = "codex"
	}
	if cfg.StateDirName == "" {
		cfg.StateDirName = ".auto-pr"
	}
	if cfg.ServerPort <= 0 {
		cfg.ServerPort = 8080
	}
	if cfg.ServerWorkers <= 0 {
		cfg.ServerWorkers = 4
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

func ResolveGuidelinesPath(repoRoot string, cfg Config) string {
	if cfg.GuidelinesFile == "" {
		return ""
	}
	if filepath.IsAbs(cfg.GuidelinesFile) {
		return cfg.GuidelinesFile
	}
	return filepath.Join(repoRoot, cfg.GuidelinesFile)
}
