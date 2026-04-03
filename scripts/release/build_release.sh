#!/usr/bin/env bash
set -euo pipefail

OUT_DIR="${1:-dist}"
mkdir -p "${OUT_DIR}"
OUT_DIR="$(cd "${OUT_DIR}" && pwd)"
VERSION="${NEEDLEX_VERSION:-dev}"

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

  GOOS="${goos}" GOARCH="${goarch}" go build -ldflags "-X github.com/josepavese/needlex/internal/buildinfo.Version=${VERSION}" -o "${work_dir}/${bin_name}" ./cmd/needle

  if [[ "${goos}" == "windows" ]]; then
    (
      cd "${work_dir}"
      zip -q "${OUT_DIR}/${archive_name}.zip" "${bin_name}"
    )
  else
    tar -C "${work_dir}" -czf "${OUT_DIR}/${archive_name}.tar.gz" "${bin_name}"
  fi

  rm -rf "${work_dir}"
}

build_one linux amd64
build_one linux arm64
build_one darwin amd64
build_one darwin arm64
build_one windows amd64
build_one windows arm64
