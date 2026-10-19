# Background Runtime

`agent-harness` can run as a named background instance. This guide covers Linux `systemd --user`. Step 21 adds a separate [Windows Task Scheduler guide](windows-runtime.md) (current-user, Interactive, Limited, at-login; native CI verified, release pending). macOS is pending Step 22. Linux behavior is unchanged.

For a task-oriented walkthrough, start with [Configure a daily brief email](configure-daily-brief.md). This document is the field-level runtime reference.

## Layout

Recommended user-level layout:

```text
Binary:        ~/.local/bin/agent-harness
Instance root: ~/.local/share/agent-harness/instances/
Instance home: ~/.local/share/agent-harness/instances/<instance>/
Config:        ~/.local/share/agent-harness/instances/<instance>/config/
State:         ~/.local/share/agent-harness/instances/<instance>/state/
Jobs state:    ~/.local/share/agent-harness/instances/<instance>/state/jobs.json
Cache:         ~/.local/share/agent-harness/instances/<instance>/cache/
Work dir:      ~/.local/share/agent-harness/instances/<instance>/work/
App logs:      ~/.local/share/agent-harness/instances/<instance>/logs/
Service logs:  systemd journal for the user service
Unit file:     ~/.config/systemd/user/agent-harness@.service
```

The compiled binary should be installed separately from runtime data. Do not run the daemon from a source checkout or release extraction directory for normal background use.

For machines that should not need a source checkout or Go toolchain, use the release-artifact install flow in [`docs/install-release.md`](install-release.md).

## Initialize an instance

```bash
agent-harness init --instance default
```

For tests or custom installs, pass an explicit instance home:

```bash
agent-harness init --instance default --home ~/.local/share/agent-harness/instances/default
```

## Configure scheduled jobs

`agent-harness init` creates a starter config file:

```text
~/.local/share/agent-harness/instances/default/config/config.yaml
```

Jobs are declared in that config. The default config creates one enabled `heartbeat` job:

```yaml
jobs:
  - name: heartbeat
    type: heartbeat
    enabled: true
    interval_seconds: 60
```

The daemon loads this list on startup, validates each job, and records per-job runtime metadata in:

```text
~/.local/share/agent-harness/instances/default/state/jobs.json
```

Each job state records:

- job name and type;
- last run time;
- last success time;
- last error, if any;
- next run time;
- output path, when a job produces a local artifact.

Multiple enabled jobs can be configured independently. Disabled jobs are skipped. Unknown job types, invalid intervals or schedules, and unsafe local output paths are treated as configuration errors and are surfaced through status output. For backward compatibility, older configs with `heartbeat_job_interval_seconds` still configure the default heartbeat job when no `jobs:` list is present.

Jobs can either use a simple interval or a wall-clock daily schedule.

Use `interval_seconds` for short smoke-test loops and "run every N seconds after the prior run" behavior:

```yaml
jobs:
  - name: heartbeat
    type: heartbeat
    enabled: true
    interval_seconds: 60
```

Use `schedule.daily_at` plus an IANA timezone when a job should run once per day at a specific local time:

```yaml
jobs:
  - name: daily-ai-email
    type: ai_email
    enabled: true
    schedule:
      daily_at: "08:00"
      timezone: "America/New_York"
```

A job must use either `interval_seconds` or `schedule`, not both. `daily_at` uses 24-hour `HH:MM` format. The daemon computes `next_run_at` in UTC from the configured timezone, so daylight-saving-time boundaries follow the local timezone rule instead of a fixed UTC offset.

### Local check-in jobs

The `local_checkin` job type appends one compact JSONL record per run to an instance-owned file. It is the first local, user-visible scheduled artifact and does not call email, Telegram, Google, or any model provider.

Example:

```yaml
jobs:
  - name: daily-checkin
    type: local_checkin
    enabled: true
    interval_seconds: 86400
    message: "agent-harness is alive"
    output_path: "work/checkins.jsonl"
```

`message` defaults to `agent-harness is alive` when omitted. `output_path` defaults to `work/checkins.jsonl` and must be a relative path that stays inside the instance home.

Each JSONL record includes the job name, timestamp, message, and success status. Job status points to the output file path but does not dump the file contents.

### Notification test jobs and notifier delivery

The `notify_test` job type exercises the delivery adapter path without adding Telegram, Google Docs, model calls, or daily-brief content. It sends a configured message through a small notifier interface.

