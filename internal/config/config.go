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

// ProviderSessionConfig defines how a provider maintains conversation context across state runs.
// Leave all fields empty to disable session support for that provider.
type ProviderSessionConfig struct {
	// InitArgs replaces Args on the first run of a session (no existing session ID).
	// If empty, the provider's Args are used unchanged.
	InitArgs []string `yaml:"init_args"`

	// ResumeArgs replaces Args when resuming an existing session.
	// Use {{.SessionID}} as a placeholder for the session ID.
	ResumeArgs []string `yaml:"resume_args"`

	// IDSource controls how the session ID is extracted from provider stdout.
	// "json"        – stdout is a single JSON object; extract IDField from it.
	// "jsonl_first" – first non-empty line of stdout is a JSON event; extract IDField from it.
	IDSource string `yaml:"id_source"`

	// IDField is a dot-notated path to the session ID within the parsed JSON.
	// Examples: "session_id", "payload.id"
	IDField string `yaml:"id_field"`

	// ResultSource controls how the text result is extracted when the provider
	// returns structured output instead of plain text.
	// "json"       – stdout is a JSON object; extract ResultField from it.
	// "jsonl_last" – scan stdout JSONL lines for the last event of ResultEventType; extract ResultField.
	// Empty        – raw stdout is the result (default when session is not configured).
	ResultSource string `yaml:"result_source"`

	// ResultField is the JSON field containing the text response.
	ResultField string `yaml:"result_field"`

	// ResultEventType is the JSONL event type to match when ResultSource is "jsonl_last".
	ResultEventType string `yaml:"result_event_type"`
}

// Enabled reports whether session support is fully configured.
func (s ProviderSessionConfig) Enabled() bool {
	return len(s.ResumeArgs) > 0 && s.IDSource != "" && s.IDField != ""
}

// ProviderCommand specifies the executable and arguments used to invoke an AI provider.
type ProviderCommand struct {
	Command string                `yaml:"command"`
	Args    []string              `yaml:"args"`
	Session ProviderSessionConfig `yaml:"session"`
}

// Config holds all runtime configuration for an AutoPR instance, loaded from ~/.auto-pr/config.yaml.
type Config struct {
	Provider               string                     `yaml:"provider"`
	GuidelinesFile         string                     `yaml:"guidelines_file"`
	StateDirName           string                     `yaml:"state_dir_name"`
	RepositoryDirs         []string                   `yaml:"repository_directories"`
	ServerPort             int                        `yaml:"server_port"`
	ServerWorkers          int                        `yaml:"server_workers"`
	CreatePR               bool                       `yaml:"create_pr"`
	MaxFixAttempts         int                        `yaml:"max_fix_attempts"`
	BaseBranch             string                     `yaml:"base_branch"`
	CheckCommands          []string                   `yaml:"check_commands"`
	FormatCommands         []string                   `yaml:"format_commands"`
	LintCommands           []string                   `yaml:"lint_commands"`
	DiscoverTicketsCommand string                     `yaml:"discover_tickets_command"`
	Providers              map[string]ProviderCommand `yaml:"providers"`
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
		DiscoverTicketsCommand: `curl -fsSL \
  -H "Content-Type: application/json" \
  -H "Shortcut-Token: ${SHORTCUT_API_TOKEN:?SHORTCUT_API_TOKEN is required}" \
  -d '{"label_name":"auto-pr","workflow_state_types":["backlog","unstarted"]}' \
  https://api.app.shortcut.com/api/v3/stories/search \
| python3 -c 'import json, sys; stories = json.load(sys.stdin); print(json.dumps([{"ticket_number": "SC-{}".format(story["id"]), "title": story["name"]} for story in stories]))'`,
		Providers: map[string]ProviderCommand{
			"gemini": {Command: "gemini", Args: []string{}},
			"codex": {
				Command: "codex",
				Args:    []string{"exec", "-"},
				Session: ProviderSessionConfig{
					InitArgs:        []string{"exec", "-", "--json"},
					ResumeArgs:      []string{"exec", "resume", "{{.SessionID}}", "--json"},
					IDSource:        "jsonl_first",
					IDField:         "payload.id",
					ResultSource:    "jsonl_last",
					ResultField:     "content",
					ResultEventType: "agent_message", // NOTE: verify against actual codex --json output
				},
			},
			"claude-code": {
				Command: "claude",
				Args:    []string{"--print"},
				Session: ProviderSessionConfig{
					InitArgs:     []string{"--print", "--output-format", "json"},
					ResumeArgs:   []string{"--print", "--output-format", "json", "--resume", "{{.SessionID}}"},
					IDSource:     "json",
					IDField:      "session_id",
					ResultSource: "json",
					ResultField:  "result",
				},
			},
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

// LogsDirPath returns the absolute path to the user-level logs directory (~/.auto-pr/logs).
func LogsDirPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}

	return filepath.Join(home, ".auto-pr", "logs"), nil
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
