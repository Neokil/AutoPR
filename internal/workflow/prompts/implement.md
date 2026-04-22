Implement the approved solution for the ticket in this worktree.

Read the ticket details from `.auto-pr/ticket.md`.
Read the approved proposal from `.auto-pr/investigation.md`.

If `.auto-pr/feedback.md` exists, incorporate that feedback into your implementation.

If a guidelines file path is listed in `.auto-pr/context.md`, read and follow those guidelines.

Implement all changes described in the proposal. Then:
1. Automatically detect and run this project's formatting and linting commands directly in the worktree.
2. Discover commands from the repository itself (package scripts, Makefile targets, tool config files, or language-native defaults).
3. If a command fails, fix the code and re-run until it passes or clearly report blockers.

If the repository is connected to GitHub, create a pull request for the changes.

Write a summary to `.auto-pr/implementation.md` with the following sections:
- Changes Made
- Notable Files Changed
- Remaining Risks
- Tests To Run

For any file references in markdown links, use repository-relative paths with optional #L anchors.
Never use absolute local filesystem paths in markdown links.

Do not output anything else — write the summary file and confirm with a single line when done.
