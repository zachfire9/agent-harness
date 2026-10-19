# Isolated native console lifecycle; no real keys, background tasks, or user PATH changes.
# Go is needed on the verification host only, never by a downloaded-binary user.
[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$root = Join-Path ([IO.Path]::GetTempPath()) ('agent-harness-onboarding-' + [guid]::NewGuid().ToString('N'))
$oldBinary = $env:HARNESS_ONBOARDING_SMOKE_BINARY
try {
    $stage = Join-Path $root 'staging'
    $extracted = Join-Path $root 'extracted with spaces'
    New-Item -ItemType Directory -Path $stage | Out-Null
    & go build -trimpath -o (Join-Path $stage 'agent-harness.exe') ./cmd/agent-harness
    if ($LASTEXITCODE -ne 0) { throw 'Native onboarding build failed' }
    Copy-Item -LiteralPath 'docs/install-release.md' -Destination (Join-Path $stage 'INSTALL.md')
    Copy-Item -LiteralPath 'docs/windows-runtime.md' -Destination (Join-Path $stage 'WINDOWS.md')
    Copy-Item -LiteralPath 'docs/windows-first-conversation.md' -Destination (Join-Path $stage 'FIRST-CONVERSATION.md')
    $zip = Join-Path $root 'agent-harness_windows_amd64.zip'
    Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $zip
    Expand-Archive -LiteralPath $zip -DestinationPath $extracted
    $env:HARNESS_ONBOARDING_SMOKE_BINARY = Join-Path $extracted 'agent-harness.exe'
    # The opt-in Go test drives hidden input via ConPTY, a loopback fake provider,
    # unique settings/credential target, temporary workspace/shortcut and stable
    # copy. It verifies a fresh config check, chat /exit, keep/rerun and removal.
    # Its defers remove only its own credential/shortcut even after a failed test.
    & go test ./internal/cli -run '^TestNativeOnboardingSmoke$' -count=1 -v -timeout 5m
    if ($LASTEXITCODE -ne 0) { throw 'Native onboarding lifecycle NOT verified' }
    Write-Host 'PASS: extracted binary setup / fresh config+credential / chat exit / rerun / removal'
} finally {
    $env:HARNESS_ONBOARDING_SMOKE_BINARY = $oldBinary
    if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Recurse -Force }
}
