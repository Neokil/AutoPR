# auto-pr

From issue to PR in one loop.

`auto-pr` helps you run ticket-to-PR workflows with a local server, CLI, and web UI.

## Quick Start

1. Build and install binaries to your PATH:

```bash
make install
source ~/.zshrc
```

2. Start the server:

```bash
auto-prd
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

Server URL override for CLI:

```bash
export AUTO_PR_SERVER_URL=http://127.0.0.1:8080
```

## Storage

- Global settings and server metadata: `~/.auto-pr/`
- Ticket artifacts per repo: `<repo>/.auto-pr/`

Add to `.gitignore`:

```gitignore
.auto-pr/
```

## More Details

For API endpoints, architecture, runtime files, and implementation details, see:

- [`docs/TECHNICAL.md`](./docs/TECHNICAL.md)
