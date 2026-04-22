# Phase 1 — Workflow Config

## Goal

Define the Go types for the workflow configuration and implement the three-level loader. At the end of this phase the runtime can load a `workflow.yaml` from any level of the hierarchy and fall back to an embedded default. No orchestration logic changes yet.

## New package: `internal/workflow/`

### `internal/workflow/types.go`

```go
type WorkflowConfig struct {
    States []StateConfig `yaml:"states"`
}

type StateConfig struct {
    Name               string        `yaml:"name"`
    Prompt             string        `yaml:"prompt"`             // path relative to prompts dir
    PrePromptCommands  []string      `yaml:"pre_prompt_commands"`
    PostPromptCommands []string      `yaml:"post_prompt_commands"`
    Actions            []ActionConfig `yaml:"actions"`
}

type ActionConfig struct {
    Label     string      `yaml:"label"`
    Type      ActionType  `yaml:"type"`

    // move_to_state
    Target    string      `yaml:"target,omitempty"`

    // run_script
    Commands  []string    `yaml:"commands,omitempty"`
    OnSuccess *ActionConfig `yaml:"on_success,omitempty"`
    OnFailure *ActionConfig `yaml:"on_failure,omitempty"`
    Always    *ActionConfig `yaml:"always,omitempty"`
}

type ActionType string

const (
    ActionProvideFeedback ActionType = "provide_feedback"
    ActionMoveToState     ActionType = "move_to_state"
    ActionRunScript       ActionType = "run_script"
)
```

`ActionConfig` is recursive — `OnSuccess`, `OnFailure`, and `Always` are themselves `ActionConfig` pointers. Leaf actions (`provide_feedback`, `move_to_state`) must not contain `Commands` or nested handlers.

Add a `Validate() error` method to `WorkflowConfig` that checks:
- All `move_to_state` targets either exist in `States` or are recognised terminal names (`done`, `cancelled`, `failed`)
- Leaf actions (`provide_feedback`, `move_to_state`) do not have `commands` or sub-handlers set
- `run_script` actions have at least one command and at least one handler (`on_success`, `on_failure`, or `always`)

### `internal/workflow/loader.go`

Resolution order (first file found wins):

1. `<repo-root>/.auto-pr/workflow.yaml`
2. `~/.auto-pr/workflow.yaml`
3. Embedded default (see below)

```go
func Load(repoRoot string) (WorkflowConfig, error)
```

The function walks the three sources, unmarshals the first YAML it finds, and calls `Validate()` before returning.

### `internal/workflow/default.go`

Embed the default workflow YAML using `//go:embed default_workflow.yaml`.

### `internal/workflow/default_workflow.yaml`

```yaml
states:
  - name: fetch-ticket-data
    prompt: prompts/fetch-ticket.md
    pre_prompt_commands: []
    post_prompt_commands: []
    actions:
      - label: "Continue"
        type: move_to_state
        target: investigation
      - label: "Cancel"
        type: move_to_state
        target: done

  - name: investigation
    prompt: prompts/investigate.md
    pre_prompt_commands: []
    post_prompt_commands: []
    actions:
      - label: "Provide Feedback"
        type: provide_feedback
      - label: "Approve"
        type: move_to_state
        target: implementation
      - label: "Decline"
        type: move_to_state
        target: done

  - name: implementation
    prompt: prompts/implement.md
    pre_prompt_commands: []
    post_prompt_commands: []
    actions:
      - label: "Provide Feedback"
        type: provide_feedback
      - label: "Fetch PR Feedback"
        type: run_script
        commands:
          - gh pr view --json reviews --jq '[.reviews[].body] | join("\n\n")'
        always:
          type: provide_feedback
      - label: "Accept"
        type: move_to_state
        target: done
      - label: "Cancel"
        type: move_to_state
        target: done
```

### Prompt file updates

Existing prompts in `~/.auto-pr/prompts/` currently use Go template syntax to receive injected values. In v3 they become plain markdown. The prompt text instructs the AI to read context from files in the worktree rather than receiving it as template values.

Update the three default prompts (`fetch-ticket.md`, `investigate.md`, `implement.md`) to reference worktree-relative paths:

| Old template variable | New file reference in prompt text |
|-----------------------|-----------------------------------|
| `{{ .Ticket }}` | `.auto-pr/ticket.md` |
| `{{ .Feedback }}` | `.auto-pr/feedback.md` (check if exists) |
| `{{ .Guidelines }}` | path noted in config `guidelines_file` |
| `{{ .ProposalPath }}` | `.auto-pr/investigation.md` |

The fetch-ticket prompt instructs the AI to fetch ticket data (via MCP or any available tool) and write the result to `.auto-pr/ticket.md`.

## Worktree file layout (established in Phase 4, documented here for reference)

```
<worktree>/
  .auto-pr/
    ticket.md          # written by fetch-ticket-data state
    feedback.md        # written before each re-run when feedback is provided; deleted after state starts
    <state-name>.md    # primary output of each state (e.g. investigation.md, implementation.md)
    <state-name>.log   # raw AI output for each state run
```

## Files changed

| File | Change |
|------|--------|
| `internal/workflow/types.go` | new |
| `internal/workflow/loader.go` | new |
| `internal/workflow/default.go` | new |
| `internal/workflow/default_workflow.yaml` | new |
| `internal/workflow/types_test.go` | new — validate() unit tests |
| `internal/workflow/loader_test.go` | new — hierarchy loading tests |

No existing files are modified in this phase.

## Definition of done

- `workflow.Load(repoRoot)` returns a valid `WorkflowConfig` in all three scenarios: project file present, global file present, neither (embedded default)
- `Validate()` rejects configs with broken target references or malformed `run_script` actions
- Unit tests pass for all loader and validation paths
