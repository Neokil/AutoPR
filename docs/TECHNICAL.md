# Technical Reference

## Architecture

The project uses a layered structure to share logic across CLI, server, and web UI:

- `internal/domain/ticket`: domain models and workflow flow-status values
- `internal/workflow`: workflow config loading, types, and embedded defaults
- `internal/application/orchestrator`: application service interface/use-cases
- `internal/application/tickets`: ticket lifecycle orchestration (config-driven state machine)
- `internal/ports`: persistence/storage contracts
- `internal/state`: JSON/filesystem-backed store adapter
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
- `POST /api/tickets/{id}/action` with `{"repo_path":"...","label":"...","message":"..."}`
- `POST /api/tickets/{id}/cleanup` with `{"repo_path":"..."}`
- `POST /api/cleanup` with `{"repo_path":"...","scope":"done|all"}`

The `GET /api/tickets/{id}` response includes an `available_actions` array when the ticket is
in the `waiting` flow status. Each entry has `label` and `type` fields drawn from the workflow
config for the current state.

## Flow Status

Ticket flow statuses:

- `pending`: ticket created, not yet started
- `running`: AI provider is executing the current state
- `waiting`: execution complete, awaiting a human action
- `done`: terminal success state
- `failed`: an error occurred; retry with `/run`
- `cancelled`: terminal cancelled state

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

## Workflow Config

Workflow is configured via a three-level hierarchy (first match wins):

1. `<repo>/.auto-pr/workflow.yaml`
2. `~/.auto-pr/workflow.yaml`
3. Embedded binary default (see `internal/workflow/default_workflow.yaml`)

Each state defines a `prompt` (relative path resolved via the same three-level hierarchy),
optional `pre_prompt_commands` / `post_prompt_commands`, and an `actions` list.

Action types:

- `provide_feedback`: prompts the user for a message, writes it as feedback, and reruns the current state with that context
- `move_to_state`: transitions to the named target state; targets `done` and `cancelled` are terminal
- `run_script`: runs one or more shell commands, then dispatches a sub-action on `on_success`, `on_failure`, or `always`

## Config Paths

Primary config:

- `~/.auto-pr/config.yaml`

Workflow config (optional overrides):

- `<repo>/.auto-pr/workflow.yaml` (per-repo)
- `~/.auto-pr/workflow.yaml` (global)

Prompt templates (optional overrides, relative paths from the `prompt` field in each state):

- `<repo>/.auto-pr/<prompt-path>`
- `~/.auto-pr/<prompt-path>`
- embedded binary default

Install behavior:

- `make install` runs `build`, `register-alias`, `init-config`, and `register-service`.
- `make uninstall` runs `unregister-service`, `unregister-alias`, `remove-config`, and `clean-build`.
- `make refresh-service` rebuilds `auto-prd` and restarts the registered daemon.
- `make service-status` reports the registered daemon state through `launchctl` or `systemctl --user`.
- `make service-logs` tails `~/.auto-pr/server/logs/stdout.log` and `stderr.log`.
- `make init-config` scaffolds `~/.auto-pr/config.yaml` if it does not exist.
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

Artifacts are written into a git worktree created for each ticket:

- Worktree path: `<repo>/.auto-pr/worktrees/<ticket-number>/`
- Branch: `auto-pr/<ticket-number>`

Files inside the worktree under `.auto-pr/`:

- `context.md`: initial ticket context (written once on first run)
- `<state-name>.prompt.md`: prompt sent to the AI provider for that state
- `<state-name>.log`: structured log of provider output for that state
- `ticket.md` / additional files written by the provider

## Environment Variables

Preferred:

- `AUTO_PR_SERVER_URL`
