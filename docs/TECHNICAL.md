# Technical Reference

## Architecture

The project uses a layered structure to share logic across CLI, server, and web UI:

- `internal/domain/ticket`: domain models and workflow state values
- `internal/application/orchestrator`: application service interface/use-cases
- `internal/ports`: persistence/storage contracts
- `internal/state`: JSON/filesystem-backed store adapter
- `internal/workflow`: orchestration logic
- `cmd/auto-pr`: CLI entrypoint (built as `auto-pr`)
- `cmd/auto-prd`: server entrypoint (built as `auto-prd`)

## Build Outputs

`make build` creates:

- `.build/auto-pr`
- `.build/auto-prd`

## Frontend

- React app source: `web/`
- Built assets: `web/dist`
- `auto-prd` serves embedded frontend at `/`
- API is served under `/api/*`

## API Endpoints

- `GET /api/health`
- `GET /api/repositories`
- `GET /api/events` (SSE stream)
- `GET /api/tickets` (optional `repo_path`)
- `GET /api/tickets/{id}?repo_path=/abs/path/to/repo`
- `GET /api/tickets/{id}/events?repo_path=/abs/path/to/repo`
- `GET /api/tickets/{id}/artifacts/{name}?repo_path=/abs/path/to/repo`
- `GET /api/jobs/{id}`
- `POST /api/tickets/{id}/run` with `{"repo_path":"..."}`
- `POST /api/tickets/{id}/resume` with `{"repo_path":"..."}`
- `POST /api/tickets/{id}/approve` with `{"repo_path":"..."}`
- `POST /api/tickets/{id}/reject` with `{"repo_path":"..."}`
- `POST /api/tickets/{id}/feedback` with `{"repo_path":"...","message":"..."}`
- `POST /api/tickets/{id}/pr` with `{"repo_path":"..."}`
- `POST /api/tickets/{id}/apply-pr-comments` with `{"repo_path":"..."}`
- `POST /api/tickets/{id}/cleanup` with `{"repo_path":"..."}`
- `POST /api/cleanup` with `{"repo_path":"...","scope":"done|all"}`

## Job Status

- `queued`
- `running`
- `done`
- `failed`

Concurrency guarantees:

- Per-ticket serialization
- Repo-wide cleanup (`done/all`) runs exclusively in that repo

Background PR monitor:

- `auto-prd` periodically (every 2 minutes) checks tickets that have a `pr_url`.
- If the linked GitHub PR state is no longer `open` (closed or merged), the server auto-runs ticket cleanup.

## Config Paths

Primary config:

- `~/.auto-pr/config.yaml`

Prompt template directory:

- `~/.auto-pr/prompts/`
- templates:
  - `ticket.md.tmpl`
  - `investigate.md.tmpl`
  - `implement.md.tmpl`
  - `pr.md.tmpl`

Install behavior:

- `make install` runs `build`, `register-alias`, `init-config`, and `register-service`.
- `make uninstall` runs `unregister-service`, `unregister-alias`, `remove-config`, and `clean-build`.
- `make refresh-service` rebuilds `auto-prd` and restarts the registered daemon.
- `make service-status` reports the registered daemon state through `launchctl` or `systemctl --user`.
- `make service-logs` tails `~/.auto-pr/server/logs/stdout.log` and `stderr.log`.
- `make init-config` scaffolds `~/.auto-pr/config.yaml` and default prompt templates if they do not exist.
- `make remove-config` removes the `~/.auto-pr` scaffolding directory.
- `make install` creates `~/.auto-pr/server/logs/` for daemon stdout/stderr.
- Existing files are kept as-is (non-destructive).
- Service-related scripts currently short-circuit and print that `launchd` / `systemd` operations are disabled.

Repository discovery config:

- `repository_directories: []`
- each entry may be:
  - a git repository folder
  - a folder containing git repository folders (direct children)

## Server Metadata

Primary path:

- `~/.auto-pr/server/state.json`

## Ticket Artifacts

Per ticket directory:

- `<repo>/.auto-pr/<ticket-number>/`

Common files:

- `state.json`
- `ticket.json`
- `log.md`
- `proposal.md`
- `final_solution.md`
- `pr.md`
- `checks.log`
- `provider/*.md|*.log`

## Environment Variables

Preferred:

- `AUTO_PR_SERVER_URL`
