#!/usr/bin/env bash
set -euo pipefail

REPO="${NEEDLEX_REPO:-Josepavese/needlex}"
VERSION="${NEEDLEX_VERSION:-latest}"
RELEASE_BASE_URL="${NEEDLEX_RELEASE_BASE_URL:-}"
SKIP_SHELL_HOOKS="${NEEDLEX_INSTALL_SKIP_SHELL_HOOKS:-0}"
SKIP_SEMANTIC_PREREQS="${NEEDLEX_INSTALL_SKIP_SEMANTIC_PREREQS:-0}"
OLLAMA_HOST="${NEEDLEX_OLLAMA_HOST:-http://127.0.0.1:11434}"
SEMANTIC_EMBEDDING_URL="${NEEDLEX_SEMANTIC_EMBEDDING_URL:-${OLLAMA_HOST}/api/embed}"
SEMANTIC_MODEL="${NEEDLEX_SEMANTIC_PROVIDER_MODEL:-embeddinggemma:latest}"
SEMANTIC_VECTOR_SPACE="${NEEDLEX_SEMANTIC_VECTOR_SPACE:-ollama-embeddinggemma-v1}"

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
  local config_path="$4"
  local tmp_wrapper="${wrapper_path}.tmp.$$"
  cat > "${tmp_wrapper}" <<EOF2
#!/usr/bin/env bash
set -euo pipefail
export NEEDLEX_HOME="${state_root}"
export NEEDLEX_CONFIG="${config_path}"
exec "${real_bin}" "\$@"
EOF2
  chmod 0755 "${tmp_wrapper}"
  mv -f "${tmp_wrapper}" "${wrapper_path}"
}

create_state_tree() {
  mkdir -p \
    "${STATE_ROOT}/analytics" \
    "${STATE_ROOT}/configs" \
    "${STATE_ROOT}/traces" \
    "${STATE_ROOT}/proofs" \
    "${STATE_ROOT}/fingerprints" \
    "${STATE_ROOT}/genome" \
    "${STATE_ROOT}/logs" \
    "${STATE_ROOT}/discovery" \
    "${STATE_ROOT}/candidates" \
    "${STATE_ROOT}/domain_graph" \
    "${STATE_ROOT}/fingerprint_graph"
  touch "${STATE_ROOT}/discovery/discovery.db"
}

install_ollama_if_missing() {
  if command -v ollama >/dev/null 2>&1; then
    return 0
  fi
  case "$(uname -s)" in
    Linux)
      curl -fsSL https://ollama.com/install.sh | sh
      ;;
    Darwin)
      if command -v brew >/dev/null 2>&1; then
        brew install ollama
      else
        echo "ollama is required but missing. Install Homebrew or download Ollama from https://ollama.com/download, then rerun this installer." >&2
        exit 1
      fi
      ;;
  esac
}

ollama_api_ready() {
  curl -fsS -m 3 "${OLLAMA_HOST}/api/tags" >/dev/null 2>&1
}

start_ollama_if_needed() {
  if ollama_api_ready; then
    return 0
  fi
  mkdir -p "${STATE_ROOT}/logs"
  nohup ollama serve >"${STATE_ROOT}/logs/ollama-install.log" 2>&1 &
  for _ in $(seq 1 20); do
    if ollama_api_ready; then
      return 0
    fi
    sleep 1
  done
  echo "ollama was installed but its API did not become ready at ${OLLAMA_HOST}" >&2
  echo "check ${STATE_ROOT}/logs/ollama-install.log" >&2
  exit 1
}

pull_embedding_model_if_needed() {
  if ollama list 2>/dev/null | awk '{print $1}' | grep -Fxq "${SEMANTIC_MODEL}"; then
    return 0
  fi
  ollama pull "${SEMANTIC_MODEL}"
}

