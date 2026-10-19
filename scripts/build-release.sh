#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-}"
if [[ -z "${VERSION}" ]]; then
  echo "usage: $0 <version>" >&2
  exit 2
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${ROOT_DIR}/dist"
SANITIZED_VERSION="$(VERSION="${VERSION}" python3 - <<'PY'
import os, re
version = os.environ.get('VERSION', '').strip() or 'dev'
print(re.sub(r'[^A-Za-z0-9._-]', '_', version))
PY
)"
COMMIT="$(git -C "${ROOT_DIR}" rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
DIRTY="false"
if ! git -C "${ROOT_DIR}" diff --quiet --ignore-submodules -- 2>/dev/null || ! git -C "${ROOT_DIR}" diff --cached --quiet --ignore-submodules -- 2>/dev/null; then
  DIRTY="true"
fi

mkdir -p "${DIST_DIR}"
rm -f "${DIST_DIR}"/agent-harness_"${SANITIZED_VERSION}"_*.tar.gz "${DIST_DIR}"/agent-harness_"${SANITIZED_VERSION}"_*.zip "${DIST_DIR}/checksums.txt"

LDFLAGS="-X github.com/zachfire9/agent-harness/internal/version.Version=${VERSION} -X github.com/zachfire9/agent-harness/internal/version.Commit=${COMMIT} -X github.com/zachfire9/agent-harness/internal/version.BuildDate=${BUILD_DATE} -X github.com/zachfire9/agent-harness/internal/version.Dirty=${DIRTY}"

targets=(
  "linux amd64"
  "linux arm64"
  "windows amd64"
)

for target in "${targets[@]}"; do
  read -r goos goarch <<<"${target}"
  work_dir="$(mktemp -d)"
  binary_name="agent-harness"
  archive="${DIST_DIR}/agent-harness_${SANITIZED_VERSION}_${goos}_${goarch}.tar.gz"
  if [[ "${goos}" == "windows" ]]; then
    binary_name="agent-harness.exe"
    archive="${DIST_DIR}/agent-harness_${SANITIZED_VERSION}_${goos}_${goarch}.zip"
  fi
  (
    cd "${ROOT_DIR}"
    GOOS="${goos}" GOARCH="${goarch}" CGO_ENABLED=0 go build -trimpath -ldflags "${LDFLAGS}" -o "${work_dir}/${binary_name}" ./cmd/agent-harness
  )
  cp "${ROOT_DIR}/docs/install-release.md" "${work_dir}/INSTALL.md"
  if [[ "${goos}" == "windows" ]]; then
    cp "${ROOT_DIR}/docs/windows-runtime.md" "${work_dir}/WINDOWS.md"
    cp "${ROOT_DIR}/docs/windows-first-conversation.md" "${work_dir}/FIRST-CONVERSATION.md"
    python3 - "${work_dir}" "${archive}" <<'PY'
import pathlib, sys, zipfile
root, archive = pathlib.Path(sys.argv[1]), sys.argv[2]
with zipfile.ZipFile(archive, 'w', zipfile.ZIP_DEFLATED) as output:
    for name in ('agent-harness.exe', 'INSTALL.md', 'WINDOWS.md', 'FIRST-CONVERSATION.md'):
        output.write(root / name, name)
PY
  else
    tar -C "${work_dir}" -czf "${archive}" "${binary_name}" "INSTALL.md"
  fi
  rm -rf "${work_dir}"
  echo "built ${archive}"
done

(
  cd "${DIST_DIR}"
  sha256sum agent-harness_"${SANITIZED_VERSION}"_*.tar.gz agent-harness_"${SANITIZED_VERSION}"_*.zip > checksums.txt
)

echo "wrote ${DIST_DIR}/checksums.txt"
