# Background Runtime MVP Implementation Plan

> **Purpose:** Start the next `agent-harness` direction from scratch with a smaller operational MVP: first make the agent installable as a background process and easy to inspect, then add useful scheduled behavior later.

## Goal

Build a generic background runtime for `agent-harness` that can initially run on the same Linux machine Alf is running on, while remaining reusable for other `agent-harness` instances on other machines.

The first useful deliverable is **not** a daily brief or birthday email. The first deliverable is:

- `agent-harness` can run unattended in the background;
- it has a stable instance name/config/state/log layout;
- Alf can check whether it is running, stopped, failing, or misconfigured;
- the same mechanism can be installed for another instance without hardcoding this machine.

## Non-goals for the MVP

Do not include these in the first step:

- daily brief content;
- birthday countdown content;
- Gmail/Google Docs integration;
- Telegram delivery;
- AWS/remote deployment;
- model calls;
- rich scheduling logic beyond keeping the process alive and observable.

Those can be added after the background runtime is reliable.

## Operating model

Initial target:

- Linux machine where Alf is currently running.
- User-level service preferred over root/system service.
- `systemd --user` is the first runtime manager.

Generic design requirement:

- Nothing should be hardcoded to Alf's current username, repo checkout, home directory, or machine name.
- Runtime paths should be derived from config, flags, or XDG-style defaults.
- Every service should have an explicit instance name, for example `default`, `daily-brief`, or `birthday-countdown`.
- Status checks should work the same way for this machine and future instances.

Recommended default paths:

```text
Binary:       ~/.local/bin/agent-harness
Config dir:   ~/.config/agent-harness/<instance>/
State dir:    ~/.local/state/agent-harness/<instance>/
Log source:   systemd journal for the user service
Unit file:    ~/.config/systemd/user/agent-harness@.service
```

If the implementation uses different paths, document why and keep them instance-aware.

---

# Step checklist

## Step 01 — Background runtime and status checks

- **Status:** Pending
- **Branch:** `step-21-background-runtime-status`
- **Pull Request:** TBD
- **Concept:** Before an agent can do useful scheduled work, it needs a reliable operational shell: install, start, stop, restart, status, logs, health, and update instructions.

### Objective

Update `agent-harness` so it can run as a background process under a generic instance name and expose clear status/error checks that Alf can run on this machine and reuse for other instances later.

### Scope

Add the smallest operational runtime that proves `agent-harness` can stay alive and be inspected:

- Add a long-running command, for example:

  ```bash
  agent-harness daemon --instance default
  ```

- Add a status command, for example:

  ```bash
  agent-harness status --instance default
  ```

- Add service-management documentation or generated unit content for a user-level systemd template:

  ```text
  agent-harness@.service
  ```

- Support commands equivalent to:

  ```bash
  systemctl --user daemon-reload
  systemctl --user enable --now agent-harness@default.service
  systemctl --user status agent-harness@default.service --no-pager
  journalctl --user -u agent-harness@default.service -n 100 --no-pager
  ```

- The daemon should write a small status/health file under the configured instance state directory, for example:

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
agent-harness daemon --instance default
agent-harness daemon --instance birthday-countdown
agent-harness status --instance default
agent-harness status --instance birthday-countdown
systemctl --user status agent-harness@default.service --no-pager
systemctl --user status agent-harness@birthday-countdown.service --no-pager
```

Instance names should be validated to avoid path traversal or unsafe systemd unit names. Keep allowed names simple, for example letters, numbers, `_`, and `-`.

### Files likely to change

Exact paths may vary after inspecting the current code, but expect something like:

- Modify: `cmd/agent-harness/main.go`
- Modify/create: `internal/cli/...`
- Create: `internal/runtime/daemon.go`
- Create: `internal/runtime/status.go`
- Create: `internal/runtime/paths.go`
- Create: `internal/runtime/service.go`
- Create: `internal/runtime/*_test.go`
- Create: `docs/background-runtime.md`
- Modify: `README.md`
- Modify: `.gitignore` if local runtime artifacts can appear in the repo

### Tests

Add deterministic unit tests for:

- valid and invalid instance names;
- deriving config/state paths from an instance name;
- writing and reading a status file;
- stale heartbeat detection;
- status classification for running/stopped/stale/config-error inputs;
- JSON status output shape;
- service unit template rendering does not include machine-specific hardcoding;
- signal/shutdown behavior where practical without flaky sleeps.

Do not require real `systemd` in unit tests. Test unit rendering and status classification with fakes.

### Manual verification on Alf's current machine

After Step 01 is implemented, Alf should be able to run:

```bash
go test ./...
go build -o ~/.local/bin/agent-harness ./cmd/agent-harness
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

- service is active;
- `agent-harness status --instance default` reports `running`;
- JSON status is parseable;
- journal logs show startup without secrets;
- stopping the service changes status to stopped/stale/unknown in a documented way;
- a deliberately bad config produces `config-error` and visible logs.

### Update workflow for future changes

Document a generic update path that Alf can run for this instance and adapt for others:

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
- config directory
- state directory
- systemd unit name

### Acceptance criteria

Step 01 is complete when:

- the background daemon can be started by a user-level service on this machine;
- the same service template can run another named instance;
- Alf can check status with one `agent-harness status --instance ...` command;
- Alf can inspect logs with one documented `journalctl` command;
- failed startup/config errors are visible without reading source code;
- all new behavior has tests or documented manual verification where true background process behavior is involved.

---

## Step 02 — Minimal scheduled job hook

- **Status:** Pending
- **Branch:** `step-22-minimal-scheduled-job-hook`
- **Pull Request:** TBD
- **Concept:** Once the process can run in the background, add a tiny generic job loop without tying it to a specific content feature.

### Objective

Add a minimal no-op or heartbeat job mechanism so a running instance can prove it wakes up on an interval and records success/failure metadata.

### Scope

- Add config for an interval or cron-like schedule.
- Add a single built-in `noop` or `heartbeat` job.
- Record last run time, last success, last error, and next run time in instance state.
- Surface job status through `agent-harness status`.
- Keep the design generic so later birthday/email/daily-brief jobs plug in as job types.

### Verification

```bash
go test ./...
systemctl --user restart agent-harness@default.service
agent-harness status --instance default
journalctl --user -u agent-harness@default.service -n 100 --no-pager
```

---

## Step 03 — First useful local job TBD

- **Status:** Pending
- **Branch:** `step-23-first-useful-local-job`
- **Pull Request:** TBD
- **Concept:** Add the first user-visible behavior only after the background runtime and status checks are reliable.

### Candidate options

Pick one later:

- configurable birthday countdown email;
- daily brief email;
- local file/log reminder;
- another small scheduled task Zach wants first.

Do not decide this in Step 01. Step 01 should stay focused on the runtime shell.

---

## Open decisions

Before implementing Step 01, confirm:

1. Should the first runtime manager be only `systemd --user`, or should docs also include cron/nohup as unsupported fallbacks?
2. What should the default instance name be: `default`, `alf`, or something else?
3. Should `agent-harness status` shell out to `systemctl` when available, or only read its own status file and tell Alf which `systemctl` command to run?
4. Should the daemon command do nothing except heartbeat in Step 01, or should it expose a tiny local health endpoint too?

## Suggested first approved scope

If Zach approves this plan, start with only Step 01:

```text
Step 01 — Background runtime and status checks
```

That gives a reusable operational base before adding birthday countdowns, Gmail, Google Docs, Telegram, AWS, or model calls.
