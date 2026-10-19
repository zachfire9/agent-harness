# Windows runtime (Step 21)

**Want to talk to the agent first? Start with [Your first Windows conversation](https://github.com/zachfire9/agent-harness/blob/main/docs/windows-first-conversation.md)** for a verified binary download, session-only API key, chat/ask, and sample-file tools. That path needs no `init`, daemon, or Task Scheduler. This guide covers optional background operation. Interactive `chat`/`ask` read process environment and the current directory's `.env`, not runtime `config.yaml` or the daemon's `config\secrets\env`. [Back to README](https://github.com/zachfire9/agent-harness/blob/main/README.md).

Windows amd64 uses **Task Scheduler, current user, Interactive token, Limited privileges**. No administrator prompt, saved password, Windows Service, or machine-wide install is required. The task starts at that user's logon, not boot. It cannot run before login or after logout; locking the session is not logout. Linux `systemd --user` remains unchanged. macOS is a separate Step 22.

**Availability/evidence:** this branch adds Windows support; v0.1.5 includes Step 20 rich Gmail bodies, not this adapter. Do not assume a Windows asset exists for an older release. Linux tests and Windows crosscompilation are not native Windows verification. Native Windows tests/vet, CLI/ACL smoke, and register/query/start/fresh heartbeat/stop/unregister lifecycle passed in [CI run 39276862161](https://github.com/zachfire9/agent-harness/actions/runs/39276862161). This is isolated CI evidence, not installation on your own machine or a published Windows release.

## Download, verify, and install

Use a published version that actually includes Windows support. In a normal, non-elevated PowerShell session, replace the placeholder below with that release's version (without `v`). Download to a new staging directory, not an instance home:

```powershell
$version = 'REPLACE_WITH_WINDOWS_RELEASE_VERSION'
$asset = "agent-harness_${version}_windows_amd64.zip"
$base = "https://github.com/zachfire9/agent-harness/releases/download/v$version"
$stage = Join-Path $env:TEMP ('agent-harness-download-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $stage | Out-Null
Invoke-WebRequest "$base/$asset" -OutFile (Join-Path $stage $asset)
Invoke-WebRequest "$base/checksums.txt" -OutFile (Join-Path $stage 'checksums.txt')
$lines = @(Get-Content (Join-Path $stage 'checksums.txt') | Where-Object { $_ -match ('^[0-9a-fA-F]{64}\s+\*?' + [regex]::Escape($asset) + '$') })
if ($lines.Count -ne 1) { throw 'Missing or ambiguous checksum entry' }
$expected = ($lines[0] -split '\s+')[0]
$actual = (Get-FileHash (Join-Path $stage $asset) -Algorithm SHA256).Hash
if ($actual -ne $expected) { throw 'Checksum mismatch; do not install' }
Expand-Archive (Join-Path $stage $asset) -DestinationPath (Join-Path $stage 'unpacked')
$bin = Join-Path $env:LOCALAPPDATA 'agent-harness\bin'
New-Item -ItemType Directory -Path $bin -Force | Out-Null
$exe = Join-Path $bin 'agent-harness.exe'
Copy-Item (Join-Path $stage 'unpacked\agent-harness.exe') $exe
& $exe version
```

The ZIP contains `agent-harness.exe`, `INSTALL.md`, and `WINDOWS.md`. Checksums detect corruption, not a compromised publisher; download both assets only from the trusted release. No Go toolchain is needed on the target. Keep the binary at this stable, current-user-owned location; the task stores its absolute path. Never run a registered task from a disposable extraction directory. Do not permit other users to write the binary, registration script, config, or instance tree.

## Initialize and protect the instance before adding credentials

```powershell
$instance = 'default'
$instanceHome = Join-Path $env:LOCALAPPDATA "agent-harness\instances\$instance"
& $exe init --instance $instance
```

Default data is under `%LOCALAPPDATA%\agent-harness\instances\<instance>\`, not roaming AppData or XDG paths. `--home <absolute-path>` overrides the whole instance home; use the same arguments for every command, including service generation. Instance names use ASCII letters/digits/underscore/hyphen, cannot start with hyphen, are at most 64 characters, and cannot be Windows device names. Config-relative paths use `/`, not backslashes, drive letters, UNC paths, or alternate data streams.

Windows mode bits such as `0600`/`0700` **do not establish private ACLs**. Before placing any API keys, Google client JSON, or OAuth tokens in the tree, protect the new instance's DACL. Run as its owning user, never against an unrelated directory:

```powershell
$sid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$acl = Get-Acl -LiteralPath $instanceHome
# Protected DACL, current user full control, inheritable by child files/directories.
$acl.SetSecurityDescriptorSddlForm('O:' + $sid + 'D:P(A;OICI;FA;;;' + $sid + ')')
Set-Acl -LiteralPath $instanceHome -AclObject $acl
Get-Acl -LiteralPath $instanceHome | Format-List Owner, AreAccessRulesProtected, Access
```

New children inherit this ACL. Existing children may retain explicit broad grants or protected DACLs: inspect `config\secrets` and **every secret file** with `Get-Acl`; repair each before adding credentials. Do not blindly recurse across a populated tree. For an existing credential file needing a private DACL, use the following before entering real values (replace the filename as needed):

```powershell
$envFile = Join-Path $instanceHome 'config\secrets\env'
# Create an empty file only if absent; never overwrite an existing credential file.
if (-not (Test-Path -LiteralPath $envFile)) { New-Item -ItemType File -Path $envFile | Out-Null }
$fileAcl = Get-Acl -LiteralPath $envFile
$fileAcl.SetSecurityDescriptorSddlForm('O:' + $sid + 'D:P(A;;FA;;;' + $sid + ')')
Set-Acl -LiteralPath $envFile -AclObject $fileAcl
Get-Acl -LiteralPath $envFile | Format-List Owner, AreAccessRulesProtected, Access
```

Use a trusted local editor to enter secrets; do not put them on command lines, in shell history, task arguments/XML, YAML, source control, status, or logs. Windows daemon startup optionally loads `config\secrets\env`, checks owner/allow ACLs, and fails closed if principals other than current user, SYSTEM, or Administrators are granted access. It rejects symlinks/non-regular files. SYSTEM/Administrators are trusted, not required grants. This validator is for this env file only: it does not prove the privacy of the whole tree or Google files, and cannot protect against administrators or compromise of your user account. Keep the parent tree private to prevent replacement races.

Env grammar is **literal** `NAME_API_KEY=value` per line; keys match `^[A-Z][A-Z0-9_]*_API_KEY$`. For example, `OPENAI_API_KEY=placeholder-only` (never a real key in docs). Empty lines and lines whose first non-whitespace character is `#` are ignored; UTF-8 with/without BOM and CRLF are accepted. Duplicate keys, empty values, `export`, PATH/proxy settings, NULs, files over 1 MiB, and overlong scanner lines are rejected. Do not surround values with shell quotes: quotes, dollar signs, spaces, `#` and `=` in the value remain literal; there is no expansion or escaping. Valid file values override inherited API keys. This automatic loader runs only for Windows **daemon** startup; interactive `jobs run` still uses its process environment. Delete the private file and revoke the provider key to remove credentials; removing a task does not revoke anything.

## Generate, review, register, and start

First test only the starter heartbeat in a new instance; configured jobs can have real delivery side effects.

```powershell
& $exe daemon --instance $instance --test
& $exe status --instance $instance --json
& $exe jobs list --instance $instance --json
& $exe service install --instance $instance
$registration = Join-Path $instanceHome 'config\register-task.ps1'
Get-Content -LiteralPath $registration
# After review, run as the same normal user:
& $registration
$taskName = "agent-harness-$instance"
Get-ScheduledTask -TaskName $taskName
Get-ScheduledTaskInfo -TaskName $taskName
Start-ScheduledTask -TaskName $taskName
& $exe status --instance $instance
```

`service install` **only writes** a UTF-8 BOM PowerShell registration script; it does not register/start a task. The generated task executes the binary directly, without shell interpretation, with the instance home as working directory, an at-logon trigger scoped to the current SID, `IgnoreNew` multiple-instance policy, no execution time limit, and three one-minute failure restart attempts. It may start/continue on battery. Do not separately run a foreground daemon for the same instance: `IgnoreNew` governs task starts, not arbitrary processes.

Registration intentionally does not use `-Force`: an existing same-name task fails rather than silently replacing it. Review/stop/unregister the old task explicitly to regenerate it. If local execution policy blocks the reviewed script, follow organizational policy; do not weaken machine-wide policy or run elevated as a workaround.

## Status, diagnostics, stop, update, removal

`status` reads a **persisted snapshot**, not a live health probe. Compare `last_heartbeat_at` with current time and query Task Scheduler state/LastTaskResult. A stale `running` snapshot may survive a forced stop, logout, crash, or reboot. Task Scheduler `Running` alone also does not prove healthy jobs. `jobs list --json` shows persisted job outcomes without dumping credentials/content.

There is no Windows journald equivalent wired here and no automatic daemon stdout/stderr capture or log rotation. The `logs` directory is reserved, not a promised task output log. Task Scheduler's Operational history (if enabled by your administrator) and `Get-ScheduledTaskInfo` provide launch/exit diagnostics, not full application logs. For application errors, stop the task first and run `& $exe daemon --instance $instance` in a normal PowerShell window; Ctrl+C requests graceful shutdown. Keep diagnostics private.

```powershell
Stop-ScheduledTask -TaskName $taskName
Get-ScheduledTask -TaskName $taskName
# Disable future logon starts while maintaining the definition:
Disable-ScheduledTask -TaskName $taskName
# Enable again when ready:
Enable-ScheduledTask -TaskName $taskName
# Removal (does not delete data or revoke secrets):
Stop-ScheduledTask -TaskName $taskName
Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
```

`Stop-ScheduledTask` can forcibly terminate the process; it is not a graceful application shutdown and does not promise a final status write. Check task/process exit before replacing the binary. For updates, stop the task, verify/download the new archive, retain a private backup of the previous binary, replace **the same stable executable**, run `version`, then `Start-ScheduledTask` and inspect fresh heartbeat/jobs. Roll back by stopping and restoring that backup. Re-register only if executable/home/definition changed. Never delete an instance home until the task is stopped/unregistered and you have deliberately backed up or revoked its credentials.

## Native verification

From a source checkout on an interactive Windows session with Go available, `./scripts/test-windows-runtime.ps1` builds into a uniquely named temporary directory, uses fake credentials and an explicitly broad then private ACL, exercises init/daemon test/status/jobs, runs the generated registration script, queries/starts/stops/unregisters a uniquely named task, and cleans up in `finally`. It requires a fresh running heartbeat and process/task evidence; existing instances/tasks are never reused.

CI runs unit tests/vet on Ubuntu and Windows and this native smoke on Windows. Release publishing depends on that reusable workflow. Hosted Windows runners may run in session 0 or lack a usable Interactive token: the smoke reports lifecycle unavailable as a **failure**, not a skipped/passing E2E. Registration/access/start failures also fail; they are not swallowed. If the hosted environment cannot provide the token, use an approved logged-in Windows runner for full lifecycle evidence before publishing. Successful crosscompilation, CLI/ACL-only output, or a workflow definition alone must not be reported as native task lifecycle verification.
