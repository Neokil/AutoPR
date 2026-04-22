# Phase 4 — Orchestrator Rewrite

## Goal

Replace the 930-line hardcoded orchestrator with a generic state machine executor that reads from `WorkflowConfig`. All transition logic moves from Go code to config. This phase also fixes the compilation breakage introduced in Phases 2 and 3.

## New orchestrator contract

```go
type Orchestrator struct {
    Cfg      config.Config
    Workflow  workflow.WorkflowConfig
    RepoRoot string
    Store    ports.StateStore
    Provider providers.AIProvider
}

// Public methods (called by HTTP handlers / CLI)
func (o *Orchestrator) StartFlow(ctx context.Context, ticketNumber string) error
func (o *Orchestrator) ApplyAction(ctx context.Context, ticketNumber, actionLabel, feedback string) error
func (o *Orchestrator) CleanupTicket(ctx context.Context, ticketNumber string) error
func (o *Orchestrator) Status(ticketNumber string) error
```

All other public methods (`RunTicket`, `ResumeTicket`, `Approve`, `Feedback`, `Reject`, `GeneratePR`, `ApplyPRComments`) are removed.

## `StartFlow`

```
1. Load or create State (status = running, currentState = first state in workflow)
2. Create git worktree  (built-in, always)
3. Create worktree skeleton: mkdir <worktree>/.auto-pr/
4. runState(ctx, st)
```

If a `State` already exists for `ticketNumber` and `FlowStatus` is not terminal (`done`/`failed`/`cancelled`), return an error — use `ApplyAction` to continue.

## `runState`

```
1. Save state (status = running)
2. Run pre_prompt_commands in worktree (stop on first non-zero exit, set status = failed)
3. Resolve prompt file path (hierarchy: <repo>/.auto-pr/<prompt> → ~/.auto-pr/<prompt> → embedded)
4. Call provider.Execute(promptPath, worktreePath, runtimeDir)
5. Write raw output to <worktree>/.auto-pr/<stateName>.log
6. Run post_prompt_commands in worktree (same failure behaviour as pre)
7. Delete feedback.md if it exists (consumed)
8. Save state (status = waiting)
```

`runState` is an internal method — it is never called directly by handlers.

## `ApplyAction`

```
1. Load state — must be status = waiting
2. Look up current StateConfig from workflow
3. Find ActionConfig matching actionLabel (case-insensitive)
4. Dispatch by ActionConfig.Type:
   - provide_feedback  → writeFeedback(feedback) → runState(ctx, st)
   - move_to_state     → transitionTo(target, st)
   - run_script        → executeScript(action, st, ctx)
```

### `writeFeedback(text)`

Write `text` to `<worktree>/.auto-pr/feedback.md`, overwriting any previous content.

### `transitionTo(target, st)`

If `target` matches a `StateConfig.Name`: update `st.CurrentState = target`, call `runState`.
If `target` does not match any state: set `st.FlowStatus = done` (or `cancelled` / `failed` based on target name), save, return.

Terminal target name mapping:

| target string | FlowStatus set |
|---------------|----------------|
| `done` | `done` |
| `cancelled` | `cancelled` |
| `failed` | `failed` |
| anything else not in states | `done` |

### `executeScript(action, st, ctx)`

```
1. Run action.Commands sequentially in worktree, capturing combined stdout+stderr
2. Determine which sub-action to dispatch:
   - exit code 0  → action.OnSuccess (if set), else action.Always (if set), else no-op
   - exit code ≠0 → action.OnFailure (if set), else action.Always (if set), else no-op
   - (Always runs in addition to on_success/on_failure if both are set)
3. Dispatch sub-action with captured output as the `feedback` argument:
   - provide_feedback  → writeFeedback(output) → runState(ctx, st)
   - move_to_state     → transitionTo(target, st)
```

Note: if both `on_success`/`on_failure` and `always` are set, `always` runs *after* the conditional handler.

## Worktree creation (built-in, extracted from current orchestrator)

The worktree creation logic already exists in `orchestrator.go`. Extract it into `internal/worktree/` (or keep in orchestrator as a private method `createWorktree`). No change to the git mechanics — branch naming, checkout, etc. stay the same.

## `CleanupTicket`

Identical to the current implementation: remove the worktree, delete the `.auto-pr/<ticketNumber>/` state directory.

## Config integration

`Orchestrator` receives a `workflow.WorkflowConfig` at construction time. The server (`cmd/auto-prd/main.go`) calls `workflow.Load(repoRoot)` once at startup per repository and passes it to `orchestrator.New`.

## Files changed

| File | Change |
|------|--------|
| `internal/application/tickets/orchestrator.go` | Full rewrite — ~930 lines → ~250 lines |
| `cmd/auto-prd/main.go` | Load `WorkflowConfig`, pass to orchestrator constructor |
| `cmd/auto-pr/main.go` | Remove subcommands that no longer exist (`approve`, `reject`, `resume`, `feedback`); replace with generic `action` subcommand |

## CLI changes

The `auto-pr` CLI currently has subcommands that map 1:1 to the old actions. In v3 the CLI needs a generic action command:

```
auto-pr action <ticket-number> <action-label> [--feedback "..."]
```

Old subcommands to remove: `approve`, `reject`, `resume`, `feedback`, `pr`, `apply-pr-comments`.
Keep: `run` (maps to `StartFlow`), `status`, `cleanup`, `wait-for-job`.

## Definition of done

- Project compiles cleanly with no references to old orchestrator methods
- `StartFlow` creates a worktree, runs the first state, saves state as `waiting`
- `ApplyAction("Approve", "")` on an `investigation` state transitions to `implementation` and runs it
- `ApplyAction("Provide Feedback", "your comment here")` re-runs the current state with feedback written to `feedback.md`
- `run_script` action dispatches correctly to `on_success` / `on_failure` / `always`
- `CleanupTicket` removes worktree and state directory
- Integration test: full flow start → investigation → approve → implementation → accept → done
