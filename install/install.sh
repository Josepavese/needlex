#!/usr/bin/env bash
set -euo pipefail

REPO="${NEEDLEX_REPO:-Josepavese/needlex}"
VERSION="${NEEDLEX_VERSION:-latest}"
RELEASE_BASE_URL="${NEEDLEX_RELEASE_BASE_URL:-}"
SKIP_SHELL_HOOKS="${NEEDLEX_INSTALL_SKIP_SHELL_HOOKS:-0}"

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

needlex_linux_data_home() {
  local candidate="${XDG_DATA_HOME:-}"
  if [[ -n "${candidate}" && "${candidate}" == *"/snap/"* && "${candidate}" != "${HOME}/"* ]]; then
    printf '%s\n' "${HOME}/.local/share"
    return
  fi
  if [[ -n "${candidate}" && "${candidate}" == "${HOME}/snap/"* ]]; then
    printf '%s\n' "${HOME}/.local/share"
    return
  fi
  printf '%s\n' "${candidate:-$HOME/.local/share}"
}

needlex_state_root() {
  case "$(uname -s)" in
    Darwin)
      printf '%s\n' "${HOME}/Library/Application Support/NeedleX"
      ;;
    *)
      printf '%s\n' "$(needlex_linux_data_home)/needlex"
      ;;
  esac
}

reconcile_path_hook() {
  local file="$1"
  local line="export PATH=\"${BIN_DIR}:\$PATH\""
  local marker='# needlex-path'
  local tmp
  mkdir -p "$(dirname "${file}")"
  touch "${file}"
  tmp="$(mktemp)"
  awk -v marker="${marker}" -v line="${line}" '
    BEGIN { skip=0 }
    skip == 1 { skip=0; next }
    $0 == marker { skip=1; next }
    $0 == line { next }
    { print }
  ' "${file}" > "${tmp}"
  mv "${tmp}" "${file}"
  printf '\n%s\n%s\n' "${marker}" "${line}" >> "${file}"
}

capture_existing_state_root() {
  local wrapper="$1"
  [[ -f "${wrapper}" ]] || return 0
  sed -n 's/^export NEEDLEX_HOME="\(.*\)"$/\1/p' "${wrapper}" | head -n1
}

cleanup_legacy_wrapper_artifacts() {
  rm -f "${BIN_DIR}/needle"
  rm -f "${LIB_DIR}/needle-real"
}

install_real_binary() {
  local source_bin="$1"
  local target_bin="$2"
  local tmp_bin="${target_bin}.tmp.$$"
  cp "${source_bin}" "${tmp_bin}"
  chmod 0755 "${tmp_bin}"
  mv -f "${tmp_bin}" "${target_bin}"
}

install_wrapper() {
  local wrapper_path="$1"
  local real_bin="$2"
  local state_root="$3"
  local tmp_wrapper="${wrapper_path}.tmp.$$"
  cat > "${tmp_wrapper}" <<EOF2
#!/usr/bin/env bash
set -euo pipefail
export NEEDLEX_HOME="${state_root}"
exec "${real_bin}" "\$@"
EOF2
  chmod 0755 "${tmp_wrapper}"
  mv -f "${tmp_wrapper}" "${wrapper_path}"
}

create_state_tree() {
  mkdir -p \
    "${STATE_ROOT}/traces" \
    "${STATE_ROOT}/proofs" \
    "${STATE_ROOT}/fingerprints" \
    "${STATE_ROOT}/genome" \
    "${STATE_ROOT}/discovery" \
    "${STATE_ROOT}/candidates" \
    "${STATE_ROOT}/domain_graph" \
    "${STATE_ROOT}/fingerprint_graph"
  touch "${STATE_ROOT}/discovery/discovery.db"
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
WRAPPER_PATH="${BIN_DIR}/needlex"
PREVIOUS_STATE_ROOT="$(capture_existing_state_root "${WRAPPER_PATH}")"

mkdir -p "${BIN_DIR}" "${LIB_DIR}"
cleanup_legacy_wrapper_artifacts
create_state_tree

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

curl -fsSL "${ASSET_URL}" -o "${TMP_DIR}/needlex.tar.gz"
tar -xzf "${TMP_DIR}/needlex.tar.gz" -C "${TMP_DIR}"
install_real_binary "${TMP_DIR}/needlex" "${REAL_BIN}"
install_wrapper "${WRAPPER_PATH}" "${REAL_BIN}" "${STATE_ROOT}"

if [[ "${SKIP_SHELL_HOOKS}" != "1" ]]; then
  reconcile_path_hook "${HOME}/.bashrc"
  reconcile_path_hook "${HOME}/.zshrc"
  reconcile_path_hook "${HOME}/.profile"
fi

printf '\nInstalled needlex to %s\n' "${WRAPPER_PATH}"
printf 'State root: %s\n' "${STATE_ROOT}"
if [[ -n "${PREVIOUS_STATE_ROOT}" && "${PREVIOUS_STATE_ROOT}" != "${STATE_ROOT}" ]]; then
  printf 'Previous state root preserved: %s\n' "${PREVIOUS_STATE_ROOT}"
fi
if [[ "${SKIP_SHELL_HOOKS}" == "1" ]]; then
  printf 'Shell PATH hooks skipped.\n'
else
  printf 'Restart your shell or run: source ~/.bashrc\n'
fi
