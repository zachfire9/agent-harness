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
- next run time.

Multiple enabled jobs can be configured independently. Disabled jobs are skipped. Unknown job types or invalid intervals are treated as configuration errors and are surfaced through status output. For backward compatibility, older configs with `heartbeat_job_interval_seconds` still configure the default heartbeat job when no `jobs:` list is present.

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
```

Manual runs update `state/jobs.json` just like scheduled daemon runs. Unknown jobs, disabled jobs, and invalid job config return controlled non-zero errors. The command does not require `systemd` or a background daemon, which makes it useful for debugging instance config before enabling the service.

Future job types should reuse this config/state shape instead of creating job-specific status files.

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

The generated template is:

```ini
[Unit]
Description=agent-harness instance %i
After=network-online.target

[Service]
Type=simple
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
