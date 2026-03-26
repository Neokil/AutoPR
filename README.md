# AutoPR

> **Warning:** This repository is in active development. Expect frequent changes, including breaking ones. APIs, configuration formats, and workflows may change without notice between versions.

From issue to PR in one loop.

AutoPR runs a ticket workflow around a local daemon, a CLI, and an embedded web UI. It stores ticket state inside each repository, executes your configured coding agent provider, and lets you review, approve, resume, and clean up work without leaving your machine.

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
3. Run a ticket.
4. Review progress or artifacts in the CLI or web UI.
5. Approve, reject, resume, generate the PR, or clean up.

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
- creates default prompt templates in `~/.auto-pr/prompts/`

Notes:

- Existing config and prompt files are kept as-is.
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
- Trigger actions from the UI

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
- `create_pr`: whether PR creation is enabled
- `max_fix_attempts`: retry budget for fix loops
- `base_branch`: optional override for the target branch
- `check_commands`: commands used for validation
- `format_commands`: commands used for formatting
- `lint_commands`: commands used for linting

Prompt templates are stored in:

- `~/.auto-pr/prompts/ticket.md.tmpl`
- `~/.auto-pr/prompts/investigate.md.tmpl`
- `~/.auto-pr/prompts/implement.md.tmpl`
- `~/.auto-pr/prompts/pr.md.tmpl`

These are scaffolded automatically and can be edited to tune the agent prompts.

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
auto-pr approve <ticket-number>
auto-pr feedback <ticket-number> --message "..."
auto-pr reject <ticket-number>
auto-pr resume <ticket-number>
auto-pr pr <ticket-number>
auto-pr apply-pr-comments <ticket-number>
auto-pr cleanup <ticket-number>
auto-pr cleanup --done
auto-pr cleanup --all
```

Typical CLI flow:

```bash
cd /path/to/repo
auto-pr run 12345
auto-pr status 12345
auto-pr approve 12345
auto-pr pr 12345
```

Notes:

- Mutating commands schedule background jobs and return a job id.
- `auto-pr wait-for-job <job-id>` blocks until the job finishes.
- `auto-pr status <ticket-number>` prints next steps when available.

## Web UI Usage

The web UI is served by `auto-prd` from the same address as the API, by default:

- `http://127.0.0.1:8080`

The UI is best for repository-level visibility and review:

- browse discovered repositories
- list tracked tickets across repositories
- inspect ticket details, proposals, and logs
- run ticket actions such as approve, reject, resume, PR generation, PR comment application, and cleanup
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
