# AutoPR v3 — Configurable Workflow

v3 replaces AutoPR's hardcoded 8-state pipeline with a fully configurable state machine. Each workflow state is defined in a YAML file alongside a prompt template. Human actions on each state (approve, provide feedback, run scripts) are also declared in config — the Go runtime becomes a generic executor rather than an opinionated pipeline.

## Motivation

The current orchestration is hardcoded: fixed states, fixed transitions, fixed provider methods (`Investigate`, `Implement`, `SummarizePR`). Adding or changing workflow steps requires modifying Go source. v3 makes the entire flow a first-class configuration artifact.

## Core design decisions

- **One provider method**: `Execute(promptPath, workDir)` replaces the four specialised methods.
- **All artifacts in the worktree**: markdown files (ticket data, proposals, feedback, logs) live inside the git worktree under `.auto-pr/`. Prompts reference them by path rather than receiving injected values.
- **Every state waits for human input**: after a state's prompt completes the system always stops and waits. There are no automatic transitions.
- **Three action types**: `provide_feedback`, `move_to_state`, `run_script` (with `on_success` / `on_failure` / `always` handlers).
- **Config hierarchy**: `<repo>/.auto-pr/workflow.yaml` → `~/.auto-pr/workflow.yaml` → binary-embedded default.
- **Worktree lifecycle**: worktree creation is a built-in pre-flow step; cleanup remains an explicit user action.

## Phases

| Phase | Document | Scope |
|-------|----------|-------|
| 1 | [phase-1-workflow-config.md](phase-1-workflow-config.md) | Config types, loader, embedded default |
| 2 | [phase-2-provider-interface.md](phase-2-provider-interface.md) | Collapse provider to single Execute method |
| 3 | [phase-3-domain-state.md](phase-3-domain-state.md) | Simplify State struct, remove hardcoded constants |
| 4 | [phase-4-orchestrator.md](phase-4-orchestrator.md) | Rewrite orchestrator as generic state machine |
| 5 | [phase-5-api.md](phase-5-api.md) | Generic action endpoint, return actions in responses |
| 6 | [phase-6-frontend.md](phase-6-frontend.md) | Dynamic action buttons, conditional feedback input |

Work phases 1–4 in order as each depends on the previous. Phases 5 and 6 can proceed in parallel once Phase 4 is done.
