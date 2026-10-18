# Build a self-contained Windows executable. Requires Go 1.27.2+ and internet
# access for initial dependency download. No Deadlock installation required.
$ErrorActionPreference = "Stop"
Set-Location -Path $PSScriptRoot

go mod download
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
go mod verify
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
go test -mod=readonly ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
New-Item -ItemType Directory -Force -Path dist | Out-Null
go build -mod=readonly -trimpath -ldflags="-s -w" -o dist/deadlock-inviter.exe .
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& (Join-Path $PSScriptRoot "scripts/collect-licenses.ps1")
Copy-Item -LiteralPath (Join-Path $PSScriptRoot "LICENSE") -Destination (Join-Path $PSScriptRoot "dist/LICENSE.txt")
$executableHash = (Get-FileHash -LiteralPath (Join-Path $PSScriptRoot "dist/deadlock-inviter.exe") -Algorithm SHA256).Hash.ToLowerInvariant()
[System.IO.File]::WriteAllText((Join-Path $PSScriptRoot "dist/SHA256SUMS.txt"), "$executableHash  deadlock-inviter.exe`n", [System.Text.UTF8Encoding]::new($false))
Write-Host "Built: $PSScriptRoot\dist\deadlock-inviter.exe"
