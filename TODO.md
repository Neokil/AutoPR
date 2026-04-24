# Features
- Use slog for all logging, convert logs to json format and align logs so they all look the same
- Integrate with LLM sessions, so the whole flow uses a single session in order to maintain context
- check what "isV2StateJSON" is for. Looks like some backwards compatibel nonsense

# Bugs
- When a prompt fails it is visible in the UI, but the button to rerun should not be called "Run" but "Retry"

# Fixes

# When finished with development we should do the following:
1. when installing we want to move the built binary to a protected location
2. update the plist to use that new location
3. can we sign the binary for authenticity?
can you restrict the KeepAlive behaviour to "KeepAlive" => { "SuccessfulExit" => false } so the process restarts only on non-zero exit codes.
can you set a Umask key?
