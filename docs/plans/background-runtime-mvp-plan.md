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

- **Status:** Pending
- **Branch:** `step-21-background-runtime-status`
- **Pull Request:** TBD
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

- **Status:** Pending
- **Branch:** `step-22-versioned-binary-install`
- **Pull Request:** TBD
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
- the plan leaves room for later GoReleaser/GitHub Releases automation without requiring it now.

---

## Step 03 — Minimal scheduled job hook

- **Status:** Pending
- **Branch:** `step-23-minimal-scheduled-job-hook`
- **Pull Request:** TBD
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

## Step 04 — First useful local job TBD

- **Status:** Pending
- **Branch:** `step-24-first-useful-local-job`
- **Pull Request:** TBD
- **Concept:** Add the first user-visible behavior only after the background runtime and status checks are reliable.

### Candidate options

Pick one later:

- configurable email job;
- daily brief email;
- local file/log reminder;
- another small scheduled task Zach wants first.

Do not decide this in Step 01. Step 01 should stay focused on the runtime shell.

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

## Open decisions

Before implementing Step 01, confirm:

1. Should the first runtime manager be only `systemd --user`, or should docs also include cron/nohup as unsupported fallbacks?
2. What should the default instance name be: `default`, `local`, or something else?
3. Should `agent-harness status` shell out to `systemctl` when available, or only read its own status file and print the relevant `systemctl` command for the operator to run?
4. Should Step 01 create explicit `PathResolver` and `ServiceManager` interfaces immediately, or keep them as small internal structs until the first non-Linux adapter is implemented?
5. Should the daemon command do nothing except heartbeat in Step 01, or should it expose a tiny local health endpoint too?

## Suggested first approved scope

If Zach approves this plan, start with only Step 01:

```text
Step 01 — Background runtime and status checks
```

That gives a reusable operational base before adding Gmail, Google Docs, Telegram, AWS, model calls, or any content-specific scheduled job.
