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

## Configure the built-in heartbeat job

`agent-harness init` creates a starter config file:

```text
~/.local/share/agent-harness/instances/default/config/config.yaml
```

Step 03 includes one built-in generic job, `heartbeat`, so the daemon can prove it wakes up on an interval before any real content jobs exist. Configure its interval with:

```yaml
heartbeat_job_interval_seconds: 60
```

The daemon records job metadata in:

```text
~/.local/share/agent-harness/instances/default/state/jobs.json
```

The heartbeat job records:

- job name and type;
- last run time;
- last success time;
- last error, if any;
- next run time.

Future job types should reuse this state shape instead of creating job-specific status files.

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
