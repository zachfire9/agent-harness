# Background Runtime MVP Implementation Plan

> **Purpose:** Start the next `agent-harness` direction from scratch with a smaller operational MVP: first make the agent installable as a background process and easy to inspect, then add useful scheduled behavior later.

## Goal

Build a generic background runtime for `agent-harness` that can initially run on a local Linux host, while remaining reusable for other `agent-harness` instances on other machines.

The first useful deliverable is **not** a content feature or delivery integration. The first deliverable is:

- `agent-harness` can run unattended in the background;
- it has a stable instance name/config/state/log layout;
- an operator can check whether it is running, stopped, failing, or misconfigured;
- the same mechanism can be installed for another instance without hardcoding a specific machine.

## Non-goals for the MVP

Do not include these in the first step:

- daily brief content;
- Gmail/Google Docs integration;
- Telegram delivery;
- AWS/remote deployment;
- model calls;
- rich scheduling logic beyond keeping the process alive and observable.

Those can be added after the background runtime is reliable.

## Operating model

Initial target:

- Local Linux host used for the first deployment.
- User-level service preferred over root/system service.
- `systemd --user` is the first runtime manager.

Generic design requirement:

- Nothing should be hardcoded to a specific username, repo checkout, home directory, or machine name.
- Runtime paths should be derived from config, flags, or XDG-style defaults.
- Every service should have an explicit instance name, for example `default`, `worker`, or `scheduler`.
- Status checks should work the same way for the first host and future instances.
- Core daemon, status-file, instance-home, and version logic should be OS-neutral; only service-manager integration and default path resolution should be OS-specific.
- The first implementation target is Linux with `systemd --user`, but the design should not prevent later macOS `launchd` or Windows service/task adapters.

Recommended Linux default paths:

```text
Binary:        ~/.local/bin/agent-harness
Instance root: ~/.local/share/agent-harness/instances/
Instance home: ~/.local/share/agent-harness/instances/<instance>/
Config:        ~/.local/share/agent-harness/instances/<instance>/config/
State:         ~/.local/share/agent-harness/instances/<instance>/state/
Cache:         ~/.local/share/agent-harness/instances/<instance>/cache/
Work dir:      ~/.local/share/agent-harness/instances/<instance>/work/
App logs:      ~/.local/share/agent-harness/instances/<instance>/logs/
Service logs:  systemd journal for the user service
Unit file:     ~/.config/systemd/user/agent-harness@.service
```

The compiled binary should not run out of the source checkout or release extraction directory. Each background instance should have a self-contained instance home. The daemon should use that instance home as its working directory and should read/write config, state, cache, work files, app logs, and status files only inside that home unless the operator explicitly configures another path.

Future platform defaults should use each OS's normal per-user application-data location while preserving the same logical instance-home structure:

```text
macOS instance home:   ~/Library/Application Support/agent-harness/instances/<instance>/
Windows instance home: %LOCALAPPDATA%\agent-harness\instances\<instance>\
```

If the implementation uses different paths, document why and keep them instance-aware.

---

# Step checklist

## Step 01 — Background runtime and status checks

- **Status:** Completed
- **Branch:** `step-21-background-runtime-status`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/23
- **Concept:** Before an agent can do useful scheduled work, it needs a reliable operational shell: install, start, stop, restart, status, logs, health, and update instructions.

### Objective

Update `agent-harness` so it can run as a background process under a generic instance name and expose clear status/error checks that an operator can run on any supported host and reuse for other instances later.

### Scope

Add the smallest operational runtime that proves `agent-harness` can stay alive and be inspected:

- Add an initialization command that creates a self-contained instance home, for example:

  ```bash
  agent-harness init --instance default
  ```

- Add a long-running command that uses the instance home, for example:

  ```bash
  agent-harness daemon --instance default
  ```

- Add a status command that reads from the instance home, for example:

  ```bash
  agent-harness status --instance default
  ```

- Allow an explicit home override for advanced installs or tests:

  ```bash
  agent-harness daemon --instance default --home ~/.local/share/agent-harness/instances/default
  agent-harness status --instance default --home ~/.local/share/agent-harness/instances/default
  ```

- Keep path resolution behind a small abstraction so platform-specific defaults can be added later without changing daemon/status behavior. Conceptually:

  ```go
  type PathResolver interface {
      InstanceHome(instance string) string
      ConfigDir(instance string) string
      StateDir(instance string) string
      CacheDir(instance string) string
      WorkDir(instance string) string
      LogDir(instance string) string
  }
  ```

- Keep service-manager operations behind a small abstraction, with Linux/systemd as the only required Step 01 implementation. Conceptually:

  ```go
  type ServiceManager interface {
      Install(instance string) error
      Start(instance string) error
      Stop(instance string) error
      Restart(instance string) error
      Status(instance string) ServiceStatus
      Logs(instance string, lines int) ([]string, error)
  }
  ```

- Add service-management documentation or generated unit content for a user-level systemd template:

  ```text
  agent-harness@.service
  ```

  The unit should set the working directory to the instance home and pass the same home to the daemon, conceptually:

  ```ini
  [Service]
  ExecStart=%h/.local/bin/agent-harness daemon --instance %i --home %h/.local/share/agent-harness/instances/%i
  WorkingDirectory=%h/.local/share/agent-harness/instances/%i
  ```

- Support commands equivalent to:

  ```bash
  systemctl --user daemon-reload
  systemctl --user enable --now agent-harness@default.service
  systemctl --user status agent-harness@default.service --no-pager
  journalctl --user -u agent-harness@default.service -n 100 --no-pager
  ```

- The daemon should write a small status/health file under the instance home state directory, for example `~/.local/share/agent-harness/instances/default/state/status.json`:

  ```json
  {
    "instance": "default",
    "status": "running",
    "started_at": "2026-01-01T12:00:00Z",
    "last_heartbeat_at": "2026-01-01T12:01:00Z",
    "pid": 12345,
    "version": "dev"
  }
  ```

- The status command should combine what it can know from local state plus service-manager hints where available.
- Status output may be human-readable by default, but should support a JSON mode for future automation:

  ```bash
  agent-harness status --instance default --json
  ```

- Handle shutdown signals cleanly and update status on graceful shutdown when possible.
- Keep secrets out of status files and logs.

