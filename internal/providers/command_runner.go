package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Neokil/AutoPR/internal/config"
	"github.com/Neokil/AutoPR/internal/shell"
)

// PromptCommandRunner runs a configured CLI command with a prompt piped to stdin.
type PromptCommandRunner struct {
	providerName string
	command      string
	args         []string
	sessionCfg   config.ProviderSessionConfig
}

// Run executes the provider command in worktreePath, writing prompt artifacts to runtimeDir.
// sessionData is the opaque session token from the previous run; empty on first run.
// Returns the extracted text output, stderr, the new session token, and any execution error.
func (r *PromptCommandRunner) Run(ctx context.Context, worktreePath, runtimeDir, phase, prompt, sessionData string) (text, stderr, newSessionData string, err error) {
	if writeErr := writePromptArtifacts(runtimeDir, phase, prompt, "", ""); writeErr != nil {
		return "", "", "", writeErr
	}

	args := r.buildArgs(sessionData)
	res, runErr := shell.Run(ctx, worktreePath, nil, prompt, r.command, args...)

	text = extractTextResult(res.Stdout, r.sessionCfg)
	newSessionData = extractSessionID(res.Stdout, r.sessionCfg)
	_ = writePromptArtifacts(runtimeDir, phase, prompt, text, res.Stderr)

	if runErr != nil {
		if looksLikeTokensExhausted(res.Stderr, res.Stdout) {
			return text, res.Stderr, newSessionData, fmt.Errorf("provider %s phase %s: %w", r.providerName, phase, ErrTokensExhausted)
		}

		return text, res.Stderr, newSessionData, fmt.Errorf("provider %s phase %s failed: %w", r.providerName, phase, runErr)
	}
	if strings.TrimSpace(text) == "" {
		return "", res.Stderr, "", fmt.Errorf("provider %s phase %s: %w", r.providerName, phase, ErrEmptyOutput)
	}

	return text, res.Stderr, newSessionData, nil
}

// buildArgs returns the args to use for this invocation based on session state.
func (r *PromptCommandRunner) buildArgs(sessionData string) []string {
	if !r.sessionCfg.Enabled() {
		return r.args
	}
	if sessionData != "" {
		return applySessionID(r.sessionCfg.ResumeArgs, sessionData)
	}
	if len(r.sessionCfg.InitArgs) > 0 {
		return r.sessionCfg.InitArgs
	}

	return r.args
}

// applySessionID substitutes {{.SessionID}} in args with the given session ID.
func applySessionID(args []string, sessionID string) []string {
	out := make([]string, len(args))
	for i, arg := range args {
		out[i] = strings.ReplaceAll(arg, "{{.SessionID}}", sessionID)
	}

	return out
}

// extractSessionID parses the provider's stdout and returns the session ID according to cfg.
func extractSessionID(stdout string, cfg config.ProviderSessionConfig) string {
	if !cfg.Enabled() {
		return ""
	}
	switch cfg.IDSource {
	case "json":
		var obj map[string]json.RawMessage
		if err := json.Unmarshal([]byte(stdout), &obj); err != nil {
			return ""
		}

		return extractJSONField(obj, cfg.IDField)
	case "jsonl_first":
		line := firstNonEmptyLine(stdout)
		var obj map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			return ""
		}

		return extractJSONField(obj, cfg.IDField)
	}

	return ""
}

// extractTextResult extracts the AI's text response from provider stdout according to cfg.
// Falls back to raw stdout when extraction fails or is not configured.
func extractTextResult(stdout string, cfg config.ProviderSessionConfig) string {
	switch cfg.ResultSource {
	case "json":
		var obj map[string]json.RawMessage
		if err := json.Unmarshal([]byte(stdout), &obj); err != nil {
			return stdout
		}
		if text := extractJSONField(obj, cfg.ResultField); text != "" {
			return text
		}

		return stdout
	case "jsonl_last":
		lines := strings.Split(stdout, "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if line == "" {
				continue
			}
			var obj map[string]json.RawMessage
			if err := json.Unmarshal([]byte(line), &obj); err != nil {
				continue
			}
			typeRaw, ok := obj["type"]
			if !ok {
				continue
			}
			var typeName string
			if err := json.Unmarshal(typeRaw, &typeName); err != nil || typeName != cfg.ResultEventType {
				continue
			}
			if text := extractJSONField(obj, cfg.ResultField); text != "" {
				return text
			}
		}

		return stdout
	default:
		return stdout
	}
}

// extractJSONField traverses a dot-notated field path in a JSON object and returns the string value.
func extractJSONField(obj map[string]json.RawMessage, field string) string {
	head, tail, _ := strings.Cut(field, ".")
	raw, ok := obj[head]
	if !ok {
		return ""
	}
	if tail == "" {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return ""
		}

		return s
	}
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(raw, &nested); err != nil {
		return ""
	}

	return extractJSONField(nested, tail)
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			return strings.TrimSpace(line)
		}
	}

	return ""
}

func looksLikeTokensExhausted(stderr, stdout string) bool {
	combined := strings.ToLower(stderr + stdout)
	patterns := []string{
		"usage limit",
		"usage_limit",
		"rate limit",
		"rate_limit",
		"quota exceeded",
		"insufficient_quota",
		"daily limit",
		"monthly limit",
		"token limit exceeded",
		"credit balance",
		"billing limit",
	}
	for _, p := range patterns {
		if strings.Contains(combined, p) {
			return true
		}
	}

	return false
}

func writePromptArtifacts(runtimeDir, phase, prompt, stdout, stderr string) error {
	inputPath := filepath.Join(runtimeDir, phase+"-input.md")
	outputPath := filepath.Join(runtimeDir, phase+"-output.md")
	stderrPath := filepath.Join(runtimeDir, phase+"-stderr.log")
	err := os.WriteFile(inputPath, []byte(prompt), 0o644) //nolint:gosec,mnd // G306: 0644 intentional for user-readable run artifacts
	if err != nil {
		return fmt.Errorf("write input file: %w", err)
	}
	_ = os.WriteFile(outputPath, []byte(stdout), 0o644)  //nolint:gosec,mnd // G306: 0644 intentional for user-readable run artifacts
	_ = os.WriteFile(stderrPath, []byte(stderr), 0o644) //nolint:gosec,mnd // G306: 0644 intentional for user-readable run artifacts

	return nil
}
