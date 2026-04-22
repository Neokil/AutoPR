# AutoPR

> **Warning:** This repository is in active development. Expect frequent changes, including breaking ones. APIs, configuration formats, and workflows may change without notice between versions.

From issue to PR in one loop.

AutoPR runs a config-driven ticket workflow around a local daemon, a CLI, and an embedded web UI. It stores ticket state inside each repository, executes your configured coding agent provider, and lets you review, approve, and clean up work without leaving your machine.

Current validation status:

- AutoPR is currently only tested with `codex`
- The active integration setup uses MCP access to Shortcut for ticket data and GitHub for PR and review workflows
- Other providers or ticketing / VCS integrations may work, but are not yet validated

## Overview

AutoPR is split into two binaries:

- `auto-prd`: the local daemon that serves the API and embedded web UI
- `auto-pr`: the CLI that talks to the daemon

At a high level, the workflow looks like this:

1. Start the daemon.
2. Point AutoPR at one or more local repositories.
3. Run a ticket — the daemon creates a worktree, writes context, and executes the first workflow state.
4. Review progress or artifacts in the CLI or web UI.
5. Apply a workflow action (advance the state, provide feedback, etc.) or clean up.

Runtime data is stored in two places:

- Global config and server metadata: `~/.auto-pr/`
- Per-repository ticket artifacts: `<repo>/.auto-pr/`

Add this to repositories you manage with AutoPR:

```gitignore
.auto-pr/
```

## Quickstart

### 1. Install the binaries and scaffold config

```bash
make install
source ~/.zshrc
```

`make install` does the following:

- builds `auto-pr` and `auto-prd`
- registers shell aliases / PATH wiring
- creates `~/.auto-pr/config.yaml` if it does not exist

Notes:

- Existing config files are kept as-is.
- Service-management targets currently no-op with a message because `launchd` / `systemd` integration is intentionally disabled.

### 2. Configure your repositories and provider

Edit `~/.auto-pr/config.yaml` and at minimum set `repository_directories` to the folders AutoPR should scan.

Starter config:

- [`config.example.yaml`](./config.example.yaml)

### 3. Start the daemon

```bash
make start
```

By default the daemon listens on `http://127.0.0.1:8080`.

### 4. Use either the CLI or Web UI

CLI:

```bash
cd /path/to/repo
auto-pr run 12345
```

Web UI:

- Open `http://127.0.0.1:8080`
- Select a repository
- Add or open a ticket
- Use the dynamic action buttons shown while the ticket is waiting

## Configuration

Primary config file:

- `~/.auto-pr/config.yaml`

Default / notable settings:

- `provider`: provider key under `providers` to use for ticket execution
- `providers.<name>.command`: executable to run for that provider
- `providers.<name>.args`: arguments passed to that executable
- `repository_directories`: list of local paths that are either git repos or directories containing git repos
- `guidelines_file`: repository-local guidance file to include, for example `CODING_GUIDELINES.md`
- `state_dir_name`: per-repo working directory name, default `.auto-pr`
- `server_port`: daemon port, default `8080`
- `server_workers`: number of background workers, default `4`
- `base_branch`: optional override for the target branch

## Workflow Config

The workflow is driven by a YAML config that defines states, prompts, and actions. AutoPR resolves config using a three-level hierarchy (first match wins):

1. `<repo>/.auto-pr/workflow.yaml`
2. `~/.auto-pr/workflow.yaml`
3. Embedded binary default

Example state:

```yaml
states:
  - name: investigation
    display_name: Investigation
    prompt: prompts/investigate.md
    primary_artifact: investigation.md
    actions:
      - label: "Provide Feedback"
        type: provide_feedback
      - label: "Approve"
        type: move_to_state
        target: implementation
      - label: "Decline"
        type: move_to_state
        target: cancelled
```

`name` is the stable backend identifier. `display_name` is the label shown in the web timeline. `primary_artifact` is the per-run markdown artifact written for that state under `.auto-pr/runs/<uuid>/artifacts/`.

Action types:

- `provide_feedback`: collects a message and reruns the current state with that context
- `move_to_state`: transitions to the named target state (`done`/`cancelled` are terminal)
- `run_script`: runs commands then dispatches a sub-action based on exit code

Prompt templates follow the same three-level hierarchy. Paths in the `prompt` field are relative to `.auto-pr/` in the repo, global config directory, or embedded defaults.

Server URL override for the CLI:

```bash
export AUTO_PR_SERVER_URL=http://127.0.0.1:8080
```

## CLI Usage

Use the CLI from inside a git repository managed by AutoPR. The CLI resolves the current repo root and sends actions to the daemon for that repository.

Main commands:

```bash
auto-pr run <ticket-number> [<ticket-number>...]
auto-pr wait-for-job <job-id>
auto-pr status [<ticket-number>]
auto-pr action <ticket-number> --label "<action-label>" [--message "..."]
auto-pr cleanup <ticket-number>
auto-pr cleanup --done
auto-pr cleanup --all
```

Notes:

- `auto-pr run` starts the workflow or reruns the current state when a ticket is already `waiting` or `failed`.
- `auto-pr action` applies a named workflow action to a waiting ticket. Use `auto-pr status <ticket>` to see available actions and their labels.
- Mutating commands schedule background jobs and return a job id.
- `auto-pr wait-for-job <job-id>` blocks until the job finishes.
- `auto-pr status <ticket-number>` prints next steps when available.

## Web UI Usage

The web UI is served by `auto-prd` from the same address as the API, by default:

- `http://127.0.0.1:8080`

The UI is best for repository-level visibility and review:

- browse discovered repositories
- list tracked tickets across repositories
- inspect ticket details and per-run state artifacts in a workflow timeline
- open execution logs from the ticket menu and inspect each run chronologically
- apply workflow actions using dynamic buttons driven by the workflow config
- follow live progress through server-sent events

The UI depends on the daemon being running and on `repository_directories` being configured correctly.

## Build And Lifecycle Commands

Common make targets:

```bash
make build
make start
make install
make uninstall
```

Additional lifecycle targets:

- `make clean-build`
- `make register-alias`
- `make unregister-alias`
- `make init-config`
- `make remove-config`
- `make register-service`
- `make unregister-service`
- `make refresh-service`
- `make service-status`
- `make service-logs`

Current behavior:

- Service-related commands currently return early because OS service integration is disabled.
- `make uninstall` removes the local scaffolding in `~/.auto-pr`, unregisters aliases, and cleans `.build/`.

## More Details

For architecture, API endpoints, runtime files, and implementation details, see:

- [`docs/TECHNICAL.md`](./docs/TECHNICAL.md)
