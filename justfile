default: lint

tidy:
    go mod tidy

bin := "quill-commit" + if os_family() == "windows" { ".exe" } else { "" }

build:
    go build -o {{bin}} .

run: build
    ./{{bin}}

lint:
    golangci-lint run ./...

test:
    go test ./...

commit:
    git add -A && git commit

precommit: lint test
    npx husky run commit-msg