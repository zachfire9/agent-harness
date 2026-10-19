# Your first Windows conversation

[README](../README.md) · [Windows background runtime guide](windows-runtime.md)

Start here to talk to the agent in a normal, **non-administrator PowerShell 5.1 or 7** window. You need Windows amd64, internet access, and an OpenAI API account with billing enabled. You do **not** need Go, Git, GitHub CLI, a source checkout, `init`, a daemon, or a service. Background scheduling is a separate, optional path in the [Windows runtime guide](windows-runtime.md).

This walkthrough uses a new sample workspace and a session-only API key; it writes no key to disk. Only paste PowerShell blocks into PowerShell. Text under `You:` belongs inside the agent's chat prompt.

## 1. Download, verify, and install the binary

Open the project's [Releases page](https://github.com/zachfire9/agent-harness/releases) in your browser. Choose a published release whose assets include both `agent-harness_<version>_windows_amd64.zip` and `checksums.txt`. **Replace the version placeholder below** with its version without the leading `v`. This is not a claim that a new Windows release has already been published: v0.1.5 has no Windows asset. If no release has the ZIP yet, stop here and wait for one.

Run this whole block. It downloads into a unique temporary folder, verifies exactly the selected ZIP's SHA256 entry, and installs to a stable per-user path:

```powershell
$ErrorActionPreference = 'Stop'
$version = 'REPLACE_WITH_WINDOWS_RELEASE_VERSION'
if ($version -eq 'REPLACE_WITH_WINDOWS_RELEASE_VERSION') { throw 'Choose a published Windows release first' }
# TLS 1.2 also supports downloads from Windows PowerShell 5.1.
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$asset = "agent-harness_${version}_windows_amd64.zip"
$base = "https://github.com/zachfire9/agent-harness/releases/download/v$version"
$stage = Join-Path $env:TEMP ('agent-harness-download-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $stage | Out-Null
Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile (Join-Path $stage $asset)
Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile (Join-Path $stage 'checksums.txt')
$lines = @(Get-Content -LiteralPath (Join-Path $stage 'checksums.txt') | Where-Object { $_ -match ('^[0-9a-fA-F]{64}\s+\*?' + [regex]::Escape($asset) + '$') })
if ($lines.Count -ne 1) { throw 'Missing or ambiguous checksum entry; do not install' }
$expected = ($lines[0] -split '\s+')[0]
$actual = (Get-FileHash -LiteralPath (Join-Path $stage $asset) -Algorithm SHA256).Hash
if ($actual -ne $expected) { throw 'Checksum mismatch; do not install' }
Write-Output 'Checksum verified'
Expand-Archive -LiteralPath (Join-Path $stage $asset) -DestinationPath (Join-Path $stage 'unpacked')
$bin = Join-Path $env:LOCALAPPDATA 'agent-harness\bin'
New-Item -ItemType Directory -Path $bin -Force | Out-Null
$exe = Join-Path $bin 'agent-harness.exe'
# Do not silently replace an existing install used by another session/task.
if (Test-Path -LiteralPath $exe) { throw 'Binary already installed; use it or follow the runtime update guide' }
Copy-Item -LiteralPath (Join-Path $stage 'unpacked\agent-harness.exe') -Destination $exe
& $exe version
if ($LASTEXITCODE -ne 0) { throw 'Binary version check failed' }
```

