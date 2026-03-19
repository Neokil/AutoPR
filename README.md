# AutoPR

From issue to PR in one loop.

`AutoPR` helps you run ticket-to-PR workflows with a local server, CLI, and web UI.

## Quick Start

1. Build and install binaries to your PATH:

```bash
make install
source ~/.zshrc
```

`make install` is composed of these steps:

- `make start`
- `make build`
- `make register-alias`
- `make init-config`
- `make register-service`

The reverse flow is available too:

- `make clean-build`
- `make unregister-alias`
- `make remove-config`
- `make unregister-service`
- `make uninstall`

It also scaffolds (without overwriting existing files):

- `~/.auto-pr/config.yaml` (from `config.example.yaml`)
- `~/.auto-pr/prompts/*.md.tmpl` (default prompt templates)
- `~/.auto-pr/server/logs/` for daemon stdout/stderr

Service-related make targets currently no-op with a message because `launchd` / `systemd` integration is intentionally disabled for now.

2. If your platform is unsupported or service setup was skipped, start the server manually:

```bash
make start
```

3. In your repository, schedule a run:

```bash
auto-pr run 12345
```

4. Wait for completion if needed:

```bash
auto-pr wait-for-job <job-id>
```

5. Open the web UI:

- http://127.0.0.1:8080

## Main Commands

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

Notes:

- Mutating commands schedule background jobs and return a job id.
- Use `wait-for-job` when you want to block until a job finishes.

## Configuration

Default config file:

- `~/.auto-pr/config.yaml`

Starter config:

- [`config.example.yaml`](./config.example.yaml)

Common settings:

- `server_port` (default `8080`)
- `server_workers` (default `4`)
- `provider` (`codex` or `gemini`)
- `repository_directories` (default `[]`): paths that either are git repos or contain git repos

Server URL override for CLI:

```bash
export AUTO_PR_SERVER_URL=http://127.0.0.1:8080
```

Prompt templates (editable):

- `~/.auto-pr/prompts/ticket.md.tmpl`
- `~/.auto-pr/prompts/investigate.md.tmpl`
- `~/.auto-pr/prompts/implement.md.tmpl`
- `~/.auto-pr/prompts/pr.md.tmpl`

These files are auto-created with defaults on first use and can be edited to tune agent queries.

## Storage

- Global settings and server metadata: `~/.auto-pr/`
- Ticket artifacts per repo: `<repo>/.auto-pr/`

Add to `.gitignore`:

```gitignore
.auto-pr/
```

Automatic cleanup behavior:

- For tickets with an open PR URL, the server periodically checks whether the PR is still open.
- If the PR is closed or merged, the ticket is automatically cleaned up.

Service management:

- `make refresh-service`
- `make service-status`
- `make service-logs`
- These currently return early because service integration is disabled.

Removal:

- `make uninstall` removes the user service, the PATH block in `~/.zshrc`, the `~/.auto-pr` scaffolding directory, and `.build/`

## More Details

For API endpoints, architecture, runtime files, and implementation details, see:

- [`docs/TECHNICAL.md`](./docs/TECHNICAL.md)
