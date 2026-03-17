# ai-orchestrator

CLI + server orchestrator for AI-assisted ticket workflows.

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

## Architecture

The project now uses a DDD-inspired layered structure so logic can be reused by CLI, server, and web clients:

- `internal/domain/ticket`: core ticket/state models and workflow state values
- `internal/application/orchestrator`: application service interface/use-cases used by clients
- `internal/ports`: storage contracts (interfaces) for persistence abstraction
- `internal/state`: current JSON/filesystem-backed store adapter implementing ports
- `internal/workflow`: orchestration logic (currently reused by the application service)
- `cmd/ai-orchestrator`: CLI adapter (calls application service)
- `cmd/orchestratord`: REST API server adapter

Notes:
- `internal/models` currently re-exports domain types as backward-compatible aliases.
- This is an incremental refactor to preserve behavior while preparing for `orchestratord` (REST + web UI).

## Install

Build and register PATH entry in your `~/.zshrc`:

```bash
make install
source ~/.zshrc
```

This builds the binaries to `.build/ai-orchestrator` and `.build/orchestratord`, then adds `.build/` to `PATH`.

If you use Codex as provider, run it in non-interactive mode in config:

```yaml
providers:
  codex:
    command: codex
    args: ["exec", "-"]
```

Checks are empty by default. Configure repo-appropriate commands in `~/.config/ai-orchestrator/config.yaml`, for example:

```yaml
server_port: 9000
check_commands:
  - npm test
  - npm run typecheck
```

During implementation, the coding agent is instructed to auto-detect and run formatter/linter commands from the repository itself (for example scripts, Make targets, and tool config files) before returning.

## Commands

```bash
ai-orchestrator run <ticket-number> <ticket-number>...
ai-orchestrator status <ticket-number>
ai-orchestrator approve <ticket-number>
ai-orchestrator feedback <ticket-number> --message "..."
ai-orchestrator reject <ticket-number>
ai-orchestrator resume <ticket-number>
ai-orchestrator pr <ticket-number>
ai-orchestrator cleanup <ticket-number>
ai-orchestrator cleanup --done
ai-orchestrator cleanup --all
```

## Server

Start the REST server (default port from config: `server_port`, default `9000`):

```bash
orchestratord
```

Optional flags:

```bash
orchestratord --port 9010
```

Server metadata/state is stored in:

- `~/.ai-orchestrator/server/state.json`

Ticket artifacts remain in each repository under:

- `<repo>/.ai-orchestrator/...`

Endpoints:

- `GET /api/health`
- `GET /api/tickets` (optional query: `repo_path`)
- `GET /api/tickets/{id}?repo_path=/abs/path/to/repo`
- `GET /api/tickets/{id}/events?repo_path=/abs/path/to/repo`
- `GET /api/tickets/{id}/artifacts/{name}?repo_path=/abs/path/to/repo`
- `POST /api/tickets/{id}/run` (JSON body: `{"repo_path":"..."}`)
- `POST /api/tickets/{id}/resume` (JSON body: `{"repo_path":"..."}`)
- `POST /api/tickets/{id}/approve` (JSON body: `{"repo_path":"..."}`)
- `POST /api/tickets/{id}/reject` (JSON body: `{"repo_path":"..."}`)
- `POST /api/tickets/{id}/feedback` (JSON body: `{"repo_path":"...","message":"..."}`)
- `POST /api/tickets/{id}/cleanup` (JSON body: `{"repo_path":"..."}`)
- `POST /api/cleanup` (JSON body: `{"repo_path":"...","scope":"done|all"}`)

## Config

Default config path:

- `~/.ai-orchestrator/config.yaml`

Legacy fallback still supported:

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
