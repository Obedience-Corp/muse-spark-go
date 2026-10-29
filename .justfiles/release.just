set working-directory := ".."

# Local release gate: source checks, module integrity, and portable compilation.
check:
    just check
    go mod verify
    CGO_ENABLED=0 GOAMD64=v1 just build cross
