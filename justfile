default: lint

tidy:
    go mod tidy

build:
    go build -o quill-commit.exe .

run: build
    ./quill-commit.exe

lint:
    golangci-lint run ./...

test:
    go test ./...

commit:
    git add -A && git commit

precommit: lint test
    npx husky run commit-msg