# ai-orchestrator

Simple CLI-only orchestrator for AI-assisted ticket workflows.

## What it does

- Runs from any git subdirectory (auto-detects repo root)
- Accepts ticket numbers as CLI args
- Fetches ticket details from Shortcut through the selected provider (provider-side MCP)
- Creates per-ticket worktrees and branches (`sc-<ticket>-<slug>`)
- Runs investigation + implementation with a switchable provider (`codex` or `gemini`)
- Stores persistent state/logs in `.ai-orchestrator/`
- Supports human approval/feedback/reject/resume in CLI
- Runs checks and generates `pr.md`
- Optionally creates a PR via `gh pr create`

## Install

Build and register PATH entry in your `~/.zshrc`:

```bash
make install
source ~/.zshrc
```

This builds the binary to `.build/ai-orchestrator` and adds `.build/` to `PATH`.

If you use Codex as provider, run it in non-interactive mode in config:

```yaml
providers:
  codex:
    command: codex
    args: ["exec", "-"]
```

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

Shortcut access is expected to be configured in your selected provider CLI (`codex` or `gemini`) via that provider's MCP/tool configuration.

When you run `ai-orchestrator run <ticket>`, the orchestrator asks the active provider to fetch ticket details for that ticket number and return normalized JSON.

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