### Health/status states

Define and document at least these states:

- `running`: service is active and heartbeat is fresh.
- `stopped`: service is inactive or no process is known.
- `starting`: process started but has not written a fresh heartbeat yet.
- `failed`: service manager reports failure or daemon wrote a terminal error.
- `stale`: status file exists but heartbeat is older than the threshold.
- `config-error`: required config is missing or invalid.
- `unknown`: status cannot be determined from available signals.

### Generic instance behavior

The implementation should work for any instance name:

```bash
agent-harness init --instance default
agent-harness init --instance worker
agent-harness daemon --instance default
agent-harness daemon --instance worker
agent-harness status --instance default
agent-harness status --instance worker
systemctl --user status agent-harness@default.service --no-pager
systemctl --user status agent-harness@worker.service --no-pager
```

Instance names should be validated to avoid path traversal or unsafe systemd unit names. Keep allowed names simple, for example letters, numbers, `_`, and `-`.

### Self-contained instance home

Each instance home should be the only default read/write location for that instance:

```text
~/.local/share/agent-harness/instances/<instance>/
  config/
    config.yaml
    env
  state/
    status.json
    jobs.json
    run-history.json
  cache/
  work/
  logs/
```

Rules:

- `agent-harness init --instance <name>` creates the directory tree and starter config files.
- `agent-harness daemon --instance <name>` derives the default home from the instance name unless `--home` is provided.
- `agent-harness status --instance <name>` reads status from the same home resolution logic.
- The service `WorkingDirectory` should be the instance home, not the source checkout and not a release extraction directory.
- Runtime files should not be written beside the compiled binary.
- Multiple instances must not share writable state unless explicitly configured.
- Secrets should live in config files or environment files under the instance home only when appropriate permissions are documented; status/log files must not echo secret values.

### Files likely to change

Exact paths may vary after inspecting the current code, but expect something like:

- Modify: `cmd/agent-harness/main.go`
- Modify/create: `internal/cli/...`
- Create: `internal/runtime/daemon.go`
- Create: `internal/runtime/status.go`
- Create: `internal/runtime/paths.go`
- Create: `internal/runtime/paths_linux.go`
- Create: `internal/runtime/service.go`
- Create: `internal/runtime/service_systemd.go`
- Create: `internal/runtime/instance_home.go`
- Create: `internal/runtime/*_test.go`
- Create: `docs/background-runtime.md`
- Modify: `README.md`
- Modify: `.gitignore` if local runtime artifacts can appear in the repo

### Tests

Add deterministic unit tests for:

- valid and invalid instance names;
- deriving the default instance home from an instance name;
- creating the instance home directory tree;
- resolving explicit `--home` overrides;
- deriving config/state/cache/work/log paths inside an instance home;
- writing and reading a status file;
- stale heartbeat detection;
- status classification for running/stopped/stale/config-error inputs;
- JSON status output shape;
- service unit template rendering does not include machine-specific hardcoding;
- Linux path defaults follow the documented instance-home layout;
- service-manager behavior can be tested with fakes without invoking real `systemd`;
- signal/shutdown behavior where practical without flaky sleeps.

Do not require real `systemd` in unit tests. Test unit rendering and status classification with fakes.

### Manual verification on the first local host

After Step 01 is implemented, an operator should be able to run:

```bash
go test ./...
go build -o ~/.local/bin/agent-harness ./cmd/agent-harness
~/.local/bin/agent-harness init --instance default
mkdir -p ~/.config/systemd/user
# Install or generate the user unit documented by the step.
systemctl --user daemon-reload
systemctl --user enable --now agent-harness@default.service
systemctl --user status agent-harness@default.service --no-pager
~/.local/bin/agent-harness status --instance default
~/.local/bin/agent-harness status --instance default --json
journalctl --user -u agent-harness@default.service -n 100 --no-pager
```

Expected result:

- the instance home exists under `~/.local/share/agent-harness/instances/default/`;
- service is active;
- `agent-harness status --instance default` reports `running`;
- JSON status is parseable;
- journal logs show startup without secrets;
- stopping the service changes status to stopped/stale/unknown in a documented way;
- a deliberately bad config produces `config-error` and visible logs.

### Update workflow for future changes

Document a generic update path that an operator can run for this instance and adapt for others:

```bash
git -C <repo-path> pull --ff-only
go test ./...
go build -o ~/.local/bin/agent-harness ./cmd/agent-harness
systemctl --user restart agent-harness@default.service
~/.local/bin/agent-harness status --instance default
journalctl --user -u agent-harness@default.service -n 100 --no-pager
```

The docs should explain which placeholders change for another machine or instance:

- `<repo-path>`
- binary install path
- instance name
- instance home/root
- config/state/cache/work/log directories inside the instance home
- systemd unit name

### Acceptance criteria

Step 01 is complete when:

- `agent-harness init --instance ...` creates a self-contained instance home;
- the background daemon can be started by a user-level service on a local Linux host;
- the same service template can run another named instance with a separate instance home;
- an operator can check status with one `agent-harness status --instance ...` command;
- an operator can inspect logs with one documented `journalctl` command;
- failed startup/config errors are visible without reading source code;
- all new behavior has tests or documented manual verification where true background process behavior is involved.

---

## Step 02 — Versioned binary build and install workflow

- **Status:** Completed
- **Branch:** `step-22-versioned-binary-install`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/24
- **Concept:** Once the runtime shell exists, package it as a versioned binary that can be installed on this host or another machine without requiring a source checkout or Go toolchain.

### Objective

Add a repeatable build/install workflow for compiled `agent-harness` binaries with embedded version metadata and documented update/rollback steps.

This step should support both modes:

1. **Development install:** build from a local checkout while iterating.
2. **Versioned install:** install a specific release artifact on another machine.

### Scope

- Add build-time version metadata such as:
  - version;
  - commit;
  - build date;
  - dirty/tree state when useful.
- Add a command such as:

  ```bash
  agent-harness version
  ```

- Include version metadata in:
  - `agent-harness version` output;
  - `agent-harness status --instance ...` human output;
  - `agent-harness status --instance ... --json` output;
  - the daemon status/health file written by Step 01.
- Add a simple release build script before adding heavier release automation, for example:

  ```bash
  scripts/build-release.sh
  ```

