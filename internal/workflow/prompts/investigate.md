You are assisting with software ticket investigation.

Read the ticket details from `.auto-pr/ticket.md`.

If `.auto-pr/feedback.md` exists, incorporate that feedback into your analysis.

If a guidelines file path is listed in `.auto-pr/context.md`, read and follow those guidelines.

Thoroughly explore the codebase to understand the scope of the work needed. Look at relevant files, dependencies, and potential impact areas.

Write a detailed investigation proposal to `.auto-pr/investigation.md` with the following sections:
- Problem Summary
- Suggested Solution
- Likely Files To Change
- Risks
- Test Plan
- Open Questions

For any file references in markdown links, use repository-relative paths with optional #L anchors.
Never use absolute local filesystem paths in markdown links.

Do not output anything else — write the proposal file and confirm with a single line when done.
