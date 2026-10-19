# Your first Windows conversation

[Project README](https://github.com/zachfire9/agent-harness#readme) · [Windows background runtime guide](https://github.com/zachfire9/agent-harness/blob/main/docs/windows-runtime.md)

Use a normal **non-administrator PowerShell 5.1 or 7** window on Windows amd64. You need internet access and an OpenAI-compatible API account with billing enabled. No Go, Git, source checkout, `init`, daemon, Task Scheduler, or installer is needed to chat.

**Availability:** this guide requires the first release containing **Step 22 (`setup`) or later**. No numbered supporting release is claimed yet; v0.1.5 does not contain it, and a Step-21-only Windows binary is insufficient. Implementation, Linux checks, and [native Windows onboarding verification](https://github.com/zachfire9/agent-harness/actions/runs/39298133159) passed on the Step 22 branch; publication is still pending. Stop if the release does not explicitly include Step 22.

## 1. Download, verify, and extract

1. Open [Releases](https://github.com/zachfire9/agent-harness/releases). Download `agent-harness_<version>_windows_amd64.zip` and `checksums.txt` from the **same** Step-22-capable release.
2. In PowerShell, run `Get-FileHash -Algorithm SHA256 -LiteralPath '<downloaded ZIP path>'`. Compare its entire hash with the line for that exact ZIP in `checksums.txt`. Stop on a missing/duplicate entry or mismatch. A matching checksum detects corruption, not a compromised publisher; download only from the trusted release. Do not bypass organizational security warnings or elevate to run an untrusted binary.
3. Right-click the ZIP → **Extract All** into a new folder. Open PowerShell in that extracted folder. The ZIP contains `agent-harness.exe`, `INSTALL.md`, `WINDOWS.md`, and `FIRST-CONVERSATION.md` (this guide).
4. Run:

```powershell
.\agent-harness.exe version
.\agent-harness.exe setup
```

Do not paste PowerShell commands at the later `You:` chat prompt. Setup needs a real terminal; redirected/piped input fails instead of waiting for secrets.

## 2. Answer the wizard

Press Enter to accept a displayed default. Enter `cancel` to stop (including at the hidden-key prompt). EOF/Ctrl+C also stops setup. Before the final review is accepted, no settings or credential are saved; after applying, cancellation does **not** undo saved changes.

- **Endpoint:** default `https://api.openai.com/v1`. Other OpenAI-compatible chat-completions providers work with their own URL/key/model. HTTPS is required; HTTP is allowed only for loopback development. Do not put a key in the endpoint URL, model name, or workspace path.
- **Model:** default `gpt-4.1-mini`; confirm availability/tool support and pricing with your provider. A default is not a promise of account access or the newest model.
- **Workspace:** default is `AgentHarnessWorkspace` under your user folder, **not the whole user folder**. Choose an absolute path to a dedicated empty/sample folder. Setup creates it only after review. File tools will access this folder and its allowed descendants. Never choose a folder full of private documents.
- **Credential:** choose `store` to enter a hidden key and save it in **Windows Credential Manager** for later launches. Choose `session` for a key kept only in this setup process, usable if you start chat at the end. It is not exported to PowerShell or saved for tomorrow. There is no plaintext fallback if Credential Manager fails. On rerun, `keep` retains the existing credential (default when one exists); `store` explicitly replaces it; `remove` deletes it. Choosing `session` also removes an old saved key after review.
- **Diagnostic logs:** first-run default **no**. Keep this off for the beginner path. Logs may contain conversation/tool data; they are not resumed conversation memory. Rerun preserves the previous preference when you press Enter.
- **Stable copy:** choose **yes** if using a temporary extraction folder. The executable goes beside settings under `interactive\bin\agent-harness.exe`. A different existing executable is a conflict, never silently replaced; move/back up the old file explicitly before upgrading.
- **User PATH:** optional, default no. Requires the stable executable. Only the current user's PATH is changed; existing entries are skipped and conflicting user-PATH installations are rejected. New terminals see the change. Machine PATH is untouched and elevation is not required. Review other installations yourself if command resolution is unexpected.
- **Shortcut:** optional, default no. Requires the stable executable. Creates **Chat with Agent Harness.lnk** beside settings. It opens the console app with an absolute executable/settings path and the chosen working directory; no key appears in its arguments. Open it from that folder or pin it yourself.
- **Review:** check the exact endpoint, model, workspace, credential action, log preference, and integration paths. Only answer **yes** to Apply if they are correct.
- **Connection test:** optional and separate from offline configuration validation. Answering yes sends a small request (no workspace file contents) and **may incur API charges**. A successful offline check does not validate billing, key, model access, or network availability. A failed connection check leaves settings saved so you can rerun setup.
- **Start chat:** choose yes. With session-only storage, this is how to use that key before it is discarded. Choose no to finish without a model request.

**Billing/privacy:** API billing is separate from ChatGPT subscriptions. Set provider usage controls before real calls; budget alerts need not be hard caps. Prompts and tool/file contents are sent to your selected provider, including summaries when needed. Tool loops can make multiple billable requests. Hidden input avoids shell-history exposure, but keys still exist in process memory; Credential Manager does not defend against compromise of your Windows account/admins. Clear the clipboard after pasting and revoke exposed keys in the provider dashboard.

## 3. Try a conversation and sample file tools

At `You:`, enter only the text after the label:

```text
You: In two sentences, what can you help me do with a sample workspace?
Agent: ...
You: Explain that more simply.
```

Answers vary. This chat keeps history in memory until exit, not across launches. Responses appear when the turn completes rather than streaming token by token.

To try files, first exit with `/exit`. Create a non-sensitive `notes.txt` in the selected workspace using Notepad, containing:

```text
Project: Weekend garden
TODO: Buy basil seeds
TODO: Water the seedlings
```

Restart chat using the stable executable or shortcut. Then ask:

```text
Use list_files to list this workspace. Do not run commands.
Use read_file to read notes.txt and tell me the project name.
Use search_files to search for TODO and list the two tasks.
```

The built-ins are `echo`, `list_files`, `read_file`, `search_files`, and restricted `run_command`. No general file-edit, browser, or desktop-control tool is provided. File tools constrain paths, block selected sensitive names (including `.env`), and cap output, but are not perfect secret filtering or full OS isolation. Use only safe sample data.

**There is no human approval dialog for command calls.** Model-supplied `confirm:true` is not human approval. The enforced exact-command allowlist is `pwd`, `git status`, `git status --short`, `git diff`, and `go test ./...`, with no shell and a timeout. These need external programs; PowerShell's `pwd` alias is not a `pwd` executable. `go test` executes repository code. Skip command examples on the beginner path.

## 4. Return later or choose another project

With Credential Manager storage, no environment block or key re-entry is needed. Open the shortcut or run:

```powershell
& "$env:APPDATA\agent-harness\interactive\bin\agent-harness.exe" config check
& "$env:APPDATA\agent-harness\interactive\bin\agent-harness.exe" chat
```

These paths assume the wizard's default settings location and stable-copy choice. Without copying, run the executable from its retained extraction folder instead. With opt-in PATH, use `agent-harness chat` in a new terminal. `ask 'your question'` uses the same saved settings but starts a separate one-shot request.

The CLI announces the selected workspace. **By default saved settings select the saved folder even when launched from another current directory.** To intentionally use your current project instead:

```powershell
agent-harness chat --workspace current
```

Or use `--workspace saved` / `--workspace '<absolute directory>'`. The selected directory is both the file-tool sandbox and working directory. Manual `tool` debug commands still use the current directory and do not restore the saved workspace. `--home`/`--instance` are background-runtime options, not chat workspace selectors.

`chat`, `ask`, and `config check` resolve **nonempty process environment > selected workspace `.env` > saved settings > built-in defaults**. For the key: **process environment > selected workspace `.env` > Credential Manager**. An explicit key skips Credential Manager reads. Existing environment/`.env` log or provider overrides still win; switching a provider does not automatically select a different key. Inspect safe `config check` output, never dump secrets. `--settings '<absolute JSON path>'` selects a separate interactive profile with its own credential reference. Linux keeps its environment/current-directory behavior unless an explicit settings path is supplied. No background YAML/private-env config is changed.

Exit with `/exit`, `exit`, `quit`, or EOF. A new launch is a **new conversation**; persistent JSONL logs are diagnostics, not automatically restored memory.

## Rerun, remove, or roll back

Run `setup` again to review current defaults. Credential replacement/removal and optional integrations require another Apply confirmation. Answering no to optional integration prompts leaves already-installed integrations intact; use removal to undo them. The shortcut is regenerated only if unchanged/owned. An externally modified shortcut is retained for manual review. Settings/credential/integration writes are not a single transaction: a disk or integration error after Apply may leave some changes applied; rerun to inspect/retry, or remove them explicitly. Workspace creation may already have occurred. Nothing deletes workspace files.

```powershell
agent-harness setup --remove
```

This asks for confirmation, deletes this profile's saved credential, settings, unchanged owned shortcut, and owned user PATH entry. Use the retained executable's full path if PATH was never enabled. For custom profiles, supply the same `--settings` path. Removal retains workspace files and the copied executable; close sessions and delete that executable/bin folder yourself if no longer needed. Do not delete another runtime installation. Deleting a credential does **not** revoke the provider key: revoke it in the provider dashboard separately. Clear any process-environment keys and close processes that inherited them. If ownership metadata is damaged or a shortcut was modified, removal fails closed for manual inspection; remove only the displayed profile's integrations yourself.

For rollback, retain a backup of the old executable and non-secret settings before an explicit upgrade; restore them manually with sessions closed. Credential replacement cannot recover a previous key—re-enter it or rotate it at the provider. Do not delete a workspace as an uninstall step.

## Troubleshooting and advanced manual fallback

- **Unknown `setup`:** this binary predates Step 22. Check `version` and the release notes; do not assume any Windows ZIP includes the wizard.
- **No interactive terminal / hidden input unavailable:** run directly in PowerShell, not through a pipeline. Do not put the key in arguments to bypass this.
- **Missing saved credential:** rerun setup with `store`, or use session-only entry and start chat during setup. Never paste keys into support messages.
- **Config works but 401/403/404/429:** inspect key/model permissions, endpoint, billing/quota and rate limits at the provider. Stop repeated billable retries until resolved. Network errors require normal TLS/network troubleshooting, not disabling verification.
- **Wrong files:** read the workspace announcement; use `--workspace current` deliberately, or rerun setup. A missing saved folder fails clearly rather than silently switching directories.
- **Unexpected provider/logs:** nonempty inherited environment and the selected workspace's `.env` override saved settings. Inspect only redacted `config check` output.
- **Chat turn failed:** use `/exit` before running PowerShell commands. Ctrl+C can stop a stuck process.

Advanced/manual setup remains available without saving a credential. In a deliberately selected sample workspace, enter a process-only key with hidden input, then run the retained binary:

```powershell
$secureKey = Read-Host 'API key (hidden)' -AsSecureString
try { $env:OPENAI_API_KEY=[System.Net.NetworkCredential]::new('', $secureKey).Password }
finally { $secureKey.Dispose(); Remove-Variable secureKey }
$env:OPENAI_BASE_URL = 'https://api.openai.com/v1'
$env:OPENAI_MODEL = 'gpt-4.1-mini'
$env:AGENT_RUN_LOGS_ENABLED = 'false'
.\agent-harness.exe config check --workspace current
.\agent-harness.exe chat --workspace current
Remove-Item Env:OPENAI_API_KEY -ErrorAction SilentlyContinue
```

Adjust the executable path if it is not in that folder. This key is inherited by child processes, not saved in Credential Manager, and is not removed from already-running children by clearing PowerShell's environment. `.env` is also an advanced supported override, but is plaintext: protect its real Windows ACL and parent directory before entering credentials, never commit it, and remember a downloaded workspace has no repository `.gitignore`. Chat's `.env` loader does not perform the background daemon's ACL validation. Prefer Credential Manager for reusable keys.

Background scheduling is separate and optional: see the [Windows runtime guide](https://github.com/zachfire9/agent-harness/blob/main/docs/windows-runtime.md).
