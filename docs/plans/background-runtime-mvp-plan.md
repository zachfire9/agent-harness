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
- **Pull Request:** TBD
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

- **Status:** Pending
- **Branch:** `step-26-local-checkin-job`
- **Pull Request:** TBD
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

- **Status:** Pending
- **Branch:** `step-27-delivery-adapter-spike`
- **Pull Request:** TBD
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

- Start with one low-risk delivery mode chosen at implementation time, such as:
  - local file outbox under the instance home; or
  - SMTP/email if credentials and target address are explicitly configured.
- If SMTP/email is selected, keep credentials in the instance config/env file and redact them from status, logs, traces, and test failures.
- Add a `notify_test` job or manual command that sends a configured test message through the adapter.
- Do not add daily brief content, Google Docs ingestion, or LLM generation in this step.

### Tests

Add deterministic tests for:

- notifier config validation;
- file outbox or fake SMTP delivery path;
- delivery errors are recorded in job state;
- secrets are redacted from logs/status/errors;
- jobs can depend on the notifier interface without knowing the concrete delivery transport.

### Verification

```bash
go test ./...
agent-harness jobs run notify-test --instance default
agent-harness jobs list --instance default
```

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

## Suggested next scope after Step 03

After Step 03, continue with the job system in thin slices instead of jumping straight to email, Google Docs, Telegram, AWS, model calls, or daily-brief content:

```text
Step 04 — Configurable job registry and schedules
Step 05 — Job inspection and manual run commands
Step 06 — First useful local job: append a timestamped check-in file
Step 07 — Delivery adapter spike for the first external notification
```

That keeps the next PRs focused on reusable scheduling plumbing and local verification before adding external integrations or richer content.
