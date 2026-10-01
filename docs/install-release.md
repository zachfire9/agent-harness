# Installing a Release Artifact

Use this flow on machines that should run `agent-harness` without a source checkout or Go toolchain.

Release artifacts are built under `dist/` by:

```bash
./scripts/build-release.sh 0.1.0-dev
```

The first required targets are:

```text
linux-amd64
linux-arm64
```

Future targets may include:

```text
darwin-arm64
darwin-amd64
windows-amd64
```

## Artifact layout

A Linux artifact is named like:

```text
agent-harness_0.1.0-dev_linux_amd64.tar.gz
```

Each archive contains:

```text
agent-harness
INSTALL.md
```

Checksums are written to:

```text
dist/checksums.txt
```

## Install or update from an artifact

Pick the artifact that matches the target machine architecture, then run:

```bash
tar -xzf agent-harness_0.1.0-dev_linux_amd64.tar.gz
install -m 0755 agent-harness ~/.local/bin/agent-harness
~/.local/bin/agent-harness version
systemctl --user restart agent-harness@default.service
~/.local/bin/agent-harness status --instance default
```

The binary lives separately from runtime data:

```text
Binary:        ~/.local/bin/agent-harness
Instance home: ~/.local/share/agent-harness/instances/<instance>/
```

Do not run the daemon directly from a release extraction directory for normal background use. Install the binary to `~/.local/bin/agent-harness`, then run it through the user service.

## Verify checksum before installing

If `checksums.txt` is available alongside the archive:

```bash
sha256sum -c checksums.txt
```

## Roll back

Keep the previous binary before replacing it if you want a manual rollback point:

```bash
cp ~/.local/bin/agent-harness ~/.local/bin/agent-harness.previous
install -m 0755 agent-harness ~/.local/bin/agent-harness
```

Rollback:

```bash
install -m 0755 ~/.local/bin/agent-harness.previous ~/.local/bin/agent-harness
systemctl --user restart agent-harness@default.service
~/.local/bin/agent-harness status --instance default
```

## Development install from source

Use this only on development hosts that update directly from a source checkout:

```bash
git -C <repo-path> pull --ff-only
go test ./...
go build -o ~/.local/bin/agent-harness ./cmd/agent-harness
~/.local/bin/agent-harness version
systemctl --user restart agent-harness@default.service
~/.local/bin/agent-harness status --instance default
```

Release-based installs should use the artifact flow above instead of requiring Go on the target machine.
