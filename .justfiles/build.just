set working-directory := ".."

all:
    go build ./...

# Build the demo executable for recording or standalone use.
demos:
    go build -o bin/muse-demo ./examples/demo

# Compile the public package and examples for supported Go targets.
cross:
    #!/usr/bin/env bash
    set -euo pipefail
    for target in linux darwin windows; do
      for arch in amd64 arm64; do
        GOOS="$target" GOARCH="$arch" go build ./...
      done
    done
