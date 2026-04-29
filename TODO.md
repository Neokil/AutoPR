# Features
- Hot-Reload of prompts. Right now prompts are only read once and reused. We should only read them when we execute them. So changes to prompts are used immediately.
- Disable "action-buttons" while an action is running. Also "Rerun" and "Move to state" in the burger menu should be disabled.
- When the ticket gets moved to a new state the FE should also transition automatically
- The feedback text-field should be a multiline textfield and the send should not be triggered on enter.
- Fetch all Tickets from shortcut that have the specific "auto-pr" tag and are not done or in progress

# Fixes

