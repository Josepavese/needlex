#!/usr/bin/env bash
set -euo pipefail

REPO="${NEEDLEX_REPO:-Josepavese/needlex}"
VERSION="${NEEDLEX_VERSION:-latest}"
RELEASE_BASE_URL="${NEEDLEX_RELEASE_BASE_URL:-}"

needlex_platform() {
  local os arch
  os="$(uname -s)"
  arch="$(uname -m)"

  case "${os}" in
    Linux) os="linux" ;;
    Darwin) os="darwin" ;;
    *)
      echo "unsupported OS: ${os}" >&2
      exit 1
      ;;
  esac

  case "${arch}" in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *)
      echo "unsupported architecture: ${arch}" >&2
      exit 1
      ;;
  esac

  printf '%s %s\n' "${os}" "${arch}"
}

needlex_state_root() {
  case "$(uname -s)" in
    Darwin)
      printf '%s\n' "${HOME}/Library/Application Support/NeedleX"
      ;;
    *)
      printf '%s\n' "${XDG_DATA_HOME:-$HOME/.local/share}/needlex"
      ;;
  esac
}

add_path_hook() {
  local file="$1"
  local line='export PATH="$HOME/.local/bin:$PATH"'
  local marker='# needlex-path'
  mkdir -p "$(dirname "${file}")"
  touch "${file}"
  if ! grep -Fq "${marker}" "${file}"; then
    printf '\n%s\n%s\n' "${marker}" "${line}" >> "${file}"
  fi
}

read -r GOOS GOARCH < <(needlex_platform)

ASSET_BASENAME="needlex_${GOOS}_${GOARCH}"
if [[ -n "${RELEASE_BASE_URL}" ]]; then
  ASSET_URL="${RELEASE_BASE_URL}/${ASSET_BASENAME}.tar.gz"
elif [[ "${VERSION}" == "latest" ]]; then
  ASSET_URL="https://github.com/${REPO}/releases/latest/download/${ASSET_BASENAME}.tar.gz"
else
  ASSET_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET_BASENAME}.tar.gz"
fi

BIN_DIR="${NEEDLEX_INSTALL_BIN_DIR:-$HOME/.local/bin}"
LIB_DIR="${NEEDLEX_INSTALL_LIB_DIR:-$HOME/.local/lib/needlex}"
STATE_ROOT="${NEEDLEX_HOME:-$(needlex_state_root)}"
REAL_BIN="${LIB_DIR}/needlex-real"

mkdir -p "${BIN_DIR}" "${LIB_DIR}" "${STATE_ROOT}/traces" "${STATE_ROOT}/proofs" "${STATE_ROOT}/fingerprints" "${STATE_ROOT}/genome" "${STATE_ROOT}/discovery"
touch "${STATE_ROOT}/discovery/discovery.db"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

curl -fsSL "${ASSET_URL}" -o "${TMP_DIR}/needlex.tar.gz"
tar -xzf "${TMP_DIR}/needlex.tar.gz" -C "${TMP_DIR}"
cp "${TMP_DIR}/needlex" "${REAL_BIN}"
chmod 0755 "${REAL_BIN}"

cat > "${BIN_DIR}/needlex" <<EOF
#!/usr/bin/env bash
set -euo pipefail
export NEEDLEX_HOME="${STATE_ROOT}"
exec "${REAL_BIN}" "\$@"
EOF
chmod 0755 "${BIN_DIR}/needlex"

cat > "${BIN_DIR}/needle" <<EOF
#!/usr/bin/env bash
exec "${BIN_DIR}/needlex" "\$@"
EOF
chmod 0755 "${BIN_DIR}/needle"

add_path_hook "${HOME}/.bashrc"
add_path_hook "${HOME}/.zshrc"

printf '\nInstalled needlex to %s\n' "${BIN_DIR}/needlex"
printf 'Compatibility alias: %s\n' "${BIN_DIR}/needle"
printf 'State root: %s\n' "${STATE_ROOT}"
printf 'Restart your shell or run: source ~/.bashrc\n'