- Build at least the primary Linux targets first, with room for more targets later:

  ```text
  linux-amd64
  linux-arm64
  ```

- Document future release targets but do not require them in this step:

  ```text
  darwin-arm64
  darwin-amd64
  windows-amd64
  ```

- Produce release artifacts under `dist/`, for example:

  ```text
  dist/agent-harness_<version>_linux_amd64.tar.gz
  dist/agent-harness_<version>_linux_arm64.tar.gz
  dist/checksums.txt
  ```

- Document manual install from a release artifact:

  ```bash
  tar -xzf agent-harness_<version>_linux_amd64.tar.gz
  install -m 0755 agent-harness ~/.local/bin/agent-harness
  agent-harness version
  systemctl --user restart agent-harness@default.service
  agent-harness status --instance default
  ```

- Document GitHub Releases as the published-version store:
  - release archives and `checksums.txt` are uploaded as GitHub Release assets;
  - generated `dist/` artifacts are not committed to git;
  - each release is tied to a `v<version>` tag;
  - the embedded binary version is `<version>` without the leading `v`;
  - target machines download a specific release asset and verify `checksums.txt` before installing.
- Document the first manual release-publishing flow with `gh release create`, while leaving GitHub Actions publishing for a later step.
- Document development install from source separately:

  ```bash
  git -C <repo-path> pull --ff-only
  go test ./...
  go build -o ~/.local/bin/agent-harness ./cmd/agent-harness
  agent-harness version
  systemctl --user restart agent-harness@default.service
  agent-harness status --instance default
  ```

- Add `dist/` to `.gitignore` if needed.
- Do not require Go to be installed on target machines that use release artifacts.
- Do not add GitHub Actions release publishing until the manual build/install path is proven.

### Files likely to change

- Create: `internal/version/version.go`
- Create: `internal/version/version_test.go`
- Modify: `cmd/agent-harness/main.go` or CLI routing files
- Modify: runtime/status code from Step 01 to include version metadata
- Create: `scripts/build-release.sh`
- Create: `docs/install-release.md`
- Modify: `docs/background-runtime.md`
- Modify: `.gitignore`
- Modify: `README.md`

### Tests

Add deterministic tests for:

- default dev version values;
- injected version metadata formatting;
- `agent-harness version` output;
- JSON status includes version metadata;
- release artifact naming avoids unsafe version strings;
- install docs do not hardcode a particular host/user path beyond documented defaults.

### Manual verification

```bash
go test ./...
./scripts/build-release.sh 0.1.0-dev
ls dist/
tar -tzf dist/agent-harness_0.1.0-dev_linux_amd64.tar.gz
install -m 0755 dist/<extracted-binary> ~/.local/bin/agent-harness
~/.local/bin/agent-harness version
systemctl --user restart agent-harness@default.service
~/.local/bin/agent-harness status --instance default
```

### Acceptance criteria

Step 02 is complete when:

- an operator can identify exactly what version/commit is running;
- a binary can be installed without a source checkout on the target machine;
- status output includes version metadata;
- release-artifact install docs place the binary separately from per-instance homes;
- update docs distinguish source-based development updates from release-artifact installs;
- published versions are documented as GitHub Releases with tagged assets and checksums;
- target-machine docs show how to download a specific version, verify it, install it, and restart the service;
- the plan leaves room for later GoReleaser/GitHub Actions release automation without requiring it now.

---

## Step 03 — Minimal scheduled job hook

- **Status:** Completed
- **Branch:** `step-23-minimal-scheduled-job-hook`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/25
- **Concept:** Once the process can run in the background, add a tiny generic job loop without tying it to a specific content feature.

### Objective

Add a minimal no-op or heartbeat job mechanism so a running instance can prove it wakes up on an interval and records success/failure metadata.

### Scope

- Add config for an interval or cron-like schedule.
- Add a single built-in `noop` or `heartbeat` job.
- Record last run time, last success, last error, and next run time in instance state.
- Surface job status through `agent-harness status`.
- Keep the design generic so later email, daily-brief, or other scheduled jobs plug in as job types.

### Verification

```bash
go test ./...
systemctl --user restart agent-harness@default.service
agent-harness status --instance default
journalctl --user -u agent-harness@default.service -n 100 --no-pager
```

---

## Step 04 — Configurable job registry and schedules

- **Status:** Completed
- **Branch:** `step-24-configurable-job-registry`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/26
- **Concept:** Turn the first hardcoded heartbeat hook into a small reusable job system where enabled jobs, intervals, and job types are declared in config rather than baked into the daemon.

### Objective

Let an instance define one or more simple scheduled jobs in its instance config, while keeping the built-in heartbeat job as the default starter job created by `init`.

### Scope

- Keep the existing `state/jobs.json` runtime state shape from Step 03.
- Replace single-purpose heartbeat scheduling with a tiny job registry, for example:

  ```go
  type Job interface {
      Name() string
      Run(ctx context.Context) error
  }
  ```

- Add config for named jobs under `config/config.yaml`, conceptually:

  ```yaml
  jobs:
    - name: heartbeat
      type: heartbeat
      enabled: true
      interval_seconds: 60
  ```

- Validate job config on startup and surface bad job config as `config-error` in status output.
- Support multiple enabled jobs with separate last-run/next-run/error state records.
- Keep scheduling deterministic with an injected clock in tests.
- Do not add email, model calls, Telegram, Google Docs, or rich cron syntax in this step.

### Tests

Add deterministic tests for:

- default `init` config includes one enabled heartbeat job;
- valid job config loads and schedules correctly;
- unknown job types fail clearly;
- disabled jobs do not run;
- multiple jobs maintain independent state in `state/jobs.json`;
- invalid intervals return config errors instead of crashing the daemon.

### Verification

```bash
go test ./...
agent-harness init --instance default --home /tmp/agent-harness-demo
agent-harness daemon --instance default --home /tmp/agent-harness-demo --test
agent-harness status --instance default --home /tmp/agent-harness-demo --json
```

---

## Step 05 — Job inspection and manual run commands

- **Status:** Completed
- **Branch:** `step-25-job-inspection-commands`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/27
- **Concept:** Operators need to inspect and trigger jobs without waiting for the daemon interval, especially before the first useful external integration exists.

