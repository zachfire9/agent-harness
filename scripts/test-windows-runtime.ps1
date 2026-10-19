# Isolated native smoke only: no real credentials, existing instances, or tasks.
# Requires a logged-in current-user interactive session. Unavailable lifecycle
# evidence is a failure (including hosted runners without an interactive token).
[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$root = Join-Path ([IO.Path]::GetTempPath()) ('agent-harness-smoke-' + [guid]::NewGuid().ToString('N'))
$instance = 'smoke-' + [guid]::NewGuid().ToString('N')
$taskName = 'agent-harness-' + $instance
$instanceHome = Join-Path $root ("instance O'Brien & space " + [char]0x03BB)
$exe = Join-Path $root 'agent-harness.exe'
$taskOwned = $false
$daemonPid = $null

function Invoke-Harness {
    param([string[]]$Arguments)
    $output = & $exe @Arguments 2>&1
    if ($LASTEXITCODE -ne 0) { throw "CLI failed ($LASTEXITCODE): $($Arguments -join ' '): $output" }
    return ($output -join "`n")
}
function Wait-Until {
    param([scriptblock]$Condition, [string]$Failure)
    $deadline = [DateTime]::UtcNow.AddSeconds(30)
    do {
        if (& $Condition) { return }
        Start-Sleep -Milliseconds 250
    } while ([DateTime]::UtcNow -lt $deadline)
    throw $Failure
}

try {
    New-Item -ItemType Directory -Path $root | Out-Null
    & go build -o $exe ./cmd/agent-harness
    if ($LASTEXITCODE -ne 0) { throw 'Native Windows build failed' }
    $common = @('--instance', $instance, '--home', $instanceHome)
    Invoke-Harness (@('init') + $common) | Write-Host
    $envFile = Join-Path $instanceHome 'config\secrets\env'
    [IO.File]::WriteAllText($envFile, "OPENAI_API_KEY=smoke-placeholder-only`n")
    $sid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
    $acl = Get-Acl -LiteralPath $envFile
    $acl.SetSecurityDescriptorSddlForm('O:' + $sid + 'D:P(A;;FA;;;' + $sid + ')(A;;FR;;;WD)')
    Set-Acl -LiteralPath $envFile -AclObject $acl
    $rejected = & $exe daemon @common --test 2>&1
    if ($LASTEXITCODE -eq 0 -or ($rejected -join "`n") -notmatch 'private environment ACL') {
        throw 'Broad fake credential ACL was not rejected for the expected reason'
    }
    if (($rejected -join "`n") -match 'smoke-placeholder-only') { throw 'Credential leaked in diagnostic' }
    $acl.SetSecurityDescriptorSddlForm('O:' + $sid + 'D:P(A;;FA;;;' + $sid + ')')
    Set-Acl -LiteralPath $envFile -AclObject $acl
    if (-not (Get-Acl -LiteralPath $envFile).AreAccessRulesProtected) { throw 'Fixture ACL inheritance remains enabled' }
    Invoke-Harness (@('daemon') + $common + @('--test')) | Write-Host
    $snapshot = Invoke-Harness (@('status') + $common + @('--json')) | ConvertFrom-Json
    if (-not $snapshot.last_heartbeat_at -or $snapshot.instance -ne $instance) { throw 'Invalid native status snapshot' }
    $jobs = Invoke-Harness (@('jobs', 'list') + $common + @('--json')) | ConvertFrom-Json
    if (-not ($jobs | ConvertTo-Json -Depth 20).Contains('heartbeat')) { throw 'Starter job missing' }
    Write-Host 'PASS: native CLI init/daemon --test/status/jobs and broad/private fake ACL checks'

    Invoke-Harness (@('service', 'install') + $common) | Write-Host
    $registration = Join-Path $instanceHome 'config\register-task.ps1'
    if (Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue) { throw 'Refusing to replace an existing task' }
    $taskOwned = $true
    & $registration | Out-Null
    $task = Get-ScheduledTask -TaskName $taskName
    if ($task.Principal.LogonType -ne 'Interactive' -or $task.Principal.RunLevel -ne 'Limited') { throw 'Unexpected task principal' }
    if ($task.Actions.Execute -ne $exe -or $task.Actions.WorkingDirectory -ne $instanceHome) { throw 'Unexpected task action' }
    Get-ScheduledTaskInfo -TaskName $taskName | Format-List | Out-Host
    # A service-session hosted runner cannot prove an Interactive-token lifecycle.
    # Do not skip or convert this to success: release publication depends on it.
    if ((Get-Process -Id $PID).SessionId -eq 0) {
        throw 'NATIVE TASK LIFECYCLE UNAVAILABLE: runner is in session 0; rerun on a logged-in interactive Windows runner. CLI/ACL evidence passed, but E2E is NOT verified.'
    }
    $statusFile = Join-Path $instanceHome 'state\status.json'
    Remove-Item -LiteralPath $statusFile
    Start-ScheduledTask -TaskName $taskName
    Wait-Until {
        if (-not (Test-Path -LiteralPath $statusFile)) { return $false }
        # The first write may still be locked or contain incomplete JSON. Retry
        # only this read/parse operation within Wait-Until's bounded deadline.
        try {
            $script:live = Get-Content -LiteralPath $statusFile -Raw -ErrorAction Stop | ConvertFrom-Json -ErrorAction Stop
        } catch {
            return $false
        }
        return $script:live.status -eq 'running' -and $script:live.last_heartbeat_at
    } 'Task did not produce a fresh running heartbeat (interactive token may be unavailable); lifecycle NOT verified'
    $daemonPid = $live.pid
    Get-Process -Id $daemonPid -ErrorAction Stop | Out-Null
    if ((Get-ScheduledTask -TaskName $taskName).State -ne 'Running') { throw 'Task not running despite snapshot' }
    Stop-ScheduledTask -TaskName $taskName
    Wait-Until {
        return (Get-ScheduledTask -TaskName $taskName).State -ne 'Running' -and -not (Get-Process -Id $daemonPid -ErrorAction SilentlyContinue)
    } 'Task/process did not stop'
    Write-Host 'PASS: native Task Scheduler register/query/start/fresh heartbeat/stop'
} finally {
    # Never leave a smoke task behind, even when the token/lifecycle is unavailable.
    try {
        if ($taskOwned) {
            $task = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
            if ($task) {
                try {
                    Stop-ScheduledTask -TaskName $taskName
                } finally {
                    Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
                    if (Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue) { throw 'Smoke task cleanup failed' }
                    Write-Host 'PASS: temporary task unregistered'
                }
            }
        }
    } finally {
        if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Recurse -Force }
    }
}
