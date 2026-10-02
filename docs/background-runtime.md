# Background Runtime

`agent-harness` can run as a named background instance. The first supported runtime manager is Linux `systemd --user`.

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

A Gmail send-only notifier can also back the same `notify_test` job when Google auth is configured with the narrow `gmail_send` scope:

```yaml
google:
  client_credentials_path: "config/secrets/google-client.json"
  token_path: "config/secrets/google-token.json"
  account_hint: "agent@example.com"
  scope_profile: "gmail_send"
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
  scope_profile: "gmail_send"
```

Supported narrow scope profiles:

- `gmail_send`: requests `https://www.googleapis.com/auth/gmail.send` for the Gmail notifier.
- `docs_readonly`: requests `https://www.googleapis.com/auth/documents.readonly` for a future Google Docs source adapter.

`agent-harness init` creates `config/secrets/` with restricted directory permissions. Store Google client credentials and token files there with `0600` file permissions. These files are local secrets and must not be committed.

Inspect Google auth state without printing token values:

```bash
agent-harness google auth status --instance default
```

Start the safe auth handoff/instructions for the configured scope profile:

```bash
agent-harness google auth start --instance default
```

Remove the local Google token file without deleting the client credentials file:

```bash
agent-harness google auth revoke --instance default --confirm revoke-google-token
```

The auth status/start commands may print safe metadata such as account hint, scope profile, requested scopes, token expiry, and configured paths. They must not print access tokens, refresh tokens, client secrets, authorization codes, or raw credential JSON. Google-backed actions may refresh an expired access token using the saved refresh token and client credentials; refresh failures are reported with controlled status-only errors. To fully revoke cloud-side access, also remove the app from the connected Google account's security settings.

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