### Objective

Add small CLI commands for listing configured jobs, inspecting job state, and manually running a job once through the same registry path used by the daemon.

### Scope

- Add a command such as:

  ```bash
  agent-harness jobs list --instance default
  agent-harness jobs list --instance default --json
  agent-harness jobs run heartbeat --instance default
  ```

- `jobs list` should show name, type, enabled/disabled status, last run, last success, next run, and last error.
- `jobs run <name>` should execute the configured job immediately and update `state/jobs.json` just like a scheduled daemon run.
- Manual runs should work without `systemd` and without a long-running daemon, so job behavior is easy to debug.
- Manual runs should reject unknown, disabled, or misconfigured jobs with controlled errors.
- Keep output secret-safe and parseable in JSON mode.

### Tests

Add deterministic tests for:

- job list output contains configured jobs and redacts config values where needed;
- JSON job list output is parseable and stable;
- manual run updates last-run/last-success state;
- manual run records errors when a fake job fails;
- unknown or disabled job names return clear non-zero CLI errors.

### Verification

```bash
go test ./...
agent-harness jobs list --instance default
agent-harness jobs run heartbeat --instance default
agent-harness jobs list --instance default --json
```

---

## Step 06 — First useful local job: append a timestamped check-in file

- **Status:** Completed
- **Branch:** `step-26-local-checkin-job`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/28
- **Concept:** The first user-visible scheduled behavior should stay local and low-risk: prove the scheduler can produce a useful artifact without email, model calls, or external accounts.

### Objective

Add a configurable local check-in job that appends a timestamped line or small JSONL event to an instance-owned file on each run.

### Scope

- Add a new job type such as `local_checkin`.
- Configure it in `config/config.yaml`, conceptually:

  ```yaml
  jobs:
    - name: daily-checkin
      type: local_checkin
      enabled: true
      interval_seconds: 86400
      message: "agent-harness is alive"
      output_path: "work/checkins.jsonl"
  ```

- Restrict `output_path` to the instance home, with a safe default under `work/` or `logs/`.
- Write one compact record per run, including job name, timestamp, message, and success/failure metadata.
- Surface the latest output location in job status without dumping the whole file.
- Keep this as a local artifact only; do not add email, Telegram, Gmail, Docs, or LLM summarization here.

### Tests

Add deterministic tests for:

- local check-in job writes one record per run;
- output path is constrained to the instance home;
- missing message uses a safe default;
- write failures are recorded in job state;
- status output points to the latest output file without leaking unrelated file contents.

### Verification

```bash
go test ./...
agent-harness jobs run daily-checkin --instance default
agent-harness jobs list --instance default
cat ~/.local/share/agent-harness/instances/default/work/checkins.jsonl
```

---

## Step 07 — Delivery adapter spike for the first external notification

- **Status:** Completed
- **Branch:** `step-27-delivery-adapter-spike`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/29
- **Concept:** After local jobs are inspectable, add one narrow delivery abstraction before choosing richer content like daily briefs.

### Objective

Create a minimal delivery interface and one configured delivery implementation, so later scheduled jobs can send a message without hardcoding the transport into each job type.

### Scope

- Add a small delivery interface, conceptually:

  ```go
  type Notifier interface {
      Send(ctx context.Context, message Message) error
  }
  ```

- Start with one low-risk delivery mode: a local file outbox under the instance home.
- Add a `notify_test` job that sends a configured test message through the adapter.
- Do not add SMTP/email credentials in this step; no secrets are required for file-outbox delivery.
- Do not add daily brief content, Google Docs ingestion, or LLM generation in this step.

### Tests

Add deterministic tests for:

- notifier config validation;
- file outbox delivery path;
- delivery errors are recorded in job state;
- message contents are not dumped into `jobs list` output;
- jobs can depend on the notifier interface without knowing the concrete delivery transport.

### Verification

```bash
go test ./...
agent-harness jobs run notify-test --instance default
agent-harness jobs list --instance default
```

---

## Step 08 — Google OAuth account connection and secret-safe status

- **Status:** Completed
- **Branch:** `step-28-google-oauth-connection`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/30
- **Concept:** Before adding Gmail or Google Docs behavior, give each instance a secure, inspectable way to connect a Google account with minimum necessary OAuth scopes and without leaking tokens into config, status, logs, docs, or PRs.

### Objective

Add the Google account/auth foundation that future Google-backed jobs can reuse: configured OAuth client metadata, token storage under the instance home, connection/status commands, revoke/delete support, and secret-safe diagnostics.

### Scope

- Add Google OAuth config under `config/config.yaml` using paths/metadata only, not inline secrets, conceptually:

  ```yaml
  google:
    client_credentials_path: "config/secrets/google-client.json"
    token_path: "config/secrets/google-token.json"
    account_hint: "agent@example.com"
    scope_profile: "gmail_send"
  ```

- Create a restricted instance-owned secrets directory, for example `config/secrets/`, with documented `0600` file permissions for credentials/tokens.
- Add Google auth/status/revoke commands, conceptually:

  ```bash
  agent-harness google auth start --instance default
  agent-harness google auth status --instance default
  agent-harness google auth revoke --instance default
  ```

- Keep allowed scope profiles explicit and narrow. Initial profiles should match near-term planned behavior, for example:
  - `gmail_send`: Gmail send-only scope for a future notifier;
  - `docs_readonly`: Google Docs read-only scope for a future document source adapter.
- Do not request broad Drive/Gmail scopes unless a later step proves they are necessary.
- Do not add Gmail sending, Docs ingestion, scheduled jobs, daily-brief content, or model calls in this step.
- Status output may show safe metadata such as connected/disconnected, account email, configured scope profile, token expiry, and token file path. It must not print access tokens, refresh tokens, client secrets, authorization codes, or raw credential JSON.
- Add a revoke/delete path that removes local token material and documents how to revoke app access in the Google account UI.

### Tests

Add deterministic tests for:

- Google config validation accepts configured paths and known scope profiles;
- unknown scope profiles fail with `config-error` or controlled CLI errors;
- secret paths are constrained to the instance home unless explicitly allowed later;
- token/client-secret values are redacted from status, errors, and logs;
- revoke/delete removes the local token file without touching unrelated config;
- commands are testable with a fake OAuth/token client and do not require live Google in unit tests.

