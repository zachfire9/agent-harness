# Configure a daily brief email

This walkthrough assembles a generic `scheduled_notification`: resolve date/countdown facts, select rotating source items, render one configured LLM prompt, send through Gmail, then advance each successful rotation. “Daily brief” is an example, not a built-in job type.

## 1. Prerequisites and version

- Linux with a working `systemd --user` session for unattended operation. Foreground inspection/testing does not require systemd.
- **v0.1.4 or a later release containing Step 19 / PR [#41](https://github.com/zachfire9/agent-harness/pull/41)**. The complete example uses `current_date`, `local_file`, and `heading_pattern`; **v0.1.3 does not support the full example**. Optional multipart Gmail bodies are available in **v0.1.5** (Step 20), not v0.1.4.
- A separately billed model-provider API key and an allowed model. The example uses OpenAI; see [provider configuration](background-runtime.md#ai-email-jobs) for alternatives.
- A Google account with Gmail, a Google Cloud project with Gmail API and Google Docs API enabled, an OAuth consent configuration, and a **Desktop app** OAuth client. For an external app in Testing, add the signing-in account as a test user. Google testing-mode grants may expire and require reauthorization.
- One Google Doc readable by that account. Never commit its real ID or your instance configuration.

For versioned installs, follow [release installation](install-release.md). Ensure `~/.local/bin` is on `PATH`, then check:

```bash
agent-harness version
```

These are Bash/Linux commands. Run all setup as your normal user, not root. Use a **new, unused instance name** (`brief-example` below) to avoid replacing an existing config or sending duplicate mail. Do not start its daemon yet.

## 2. Initialize the instance and sources

```bash
export BRIEF_HOME="$HOME/.local/share/agent-harness/instances/brief-example"
agent-harness init --instance brief-example --home "$BRIEF_HOME"
mkdir -p "$BRIEF_HOME/data"
chmod 700 "$BRIEF_HOME/config/secrets"
```

The explicit `--home` intentionally matches the fixed location used by the generated systemd template, even when your shell sets `XDG_DATA_HOME`. Keep using it for all CLI commands below.

In your editor, create `$BRIEF_HOME/data/reference-cards.txt` with this synthetic content:

```text
Example reference cards

1 - Cedar
A fictional reference card about a tree.
- First practice fact.
- Second practice fact.

2 - Maple
Another fictional card.
- A different practice fact.
```

Create/share a Google Doc with the signing-in Google account. Put one vocabulary item on each non-empty line, **without a document-title paragraph**:

```text
resilient
curious
resourceful
```

Copy its ID from `https://docs.google.com/document/d/DOC_ID/edit`. The private config maps this ID to a friendly alias; the job uses the alias, not the URL.

## 3. Write the complete configuration

Replace the *new instance's* `$BRIEF_HOME/config/config.yaml` with the following. Replace fake email addresses and `REPLACE_WITH_DOC_ID` locally; choose a model available to your provider account. Keep the indentation, list layout, and quoted prompt on one physical line: the runtime reads a limited YAML subset, not every valid YAML form. Do not use folded/multiline YAML prompts.

```yaml
# daily-brief-example
# Private instance configuration: do not commit real values.
google:
  client_credentials_path: "config/secrets/google-client.json"
  token_path: "config/secrets/google-token.json"
  account_hint: "agent@example.com"
  scopes:
    - gmail_send
    - docs_readonly
llms:
  default:
    provider: openai
    model: "gpt-4o-mini"
  providers:
    - provider: openai
      api_key_env: OPENAI_API_KEY
      models:
        - "gpt-4o-mini"
notifier:
  type: gmail
  gmail:
    from: "agent@example.com"
    to:
      - "recipient@example.com"
    subject_prefix: "[Morning note]"
sources:
  google_docs:
    vocabulary: "REPLACE_WITH_DOC_ID"
jobs:
  - name: morning-note
    type: scheduled_notification
    enabled: true
    schedule:
      daily_at: "08:00"
      timezone: "America/New_York"
    context:
      - id: today_date
        type: current_date
        timezone: "America/New_York"
        format: "Monday, January 2, 2006"
      - id: countdown
        type: days_until_date
        date: "2030-01-01"
        label: "the example milestone"
        output_format: "{days} days until {label}"
      - id: word
        type: rotating_source_item
        source:
          type: google_doc
          alias: vocabulary
          parser: non_empty_lines
        selection:
          strategy: round_robin
          progress_key: morning-word
      - id: card
        type: rotating_source_item
        source:
          type: local_file
          path: "data/reference-cards.txt"
          parser: heading_pattern
          heading_pattern: '^[0-9]+\s*-\s*\S+'
        selection:
          strategy: round_robin
          progress_key: morning-card
    render:
      type: llm_template
      prompt: "Write a concise plain-text email with these sections in order: Date, Countdown, Vocabulary, Reference card. The authoritative date is {{ today_date }}; use it rather than inferring today's date. In Date, show that date and translate it into Spanish and French, spelling out day numbers naturally, one language per line. In Countdown, copy {{ countdown }}. In Vocabulary, use {{ word }} and add a short definition and example sentence. In Reference card, preserve the title and facts from {{ card }} without inventing facts. Use blank lines between sections and short readable lines. No Markdown headings, asterisks, HTML, or tables. Return only the email body."
      max_chars: 2400
```

### What to customize

- **Delivery:** `from` must identify the authenticated Gmail account or a permitted send-as identity. `account_hint` is metadata, not account selection or authentication. Check every `to` recipient before a send.
- **Schedule:** `daily_at` is 24-hour local time in `schedule.timezone`; DST follows that timezone. Set the separate date-context timezone too if you want both to use the same local day. Do not combine `schedule` and `interval_seconds`.
- **Countdown:** choose date/label in config. The current `days_until_date` calculation uses **UTC calendar days**, independently of the date context/schedule timezone; it can differ near local midnight. Do not ask the model to recalculate it.
- **Languages and order:** change `render.prompt`, not Go code. Date facts are deterministic; model translations/wording may vary or be wrong. The job makes one model request for the whole body, not one request per section.
- **Sources:** `non_empty_lines` means one non-empty line per item; `heading_with_bullets` handles a title followed by actual textual bullet lines; `heading_pattern` handles recognizable headings followed by plain lines, bullets, or both. See [parser details](background-runtime.md#pattern-delimited-source-cards).
- **Paths:** local source paths are relative to the instance home, not the shell's working directory. Do not use absolute paths or `../`.
- **Rotation:** assign a unique stable `progress_key` to every independently rotating section. Each wraps at its own end; restart does not reset it. The active item-ID cycle snapshot refreshes at cycle boundaries. Do not edit/delete live progress files for ordinary formatting tests.
- **Templates:** use exact `{{ context_id }}` placeholders, matching each context `id`. Single braces are not prompt substitutions. Review output for accidentally unexpanded placeholders.
- **Size/format:** `max_chars` is an output cap, not a guarantee of complete sections; allow enough room. This complete example uses default plain Gmail output and works with v0.1.4. Markdown markers remain literal in plain mode.

### Optional rich Gmail bodies (Step 20, v0.1.5)

Use v0.1.5 or later for rich bodies. In an isolated instance, add `body_format: multipart` at the same indentation as `subject_prefix` under `notifier.gmail` (after the `to` list). Omit it or use `plain` for legacy behavior. Only `plain` and `multipart` are accepted, not `html`: rich messages deliberately include a plain fallback rather than HTML-only mail. Live changes/delivery still require separate approval.

Keep the existing single-line `render.prompt` and content contexts, including `today_date`. If desired, ask that same model request to use `# ` / `## ` / `### ` headings, blank-line-separated paragraphs, and flat `- ` / `* ` bullets. The app converts this small subset into HTML and escapes all content; ordinary unmarked text works too. Bold, links, images, and raw HTML are not interpreted. No additional model call or language-specific renderer is involved. The plain part retains the textual markers; only line endings are normalized for MIME. See the [precise renderer/MIME contract](background-runtime.md#gmail-body-formats-step-20-v015).

For branch verification, use the fake-model/loopback Gmail test `go test ./internal/runtime -run TestGmailMultipartDeliveryProgressAndSafeState -count=1` from the checkout, not a live manual `jobs run`. It uses temporary state and verifies failure/retry without sending mail or consuming real rotation items. Release/install remains a separate post-merge step; a later real-mail check requires separate approval and an isolated instance.

## 4. Add credentials without putting them in YAML

### Model key: service and manual CLI are different environments

Create `$BRIEF_HOME/config/secrets/env` in a private editor containing `OPENAI_API_KEY=REPLACE_WITH_REAL_KEY`. Replace that placeholder locally; never paste the real key into a PR, chat, screenshot, command argument, or shell history.

```bash
chmod 600 "$BRIEF_HOME/config/secrets/env"
```

The generated systemd unit loads this file automatically. Manual `jobs run` does **not** automatically load it. Before a manual model call, export the key in that shell (this reads without echoing or putting the value into shell history):

```bash
read -rsp 'OpenAI API key: ' OPENAI_API_KEY; printf '\n'
export OPENAI_API_KEY
```

Do not print the environment or enable shell tracing. See [service environment details](background-runtime.md#systemd-user-service).

### Google OAuth: auth start is a handoff, not a login wizard

Download your Desktop OAuth client JSON from Google Cloud and save it privately as `$BRIEF_HOME/config/secrets/google-client.json`. Configure only Gmail send and Docs read-only permissions. The CLI command below prints the requested scopes and paths; **it does not open a browser, exchange an authorization code, or create a token**:

```bash
agent-harness google auth start --instance brief-example --home "$BRIEF_HOME"
```

Complete authorization using a trusted local OAuth flow with those client credentials and scopes. One option is Google's Python OAuth library on a trusted desktop with a browser (Python is an optional setup dependency, not a daemon dependency):

```bash
python3 -m venv "$BRIEF_HOME/work/oauth-venv"
"$BRIEF_HOME/work/oauth-venv/bin/python" -m pip install google-auth-oauthlib
```

Save the following as `$BRIEF_HOME/work/authorize-google.py`, then run it with that venv. It requests offline access, stores the token in the runtime's expected JSON shape, and does not print token values. Do not run with HTTP debug logging or paste authorization URLs/codes into tickets. If the daemon host is headless, perform this on a trusted desktop instead and securely transfer the client/token files into the matching secret paths on the host; the loopback browser flow below is not a remote/headless login solution.

```python
import json
import os
from datetime import timezone
from pathlib import Path
from google_auth_oauthlib.flow import InstalledAppFlow

home = Path(os.environ["BRIEF_HOME"])
secrets = home / "config" / "secrets"
os.umask(0o077)
scopes = [
    "https://www.googleapis.com/auth/gmail.send",
    "https://www.googleapis.com/auth/documents.readonly",
]
flow = InstalledAppFlow.from_client_secrets_file(
    str(secrets / "google-client.json"), scopes=scopes
)
credentials = flow.run_local_server(
    port=0, access_type="offline", prompt="consent", authorization_prompt_message=""
)
if not credentials.refresh_token or credentials.expiry is None:
    raise SystemExit("Offline credentials missing; repeat consent on the trusted desktop.")
expiry = credentials.expiry
if expiry.tzinfo is None:
    expiry = expiry.replace(tzinfo=timezone.utc)
target = secrets / "google-token.json"
# Exclusive creation prevents accidentally replacing an existing working grant.
with target.open("x", encoding="utf-8") as output:
    json.dump({
        "access_token": credentials.token,
        "refresh_token": credentials.refresh_token,
        "token_type": "Bearer",
        "expiry": expiry.astimezone(timezone.utc).isoformat().replace("+00:00", "Z"),
        "scope": " ".join(credentials.granted_scopes or scopes),
    }, output)
target.chmod(0o600)
```

```bash
"$BRIEF_HOME/work/oauth-venv/bin/python" "$BRIEF_HOME/work/authorize-google.py"
chmod 600 "$BRIEF_HOME/config/secrets/google-client.json" "$BRIEF_HOME/config/secrets/google-token.json"
agent-harness google auth status --instance brief-example --home "$BRIEF_HOME"
```

Confirm the expected account in the browser consent screen and required scopes in the status output. `connected` means a token file is present, not proof that every API action will succeed. Changing scopes in config does not grant them to an old token; reauthorization is required. Never check token contents with `cat` or paste them into logs. The app refreshes expired access tokens if the refresh token/client credentials remain valid. See [Google reference and revocation](background-runtime.md#configure-google-account-connection-metadata).

## 5. Validate without sending or consuming rotation items

```bash
agent-harness jobs list --instance brief-example --home "$BRIEF_HOME" --json
agent-harness google auth status --instance brief-example --home "$BRIEF_HOME"
agent-harness google docs read vocabulary --instance brief-example --home "$BRIEF_HOME"
```

`jobs list` checks configuration without invoking the model or sending mail, but does not prove external credentials or source readability. The explicit Docs read does contact Google (and may refresh a token); it prints source text, so use a private terminal. It does not advance notification cursors. Confirm the doc has the expected individual lines and the local source file exists.

There is **no notification render-only dry-run command**. `daemon --test` is not an email preview. Do not use `jobs run` against your production instance just to inspect formatting.

## 6. Optional controlled test: real email, isolated state

Every successful manual `jobs run` sends an actual email, uses billable model tokens, advances the selected rotation items in that instance, and updates job scheduling metadata. Keep the main example instance unstarted while testing. Use a separate private copy with the same config/routing and its own state:

```bash
export BRIEF_TEST_HOME="$(mktemp -d)"
chmod 700 "$BRIEF_TEST_HOME"
cp -a "$BRIEF_HOME/." "$BRIEF_TEST_HOME/"
agent-harness jobs list --instance brief-test --home "$BRIEF_TEST_HOME" --json
```

Review the copied config's recipient first; optionally change it to a test inbox. Copying the home also copies credentials: keep it private and remove it afterward. The manual command uses your exported `OPENAI_API_KEY`, not the copied service env file. **The next command sends mail:**

```bash
agent-harness jobs run morning-note --instance brief-test --home "$BRIEF_TEST_HOME"
agent-harness jobs list --instance brief-test --home "$BRIEF_TEST_HOME" --json
```

Verify receipt in the intended inbox: section order, correct date, translations, source facts, readable plain text, and no literal template placeholders. “Succeeded” records notifier acceptance, not guaranteed inbox placement; check spam/filtering and recipient routing too. The production cursors/schedule are untouched; test-copy cursors advance. Repeated tests consume further test-copy items. For an identical-input retry, make a fresh isolated copy from the still-unstarted original rather than altering live cursors. A send error leaves progress pending for retry, but does not guarantee no delivery occurred (for example an ambiguous network failure).

Delete the test copy and copied credentials when finished; the guard refuses a missing/non-temporary path:

```bash
python3 -c 'import os, pathlib, shutil, tempfile; p=pathlib.Path(os.environ["BRIEF_TEST_HOME"]).resolve(); assert p.parent == pathlib.Path(tempfile.gettempdir()).resolve() and p.name.startswith("tmp."); shutil.rmtree(p)'
unset BRIEF_TEST_HOME OPENAI_API_KEY
```

## 7. Enable the real schedule

Only proceed after verifying credentials, config, recipient, and (optionally) the controlled test. Stop any older sender that would deliver the same brief. Use the standard instance location above: the generated service template does not follow arbitrary `--home` paths.

```bash
agent-harness service install
systemctl --user daemon-reload
systemctl --user enable --now agent-harness@brief-example.service
agent-harness status --instance brief-example --home "$BRIEF_HOME" --json
systemctl --user status agent-harness@brief-example.service --no-pager
journalctl --user -u agent-harness@brief-example.service -n 100 --no-pager
agent-harness jobs list --instance brief-example --home "$BRIEF_HOME" --json
```

`service install` writes a shared user-service template; on a host with other instances, review it before replacing an existing template. Confirm service active state, a fresh heartbeat, the job's `next_run_at` (UTC), and after the scheduled run its success metadata **and actual email receipt**. If user services must survive logout, check your host's user-lingering policy with the administrator. The service environment is separate from your shell.

Config is loaded at daemon startup. After later config/secret changes, validate and restart this instance:

```bash
agent-harness jobs list --instance brief-example --home "$BRIEF_HOME" --json
systemctl --user restart agent-harness@brief-example.service
agent-harness status --instance brief-example --home "$BRIEF_HOME" --json
```

Do not reset progress to change wording, languages, layout, or delivery time. Before upgrading from a temporary dev build with custom language placeholders, follow the [date configuration migration note](background-runtime.md#date-facts-and-configured-translation-requests).

## Troubleshooting

- **Config error / omitted sections:** use v0.1.4 or later for the Step 19 example; preserve supported YAML indentation/list style and single-line prompts. Check context IDs and double-brace substitutions. The full example is not v0.1.3-compatible. `body_format: multipart` additionally needs v0.1.5 or later (Step 20); v0.1.4 does not render rich mail.
- **Missing key in manual run:** export the configured key name in the current shell. The service's secret env file is not automatically sourced by CLI commands.
- **Missing key only in daemon:** check the instance's `config/secrets/env`, variable name, permissions, and restart the service; a key exported in your terminal does not configure the service.
- **Google disconnected / scope error:** `auth start` only prints a handoff. Complete consent, check the saved scopes with `auth status`, enable the APIs, and reauthorize expired/revoked testing grants. Do not broaden to full Gmail/Drive access as a workaround.
- **Docs access or empty items:** share with the actual signing-in account, check ID/alias, inspect with `google docs read` privately, and choose a parser matching the text. Blank lines do not delimit cards for `non_empty_lines`.
- **Unexpected repeat/skip:** check the instance/home and distinct progress keys. Failed sends retain the item; successful manual sends consume it. Active cycle snapshots are not reset by every source edit.
- **Wrong date / countdown:** set the date context's timezone explicitly; inspect the UTC-countdown limitation above. Translations should use the supplied date, not the model's idea of today.
- **Succeeded but no email:** verify recipient and authenticated sender, check spam/rules, inspect safe job/service status, then use an isolated controlled send. Do not expose tokens, raw API responses, or private source text in bug reports.
- **Stopped service:** inspect systemd status/journal and user-session policy. App `status` is file/heartbeat-based, not an independent `systemctl` check.

For field-level reference, see [background runtime](background-runtime.md). For implementation status and deferred rich-email support, see the [living plan](plans/background-runtime-mvp-plan.md).
