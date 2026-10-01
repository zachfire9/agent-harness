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
rm -f "${DIST_DIR}"/agent-harness_"${SANITIZED_VERSION}"_*.tar.gz "${DIST_DIR}/checksums.txt"

LDFLAGS="-X github.com/zachfire9/agent-harness/internal/version.Version=${VERSION} -X github.com/zachfire9/agent-harness/internal/version.Commit=${COMMIT} -X github.com/zachfire9/agent-harness/internal/version.BuildDate=${BUILD_DATE} -X github.com/zachfire9/agent-harness/internal/version.Dirty=${DIRTY}"

targets=(
  "linux amd64"
  "linux arm64"
)

for target in "${targets[@]}"; do
  read -r goos goarch <<<"${target}"
  work_dir="$(mktemp -d)"
  binary_name="agent-harness"
  archive="${DIST_DIR}/agent-harness_${SANITIZED_VERSION}_${goos}_${goarch}.tar.gz"
  (
    cd "${ROOT_DIR}"
    GOOS="${goos}" GOARCH="${goarch}" CGO_ENABLED=0 go build -trimpath -ldflags "${LDFLAGS}" -o "${work_dir}/${binary_name}" ./cmd/agent-harness
  )
  cp "${ROOT_DIR}/docs/install-release.md" "${work_dir}/INSTALL.md"
  tar -C "${work_dir}" -czf "${archive}" "${binary_name}" "INSTALL.md"
  rm -rf "${work_dir}"
  echo "built ${archive}"
done

(
  cd "${DIST_DIR}"
  sha256sum agent-harness_"${SANITIZED_VERSION}"_*.tar.gz > checksums.txt
)

echo "wrote ${DIST_DIR}/checksums.txt"