### Verification

```bash
go test ./...
agent-harness google auth status --instance default
agent-harness google auth revoke --instance default --confirm revoke-google-token
```

For live manual OAuth verification, use a throwaway/test Google account or the dedicated agent account and document the exact scopes requested before approving the consent screen.

---

## Step 09 — Gmail notifier delivery adapter

- **Status:** Completed
- **Branch:** `step-29-gmail-notifier`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/31
- **Concept:** Once Google auth is observable and revocable, add the first real network notifier by adapting the existing delivery interface to Gmail send-only delivery.

### Objective

Add a Gmail-backed notifier that can send a configured test notification through the same `Notifier` interface introduced in Step 07, using the Step 08 token handling and the narrow Gmail send scope.

### Scope

- Add a notifier config option, conceptually:

  ```yaml
  notifier:
    type: gmail
    gmail:
      from: "agent@example.com"
      to:
        - "operator@example.com"
      subject_prefix: "[agent-harness]"
  ```

- Reuse the existing `notify_test` job as the first Gmail smoke test instead of adding daily-brief content.
- Require the `gmail_send` scope profile and fail clearly if the connected token does not have the required scope.
- Keep file outbox available as the safe local notifier for tests and non-Google installs.
- Keep message bodies compact and avoid logging full delivered content by default.
- Do not add inbound Gmail reading, Docs ingestion, daily-brief sections, model calls, or Telegram delivery in this step.

### Tests

Add deterministic tests for:

- Gmail notifier config validation;
- missing/expired token returns a controlled delivery error;
- required Gmail send scope is enforced;
- Gmail API calls are made through a fake client in unit tests;
- job failure state records safe error metadata without credentials or full message body;
- file outbox notifier still works after adding Gmail support.

### Verification

```bash
go test ./...
agent-harness google auth status --instance default
agent-harness jobs run notify-test --instance default
agent-harness jobs list --instance default
```

Live verification should send one test email to a configured operator address, then confirm the job state records success without exposing OAuth tokens or raw API responses.

---

## Step 10 — Google OAuth token refresh for Google-backed actions

- **Status:** Completed
- **Branch:** `step-30-google-token-refresh`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/32
- **Concept:** Before adding more Google API consumers, make Google-backed actions refresh expired access tokens safely using the saved refresh token and configured client credentials.

### Objective

Add shared Google token-refresh plumbing that can be reused by Gmail and future Google Docs access, so scheduled/manual Google-backed jobs do not fail merely because the short-lived access token expired.

### Scope

- Detect expired or nearly expired access tokens before a Google API call.
- Use the saved refresh token plus configured OAuth client credentials to call Google's token endpoint.
- Rewrite the local token file with the fresh access token, updated expiry, preserved refresh token, and scopes.
- Wire the Gmail notifier through the shared helper so `notify_test` can send after a token refresh.
- Keep errors secret-safe and status-only; never print access tokens, refresh tokens, client secrets, raw token endpoint responses, or email bodies.
- Do not add Google Docs reading, daily brief content, model calls, Telegram delivery, or a broader OAuth scope profile in this step.

### Tests

Add deterministic tests for:

- expired token with refresh token calls a fake token endpoint before Gmail send;
- token refresh request uses expected OAuth form fields;
- refreshed access token is used for the Gmail API request;
- token file is rewritten with the fresh access token while preserving the refresh token;
- expired token without refresh token returns a controlled safe error.

### Verification

```bash
go test ./...
go build -o /tmp/agent-harness-step10 ./cmd/agent-harness
agent-harness jobs run notify-test --instance default
```

Live verification may use the dedicated agent Google account by forcing an expired local access token with a valid refresh token, then confirming one Gmail `notify_test` send succeeds after refresh without exposing token values.

---

## Step 11 — Configurable LLM settings and AI email job

- **Status:** Completed
- **Branch:** `step-31-ai-email-job`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/33
- **Concept:** Add the first model-backed scheduled job with safe global LLM defaults and per-job overrides, then send the bounded generated content through the existing notifier.

### Objective

Add an `ai_email` job type that can be run manually or by the daemon. The job should resolve a default provider/model, allow per-job provider/model selection without repeating credentials, generate bounded text from a configured prompt, and deliver that text through the already-configured notifier.

### Scope

- Add safe provider-scoped LLM config with default provider/model selection, conceptually:

  ```yaml
  llms:
    default:
      provider: openrouter
      model: openai/gpt-4o-mini
    providers:
      - provider: openrouter
        api_key_env: OPENROUTER_API_KEY
        models:
          - openai/gpt-4o-mini
          - anthropic/claude-3-5-haiku-latest
      - provider: openai
        api_key_env: OPENAI_API_KEY
        models:
          - gpt-4o-mini
          - gpt-4.1-mini
  ```

- Add per-job LLM provider/model selection without repeating key settings, conceptually:

  ```yaml
  jobs:
    - name: daily-ai-email
      type: ai_email
      enabled: true
      interval_seconds: 86400
      prompt: "Write a concise daily learning note for Zach."
      max_chars: 1200
    - name: weekly-ai-email
      type: ai_email
      enabled: true
      interval_seconds: 604800
      prompt: "Write a practical software engineering tip."
      max_chars: 1500
      llm:
        provider: openai
        model: gpt-4o-mini
  ```

- Keep profile support for multiple accounts on the same provider: by default a provider entry's profile name is the provider name; if a second account for the same provider is configured, it needs an explicit `profile`, and jobs can select that profile.
- Resolve LLM settings as: per-job `llm` selector first, default provider/model second.
- Keep raw API keys out of YAML job entries; provider/profile config may reference environment variable names or later secret-file paths only.
- Add a small fake-testable LLM client/interface for job execution.
- Bound generated content with `max_chars` before delivery.
- Send generated content through the existing notifier interface, so Gmail/file-outbox behavior stays shared.
- Record only safe job metadata in `state/jobs.json`; do not dump API keys, full provider responses, or full generated email bodies.
- Keep recipient selection in the existing notifier config for this step; do not add per-job recipients unless needed later.
- Do not add Google Docs reading, source adapters, daily brief rotations, Telegram delivery, or model-driven rich brief composition in this step.

