# Phase 5 — API Layer

## Goal

Replace the typed action endpoints (`/approve`, `/reject`, `/feedback`, etc.) with a single generic action endpoint. Ticket responses include the available actions for the current state so that clients never need to hardcode state→action mappings.

## New endpoints

### `POST /api/tickets/{repoId}/{ticketNumber}/action`

Replaces: `/approve`, `/reject`, `/feedback`, `/resume`, `/pr`, `/apply-pr-comments`

Request body:
```json
{
  "action": "Approve",
  "feedback": ""
}
```

`feedback` is only used when the resolved action type is `provide_feedback` or when a `run_script` result chains into `provide_feedback`. It is ignored otherwise.

Response: `202 Accepted` with a job ID (same as current action endpoints).

Error cases:
- `400` — action label not found in current state's config
- `409` — flow is not in `waiting` status (already running or terminal)

### `POST /api/tickets/{repoId}/{ticketNumber}/run`

Unchanged — maps to `orchestrator.StartFlow`. Returns `409` if flow is already active.

### `POST /api/tickets/{repoId}/{ticketNumber}/cleanup`

Unchanged.

### Endpoints to remove

| Old endpoint | Replaced by |
|--------------|-------------|
| `POST .../approve` | `POST .../action` with `{"action":"Approve"}` |
| `POST .../reject` | `POST .../action` with `{"action":"Decline"}` |
| `POST .../feedback` | `POST .../action` with `{"action":"Provide Feedback","feedback":"..."}` |
| `POST .../resume` | removed — no equivalent (state machine always waits, no resume needed) |
| `POST .../pr` | removed — PR creation is a prompt step in implementation state |
| `POST .../apply-pr-comments` | `POST .../action` with `{"action":"Fetch PR Feedback"}` |

## Updated ticket response

`GET /api/tickets/{repoId}/{ticketNumber}` adds `available_actions` to the response:

```json
{
  "ticket_number": "123",
  "current_state": "investigation",
  "flow_status": "waiting",
  "worktree_path": "/path/to/worktree",
  "branch_name": "auto-pr/123",
  "pr_url": "",
  "available_actions": [
    {
      "label": "Provide Feedback",
      "type": "provide_feedback"
    },
    {
      "label": "Approve",
      "type": "move_to_state",
      "target": "implementation"
    },
    {
      "label": "Decline",
      "type": "move_to_state",
      "target": "done"
    }
  ],
  "updated_at": "2026-04-22T10:00:00Z"
}
```

When `flow_status` is `running` or terminal, `available_actions` is an empty array.

The `ActionConfig` fields returned to the client should omit `commands`, `on_success`, `on_failure`, and `always` — the client only needs `label` and `type` (to know if it should show a feedback input).

## `contracts/api/types.go` changes

Remove: `ApproveRequest`, `FeedbackRequest`, `RejectRequest`, `ResumeRequest`

Add:
```go
type ActionRequest struct {
    Action   string `json:"action"`
    Feedback string `json:"feedback,omitempty"`
}

type AvailableAction struct {
    Label  string `json:"label"`
    Type   string `json:"type"`
}

// Updated TicketDetail
type TicketDetail struct {
    TicketNumber     string            `json:"ticket_number"`
    CurrentState     string            `json:"current_state"`
    FlowStatus       string            `json:"flow_status"`
    WorktreePath     string            `json:"worktree_path"`
    BranchName       string            `json:"branch_name"`
    PRURL            string            `json:"pr_url,omitempty"`
    LastError        string            `json:"last_error,omitempty"`
    AvailableActions []AvailableAction `json:"available_actions"`
    UpdatedAt        time.Time         `json:"updated_at"`
}
```

## Handler changes in `cmd/auto-prd/main.go`

Remove: `handleApproveTicket`, `handleFeedbackTicket`, `handleRejectTicket`, `handleResumeTicket`, `handlePRTicket`, `handleApplyPRComments`

Add: `handleActionTicket` — validates body, calls `orchestrator.ApplyAction`, enqueues job, returns job ID.

Update: `handleGetTicket` — populate `AvailableActions` by looking up current state in `workflow.WorkflowConfig`.

## Files changed

| File | Change |
|------|--------|
| `cmd/auto-prd/main.go` | Remove 6 handlers, add `handleActionTicket`, update route table |
| `internal/contracts/api/types.go` | Remove old request types, add `ActionRequest`, `AvailableAction`, update `TicketDetail` |

## Definition of done

- `POST .../action` returns `202` for a valid action on a waiting ticket
- `POST .../action` returns `400` for an unknown action label
- `GET .../ticket` response includes `available_actions` populated from workflow config
- Old typed endpoints are removed and return `404`
- The `auto-pr` CLI `action` subcommand works end-to-end via the new endpoint
