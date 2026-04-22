# Phase 3 — Domain State

## Goal

Simplify the `ticket.State` struct and remove the hardcoded workflow state constants. State names become plain strings matching names declared in `workflow.yaml`. All artifact path fields (`ProposalPath`, `FinalPath`, etc.) are removed because paths are now deterministic from the worktree root and state name.

## Current `State` struct (to be simplified)

```go
type State struct {
    TicketNumber    string        `json:"ticket_number"`
    BranchName      string        `json:"branch_name"`
    WorktreePath    string        `json:"worktree_path"`
    Status          WorkflowState `json:"status"`
    Approved        bool          `json:"approved"`
    FixAttempts     int           `json:"fix_attempts"`
    LastError       string        `json:"last_error,omitempty"`
    LastFeedback    string        `json:"last_feedback,omitempty"`
    CreatedAt       time.Time     `json:"created_at"`
    UpdatedAt       time.Time     `json:"updated_at"`
    ProposalPath    string        `json:"proposal_path"`
    FinalPath       string        `json:"final_solution_path"`
    LogPath         string        `json:"log_path"`
    PRPath          string        `json:"pr_path"`
    ChecksLogPath   string        `json:"checks_log_path"`
    TicketJSONPath  string        `json:"ticket_json_path"`
    ProviderDirPath string        `json:"provider_dir_path"`
    PRURL           string        `json:"pr_url,omitempty"`
}
```

## New `State` struct

```go
type FlowStatus string

const (
    FlowStatusRunning  FlowStatus = "running"
    FlowStatusWaiting  FlowStatus = "waiting"   // waiting for human action
    FlowStatusDone     FlowStatus = "done"
    FlowStatusFailed   FlowStatus = "failed"
    FlowStatusCancelled FlowStatus = "cancelled"
)

type State struct {
    TicketNumber string     `json:"ticket_number"`
    CurrentState string     `json:"current_state"`  // matches a StateConfig.Name
    FlowStatus   FlowStatus `json:"flow_status"`
    BranchName   string     `json:"branch_name"`
    WorktreePath string     `json:"worktree_path"`
    LastError    string     `json:"last_error,omitempty"`
    PRURL        string     `json:"pr_url,omitempty"`
    CreatedAt    time.Time  `json:"created_at"`
    UpdatedAt    time.Time  `json:"updated_at"`
}
```

Fields removed and why:

| Removed field | Reason |
|---------------|--------|
| `Status WorkflowState` | Replaced by `CurrentState` (config-driven) + `FlowStatus` (runtime status) |
| `Approved bool` | Was only used to track the investigate→implement gate; no longer needed |
| `FixAttempts int` | Retry logic removed; feedback loops are explicit user actions |
| `LastFeedback string` | Feedback is written to `<worktree>/.auto-pr/feedback.md` |
| `ProposalPath` | Deterministic: `<worktree>/.auto-pr/investigation.md` |
| `FinalPath` | Deterministic: `<worktree>/.auto-pr/implementation.md` |
| `LogPath` | Deterministic: `<worktree>/.auto-pr/<state-name>.log` |
| `PRPath` | Deterministic: `<worktree>/.auto-pr/implementation.md` (same file) |
| `ChecksLogPath` | Replaced by post_prompt_commands output in state log |
| `TicketJSONPath` | Ticket data now lives in `<worktree>/.auto-pr/ticket.md` |
| `ProviderDirPath` | Managed internally by orchestrator, not stored in state |

`PRURL` is kept because it's set by an external system (GitHub) and cannot be derived.

## Path helpers

Add a package-level helper so callers never construct paths by hand:

```go
func ArtifactPath(worktreePath, name string) string {
    return filepath.Join(worktreePath, ".auto-pr", name)
}
// e.g. ArtifactPath(st.WorktreePath, "feedback.md")
//      ArtifactPath(st.WorktreePath, "investigation.md")
//      ArtifactPath(st.WorktreePath, st.CurrentState+".log")
```

## Changes to `lifecycle.go`

All existing lifecycle methods (`ShouldInvestigate`, `ShouldImplement`, `ApproveForImplementation`, `ApplyFeedback`, `RejectByHuman`, `WaitsForHumanInput`, `ShouldGeneratePROnRun`) are deleted.

`NextStepsCLI()` is updated to use `FlowStatus` and `CurrentState` instead of the removed constants.

## Changes to `types.go`

Remove the `WorkflowState` type and all 8 constants (`StateQueued`, `StateInvestigating`, etc.). The `ticket.Ticket` struct is unchanged.

## `state.json` migration

Existing `state.json` files on disk use the old schema. The state store loader (`internal/state/store.go`) should detect the old schema (presence of `"status"` key with a v2 value like `"investigating"`) and return a clear error rather than silently loading corrupt state. No automatic migration — v3 flows start fresh.

## Files changed

| File | Change |
|------|--------|
| `internal/domain/ticket/types.go` | Replace `WorkflowState` constants + old `State` struct with new |
| `internal/domain/ticket/lifecycle.go` | Delete all methods except `Touch()`, add `ArtifactPath` helper |
| `internal/state/store.go` | Add old-schema detection and error |
| Any file importing `ticket.StateXxx` constants | Update to `FlowStatus` or string comparisons |

## Definition of done

- `State` struct matches the new definition above
- All `ticket.StateXxx` constant references removed from the codebase
- `lifecycle.go` contains only `Touch()` and `ArtifactPath`
- State store returns a descriptive error on v2 state files
- Project compiles (orchestrator call sites will be broken until Phase 4)