### Tests

Add deterministic tests for:

- provider-scoped LLM config parsing and validation;
- per-job `llm` provider/model selector parsing and resolution;
- `ai_email` uses the default provider/model when no job override is selected;
- `ai_email` uses the selected provider/model when configured;
- duplicate provider entries without explicit profile names fail clearly;
- missing usable LLM config fails clearly for `ai_email` jobs;
- fake LLM output is bounded by `max_chars`;
- notifier receives the bounded generated content;
- LLM/notifier failures are recorded as failed job state with secret-safe errors;
- state/status output does not contain API keys or raw provider responses.

### Verification

```bash
go test ./...
go build -o /tmp/agent-harness-step11 ./cmd/agent-harness
agent-harness jobs run daily-ai-email --instance default
agent-harness jobs list --instance default --json
```

Live verification may use a cheap/default model profile and the dedicated agent Gmail account to send one manual `ai_email` smoke-test message to a configured recipient.

---

## Step 12 — Local background service for scheduled AI email

- **Status:** Completed
- **Branch:** `step-32-local-ai-email-service`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/34
- **Concept:** Install and run the app as a local background service on the agent machine with a real scheduled AI-email job.

### Objective

Configure the local instance on the agent machine so the daemon runs unattended and periodically sends AI-generated content to the user-specified recipient through Gmail.

### Scope

- Install/update the local binary from the merged app code.
- Configure the local instance home, conceptually:

  ```text
  ~/.local/share/agent-harness/
  ```

- Configure local secrets under `config/secrets/` or environment files with restricted permissions.
- Configure Gmail notifier recipients using the existing global notifier config, conceptually:

  ```yaml
  notifier:
    type: gmail
    gmail:
      from: "alf.fire9@gmail.com"
      to:
        - "user@example.com"
      subject_prefix: "[agent-harness]"
  ```

- Configure one temporary short-interval AI-email smoke job, then switch to the intended cadence after verification.
- Install/enable/restart the user-level service, conceptually:

  ```bash
  agent-harness service install
  systemctl --user enable --now agent-harness@default
  ```

- Verify daemon health, scheduled job state, Gmail delivery, and token refresh behavior.
- Keep local real recipient emails, API keys, OAuth tokens, client secrets, and machine-specific secret values out of committed files and PR text.
- Do not add new app features unless needed to make the already-supported config/service path work.

### Tests

This step is mostly operational, but should still include executable verification where possible:

- `go test ./...` from the merged code before install;
- service template/status command smoke tests if code/docs change;
- manual local verification that daemon status becomes healthy;
- manual local verification that a scheduled `ai_email` job succeeds;
- manual local verification that the received email content is AI-generated and bounded.

### Verification

```bash
go test ./...
go build -o /tmp/agent-harness-step12 ./cmd/agent-harness
agent-harness service install
systemctl --user enable --now agent-harness@default
systemctl --user status agent-harness@default
agent-harness status --instance default --json
agent-harness jobs list --instance default --json
```

Live verification should send at most one or two smoke-test emails, then restore the configured interval to the intended cadence.

---

## Step 13 — Time-of-day scheduling for daily jobs

- **Status:** Completed
- **Branch:** `step-33-time-of-day-scheduling`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/35
- **Concept:** Add wall-clock daily scheduling so background jobs can run at a configured local time instead of only every N seconds after their previous run.

### Objective

Let configured jobs run once per day at an explicit time and timezone, such as `08:00` in `America/New_York`, while preserving the existing `interval_seconds` behavior for simple interval jobs and smoke tests.

### Scope

- Extend job config with a daily schedule option, conceptually:

  ```yaml
  jobs:
    - name: daily-ai-email
      type: ai_email
      enabled: true
      schedule:
        daily_at: "08:00"
        timezone: "America/New_York"
      prompt: "Write a concise daily learning note."
      max_chars: 1200
  ```

- Keep `interval_seconds` supported for existing jobs and short smoke-test loops.
- Validate schedule config clearly:
  - `daily_at` must be a valid 24-hour `HH:MM` value;
  - `timezone` must be a valid IANA timezone name;
  - a job should use either `interval_seconds` or `schedule`, not both.
- Compute `next_run_at` from wall-clock time in the configured timezone.
- If today's configured time has already passed, schedule the next run for tomorrow.
- Preserve existing job-state shape where practical so `status` and `jobs list` keep working.
- Do not add richer cron syntax yet.
- Do not require Google Docs, release automation, or new content types in this step.

### Tests

Add deterministic tests for:

- parsing and validating daily schedule config;
- rejecting malformed times, unknown timezones, and mixed interval-plus-schedule config;
- computing the next run for later today;
- computing the next run for tomorrow when today's time already passed;
- honoring timezone boundaries rather than using only UTC;
- preserving existing interval-based job behavior.

### Verification

```bash
go test ./...
go build -o /tmp/agent-harness-step13 ./cmd/agent-harness
agent-harness daemon --instance default --test
agent-harness jobs list --instance default --json
```

Live verification can use a near-future wall-clock time with a harmless local/file notifier or bounded AI-email smoke job, then restore the intended daily time.

---

## Step 14 — Automated GitHub release binaries

- **Status:** Completed
- **Branch:** `step-34-automated-release-binaries`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/36
- **Concept:** Once the LLM email functionality works end-to-end and can be scheduled at a real daily time, publish the first versioned app binary so installs can use downloaded release artifacts instead of a source checkout and local `go build`.

### Objective

Add an automated GitHub release workflow that turns a version tag into downloadable, checksum-verified release assets for Linux installs. Use this to create the first app version after the `ai_email` job and local background-service smoke path are working.

### Scope

- Add a GitHub Actions release workflow, conceptually:

  ```text
  .github/workflows/release.yml
  ```

- Trigger release publishing from version tags such as:

  ```text
  v0.1.0
  ```

- Run the test suite before publishing assets:

  ```bash
  go test ./...
  ```

- Build at least the first Linux targets:

  ```text
  linux/amd64
  linux/arm64
  ```

- Package artifacts using the existing release shape, conceptually:

  ```text
  agent-harness_0.1.0_linux_amd64.tar.gz
  agent-harness_0.1.0_linux_arm64.tar.gz
  checksums.txt
  ```

