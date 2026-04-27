# Features
- Use slog for all logging, convert logs to json format and align logs so they all look the same
- Integrate with LLM sessions, so the whole flow uses a single session in order to maintain context
- check what "isV2StateJSON" is for. Looks like some backwards compatibel nonsense
- Detect when tokens are used up, then save the session so it can be picked up again when the user reruns the action and show an appropriate error

# Bugs
- When a prompt fails it is visible in the UI, but the button to rerun should not be called "Run" but "Retry"

# Fixes