The safe default notifier is a local file outbox under the instance home:

```yaml
jobs:
  - name: notify-test
    type: notify_test
    enabled: true
    interval_seconds: 3600
    message: "delivery adapter works"
    outbox_path: "work/outbox.jsonl"
```

`message` defaults to `agent-harness notification test` when omitted. `outbox_path` defaults to `work/outbox.jsonl` and must be a relative path that stays inside the instance home.

Each outbox JSONL record includes the job name, timestamp, message, and `transport: "file_outbox"`. Job status points to the outbox path but does not dump message contents into `jobs list` output. Delivery failures are recorded in `state/jobs.json` as failed job state with `last_error`.

A Gmail send-only notifier can also back the same `notify_test` job when Google auth has a token with the Gmail send scope. Configure the requested auth scope with `google.scopes` when generating or regenerating the token:

```yaml
google:
  client_credentials_path: "config/secrets/google-client.json"
  token_path: "config/secrets/google-token.json"
  account_hint: "agent@example.com"
  scopes:
    - gmail_send
notifier:
  type: gmail
  gmail:
    from: "agent@example.com"
    to:
      - "operator@example.com"
    subject_prefix: "[agent-harness]"
jobs:
  - name: notify-test
    type: notify_test
    enabled: true
    interval_seconds: 3600
    message: "delivery adapter works"
```

The Gmail notifier requires a local Google token file with the `https://www.googleapis.com/auth/gmail.send` scope. If the access token is expired or close to expiry, the app uses the saved refresh token and configured client credentials to refresh it before sending, then rewrites the token file with the new access token and expiry. It sends compact MIME content through the Gmail API and stores only safe delivery state/errors in `state/jobs.json`; it must not print OAuth token values or raw API responses.

### AI email jobs

The `ai_email` job type generates bounded email body content with a configured LLM, then sends it through the same notifier path used by `notify_test`. It supports provider-level credentials with one or more allowed models per provider, a default provider/model, and per-job provider/model selection when a job should use something other than the default.

Example with multiple connected providers and a default model:

```yaml
llms:
  default:
    provider: openrouter
    model: "openai/gpt-4o-mini"
  providers:
    - provider: openrouter
      api_key_env: OPENROUTER_API_KEY
      models:
        - "openai/gpt-4o-mini"
        - "anthropic/claude-3-5-haiku-latest"
    - provider: openai
      api_key_env: OPENAI_API_KEY
      models:
        - "gpt-4o-mini"
        - "gpt-4.1-mini"
notifier:
  type: gmail
  gmail:
    from: "agent@example.com"
    to:
      - "operator@example.com"
    subject_prefix: "[agent-harness]"
jobs:
  - name: daily-ai-email
    type: ai_email
    enabled: true
    interval_seconds: 86400
    prompt: "Write a concise daily learning note for Zach."
    max_chars: 1200
```

A job can select a non-default provider/model without repeating key settings:

```yaml
jobs:
  - name: weekly-ai-email
    type: ai_email
    enabled: true
    interval_seconds: 604800
    prompt: "Write a practical software engineering tip."
    max_chars: 1500
    llm:
      provider: openai
      model: "gpt-4o-mini"
```

If you need multiple accounts for the same provider, give the additional entry an explicit `profile` and reference that profile from the job:

```yaml
llms:
  providers:
    - provider: openai
      profile: openai-work
      api_key_env: OPENAI_WORK_API_KEY
      models:
        - "gpt-4o-mini"
jobs:
  - name: work-ai-email
    type: ai_email
    enabled: true
    interval_seconds: 86400
    prompt: "Write a concise work note."
    max_chars: 1200
    llm:
      profile: openai-work
      model: "gpt-4o-mini"
```

`api_key_env` names an environment variable in a provider/profile entry; raw API keys must not be stored in `config/config.yaml`, and job entries should only reference provider/profile/model selectors. By default, the profile name is the provider name; if you configure the same provider more than once, use an explicit `profile`. `provider` currently supports `openrouter`, `openai`, or `openai_compatible` with `base_url`. Job state records success/failure metadata only; it does not store the prompt, generated email body, API key, or raw provider response.

### Scheduled notification jobs

