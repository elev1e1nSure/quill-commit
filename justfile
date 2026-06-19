bin := "quill-commit" + if os_family() == "windows" { ".exe" } else { "" }

[private]
[unix]
default:
    @printf "\033[1;94mquill-commit\033[0m · auto git committer\n\n"
    @printf "\033[1mcommands\033[0m\n"
    @printf "  \033[94mbuild\033[0m      compile binary\n"
    @printf "  \033[94mrun\033[0m        build and run\n"
    @printf "  \033[94mtest\033[0m       run tests\n"
    @printf "  \033[94mlint\033[0m       golangci-lint\n"
    @printf "  \033[94mtidy\033[0m       go mod tidy\n"
    @printf "  \033[94mcommit\033[0m     git add -A && commit\n"

[private]
[windows]
default:
    @powershell -NoLogo -NoProfile -Command "$e=[char]27; Write-Host \"${e}[1;94mquill-commit${e}[0m · auto git committer\"; Write-Host ''; Write-Host \"${e}[1mcommands${e}[0m\"; Write-Host \"  ${e}[94mbuild${e}[0m      compile binary\"; Write-Host \"  ${e}[94mrun${e}[0m        build and run\"; Write-Host \"  ${e}[94mtest${e}[0m       run tests\"; Write-Host \"  ${e}[94mlint${e}[0m       golangci-lint\"; Write-Host \"  ${e}[94mtidy${e}[0m       go mod tidy\"; Write-Host \"  ${e}[94mcommit${e}[0m     git add -A && commit\""

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
