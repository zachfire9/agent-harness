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
Cache:         ~/.local/share/agent-harness/instances/<instance>/cache/
Work dir:      ~/.local/share/agent-harness/instances/<instance>/work/
App logs:      ~/.local/share/agent-harness/instances/<instance>/logs/
Service logs:  systemd journal for the user service
Unit file:     ~/.config/systemd/user/agent-harness@.service
```

The compiled binary should be installed separately from runtime data. Do not run the daemon from a source checkout or release extraction directory for normal background use.

## Initialize an instance

```bash
agent-harness init --instance default
```

For tests or custom installs, pass an explicit instance home:

```bash
agent-harness init --instance default --home ~/.local/share/agent-harness/instances/default
```

## Run one heartbeat smoke check

```bash
agent-harness daemon --instance default --once
agent-harness status --instance default
agent-harness status --instance default --json
```

`--once` writes a single running heartbeat/status file and exits. Normal background use runs without `--once` under the service manager.

## systemd user service

Install this template as:

```text
~/.config/systemd/user/agent-harness@.service
```

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

Enable and start an instance:

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

```bash
git pull --ff-only
go test ./...
go build -o ~/.local/bin/agent-harness ./cmd/agent-harness
systemctl --user restart agent-harness@default.service
agent-harness status --instance default
```

If Go is installed outside `PATH`, use that local Go binary explicitly when running the test/build commands.