The `scheduled_notification` job type composes reusable notification pieces without hardcoding user-specific sections in Go. Each job declares named context values, renders a prompt template with those values, sends one LLM-generated body through the configured notifier, and advances any rotating-source progress only after delivery succeeds.

Supported context producers:

- `days_until_date`: computes a deterministic date countdown from config using UTC calendar days.
- `current_date`: supplies the job run date with optional timezone and standard Go layout; translation instructions belong in the configured LLM prompt.
- `rotating_source_item`: reads a configured source, parses items, selects the next item with durable round-robin progress, and exposes the selected item text as a context value.

Example:

```yaml
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
      - "operator@example.com"
    subject_prefix: "[agent-harness]"
sources:
  google_docs:
    vocabulary_doc_id: "GOOGLE_DOC_ID"
jobs:
  - name: morning-note
    type: scheduled_notification
    enabled: true
    schedule:
      daily_at: "08:00"
      timezone: "America/New_York"
    context:
      - id: countdown
        type: days_until_date
        date: "2063-10-28"
        label: "Zach's 80th birthday"
        output_format: "{days} days until {label}"
      - id: word
        type: rotating_source_item
        source:
          type: google_doc
          alias: vocabulary_doc_id
          parser: non_empty_lines
        selection:
          strategy: round_robin
          progress_key: vocabulary-daily-brief
    render:
      type: llm_template
      prompt: "Write the complete email body. Countdown first: {{ countdown }}. Then word: {{ word }}. Include a concise definition and example sentence. Return only the body."
      max_chars: 1200
```

The app code only knows generic context producer and renderer types; labels such as daily brief, birthday, or vocabulary live in instance config. `state/progress/<progress_key>.json` stores cursor metadata and item IDs, not document contents, prompts, generated message bodies, tokens, or API keys.

### Gmail body formats (Step 20, v0.1.5)

**Availability:** v0.1.4 includes Step 19 date/local-source/card parsing features. The `body_format` option below is available in **v0.1.5** (Step 20). v0.1.4 does not render rich mail; older binaries may ignore unknown keys. Release installation remains a separately approved action.

```yaml
notifier:
  type: gmail
  gmail:
    from: "agent@example.com"
    to:
      - "operator@example.com"
    subject_prefix: "[agent-harness]"
    body_format: multipart
```

- Omit `body_format` (or set `plain`) to retain the existing plain-text message layout and body bytes. `multipart` opts into `multipart/alternative`, with UTF-8 `text/plain` first and app-generated `text/html` second. Both parts use quoted-printable transfer encoding and MIME CRLF framing. The fallback retains textual content/markers, normalizing line endings and replacing malformed UTF-8 with U+FFFD in both alternatives (for example if the existing byte-based output cap splits a character). Valid UTF-8 content is preserved. Non-ASCII subjects use MIME encoded words; control characters in configured headers and job names are rejected with non-content errors.
- Only `plain` and `multipart` are supported (case-sensitive). `html`/HTML-only and unknown values are rejected early. Requiring a plain alternative is deliberate for accessibility, compatibility, and inspection. This setting applies to Gmail notifications; it does not change `file_outbox` or the model/render schema.
- HTML syntax is deliberately limited: after trimming surrounding whitespace on each line, `# `, `## `, or `### ` followed by nonempty text produce headings; consecutive `- ` or `* ` lines with nonempty text produce a flat unordered list. Blank lines close a paragraph/list. Other consecutive lines form a paragraph with `<br>` line breaks. Headings and list transitions also close the previous block. Ordinary text without markers is readable as paragraphs.
- Everything else is **literal escaped text**, including raw HTML, Markdown bold, links, image syntax, code fences, numbered lists, and headings deeper than level three. Indentation is ignored, so indented bullets flatten rather than nest. No inline Markdown is interpreted. All source text is HTML-escaped, including text within headings and bullets. The app emits only a fixed document wrapper, headings, paragraphs, breaks, and list tags: no links, images, scripts, CSS, or source-controlled attributes. Email clients may independently auto-link visible text; the renderer does not generate anchors.
- The renderer is a single non-recursive linear pass, not a general Markdown engine. It has no fetching, sanitization dependencies, or extra model call. Existing `render.max_chars` caps scheduled model output before delivery; MIME/HTML escaping can expand the wire size. Formatting is not a client-specific visual guarantee. Avoid tabs/indentation-dependent layouts.
- Keep returning textual content once from the configured model prompt. Optionally request headings and flat bullets in that same single-line prompt; do not request HTML. Keep language, source selection, dates, and section wording in existing instance config. The `current_date` context below composes with this notifier without special rich-output logic.
- Generated bodies and MIME stay in memory during Gmail delivery. Job/progress state stores metadata only; HTTP failure response bodies are not included in errors. `file_outbox`, if selected instead, intentionally writes message content locally and is not a secret-free output adapter.

