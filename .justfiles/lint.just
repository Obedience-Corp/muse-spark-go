set working-directory := ".."

all:
    go vet ./...
    test -z "$(gofmt -l pkg examples)"
