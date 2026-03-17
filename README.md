# ai-orchestrator

Simple CLI-only orchestrator for AI-assisted ticket workflows.

## What it does

- Runs from any git subdirectory (auto-detects repo root)
- Accepts ticket numbers as CLI args
- Fetches ticket details from Shortcut via a configured MCP command
- Creates per-ticket worktrees and branches (`sc-<ticket>-<slug>`)
- Runs investigation + implementation with a switchable provider (`codex` or `gemini`)
- Stores persistent state/logs in `.ai-orchestrator/`
- Supports human approval/feedback/reject/resume in CLI
- Runs checks and generates `pr.md`
- Optionally creates a PR via `gh pr create`

## Install

Build binary:

```bash
go build -o ai-orchestrator ./cmd/ai-orchestrator
```

Move/copy `ai-orchestrator` into your `PATH`.

## Commands

```bash
ai-orchestrator run <ticket-number> [<ticket-number>...]
ai-orchestrator status [<ticket-number>]
ai-orchestrator approve <ticket-number>
ai-orchestrator feedback <ticket-number> --message "..."
ai-orchestrator reject <ticket-number>
ai-orchestrator resume <ticket-number>
ai-orchestrator pr <ticket-number>
```

## Config

Default config path:

- `~/.config/ai-orchestrator/config.yaml`

Use [`config.example.yaml`](./config.example.yaml) as a starting point.

### Shortcut MCP

`shortcut_mcp.command` should be a local command that returns JSON ticket data for a ticket number.

Supported JSON shapes:

1. Direct tool shape:

```json
{
  "number": "12345",
  "id": "12345",
  "title": "Fix login race condition",
  "description": "...",
  "acceptance_criteria": "...",
  "priority": "high",
  "url": "https://app.shortcut.com/..."
}
```

2. Wrapped:

```json
{ "ticket": { ...same fields... } }
```

3. Shortcut story-like shape with fields such as `name`, `description`, `app_url`, `labels`.

If `shortcut_mcp.args` includes `{ticket}`, it will be replaced. Otherwise the ticket number is appended as the final arg.

## Runtime files

Per ticket runtime state is stored at:

- `.ai-orchestrator/<ticket-number>/`

Key files:

- `state.json`
- `ticket.json`
- `log.md`
- `proposal.md`
- `final_solution.md`
- `pr.md`
- `checks.log`
- `provider/*.md|*.log`

## Important

Do not commit runtime artifacts.

Add this to your repo `.gitignore`:

```gitignore
.ai-orchestrator/
```

The tool also auto-adds this entry when missing.

## Minimal workflow

1. `ai-orchestrator run 12345`
2. Review `.ai-orchestrator/12345/proposal.md`
3. `ai-orchestrator approve 12345` or `ai-orchestrator feedback 12345 --message "..."`
4. `ai-orchestrator status 12345`
5. Use `.ai-orchestrator/12345/pr.md` (or enable `create_pr`)
