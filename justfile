bin := "quill-commit" + if os_family() == "windows" { ".exe" } else { "" }

[private]
default:
    @printf "\033[1;94mquill-commit\033[0m \033[2m·\033[0m auto git committer\n\n"
    @printf "\033[1mcommands\033[0m\n"
    @printf "  \033[94mbuild\033[0m      compile binary\n"
    @printf "  \033[94mrun\033[0m        build and run\n"
    @printf "  \033[94mtest\033[0m       run tests\n"
    @printf "  \033[94mlint\033[0m       golangci-lint\n"
    @printf "  \033[94mtidy\033[0m       go mod tidy\n"
    @printf "  \033[94mcommit\033[0m     git add -A && commit\n"

build:
    go build -o {{bin}} .

run: build
    ./{{bin}}

lint:
    golangci-lint run ./...

test:
    go test ./...

tidy:
    go mod tidy

commit:
    git add -A && git commit
