set working-directory := ".."

all:
    go test -timeout 90s ./...

race:
    go test -race -timeout 120s ./...

# Host tests use subprocess protocol fixtures and do not mutate workspaces.
coverage:
    mkdir -p coverage
    go test -race -coverprofile=coverage/coverage.out -timeout 120s ./...
    go tool cover -func=coverage/coverage.out

# Run all checks with network disabled and read-only source/module cache.
container version='1.24':
    docker run --rm --network none --mount type=bind,src="{{justfile_directory()}}",dst=/src,readonly --mount type=bind,src="$(go env GOMODCACHE)",dst=/go/pkg/mod,readonly -w /src golang:{{version}}-bookworm sh -c 'go vet ./... && go test -race -timeout 120s ./... && go build ./...'

# Real Muse demos mutate only an ephemeral container's isolated fixture.
# Supply a Linux binary matching Docker's architecture; it is mounted read-only.
native muse_binary:
    docker run --rm --network none --mount type=bind,src="{{muse_binary}}",dst=/muse,readonly --mount type=bind,src="{{justfile_directory()}}",dst=/src,readonly --mount type=bind,src="$(go env GOMODCACHE)",dst=/go/pkg/mod,readonly -w /src golang:1.24-bookworm go run ./examples/demo -muse /muse -demo all
