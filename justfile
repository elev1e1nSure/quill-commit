bin := "quill-commit" + if os_family() == "windows" { ".exe" } else { "" }


build:
    @go build -o {{bin}} .

run: build
    @./{{bin}}

lint:
    @golangci-lint run ./...

test:
    @go test ./...

tidy:
    @go mod tidy

commit:
    @git add -A && git commit
