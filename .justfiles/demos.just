set working-directory := ".."

# One-shot structured result using the installed Muse echo provider.
basic:
    go run ./examples/demo -demo basic

# Print runtime records as they arrive.
streaming:
    go run ./examples/demo -demo streaming

# Two turns through one MSP host; pass --interactive for a prompt loop.
sessions *args:
    go run ./examples/demo -demo sessions {{args}}

# Stop the host, reopen the same durable session and replay its history.
resume:
    go run ./examples/demo -demo resume

# Cancel and interrupt native turns, checking their terminal outcomes.
cancel:
    go run ./examples/demo -demo cancel

# Select a restrictive approval mode and inspect pending requests.
permissions:
    go run ./examples/demo -demo permissions

# Typed process failure and context cancellation.
errors:
    go run ./examples/demo -demo errors

# All real-binary demos, isolated from user configuration.
all:
    go run ./examples/demo -demo all

# Render the real demo commands; requires VHS, ffmpeg and Muse.
record:
    vhs docs/demos/muse.tape
