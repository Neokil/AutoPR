Read `.auto-pr/run-context.md`.

Read the ticket number from the `Ticket Number` field in `.auto-pr/run-context.md`.

Fetch the Shortcut ticket details for that ticket number using the `short` CLI:
- Run `short story <ticket-number> --quiet` to get the full story details.

Derive the branch name from the ticket number and title using this format: `sc-<id>-<slugified-title>`, where the slugified title is lowercase, with non-alphanumeric characters replaced by hyphens, consecutive hyphens collapsed, and leading/trailing hyphens removed. Example: ticket `67523` titled "Extend internal payload of loyalty" → `sc-67523-extend-internal-payload-of-loyalty`.

Write the full ticket details as a markdown document to the `Current Primary Artifact` path listed in `.auto-pr/run-context.md`. Use the following structure exactly:

1. A top-level heading (`#`) containing the ticket title.
2. Single-line metadata entries (no sub-headings) immediately after the title, each separated by a blank line:
   - `ID: <id>`
   - `Priority: <priority>`
   - `URL: <url>`
   - `Labels: <labels>`
   - `Branch: <branch-name>`
3. Sections (`##`) for the richer content:
   - Description
   - Acceptance criteria
   - Parent ticket context (if any)
   - Epic context (if any)

   If the content of any section contains markdown headings, demote them so they nest correctly under the section. A `#` heading inside a `##` section becomes `###`, a `##` becomes `####`, and so on.

Do not output anything else — write the file and confirm with a single line when done.