Expected: `Checksum verified`, then build/version metadata matching your selected release. Keep using `& $exe` (PowerShell's call operator); no PATH change is required. The ZIP also contains `INSTALL.md` and `WINDOWS.md`.

If a download returns 404, check the release version and asset name; do not substitute a Linux archive. If verification fails, stop and download both assets again from the trusted release. A matching checksum detects corruption, **not a compromised publisher**. Do not bypass Windows/organizational security warnings or run as administrator to make an untrusted binary work. For an existing install, set `$exe = Join-Path $env:LOCALAPPDATA 'agent-harness\bin\agent-harness.exe'` and check `& $exe version`; see the [runtime guide](windows-runtime.md#status-diagnostics-stop-update-removal) for updates.

## 2. Create a safe workspace

The **current working directory is the workspace**, not the executable's directory or a runtime instance home. Do not start from your user profile, Downloads, or a folder full of private documents. This block deliberately refuses to overwrite an existing sample folder:

```powershell
$workspace = Join-Path $env:LOCALAPPDATA 'agent-harness\workspaces\first-conversation'
if (Test-Path -LiteralPath $workspace) { throw 'Sample workspace already exists; use step 7 to return, or choose a new empty folder' }
New-Item -ItemType Directory -Path $workspace -Force | Out-Null
Set-Location -LiteralPath $workspace
@(
    'Project: Weekend garden'
    'TODO: Buy basil seeds'
    'TODO: Water the seedlings'
) | Set-Content -LiteralPath '.\notes.txt' -Encoding ASCII
Get-Location
Get-Content -LiteralPath '.\notes.txt'
```

Expected: a path ending in `agent-harness\workspaces\first-conversation` and the three sample lines. You created this file with PowerShell; the agent does not have a general-purpose file-write tool.

## 3. Add a session-only API key and check configuration

Create a key in the [OpenAI Platform](https://platform.openai.com/api-keys). **ChatGPT subscriptions do not pay for API requests.** Enable API billing and review pricing, model access, and usage/budget alerts in your account before continuing; a budget alert is not necessarily a hard spending cap. Use a dedicated project/key with only the access needed. Tool loops and conversation summaries can make multiple billable model calls per turn.

Paste the key only into the hidden prompt below, not into a command, chat message, screenshot, or document. This sets the process environment for this PowerShell window and its child processes, not a persistent user/machine variable:

```powershell
$secureKey = Read-Host 'Paste your OpenAI API key (hidden)' -AsSecureString
try {
    $env:OPENAI_API_KEY = [System.Net.NetworkCredential]::new('', $secureKey).Password
} finally {
    $secureKey.Dispose()
    Remove-Variable secureKey
}
$env:OPENAI_BASE_URL = 'https://api.openai.com/v1'
$env:OPENAI_MODEL = 'gpt-4.1-mini'
$env:AGENT_SUMMARY_MODEL = 'gpt-4.1-mini'
$env:AGENT_RUN_LOGS_ENABLED = 'false'
$env:VECTOR_STORE_PROVIDER = 'none'
& $exe config check
if ($LASTEXITCODE -ne 0) { throw 'Fix the config error before starting chat' }
```

Expected output starts with `config ok`. The next line includes `OPENAI_API_KEY=***`, `OPENAI_BASE_URL=https://api.openai.com/v1`, `OPENAI_MODEL=gpt-4.1-mini`, and `AGENT_RUN_LOGS_ENABLED=false`. The model names above are the code's current defaults, not a promise of the newest model or of access on your account. This check is offline: it validates local configuration, **not** the key, billing, network, or model availability.

For `chat`, `ask`, and `config check`, configuration comes from **nonempty process environment values first**, then a `.env` in the current working directory, then optional built-in defaults. They do **not** read the background runtime's `config.yaml` or `config\secrets\env`. Do not run `init` to fix chat configuration. `--home`/`--instance` are not chat workspace selectors.

The hidden prompt avoids putting the key in shell command history, but the application still needs plaintext in process memory/environment. This is not a secret vault and does not protect against compromise of your Windows account or administrators. Clear your clipboard after pasting a key; revoke it in the provider dashboard if exposed.

## 4. Have a first conversation

**Privacy before you send:** prompts and any file contents returned by tools are sent to the configured external model API (including summarization when needed). Use only the sample file. File path checks, blocked names, and log redaction are not perfect secret filtering or a complete security sandbox. Never put credentials or sensitive documents into this workspace. Logging is disabled above because default run logs can contain prompts, file contents, and answers; disabling logs does not prevent API transmission or terminal output.

```powershell
& $exe chat
```

Expected immediately: `You: `, with no welcome banner. Type a single line and press Enter:

```text
You: In two sentences, what can you help me do with this sample workspace?
Agent: ...
You: Explain that more simply.
Agent: ...
```

Type only the text after `You:`; the program prints the label itself. Answers vary, and the response is printed when the turn finishes rather than streamed token by token. The second message can refer to the first because this process retains conversation history.

If you see `chat turn failed: ...`, the chat may return to `You:` instead of exiting. Use `/exit` before running PowerShell commands to fix the issue; troubleshooting is below.

## 5. Ask it to use local file tools

At the same `You:` prompt, try these one at a time:

```text
Use list_files to list this workspace. Do not run commands.
Use read_file to read notes.txt, then tell me the project name.
Use search_files to search for TODO in this workspace and list the two tasks.
```

Expected: the file listing includes `notes.txt`; the project is `Weekend garden`; the tasks are buying basil seeds and watering the seedlings. These are model-driven requests, so wording and tool choices can vary. If it guesses without reading, explicitly ask again to use the named tool. Optional diagnostics: leave chat with `/exit`, then run `& $exe chat --trace`; trace reports tool names and sizes without intentionally dumping full contents, while the normal answer can still quote your file.

The built-ins are `echo`, `list_files`, `read_file`, `search_files`, and `run_command`. File tools constrain paths to the workspace, block selected sensitive names such as `.env`, and cap long output. Those safeguards do not classify all private content. There is no general file-edit, browser, or desktop-control tool in this CLI.

`run_command` accepts only the exact allowlist: `pwd`, `git status`, `git status --short`, `git diff`, and `go test ./...`. It executes programs **without a shell**, so PowerShell's `pwd` alias does not supply a `pwd` executable on Windows. Git/Go commands need those programs installed; Git also needs a repository, which this new sample workspace is not. Skip command examples for this first conversation. Later, only in an intentionally chosen non-sensitive Git repository with Git installed, you could ask it to run `git status --short`.

**There is no human approval dialog for command calls.** `confirm:true` is a field supplied by the model in its tool arguments, not proof that you reviewed/approved a command. The allowlist and timeout are the actual enforced command restrictions; this is not arbitrary shell access or OS isolation (for example, `go test` can execute repository code).

## 6. Exit, try one-shot ask, and remove the key

At `You:`, type:

```text
/exit
```

Expected: `Goodbye.` and your PowerShell prompt. `exit` and `quit` also work. If a network call hangs, Ctrl+C can terminate the process; restart when ready.

For a separate single question, run this in PowerShell (it makes a billable API call):

```powershell
& $exe ask 'Use read_file to read notes.txt and summarize its two TODO items.'
```

Expected: a summary of the two sample tasks, then return to PowerShell, with no `You:` prompt. `ask` does not share the previous chat's history.

When finished:

```powershell
Remove-Item Env:OPENAI_API_KEY -ErrorAction SilentlyContinue
```

This removes the key from this PowerShell process, not from already-running child processes or the provider account. Close any other processes to which you passed it. Revoke/delete the key in the provider dashboard when no longer needed. No key file was created by this walkthrough.

## 7. Come back tomorrow (fresh PowerShell window)

Do not repeat the download or sample-file creation. Restore the executable/workspace paths and enter the key again:

```powershell
$exe = Join-Path $env:LOCALAPPDATA 'agent-harness\bin\agent-harness.exe'
$workspace = Join-Path $env:LOCALAPPDATA 'agent-harness\workspaces\first-conversation'
if (-not (Test-Path -LiteralPath $exe)) { throw 'Installed binary not found; check step 1' }
if (-not (Test-Path -LiteralPath $workspace)) { throw 'Workspace not found; check step 2' }
Set-Location -LiteralPath $workspace
$secureKey = Read-Host 'Paste your OpenAI API key (hidden)' -AsSecureString
try {
    $env:OPENAI_API_KEY = [System.Net.NetworkCredential]::new('', $secureKey).Password
} finally {
    $secureKey.Dispose()
    Remove-Variable secureKey
}
$env:OPENAI_BASE_URL = 'https://api.openai.com/v1'
$env:OPENAI_MODEL = 'gpt-4.1-mini'
$env:AGENT_SUMMARY_MODEL = 'gpt-4.1-mini'
$env:AGENT_RUN_LOGS_ENABLED = 'false'
$env:VECTOR_STORE_PROVIDER = 'none'
& $exe config check
if ($LASTEXITCODE -ne 0) { throw 'Fix the config error before starting chat' }
& $exe chat
```

Expected: `config ok`, a redacted config summary, then `You: `. Ask it to read `notes.txt` again. **This is a new conversation, not a resumed session.** Files remain on disk, but chat history/summaries live only in memory. Even if you later enable JSONL logs, those logs are diagnostics, not automatically reloaded conversational memory. After `/exit`, remove `Env:OPENAI_API_KEY` again as in step 6.

## Troubleshooting

- **`& $exe` fails / executable not found:** restore `$exe` from step 7 and use `Test-Path -LiteralPath $exe`. Do not paste PowerShell commands at `You:`. Check that you downloaded/extracted the Windows amd64 ZIP.
- **`config error: OPENAI_API_KEY is required`:** repeat the hidden-key prompt in this same PowerShell window. A key set in another window or daemon secret file does not configure this process.
- **`config ok`, then status 401/403:** check the key and project/model permissions in the provider dashboard. Do not print the environment or paste the key into support messages.
- **Status 429 / quota or billing error:** check API billing, credits, usage limits, and rate limits. Stop repeated retries until resolved; a ChatGPT subscription is separate.
- **Model not found / status 404:** check `OPENAI_BASE_URL` in `config check` and confirm model availability with the provider. If selecting another supported tool-capable chat-completions model, set both `OPENAI_MODEL` and `AGENT_SUMMARY_MODEL` explicitly.
- **Network/TLS error or long wait:** check connectivity, firewall, and organizational proxy policy. Do not disable TLS certificate checks. Ctrl+C can stop a stuck process.
- **File not found / wrong folder:** leave chat, run `Get-Location` and `Get-ChildItem`, then `Set-Location -LiteralPath $workspace` and restart. An outside-workspace path is rejected; copy only a non-sensitive sample into the workspace instead.
- **Unexpected settings:** inspect the safe `config check` output. Inherited environment values or a local `.env` can affect settings not explicitly set above. Never dump all environment variables or the contents of a secret file into diagnostics.
- **Truncated output / context warning:** use small files and focused questions. Long conversations may be summarized; this does not preserve every detail or add durable memory.

## Optional later: persistent credentials (not needed here)

Session-only entry is the recommended first path. If you later choose `.env` persistence, remember that interactive chat reads it **only from its current working directory**, not an arbitrary external key-file path. Protect the parent folder and empty `.env` with a current-user private Windows ACL **before** entering credentials in a trusted editor; verify the actual file's ACL after saving. See the [runtime guide's ACL examples](windows-runtime.md#initialize-and-protect-the-instance-before-adding-credentials) for the principles, but apply them only to your deliberately chosen workspace/file: its daemon `config\secrets\env` is a different loader, not a chat key store. Chat's `.env` loader does not perform the daemon's ACL validation. Windows mode bits alone are not private ACLs.

A downloaded-binary workspace has **no repository `.gitignore` by default**. If you later initialize Git, add `.env` to `.gitignore` before staging anything; an ignore rule does not protect already-tracked files, backups, cloud sync, other programs, or other users. File-tool blocking of `.env` is not encryption. To remove a persistent key, delete its local file, clear inherited process variables/close sessions, and revoke the provider key; deleting a file alone does not revoke it.

For optional scheduled jobs and background startup after you are comfortable with interactive use, continue to the [Windows runtime guide](windows-runtime.md). That separate setup is not required to chat.
