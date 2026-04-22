# Phase 6 — Frontend

## Goal

Replace all hardcoded state→action mappings in the React frontend with dynamic rendering driven by the `available_actions` field in the API response. At the end of this phase no frontend code references specific state names or action names.

## What changes

### Remove `allowedActions()` switch

`web/src/App.tsx` lines 34–53 currently contain:

```ts
function allowedActions(status: string): Action[] {
  switch (status) {
    case "queued":       return ["run", "cleanup"];
    case "proposal_ready":
    case "waiting_for_human": return ["approve", "reject"];
    ...
  }
}
```

Delete this function entirely. Available actions now come from the API (`ticket.available_actions`).

### Remove typed API call functions

`web/src/api.ts` currently exports individual functions per action:

```ts
approveTicket(repoId, ticketNumber)
rejectTicket(repoId, ticketNumber)
feedbackTicket(repoId, ticketNumber, message)
resumeTicket(repoId, ticketNumber)
createPR(repoId, ticketNumber)
applyPRComments(repoId, ticketNumber)
```

Replace all of these with a single:

```ts
applyAction(repoId: string, ticketNumber: string, action: string, feedback?: string): Promise<Job>
```

Keep: `runTicket`, `cleanupTicket` (unchanged endpoints).

### Updated `types.ts`

Add `AvailableAction` and update `TicketDetails`:

```ts
export interface AvailableAction {
  label: string;
  type: string;   // "provide_feedback" | "move_to_state" | "run_script"
}

export interface TicketDetails {
  ticket_number: string;
  current_state: string;
  flow_status: string;       // "running" | "waiting" | "done" | "failed" | "cancelled"
  worktree_path: string;
  branch_name: string;
  pr_url?: string;
  last_error?: string;
  available_actions: AvailableAction[];
  updated_at: string;
}
```

Remove from types: `TicketSummary.status` (replace with `current_state` + `flow_status`).

### Action button rendering

Current button rendering is a series of `if (actions.includes("approve"))` checks. Replace with:

```tsx
{ticket.available_actions.map((action) => (
  <button
    key={action.label}
    onClick={() => handleAction(action)}
  >
    {action.label}
  </button>
))}
```

### Feedback input visibility

Currently the feedback textarea is always visible on `proposal_ready`/`waiting_for_human`. In v3, show it when any available action has `type === "provide_feedback"` or when a `run_script` action chains into `provide_feedback` (the API response doesn't expose script internals, so the client just checks `type === "provide_feedback"` directly on the action):

```ts
const hasFeedbackAction = ticket.available_actions.some(
  (a) => a.type === "provide_feedback"
);
```

When `hasFeedbackAction` is true, show the textarea. Any action button clicked while the textarea has content sends that content as the `feedback` field in the request body. If the textarea is empty, `feedback` is omitted.

Note: a `run_script` action that chains into `provide_feedback` will have `type === "run_script"` in the response — its internals are server-side only. The user would click the button label (e.g. "Fetch PR Feedback") without filling the textarea; the script output becomes the feedback automatically. So the textarea should only appear alongside explicitly `provide_feedback`-typed actions.

### `TicketList` component

`web/src/TicketList.tsx` currently shows status badges based on the old state names. Update to use `flow_status` for the badge colour/icon and `current_state` for the label text:

```tsx
// badge colour driven by flow_status: running=blue, waiting=yellow, done=green, failed=red, cancelled=grey
// label text: ticket.current_state (e.g. "investigation", "implementation")
```

### Removing the "Open PR" button special case

Currently there's a dedicated "Open PR" button that appears when `pr_url` is set, wired to an explicit state check. In v3 `pr_url` can appear at any point. Keep the button but decouple it from state: show it whenever `ticket.pr_url` is non-empty, regardless of `current_state` or `flow_status`.

## Files changed

| File | Change |
|------|--------|
| `web/src/App.tsx` | Remove `allowedActions`, remove per-action handler functions, add generic `handleAction`, update button rendering, update feedback textarea visibility logic |
| `web/src/api.ts` | Remove 6 typed action functions, add `applyAction` |
| `web/src/types.ts` | Add `AvailableAction`, update `TicketDetails`, update `TicketSummary` |
| `web/src/TicketList.tsx` | Update badge to use `flow_status` + `current_state` |

## Definition of done

- No references to `"proposal_ready"`, `"waiting_for_human"`, `"investigating"`, `"implementing"`, `"validating"`, `"pr_ready"` remain in frontend source
- Action buttons render correctly for any workflow config (not just the embedded default)
- Feedback textarea appears only when a `provide_feedback` action is available
- "Fetch PR Feedback" button works end-to-end: clicks → `applyAction` → server runs script → state re-runs
- `TicketList` status badge works with `flow_status`
- `run` and `cleanup` actions still work
