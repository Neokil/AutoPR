Implement the approved solution for the ticket in this worktree.

Read `.auto-pr/run-context.md`.

Read the ticket details from the latest `fetch-ticket-data` artifact path listed in the `Latest State Artifacts` section of `.auto-pr/run-context.md`.
Read the approved proposal from the latest `investigation` artifact path listed in the `Latest State Artifacts` section of `.auto-pr/run-context.md`.

If the `Feedback File` listed in `.auto-pr/run-context.md` exists, incorporate that feedback into your implementation.

If a `Guidelines File` is listed in `.auto-pr/run-context.md`, read and follow those guidelines.

Implement all changes described in the proposal. Then:
1. Automatically detect and run this project's formatting and linting commands directly in the worktree.
2. Discover commands from the repository itself (package scripts, Makefile targets, tool config files, or language-native defaults).
3. If a command fails, fix the code and re-run until it passes or clearly report blockers.

If the repository is connected to GitHub, create a pull request for the changes.

Write a summary to the `Current Primary Artifact` path listed in `.auto-pr/run-context.md` with the following sections:
- Changes Made
- Notable Files Changed
- Remaining Risks
- Tests To Run

For any file references in markdown links, use repository-relative paths with optional #L anchors.
Never use absolute local filesystem paths in markdown links.

Do not output anything else — write the summary file and confirm with a single line when done.