- Embed version metadata from the tag, commit, build date, and dirty state.
- Upload release archives and `checksums.txt` as GitHub Release assets.
- Document the install/update flow as: download artifact, verify checksum, install binary, verify `agent-harness version`.
- Keep generated `dist/` artifacts out of git.
- Do not add self-update yet.
- Do not require macOS/Windows release artifacts in this first automated release step.

### Tests

Add deterministic/reviewable verification for:

- release workflow exists and is tag-triggered;
- release script/workflow uses the expected Linux targets;
- generated archive names/checksum docs match the documented install commands;
- version metadata is embedded in built binaries;
- `dist/` remains uncommitted/ignored;
- install docs do not contain machine-specific paths, tokens, or private URLs.

### Verification

```bash
go test ./...
scripts/build-release.sh 0.1.0
sha256sum -c dist/checksums.txt
```

After merge, live release verification should create a tag such as `v0.1.0`, confirm GitHub Actions publishes release assets, then install from the downloaded Linux asset on the agent machine and verify:

```bash
agent-harness version
agent-harness status --instance default --json
```

---

## Step 15 — Google Docs read-only source adapter

- **Status:** Completed
- **Branch:** `step-35-google-docs-source`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/37
- **Concept:** Add read-only document access as a reusable source adapter before building any rich daily-brief or summarization behavior.

### Objective

Read a configured Google Doc using a minimum-scope Google token and expose the retrieved text through a small source interface and debug command, without yet turning it into a scheduled content product.

### Scope

- Add a Google Docs source config, conceptually:

  ```yaml
  sources:
    google_docs:
      vocabulary_doc_id: "placeholder-doc-id"
  ```

- Read with a token that has Docs capability (`documents.readonly` or broader `documents`); keep scope-profile/scopes config as auth-generation guidance rather than proof that the current token has permission.
- Add a debug/read command that fetches safe metadata and text from a configured document, conceptually:

  ```bash
  agent-harness google docs read vocabulary_doc_id --instance default
  ```

- Keep document IDs configurable and placeholder-only in committed examples.
- Avoid storing full document content in status files or job state. If caching is added, store it under `cache/` with explicit docs and no credentials.
- Do not add model summaries, daily brief rendering, or schedule-driven Docs jobs in this step.

### Tests

Add deterministic tests for:

- Docs source config validation;
- missing/unknown document aliases fail clearly;
- required read-only scope is enforced;
- fake Docs client returns document text for parser/source tests;
- status/job output never dumps full document content;
- document IDs and API errors are handled without exposing tokens or credential JSON.

### Verification

```bash
go test ./...
agent-harness google auth status --instance default
agent-harness google docs read vocabulary_doc_id --instance default
```

Live verification should use a test/shared document with non-sensitive content first.

---

## Step 16 — Google OAuth scopes list config

- **Status:** Completed
- **Branch:** `step-36-google-scopes-list-config`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/38
- **Concept:** Replace the single `scope_profile` setting with a composable scopes list for Google OAuth token generation, while keeping the existing profile key as backward-compatible config.

### Objective

Make Google OAuth authorization easier to combine for jobs that need multiple Google capabilities, such as reading Docs and sending Gmail, without inventing combined profile names like `gmail_send_docs_readonly`.

### Scope

- Add first-class list config, conceptually:

  ```yaml
  google:
    scopes:
      - gmail_send
      - docs_readonly
  ```

- Expand known aliases to concrete OAuth scopes, for example:

  ```text
  gmail_send    -> https://www.googleapis.com/auth/gmail.send
  docs_readonly -> https://www.googleapis.com/auth/documents.readonly
  ```

- Keep `google.scope_profile` as a legacy/backward-compatible single-alias input.
- Prefer `google.scopes` when both `scopes` and `scope_profile` are present, and document that precedence clearly.
- Allow `google auth start` to request the union of configured scopes.
- Update `google auth status` to show requested aliases/scopes and actual token scopes without printing token values.
- Keep runtime operations focused on token/API capability rather than treating configured scopes as proof of permission.
- Do not add named Google connections in this step; keep that as a later extension if multiple accounts/token files become necessary.

### Tests

Add deterministic tests for:

- parsing `google.scopes` as a list;
- legacy `scope_profile` still works;
- `scopes` takes precedence over `scope_profile`;
- auth-start URLs include all requested concrete scopes;
- unknown scope aliases fail clearly;
- status/auth output remains secret-safe and does not print access or refresh tokens.

### Verification

```bash
go test ./...
agent-harness google auth start --instance default
agent-harness google auth status --instance default
```

Use placeholder/non-sensitive config examples only; do not commit token files or client credentials.

---

## Step 17 — Durable rotating item cursors for document-backed sources

- **Status:** Planned
- **Branch:** `step-37-durable-rotating-item-cursors`
- **Pull Request:** TBD
- **Concept:** Add generic durable progress tracking for sources that should emit one item/card at a time across scheduled runs, before wiring that behavior into a delivery job.

### Objective

Add a reusable progress/cursor layer that can select the next item from a parsed source, persist that position across daemon restarts, wrap after the last item, and remain broad enough for vocabulary words, title-plus-bullet cards, quotes, prompts, and other rotating personal-knowledge sources.

### Scope

- Add a small persistent progress store under instance state, conceptually:

  ```text
  state/progress/<progress-key>.json
  ```

- Add a generic round-robin selector that operates on parsed item IDs rather than vocabulary-specific records.
- Support a cycle snapshot model:
  - parse the source into ordered items/cards at the start of a cycle;
  - persist the ordered item IDs and cursor;
  - advance through that snapshot until exhausted;
  - refresh the snapshot only at the next cycle boundary so doc edits do not unexpectedly skip/repeat items mid-cycle.
- Add parser primitives for at least:
  - `non_empty_lines` for simple one-word/one-item-per-line documents;
  - `heading_with_bullets` or an equivalent card parser that can represent a title plus bullet-point body.
- Keep parser output generic, conceptually:

  ```json
  {
    "id": "perspicacious",
    "title": "Perspicacious",
    "body": [
      "having keen mental perception",
      "example sentence..."
    ]
  }
  ```

