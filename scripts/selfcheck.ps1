# Code self-check script (docs/ARCHITECTURE.md section 13).
# Usage: powershell -File scripts\selfcheck.ps1
# Non-zero exit code means a check failed.
$ErrorActionPreference = "Stop"

Write-Host "=== 1/4 go build ./... ==="
& go build ./...
if ($LASTEXITCODE -ne 0) { Write-Host "BUILD FAILED"; exit 1 }

Write-Host "=== 2/4 go vet ./... ==="
& go vet ./...
if ($LASTEXITCODE -ne 0) { Write-Host "VET FAILED"; exit 1 }

Write-Host "=== 3/4 gofmt -l . (expect no output) ==="
$fmt = & gofmt -l .
if ($fmt) {
  Write-Host "gofmt found files needing format:"
  Write-Host $fmt
  Write-Host "Run: gofmt -w <files> then retry."
  exit 1
}

Write-Host "=== 4/4 go test -race ./... ==="
& go test -race ./...
if ($LASTEXITCODE -ne 0) { Write-Host "TEST FAILED"; exit 1 }

Write-Host ""
Write-Host "OK: build / vet / gofmt / race test all passed"