verify_embedding_endpoint() {
  local payload
  payload="{\"model\":\"${SEMANTIC_MODEL}\",\"input\":[\"Needle-X semantic install probe\"]}"
  if ! curl -fsS -m 20 -H "Content-Type: application/json" -d "${payload}" "${SEMANTIC_EMBEDDING_URL}" >/dev/null; then
    echo "embedding endpoint probe failed: ${SEMANTIC_EMBEDDING_URL} model=${SEMANTIC_MODEL}" >&2
    exit 1
  fi
}

ensure_semantic_prereqs() {
  if [[ "${SKIP_SEMANTIC_PREREQS}" == "1" ]]; then
    echo "Semantic prerequisite install skipped by NEEDLEX_INSTALL_SKIP_SEMANTIC_PREREQS=1"
    return 0
  fi
  install_ollama_if_missing
  start_ollama_if_needed
  pull_embedding_model_if_needed
  verify_embedding_endpoint
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
CONFIG_PATH="${NEEDLEX_CONFIG:-${STATE_ROOT}/configs/needlex.json}"
REAL_BIN="${LIB_DIR}/needlex-real"
WRAPPER_PATH="${BIN_DIR}/needlex"
PREVIOUS_STATE_ROOT="$(capture_existing_state_root "${WRAPPER_PATH}")"

mkdir -p "${BIN_DIR}" "${LIB_DIR}"
create_state_tree

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

curl -fsSL "${ASSET_URL}" -o "${TMP_DIR}/needlex.tar.gz"
tar -xzf "${TMP_DIR}/needlex.tar.gz" -C "${TMP_DIR}"
install_real_binary "${TMP_DIR}/needlex" "${REAL_BIN}"
NEEDLEX_HOME="${STATE_ROOT}" \
NEEDLEX_CONFIG="${CONFIG_PATH}" \
NEEDLEX_SEMANTIC_EMBEDDING_URL="${SEMANTIC_EMBEDDING_URL}" \
NEEDLEX_SEMANTIC_PROVIDER_MODEL="${SEMANTIC_MODEL}" \
NEEDLEX_SEMANTIC_VECTOR_SPACE="${SEMANTIC_VECTOR_SPACE}" \
NEEDLEX_MODELS_BASE_URL="${OLLAMA_HOST}/v1" \
  "${REAL_BIN}" config init
ensure_semantic_prereqs
install_wrapper "${WRAPPER_PATH}" "${REAL_BIN}" "${STATE_ROOT}" "${CONFIG_PATH}"

if [[ "${SKIP_SHELL_HOOKS}" != "1" ]]; then
  reconcile_path_hook "${HOME}/.bashrc"
  reconcile_path_hook "${HOME}/.zshrc"
  reconcile_path_hook "${HOME}/.profile"
fi

printf '\nInstalled needlex to %s\n' "${WRAPPER_PATH}"
printf 'State root: %s\n' "${STATE_ROOT}"
printf 'Config: %s\n' "${CONFIG_PATH}"
printf 'Runtime log: %s\n' "${STATE_ROOT}/logs/needlex.jsonl"
printf 'Semantic endpoint: %s\n' "${SEMANTIC_EMBEDDING_URL}"
printf 'Semantic model: %s\n' "${SEMANTIC_MODEL}"
printf 'Agent skill: https://github.com/%s/tree/main/skills/needlex-web-retrieval\n' "${REPO}"
printf 'Codex skill install: python3 ~/.codex/skills/.system/skill-installer/scripts/install-skill-from-github.py --repo %s --path skills/needlex-web-retrieval\n' "${REPO}"
if [[ -n "${PREVIOUS_STATE_ROOT}" && "${PREVIOUS_STATE_ROOT}" != "${STATE_ROOT}" ]]; then
  printf 'Previous state root preserved: %s\n' "${PREVIOUS_STATE_ROOT}"
fi
if [[ "${SKIP_SHELL_HOOKS}" == "1" ]]; then
  printf 'Shell PATH hooks skipped.\n'
else
  printf 'Restart your shell or run: source ~/.bashrc\n'
fi
