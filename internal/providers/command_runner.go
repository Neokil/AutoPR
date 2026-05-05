package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
func (r *PromptCommandRunner) Run(ctx context.Context, worktreePath, runtimeDir, phase, prompt, sessionData string) (string, string, string, error) {
	writeErr := writePromptArtifacts(runtimeDir, phase, prompt, "", "")
	if writeErr != nil {
		return "", "", "", writeErr
	}

	args := r.buildArgs(sessionData)
	res, runErr := shell.Run(ctx, worktreePath, nil, prompt, r.command, args...)

	textResult := extractTextResult(res.Stdout, r.sessionCfg)
	newSessionData := extractSessionID(res.Stdout, r.sessionCfg)
	_ = writePromptArtifacts(runtimeDir, phase, prompt, textResult, res.Stderr)

	if runErr != nil {
		if looksLikeTokensExhausted(res.Stderr, res.Stdout) {
			return textResult, res.Stderr, newSessionData, fmt.Errorf("provider %s phase %s: %w", r.providerName, phase, ErrTokensExhausted)
		}

		return textResult, res.Stderr, newSessionData, fmt.Errorf("provider %s phase %s failed: %w", r.providerName, phase, runErr)
	}
	if sessionErr := extractProviderSessionError(res.Stderr); sessionErr != "" && strings.TrimSpace(textResult) == "" {
		return textResult, res.Stderr, newSessionData, fmt.Errorf("provider %s phase %s: %w: %s", r.providerName, phase, ErrSessionFailed, sessionErr)
	}
	if strings.TrimSpace(textResult) == "" {
		return "", res.Stderr, "", fmt.Errorf("provider %s phase %s: %w", r.providerName, phase, ErrEmptyOutput)
	}

	return textResult, res.Stderr, newSessionData, nil
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
		err := json.Unmarshal([]byte(stdout), &obj)
		if err != nil {
			return ""
		}

		return extractJSONField(obj, cfg.IDField)
	case "jsonl_first":
		line := firstNonEmptyLine(stdout)
		var obj map[string]json.RawMessage
		err := json.Unmarshal([]byte(line), &obj)
		if err != nil {
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
		err := json.Unmarshal([]byte(stdout), &obj)
		if err != nil {
			return stdout
		}
		if text := extractJSONField(obj, cfg.ResultField); text != "" {
			return text
		}

		return stdout
	case "jsonl_last":
		lines := strings.Split(stdout, "\n")
		for _, line := range slices.Backward(lines) {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var obj map[string]json.RawMessage
			unmarshalErr := json.Unmarshal([]byte(line), &obj)
			if unmarshalErr != nil {
				continue
			}
			typeRaw, ok := obj["type"]
			if !ok {
				continue
			}
			var typeName string
			unmarshalErr = json.Unmarshal(typeRaw, &typeName)
			if unmarshalErr != nil || typeName != cfg.ResultEventType {
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
		var strVal string
		err := json.Unmarshal(raw, &strVal)
		if err != nil {
			return ""
		}

		return strVal
	}
	var nested map[string]json.RawMessage
	err := json.Unmarshal(raw, &nested)
	if err != nil {
		return ""
	}

	return extractJSONField(nested, tail)
}

func firstNonEmptyLine(s string) string {
	for line := range strings.SplitSeq(s, "\n") {
		if strings.TrimSpace(line) != "" {
			return strings.TrimSpace(line)
		}
	}

	return ""
}

// extractProviderSessionError scans stderr for ERROR-level log lines emitted by the
// provider process even when it exits successfully. Returns the last such line found,
// so transient mid-run errors are superseded by any later ones.
func extractProviderSessionError(stderr string) string {
	var last string
	for line := range strings.SplitSeq(stderr, "\n") {
		if strings.Contains(line, " ERROR ") {
			last = strings.TrimSpace(line)
		}
	}

	return last
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
	err := os.WriteFile(inputPath, []byte(prompt), 0o644) //nolint:gosec // G703: runtimeDir is an internal trusted path
	if err != nil {
		return fmt.Errorf("write input file: %w", err)
	}
	_ = os.WriteFile(outputPath, []byte(stdout), 0o644) 
	_ = os.WriteFile(stderrPath, []byte(stderr), 0o644)

	return nil
}
