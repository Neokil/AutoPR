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

func legacyConfigPaths() ([]string, error) {
	paths := []string{}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	paths = append(paths, filepath.Join(home, ".ai-orchestrator", "config.yaml"))

	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(home, ".config")
	}
	paths = append(paths, filepath.Join(base, "ai-orchestrator", "config.yaml"))
	return paths, nil
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
			legacyPaths, lerr := legacyConfigPaths()
			if lerr != nil {
				return cfg, lerr
			}
			found := false
			for _, legacyPath := range legacyPaths {
				legacyData, lerr := os.ReadFile(legacyPath)
				if lerr != nil {
					if os.IsNotExist(lerr) {
						continue
					}
					return cfg, fmt.Errorf("read legacy config file %s: %w", legacyPath, lerr)
				}
				data = legacyData
				found = true
				break
			}
			if !found {
				return cfg, nil
			}
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
