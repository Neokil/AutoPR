package providers

import (
	"bytes"
	"fmt"
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

var defaultPromptTemplates = map[string]string{
	tplTicket: `Fetch Shortcut ticket details for story/ticket number {{.TicketNumber}} using your configured MCP integration.

Return ONLY valid JSON (no markdown fences, no extra text) with this shape:
{
  "number": "{{.TicketNumber}}",
  "id": "string",
  "title": "string",
  "description": "string",
  "acceptance_criteria": "string",
  "priority": "string",
  "url": "string",
  "labels": ["string"],
  "workflow_fields": {"key":"value"},
  "parent_ticket": {
    "id": "string",
    "number": "string",
    "title": "string",
    "description": "string",
    "url": "string"
  },
  "epic": {
    "id": "string",
    "title": "string",
    "description": "string",
    "url": "string"
  }
}

Also fetch parent ticket and epic context if available in Shortcut.
If unavailable, use null for parent_ticket/epic and empty values for unknown primitive fields.
`,
	tplInvestigate: `You are assisting with software ticket investigation.

Ticket #{{.TicketNumber}}: {{.TicketTitle}}
URL: {{.TicketURL}}

Description:
{{.TicketDescription}}

Acceptance Criteria:
{{.TicketAcceptanceCriteria}}

Related Context:
{{.RelatedContext}}

Repo path: {{.RepoPath}}
Worktree path: {{.WorktreePath}}
Guidelines file: {{.GuidelinesPath}}
Existing log path: {{.LogPath}}
Existing proposal path: {{.ProposalPath}}
Human feedback: {{.Feedback}}

Return markdown with sections:
- Problem Summary
- Suggested Solution
- Likely Files To Change
- Risks
- Test Plan
- Open Questions
`,
	tplImplement: `Implement the approved solution for the following ticket in this worktree.

Ticket #{{.TicketNumber}}: {{.TicketTitle}}
Description:
{{.TicketDescription}}

Related Context:
{{.RelatedContext}}

Use proposal at: {{.ProposalPath}}
Use log at: {{.LogPath}}
Guidelines file: {{.GuidelinesPath}}

If validation failed previously, address these failures:
{{.FailureContext}}

Before you finish, automatically detect and run this project's formatting and linting commands directly in the worktree.
Do not rely on preconfigured command lists.
Discover commands from the repository itself (for example package scripts, Makefile targets, tool config files, or language-native defaults).
Prefer project-defined commands when available, and only fall back to sensible language defaults if no project command is defined.
If a command fails, fix the code and re-run until it passes or clearly report blockers.

After making changes, return markdown with sections:
- Changes Made
- Notable Files Changed
- Remaining Risks
- Tests To Run
`,
	tplPR: `Generate a PR description in markdown.

Ticket #{{.TicketNumber}}: {{.TicketTitle}}
Description:
{{.TicketDescription}}

Related Context:
{{.RelatedContext}}

Use these files as source of truth:
- worktree: {{.WorktreePath}}
- log: {{.LogPath}}
- proposal: {{.ProposalPath}}
- final solution: {{.FinalSolutionPath}}
- checks: {{.ChecksLogPath}}

Include sections:
- Summary
- Problem Being Solved
- Implementation Overview
- Risks / Follow-ups
- Test Failures / Blockers (only when checks/tests failed)
`,
}

func renderPromptTemplate(promptsDir, name string, data interface{}) (string, error) {
	defaultTemplate, ok := defaultPromptTemplates[name]
	if !ok {
		return "", fmt.Errorf("unknown prompt template: %s", name)
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
