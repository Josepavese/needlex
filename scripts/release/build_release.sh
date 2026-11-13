#!/usr/bin/env bash
set -euo pipefail

OUT_DIR="${1:-dist}"
mkdir -p "${OUT_DIR}"
OUT_DIR="$(cd "${OUT_DIR}" && pwd)"
VERSION="${NEEDLEX_VERSION:-dev}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# The shipped agent skill carries its own version marker. A release whose binary
# and skill disagree would tell every installed agent that its copy is stale, so
# the build refuses to produce such artifacts.
verify_skill_version() {
  local skill_file="${REPO_ROOT}/skills/needlex-web-retrieval/SKILL.md"
  local skill_version
  if [[ ! -f "${skill_file}" ]]; then
    echo "shipped skill not found: ${skill_file}" >&2
    exit 1
  fi
  skill_version="$(sed -n 's/^version:[[:space:]]*//p' "${skill_file}" | head -1 | tr -d '[:space:]')"
  if [[ -z "${skill_version}" ]]; then
    echo "skill version marker missing in ${skill_file}" >&2
    exit 1
  fi
  if [[ "${VERSION}" == "dev" ]]; then
    return 0
  fi
  if [[ "${skill_version#v}" != "${VERSION#v}" ]]; then
    echo "skill version ${skill_version} does not match release version ${VERSION}" >&2
    exit 1
  fi
}

verify_skill_version

# Native archives must carry the project license and the third-party notices.
# A release without them is not redistributable, so the build fails instead.
for required in LICENSE THIRD_PARTY_NOTICES.txt; do
  if [[ ! -s "${REPO_ROOT}/${required}" ]]; then
    echo "release material missing: ${REPO_ROOT}/${required}" >&2
    exit 1
  fi
done

build_one() {
  local goos="$1"
  local goarch="$2"
  local bin_name="needlex"
  local archive_name="needlex_${goos}_${goarch}"
  local work_dir
  work_dir="$(mktemp -d)"

  if [[ "${goos}" == "windows" ]]; then
    bin_name="needlex.exe"
  fi

  GOOS="${goos}" GOARCH="${goarch}" go build -ldflags "-X github.com/josepavese/needlex/internal/platform/buildinfo.Version=${VERSION}" -o "${work_dir}/${bin_name}" ./cmd/needle
  cp "${REPO_ROOT}/LICENSE" "${REPO_ROOT}/THIRD_PARTY_NOTICES.txt" "${work_dir}/"

  if [[ "${goos}" == "windows" ]]; then
    (
      cd "${work_dir}"
      zip -q "${OUT_DIR}/${archive_name}.zip" "${bin_name}" LICENSE THIRD_PARTY_NOTICES.txt
    )
    bash "${REPO_ROOT}/scripts/release/check_archive_licenses.sh" "${OUT_DIR}/${archive_name}.zip"
  else
    tar -C "${work_dir}" -czf "${OUT_DIR}/${archive_name}.tar.gz" "${bin_name}" LICENSE THIRD_PARTY_NOTICES.txt
    bash "${REPO_ROOT}/scripts/release/check_archive_licenses.sh" "${OUT_DIR}/${archive_name}.tar.gz"
  fi

  rm -rf "${work_dir}"
}

build_one linux amd64
build_one linux arm64
build_one darwin amd64
build_one darwin arm64
build_one windows amd64
build_one windows arm64