For isolated verification, run `go test ./internal/runtime -run 'TestGmail' -count=1`. The multipart delivery/progress smoke test uses a temporary instance, fake model/token, and loopback HTTP server, including a failed send and successful retry. It decodes actual MIME and checks persisted state for leaks without external credentials or mail. Do not point a development smoke test at a live instance, consume live rotation state, install a dev binary, or start a service. Any later controlled real send needs separate approval and an isolated instance after review/release.

### Date facts and configured translation requests

`current_date` supplies an authoritative date derived from the job run timestamp. It accepts an optional IANA `timezone` (UTC by default) and a Go date-layout `format` (`Monday January 2, 2006` by default). Invalid timezones are rejected during config loading. A literal `\n` in the layout becomes a line break. Ordinary Go date formatting uses English weekday/month names; the runtime does not translate them or spell out numbers.

For a scheduled notification, add date context and put translation instructions in the existing render prompt. This excerpt illustrates only the date portion; retain other context and prompt sections when adapting an existing job:

```yaml
context:
  - id: today_date
    type: current_date
    timezone: "America/New_York"
    format: "Monday, January 2, 2006"
render:
  type: llm_template
  prompt: "The authoritative current date is {{ today_date }}. Use this date; do not infer today's date yourself. Write the date in Spanish, Italian, French, and German. Spell out the day number using natural conventions for each language. Put each language on its own line. Return only the notification body."
  max_chars: 1200
```

The prompt stays on one physical line for compatibility with the runtime's YAML subset reader. Languages, wording, and output structure belong to instance config. Translation happens in the same single model call already used to render the notification, not in an additional request. Date/timezone resolution is deterministic; model-produced wording is not guaranteed to be identical or error-free. Tests can verify the exact date and instructions sent to a fake model without claiming to validate real translations.

**Migrating temporary dev configurations:** custom placeholders such as `{weekday_es}`, `{month_it}`, `{day_fr}`, and `{day_de_ord}` are no longer expanded. Before deploying this version, replace localized layouts with a standard Go layout and move translation/spelled-out-number instructions into `render.prompt`. Also replace any old prompt instruction to copy pretranslated date lines verbatim. Unknown placeholder text remains literal; it is not a supported localization API. Private instance config and credentials do not belong in the repository.

### Pattern-delimited source cards

Use `parser: heading_pattern` when a source has recognizable headings followed by plain text, bullets, or both. For example, this context entry can be added to a scheduled notification job (with a matching `{{ card }}` placeholder in its render prompt):

```yaml
context:
  - id: card
    type: rotating_source_item
    source:
      type: local_file
      path: "data/example-cards.txt"
      parser: heading_pattern
      heading_pattern: '^[0-9]+\s*-\s*\S+'
    selection:
      strategy: round_robin
      progress_key: example-cards
```

Fake `data/example-cards.txt` contents:

```text
Example reference cards (this preamble is ignored).

1 - Cedar
An evergreen tree in this fictional collection.
2024 observations are a fact, not a new heading.
- Grows in the example garden.

2 - Maple
A second example card.
* Has a different leaf shape.
```

The configured Go regular expression is checked against each trimmed non-empty line. Only matching lines start a new card; the complete matching line becomes its title. All subsequent non-empty plain or bullet lines belong to that card's body until the next matching heading. Leading `- `, `* `, and `• ` bullet markers are removed, empty bullets are skipped, and text before the first heading is ignored. The rendered context contains the title and body joined by newlines. The pattern must be non-empty, syntactically valid, and must not match empty text; otherwise configuration loading fails. Single-quoted YAML preserves regex backslashes.

The same parser and `heading_pattern` work with `type: google_doc` and a configured `alias` instead of a local `path`. Use a separate `progress_key` for each independent rotation. Selection does not advance progress; successful notification delivery advances the selected item and wraps to the first card after the last, as with the existing parsers. `non_empty_lines` and `heading_with_bullets` remain available unchanged.

