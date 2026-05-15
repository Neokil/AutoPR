Read `.auto-pr/run-context.md`.

Read the ticket number from the `Ticket Number` field in `.auto-pr/run-context.md`.

Determine the ticket source from the prefix of the ticket number and fetch the details accordingly:

**Shortcut ticket (`SC-` prefix):**
- Run `short story <ticket-number> --quiet` to get the full story details.
- Derive the branch name in this format: `sc-<id>-<slugified-title>`, where the slugified title is lowercase, with non-alphanumeric characters replaced by hyphens, consecutive hyphens collapsed, and leading/trailing hyphens removed. Example: `SC-67523` titled "Extend internal payload of loyalty" → `sc-67523-extend-internal-payload-of-loyalty`.

**GitHub Issue (`GH-` prefix):**
- Extract the numeric issue number (the part after `GH-`).
- Run `gh issue view <number> --json number,title,body,labels,url` to get the issue details.
- Derive the branch name in this format: `gh-<number>-<slugified-title>`. Example: `GH-14` titled "Add GitHub Issues support" → `gh-14-add-github-issues-support`.

Write the full ticket details as a markdown document to the `Current Primary Artifact` path listed in `.auto-pr/run-context.md`. Use the following structure exactly:

1. A top-level heading (`#`) containing the ticket title.
2. Single-line metadata entries (no sub-headings) immediately after the title, each separated by a blank line:
   - `ID: <ticket-number>`
   - `URL: <url>`
   - `Labels: <labels>`
   - `Branch: <branch-name>`
   - For Shortcut tickets only: `Priority: <priority>`
3. Sections (`##`) for the richer content:
   - Description
   - Acceptance criteria (if present)
   - Parent ticket context (if any, Shortcut only)
   - Epic context (if any, Shortcut only)

   If the content of any section contains markdown headings, demote them so they nest correctly under the section. A `#` heading inside a `##` section becomes `###`, a `##` becomes `####`, and so on.

Do not output anything else — write the file and confirm with a single line when done.
