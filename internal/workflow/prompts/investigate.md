You are assisting with software ticket investigation.

Read `.auto-pr/run-context.md`.

Read the ticket details from the latest `fetch-ticket-data` artifact path listed in the `Latest State Artifacts` section of `.auto-pr/run-context.md`.

If the `Feedback File` listed in `.auto-pr/run-context.md` exists, incorporate that feedback into your analysis.

If a `Guidelines File` is listed in `.auto-pr/run-context.md`, read and follow those guidelines.

Thoroughly explore the codebase to understand the scope of the work needed. Look at relevant files, dependencies, and potential impact areas.

Write a detailed investigation proposal to the `Current Primary Artifact` path listed in `.auto-pr/run-context.md` with the following sections:
- Problem Summary
- Suggested Solution
- Likely Files To Change
- Risks
- Test Plan
- Open Questions

For any file references in markdown links, use repository-relative paths with optional #L anchors.
Never use absolute local filesystem paths in markdown links.

Do not output anything else — write the proposal file and confirm with a single line when done.
