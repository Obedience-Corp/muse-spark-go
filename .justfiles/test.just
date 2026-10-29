set working-directory := ".."

all:
    go test ./...

race:
    go test -race ./...
