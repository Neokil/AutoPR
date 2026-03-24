# Features
- Convert logs to json format
- Save query-configs per provider (so we can have different queries per provider and can easily switch between providers)
- When the PR was created there should be an "Open PR" button on the top

# Bugs
- When a PR is created the "Apply PR Comments" button appears but is disabled. You have to reload the page to make it available.

# Fixes

# When finished with development we should do the following:
1. when installing we want to move the built binary to a protected location
2. update the plist to use that new location
3. can we sign the binary for authenticity?
can you restrict the KeepAlive behaviour to "KeepAlive" => { "SuccessfulExit" => false } so the process restarts only on non-zero exit codes.
can you set a Umask key?
