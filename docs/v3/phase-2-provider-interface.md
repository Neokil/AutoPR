# Phase 2 — Provider Interface

## Goal

Collapse the four specialised provider methods (`GetTicket`, `Investigate`, `Implement`, `SummarizePR`) into a single `Execute` method. The provider no longer needs to know what *kind* of task it is performing — it just runs a prompt in a working directory.

## Current interface (to be replaced)

```go
// internal/providers/provider.go
type AIProvider interface {
    Name() string
    GetTicket(ctx context.Context, ticketNumber, repoPath, runtimeDir string) (ticket.Ticket, string, error)
    Investigate(ctx context.Context, req InvestigateRequest, runtimeDir string) (InvestigateResult, error)
    Implement(ctx context.Context, req ImplementRequest, runtimeDir string) (ImplementResult, error)
    SummarizePR(ctx context.Context, req PRRequest, runtimeDir string) (PRResult, error)
}
```

## New interface

```go
// internal/providers/provider.go
type ExecuteRequest struct {
    PromptPath string // absolute path to the prompt file
    WorkDir    string // worktree root — AI execution is scoped here
    RuntimeDir string // provider-specific scratch space (unchanged)
}

type ExecuteResult struct {
    RawOutput string
}

type AIProvider interface {
    Name() string
    Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error)
}
```

All the context the AI needs (ticket data, guidelines, previous outputs, feedback) is in files within `WorkDir`. The prompt file at `PromptPath` tells the AI what files to read and what to write. The provider implementation just runs its CLI tool against the prompt in that working directory — same as today, minus the template rendering.

## Changes to `internal/providers/cli_provider.go`

The current implementation builds a rendered prompt string from a Go template, pipes it to the provider CLI, and captures output. In v3:

1. Remove template rendering — read the prompt file as plain text
2. Remove the four specialised `buildXxxPrompt` / `renderXxxTemplate` functions
3. Implement `Execute` by piping the raw prompt file content to the CLI with `WorkDir` as the working directory (same mechanism, fewer moving parts)
4. Remove `InvestigateRequest`, `InvestigateResult`, `ImplementRequest`, `ImplementResult`, `PRRequest`, `PRResult` types from `provider.go`

The CLI invocation itself (`cmd + args`, stdin pipe, output capture) does not change.

## Impact on callers

The only caller of the provider methods is the orchestrator (`internal/application/tickets/orchestrator.go`). Those call sites are replaced in Phase 4. In this phase the orchestrator still compiles — the old methods can be deleted from the interface and `cli_provider.go`, and the orchestrator's call sites updated to call a temporary stub or left to fail compilation until Phase 4 is complete.

Recommended approach: do Phase 2 and Phase 3 on the same branch pass, then fix compilation in Phase 4.

## Files changed

| File | Change |
|------|--------|
| `internal/providers/provider.go` | Replace interface; remove old request/result types |
| `internal/providers/cli_provider.go` | Replace 4 method implementations with `Execute` |
| `internal/providers/cli_provider_test.go` | Update tests to use `Execute` |

## Definition of done

- `AIProvider` interface has exactly one method beyond `Name()`: `Execute`
- `cli_provider.go` implements `Execute` — reads prompt file, pipes to CLI, captures output
- All old request/result types removed from `provider.go`
- Existing provider tests updated and passing (compilation errors in the orchestrator are acceptable at this stage, to be fixed in Phase 4)
