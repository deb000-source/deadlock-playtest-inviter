param([string]$OutputPath = "")

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
if (-not $OutputPath) {
    $OutputPath = Join-Path $projectRoot "dist/THIRD_PARTY_LICENSES.txt"
}
Push-Location -LiteralPath $projectRoot
try {
    $moduleRows = @(go list -mod=readonly -deps -f '{{with .Module}}{{if not .Main}}{{.Path}}|{{.Version}}|{{.Dir}}{{end}}{{end}}' .)
    if ($LASTEXITCODE -ne 0) { throw "Cannot list build dependencies" }
    $goRoot = go env GOROOT
    if ($LASTEXITCODE -ne 0) { throw "Cannot locate Go license" }
    $goVersion = go env GOVERSION
    if ($LASTEXITCODE -ne 0) { throw "Cannot determine Go version" }
    $components = @("Go standard library and runtime|$goVersion|$goRoot") + @($moduleRows | Where-Object { $_.Trim() } | Sort-Object -Unique)
    $text = [System.Text.StringBuilder]::new()
    [void]$text.AppendLine("Third-party license texts for this build of Deadlock Playtest Inviter.")
    foreach ($component in $components) {
        $parts = $component -split '\|', 3
        if ($parts.Count -ne 3 -or -not $parts[2]) { throw "Missing dependency source directory" }
        $licenseFiles = @(Get-ChildItem -LiteralPath $parts[2] -File | Where-Object { $_.Name -match '^(LICENSE|COPYING|NOTICE|PATENTS)(\..+)?$' } | Sort-Object Name)
        if (-not ($licenseFiles | Where-Object { $_.Name -match '^(LICENSE|COPYING)(\..+)?$' })) {
            throw "No license file found for $($parts[0])"
        }
        [void]$text.AppendLine("`n============================================================")
        [void]$text.AppendLine("$($parts[0]) $($parts[1])")
        foreach ($licenseFile in $licenseFiles) {
            [void]$text.AppendLine("`n--- $($licenseFile.Name) ---")
            [void]$text.AppendLine([System.IO.File]::ReadAllText($licenseFile.FullName))
        }
    }
    $absoluteOutput = [System.IO.Path]::GetFullPath($OutputPath)
    [void][System.IO.Directory]::CreateDirectory([System.IO.Path]::GetDirectoryName($absoluteOutput))
    [System.IO.File]::WriteAllText($absoluteOutput, $text.ToString(), [System.Text.UTF8Encoding]::new($false))
    Write-Output "Collected licenses for $($components.Count) components: $absoluteOutput"
} finally {
    Pop-Location
}
