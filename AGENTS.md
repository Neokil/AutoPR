# AGENTS.md

This file gives coding agents the minimum context needed to work safely in this repository.

## Project Summary

AutoPR is a local ticket workflow system with:

- `auto-prd`: daemon serving the API and embedded web UI
- `auto-pr`: CLI client for interacting with the daemon from a managed repository

The system coordinates repository-local ticket state under `.auto-pr/`, runs configured coding-agent providers, and supports review and workflow transitions through both CLI and web UI.

## Repository Layout

- `cmd/auto-pr`: CLI entrypoint
- `cmd/auto-prd`: daemon entrypoint
- `internal/application`: workflow orchestration and ticket application logic
- `internal/server`: HTTP transport, SSE, repository and job endpoints
- `internal/workflow`: workflow config loading, defaults, prompts, and types
- `internal/config`: user config loading
- `internal/state`, `internal/serverstate`: persistence
- `internal/providers`: provider execution and command runner integration
- `internal/api`: generated API types and generation hooks
- `web/`: React + TypeScript frontend, embedded into the daemon binary
- `openapi/`: OpenAPI source used to generate frontend and backend types
- `e2e/`: Dockerized Playwright end-to-end tests

## Build And Test

Prefer the existing make targets:

- `make build`: generate OpenAPI artifacts, build frontend, build both Go binaries into `.build/`
- `make start`: build and run the daemon
- `make clean-build`: remove `.build/`
- `make test-e2e`: run the Dockerized E2E suite

Useful direct commands:

- `go test ./...`
- `cd web && npm test`
- `cd web && npm run lint`
- `cd web && npm run typecheck`

## Verification After Changes

After implementing any feature or fix, verify correctness by running all of:

- `go test ./...` — backend unit tests
- `golangci-lint run` — backend linting
- `cd web && npm test` — frontend tests

All three must pass before considering the work done.

## Agent Working Rules

- Check whether a file is generated before editing it.
- Prefer changing source inputs instead of generated outputs.
- Keep CLI, daemon, OpenAPI, and web types in sync when changing API shapes.
- Preserve repository-local runtime state semantics under `.auto-pr/`; do not commit runtime artifacts.
- Avoid introducing new infrastructure or dependencies unless they clearly support the existing daemon/CLI/web architecture.

## Generated Or Derived Files

Treat these as generated or derived unless the task explicitly requires regeneration behavior changes:

- `web/src/generated/api.ts`
- `internal/api/generated.go`
- `web/dist/`
- `.build/`

If API contracts change, update the OpenAPI source and the relevant generators rather than hand-editing generated files.

## Repo-Specific Notes

- `Makefile` runs OpenAPI validation and generation as part of `make build`.
- The frontend build currently runs `npm install` during `make build`, so dependency changes affect build behavior directly.
- The README references `docs/TECHNICAL.md`, but that file is not present in the current repository state. Do not rely on it.
- Current validation is centered on the `codex` provider and Shortcut/GitHub integrations, per `README.md`.

## When Making Changes

- For backend changes, inspect corresponding CLI and web assumptions before finalizing.
- For API changes, review both `internal/server` and `web/src/api.ts` / generated client usage.
- For workflow changes, inspect defaults in `internal/workflow/` and repository-level override behavior described in `README.md`.
- For UI changes, keep the embedded-web deployment model in mind: the daemon serves the built frontend bundle.
