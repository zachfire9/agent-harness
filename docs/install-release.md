# Installing a Release Artifact

Use this flow on machines that should run `agent-harness` without a source checkout or Go toolchain.

Release artifacts are built under `dist/` by:

```bash
./scripts/build-release.sh 0.1.0-dev
```

For published versions, GitHub Releases is the source of truth. The local `dist/` directory is generated output for staging/upload only; do not commit release archives or checksums to git.

Each published version should have:

```text
Git tag:        v0.1.0
GitHub Release: v0.1.0
Assets:
  agent-harness_0.1.0_linux_amd64.tar.gz
  agent-harness_0.1.0_linux_arm64.tar.gz
  checksums.txt
```

The release tag uses a leading `v`, while the embedded binary version omits it:

```text
Tag:            v0.1.0
Binary version: 0.1.0
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

## Download a published version

Pick the version and architecture for the target machine. For example:

```bash
VERSION=0.1.0
ARCH=linux_amd64

curl -L -o /tmp/agent-harness.tar.gz \
  "https://github.com/zachfire9/agent-harness/releases/download/v${VERSION}/agent-harness_${VERSION}_${ARCH}.tar.gz"

curl -L -o /tmp/checksums.txt \
  "https://github.com/zachfire9/agent-harness/releases/download/v${VERSION}/checksums.txt"
```

Then verify and install from `/tmp`:

```bash
cd /tmp
sha256sum -c checksums.txt --ignore-missing
tar -xzf agent-harness.tar.gz
install -m 0755 agent-harness ~/.local/bin/agent-harness
~/.local/bin/agent-harness version
systemctl --user restart agent-harness@default.service
~/.local/bin/agent-harness status --instance default
```

## Install or update from an artifact

If the artifact is already present locally, pick the archive that matches the target machine architecture, then run:

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

## Publishing a release

For the current manual release process:

```bash
VERSION=0.1.0

git tag "v${VERSION}"
./scripts/build-release.sh "${VERSION}"
gh release create "v${VERSION}" \
  dist/agent-harness_${VERSION}_linux_amd64.tar.gz \
  dist/agent-harness_${VERSION}_linux_arm64.tar.gz \
  dist/checksums.txt \
  --title "v${VERSION}" \
  --generate-notes
```

Development rules:

- Do not commit generated `dist/` artifacts.
- Every published release should be tied to a git tag.
- The release tag should be `v<version>` and the binary version should be `<version>`.
- Upload `checksums.txt` with every release.
- Target machines should not need Go or a source checkout.
- Instance homes must stay separate from binary and version storage.
- Publishing is manual for now; a later step can move this into GitHub Actions on tag push.

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
