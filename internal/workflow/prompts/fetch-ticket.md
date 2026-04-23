Read `.auto-pr/run-context.md`.

Read the ticket number from the `Ticket Number` field in `.auto-pr/run-context.md`.

Fetch the Shortcut ticket details for that ticket number using your configured MCP integration.

Write the full ticket details as a markdown document to the `Current Primary Artifact` path listed in `.auto-pr/run-context.md`. Include sections with the following information:
- ID
- Title
- Description
- Acceptance criteria
- Priority
- URL
- Labels
- Parent ticket context (if any)
- Epic context (if any)

Do not output anything else — write the file and confirm with a single line when done.