## Inspect and manually run jobs

List configured jobs without needing a long-running daemon:

```bash
agent-harness jobs list --instance default
agent-harness jobs list --instance default --json
```

`jobs list` combines the current `config/config.yaml` job declarations with any existing `state/jobs.json` runtime state. It shows enabled/disabled status plus last run, last success, next run, and last error when those fields exist.

Run a configured job immediately through the same registry used by the daemon:

```bash
agent-harness jobs run heartbeat --instance default
agent-harness jobs run daily-checkin --instance default
agent-harness jobs run notify-test --instance default
agent-harness jobs run daily-ai-email --instance default
```

Manual runs update `state/jobs.json` just like scheduled daemon runs. Unknown jobs, disabled jobs, and invalid job config return controlled non-zero errors. The command does not require `systemd` or a background daemon, which makes it useful for debugging instance config before enabling the service.

Future job types should reuse this config/state shape instead of creating job-specific status files.

## Configure Google account connection metadata

Google integrations are configured with paths and safe metadata only. Do not paste client secrets, authorization codes, access tokens, or refresh tokens into `config/config.yaml`.

Example:

```yaml
google:
  client_credentials_path: "config/secrets/google-client.json"
  token_path: "config/secrets/google-token.json"
  account_hint: "agent@example.com"
  scopes:
    - gmail_send
    - docs_readonly
```

Supported narrow scope aliases:

- `gmail_send`: requests `https://www.googleapis.com/auth/gmail.send` for the Gmail notifier.
- `docs_readonly`: requests `https://www.googleapis.com/auth/documents.readonly` for the Google Docs source adapter.

`google.scopes` is the preferred config for token generation because it composes multiple Google capabilities without inventing combined profile names. Older configs may still use the legacy single-value key:

```yaml
google:
  scope_profile: "gmail_send"
```

When both `scopes` and `scope_profile` are present, `scopes` takes precedence. These settings describe which permissions `google auth start` should request; runtime Google calls still depend on the actual saved token scopes and Google API permissions.

`agent-harness init` creates `config/secrets/` with restricted directory permissions. Store Google client credentials and token files there with `0600` file permissions. These files are local secrets and must not be committed.

Inspect Google auth state without printing token values:

```bash
agent-harness google auth status --instance default
```

Start the safe auth handoff/instructions for the configured scope aliases:

```bash
agent-harness google auth start --instance default
```

Remove the local Google token file without deleting the client credentials file:

```bash
agent-harness google auth revoke --instance default --confirm revoke-google-token
```

The auth status/start commands may print safe metadata such as account hint, legacy scope profile, requested scope aliases, requested concrete scopes, token scopes, token expiry, and configured paths. They must not print access tokens, refresh tokens, client secrets, authorization codes, or raw credential JSON. Google-backed actions may refresh an expired access token using the saved refresh token and client credentials; refresh failures are reported with controlled status-only errors. To fully revoke cloud-side access, also remove the app from the connected Google account's security settings.

## Configure Google Docs read-only sources

For Google Docs sources, first share the document with the configured Google account, then copy the document ID from its URL:

```text
https://docs.google.com/document/d/DOC_ID_HERE/edit
```

Configure local aliases under `sources.google_docs`:

```yaml
google:
  client_credentials_path: "config/secrets/google-client.json"
  token_path: "config/secrets/google-token.json"
  account_hint: "agent@example.com"
  scopes:
    - docs_readonly
sources:
  google_docs:
    vocabulary_doc_id: "DOC_ID_HERE"
```

Read a configured document source by alias:

```bash
agent-harness google docs read vocabulary_doc_id --instance default
```

The command calls the Google Docs API for the configured document ID using the instance's saved OAuth token. `google.scopes` tells `google auth start` which scopes to request; the read command itself accepts a saved token that includes either `https://www.googleapis.com/auth/documents.readonly` or the broader `https://www.googleapis.com/auth/documents` scope. If the access token is expired and refresh metadata exists, the app refreshes it before reading.

Step 15 intentionally does not search Google Drive by title. Drive discovery would require extra Drive API scope and ambiguity handling. Configure exact document IDs explicitly, then use the read command as a smoke test before wiring future jobs to that source.

The document text is printed to stdout for the explicit read command. It is not written into `state/status.json` or `state/jobs.json`, and errors must not include access tokens, refresh tokens, client secrets, or raw credential JSON.

