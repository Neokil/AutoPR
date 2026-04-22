# Features
- Convert logs to json format
- Make the workflow configurable
    - Each state is defined with a prompt that will be executed when we enter the state
    - Each state defines Actions. Actions can be
        - Provide Feedback: Gets a string with feedback from the user and reexecutes the prompt with that additional information
        - Move to state: Moves the workflow to a different state
        - Example: We are in the proposal phase and we have 3 actions defined:
            - "Review": Provide Feedback action
            - "Approve": Move to next state
            - "Decline": Move to "declined" state
- Integrate with LLM sessions, so the whole flow uses a single session in order to maintain context


# Bugs
- 

# Fixes

# When finished with development we should do the following:
1. when installing we want to move the built binary to a protected location
2. update the plist to use that new location
3. can we sign the binary for authenticity?
can you restrict the KeepAlive behaviour to "KeepAlive" => { "SuccessfulExit" => false } so the process restarts only on non-zero exit codes.
can you set a Umask key?