- Advance the cursor only after the caller reports successful delivery or completion; failed sends should retry the same item next time.
- Store progress metadata and item IDs, not full Google Doc contents or delivered message bodies, in durable state.
- Keep this step source/selection focused. Do not add the scheduled email/digest job yet, and do not call an LLM.

### Tests

Add deterministic tests for:

- round-robin selection advances from item to item and wraps after the last item;
- state survives reloads from disk and writes atomically;
- `non_empty_lines` parsing trims blank lines and creates stable item IDs;
- title-plus-bullet parsing creates stable cards with title/body fields;
- snapshot refresh occurs only at cycle boundaries;
- cursor does not advance when completion/delivery fails;
- progress state does not persist full source text, credentials, or delivered message bodies.

### Verification

```bash
go test ./...
agent-harness progress show vocabulary-word-daily --instance default
agent-harness sources items preview vocabulary_doc_id --parser non_empty_lines --instance default
```

The CLI names are illustrative; keep this slice small and prefer tests over broad user-facing commands if the command surface starts to grow.

---

## Step 18 — First Google-backed scheduled digest job

- **Status:** Planned
- **Branch:** `step-38-google-doc-digest-job`
- **Pull Request:** TBD
- **Concept:** Combine the scheduler, Google Docs source adapter, durable rotating item cursors, and notifier with a small deterministic digest before introducing richer daily-brief logic or model calls.

### Objective

Add a narrow scheduled job that reads one configured Google Doc, selects the next parsed item/card with durable round-robin progress, renders a deterministic short message, and sends it through the configured notifier.

### Scope

- Add a job type such as `google_doc_digest`, conceptually:

  ```yaml
  jobs:
    - name: vocabulary-digest
      type: google_doc_digest
      enabled: true
      interval_seconds: 86400
      source: vocabulary_doc_id
      parser: non_empty_lines
      selection:
        strategy: round_robin
        progress_key: vocabulary-word-daily
      notifier: default
      max_items: 1
  ```

- Keep rendering deterministic and templated. Do not call an LLM in this step.
- Use one configured source and a small bounded output so delivery can be verified safely.
- Record normal job metadata in `state/jobs.json` and progress metadata in `state/progress/`, but do not store full document content or full delivered message there.
- Advance the progress cursor only after the notifier reports success.
- Keep this separate from a full daily brief. Additional sections, enrichment, model polish, and multiple source documents should be later steps.

### Tests

Add deterministic tests for:

- configured Google Doc digest job reads through the source interface;
- digest selects the next item/card through the durable progress layer;
- digest rendering is bounded and deterministic;
- notifier receives the expected compact message through a fake notifier;
- source/notifier failures are recorded as failed job state with secret-safe errors and do not advance progress;
- disabled digest jobs do not fetch Docs or send notifications;
- no model calls are made.

### Verification

```bash
go test ./...
agent-harness jobs run vocabulary-digest --instance default
agent-harness jobs list --instance default
agent-harness status --instance default --json
```

Live verification should start with manual `jobs run` before enabling the digest on an unattended interval.

---

## Future TODO — macOS and Windows runtime adapters

- **Status:** Future
- **Branch:** TBD
- **Pull Request:** TBD
- **Concept:** After the Linux/systemd runtime and versioned install flow are proven, add OS-specific service adapters while preserving the same user-facing commands and instance-home model.

### Objective

Support the same core commands on additional operating systems:

```bash
agent-harness init --instance default
agent-harness daemon --instance default
agent-harness status --instance default
agent-harness version
```

### Future macOS scope

- Add macOS path defaults:

  ```text
  ~/Library/Application Support/agent-harness/instances/<instance>/
  ```

- Add a `launchd` service-manager adapter.
- Generate or document per-user LaunchAgent plists under:

  ```text
  ~/Library/LaunchAgents/
  ```

- Document commands equivalent to:

  ```bash
  launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.agent-harness.default.plist
  launchctl print gui/$(id -u)/com.agent-harness.default
  launchctl bootout gui/$(id -u)/com.agent-harness.default
  ```

### Future Windows scope

- Add Windows path defaults:

  ```text
  %LOCALAPPDATA%\agent-harness\instances\<instance>\
  ```

- Add a Windows Service or Task Scheduler adapter.
- Ensure `agent-harness.exe status --instance default --json` stays compatible with the same status-file schema.
- Keep PowerShell install/update/status docs separate from Linux and macOS docs.

### Guardrails

- Do not fork daemon/job logic by OS.
- Keep OS-specific code limited to path defaults, service-manager integration, install docs, and platform-specific tests.
- Do not make macOS or Windows support a prerequisite for the Linux runtime MVP.

---

## Resolved Step 01 decisions

Use these decisions when implementing Step 01:

1. **Runtime manager:** Step 01 supports `systemd --user` only. Do not document cron/nohup as supported fallback runtime managers.
2. **Default instance name:** Use `default` as the default example instance name throughout docs, tests, and generated examples.
3. **Status behavior:** `agent-harness status` should read app-owned status files and print service-manager hints. It should not shell out to `systemctl` by default in Step 01.
4. **Runtime abstractions:** Step 01 should create small internal path/service abstractions for testability and future OS adapters, with Linux/systemd as the only concrete service-manager implementation.
5. **Health check:** Step 01 should use heartbeat/status files only. Do not add a local HTTP health endpoint in the MVP.

## Suggested next scope after Step 07

After Step 07, continue with Google integration and release packaging in thin, security-first slices instead of jumping straight to a full daily brief, broad account access, Telegram, AWS, or model calls:

```text
Step 08 — Google OAuth account connection and secret-safe status
Step 09 — Gmail notifier delivery adapter
Step 10 — Google OAuth token refresh for Google-backed actions
Step 11 — Configurable LLM settings and AI email job
Step 12 — Local background service for scheduled AI email
Step 13 — Time-of-day scheduling for daily jobs
Step 14 — Automated GitHub release binaries
Step 15 — Google Docs read-only source adapter
Step 16 — First Google-backed scheduled digest job
```

That sequence keeps the next PRs focused on minimum necessary access, revocable OAuth credentials, observable external delivery, one bounded AI-generated email job, real wall-clock daily scheduling, a downloadable first app version, and deterministic source/digest behavior before adding richer daily-brief content, rotations, enrichment, broad scopes, or alternate delivery channels.