## Durable rotating item cursors

Document-backed jobs can use a small progress subsystem to select one parsed item at a time and remember the next item across daemon restarts. Progress state is stored under the instance state directory:

```text
~/.local/share/agent-harness/instances/default/state/progress/<progress-key>.json
```

The progress state records safe metadata only:

- progress key;
- selection strategy, currently `round_robin`;
- next item index and cycle count;
- active cycle snapshot of item IDs;
- source alias and parser name;
- pending/last selected item IDs and timestamps.

It does not store full Google Doc text, delivered message bodies, OAuth tokens, refresh tokens, client secrets, or credential JSON.

Current parsers are available to `scheduled_notification` rotating sources (`google_doc` or instance-relative `local_file`):

- `non_empty_lines`: trims blank lines and treats each remaining line as one item;
- `heading_with_bullets`: treats a heading/title followed by bullet lines as a card with `title` and `body` fields.
- `heading_pattern`: uses a configured regular expression to delimit headings with plain or bulleted body lines; see [pattern-delimited source cards](#pattern-delimited-source-cards).

The selector uses cycle snapshots: it parses the current source into ordered item IDs at the start of a cycle, walks that snapshot until the last item, and refreshes the snapshot only at the next cycle boundary. This avoids surprising skips or repeats if a document is edited midway through a cycle.

Cursor advancement is explicit. A caller selects/peeks the next item, performs the work, and advances progress only after successful completion or delivery. Failed sends should leave the cursor unchanged so the same item can be retried.

Step 18 connects source selection and progress to scheduled notification delivery; Step 19 extends date context, local sources, and configurable card parsing. See the [setup walkthrough](configure-daily-brief.md) for a complete example.

## Run a daemon smoke test

```bash
agent-harness daemon --instance default --test
agent-harness status --instance default
agent-harness status --instance default --json
```

`--test` writes a single sample heartbeat/status file and exits. This verifies the daemon/status file path and built-in heartbeat job state without starting a long-running service. Normal background use runs without `--test` under the service manager.

## systemd user service

Install the user-level systemd template:

```bash
agent-harness service install
```

This writes:

```text
~/.config/systemd/user/agent-harness@.service
```

For tests or custom installs, pass an explicit config home:

```bash
agent-harness service install --config-home ~/.config
```

For scheduled jobs that need provider credentials, put environment-variable assignments in the instance-local secret environment file:

```bash
mkdir -p ~/.local/share/agent-harness/instances/default/config/secrets
chmod 700 ~/.local/share/agent-harness/instances/default/config/secrets
printf 'OPENROUTER_API_KEY=...\n' > ~/.local/share/agent-harness/instances/default/config/secrets/env
chmod 600 ~/.local/share/agent-harness/instances/default/config/secrets/env
```

The generated systemd unit loads this file with `EnvironmentFile=-.../config/secrets/env`. The leading `-` means the service can still start when the file is absent. Do not commit this file or paste real values into PRs, issues, docs, or logs.

The generated template is:

```ini
[Unit]
Description=agent-harness instance %i
After=network-online.target

[Service]
Type=simple
EnvironmentFile=-%h/.local/share/agent-harness/instances/%i/config/secrets/env
ExecStart=%h/.local/bin/agent-harness daemon --instance %i --home %h/.local/share/agent-harness/instances/%i
WorkingDirectory=%h/.local/share/agent-harness/instances/%i
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=default.target
```

After installing the template, enable and start an instance:

```bash
systemctl --user daemon-reload
systemctl --user enable --now agent-harness@default.service
```

Check service status and logs:

```bash
agent-harness status --instance default
systemctl --user status agent-harness@default.service --no-pager
journalctl --user -u agent-harness@default.service -n 100 --no-pager
```

`agent-harness status` reads the app-owned status file and prints service-manager hints. It does not shell out to `systemctl` by default.

## Update from source during development

Use this flow only on development hosts that update directly from a source checkout. Release-based installs should use the versioned binary update flow once release artifacts exist.

```bash
git pull --ff-only
go test ./...
go build -o ~/.local/bin/agent-harness ./cmd/agent-harness
systemctl --user restart agent-harness@default.service
agent-harness status --instance default
```

If Go is installed outside `PATH`, use that local Go binary explicitly when running the test/build commands.
