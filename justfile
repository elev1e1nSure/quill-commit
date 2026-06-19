set windows-shell := ["powershell.exe", "-NoLogo", "-Command"]

bin := "quill-commit" + if os_family() == "windows" { ".exe" } else { "" }

[windows]
build:
    @go build -o {{bin}} .; $ec=$LASTEXITCODE; if ($ec -eq 0) { Write-Host "  built  " -NoNewline -ForegroundColor DarkGray; Write-Host "{{bin}}" -ForegroundColor Cyan }; exit $ec

[unix]
build:
    @go build -o {{bin}} . && printf "  built  \033[96m{{bin}}\033[0m\n"


[windows]
test:
    @go test ./...; $ec=$LASTEXITCODE; if ($ec -eq 0) { Write-Host "  tests  " -NoNewline -ForegroundColor DarkGray; Write-Host "ok" -ForegroundColor Green }; exit $ec

[unix]
test:
    @go test ./... && printf "  tests  \033[92mok\033[0m\n"

[windows]
lint:
    @golangci-lint run ./...; $ec=$LASTEXITCODE; if ($ec -eq 0) { Write-Host "  lint   " -NoNewline -ForegroundColor DarkGray; Write-Host "clean" -ForegroundColor Green }; exit $ec

[unix]
lint:
    @golangci-lint run ./... && printf "  lint   \033[92mclean\033[0m\n"

[windows]
tidy:
    @go mod tidy; $ec=$LASTEXITCODE; if ($ec -eq 0) { Write-Host "  tidy   " -NoNewline -ForegroundColor DarkGray; Write-Host "done" -ForegroundColor Green }; exit $ec

[unix]
tidy:
    @go mod tidy && printf "  tidy   \033[92mdone\033[0m\n"

commit:
    @git add -A
    @git commit
