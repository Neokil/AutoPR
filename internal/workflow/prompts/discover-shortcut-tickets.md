Use the `short` CLI to search for stories that meet ALL of the following criteria:
1. Have the label "auto-pr"
2. Are NOT in a workflow state with type "done"
3. Are NOT in a workflow state with type "in progress"

Run: `short search --label 'auto-pr' --quiet`

Review the output and exclude any stories whose workflow state is of type "done" or "in progress" (e.g. states named "Completed", "Done", "In Progress", etc.).

Output ONLY a JSON array and nothing else — no prose, no markdown fences. Each element must have exactly two string fields: "ticket_number" (the Shortcut story ID, e.g. "SC-123") and "title" (the story name).

Example output:
[{"ticket_number":"SC-123","title":"Fix login bug"},{"ticket_number":"SC-456","title":"Add dark mode"}]

If no matching stories are found, output an empty array: []
