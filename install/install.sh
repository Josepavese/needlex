#!/usr/bin/env bash
set -euo pipefail

REPO="${NEEDLEX_REPO:-Josepavese/needlex}"
VERSION="${NEEDLEX_VERSION:-latest}"
RELEASE_BASE_URL="${NEEDLEX_RELEASE_BASE_URL:-}"
SKIP_SHELL_HOOKS="${NEEDLEX_INSTALL_SKIP_SHELL_HOOKS:-0}"
SKIP_SEMANTIC_PREREQS="${NEEDLEX_INSTALL_SKIP_SEMANTIC_PREREQS:-0}"
SKIP_RENDER_PREREQS="${NEEDLEX_INSTALL_SKIP_RENDER_PREREQS:-0}"
OLLAMA_HOST="${NEEDLEX_OLLAMA_HOST:-http://127.0.0.1:11434}"
SEMANTIC_EMBEDDING_URL="${NEEDLEX_SEMANTIC_EMBEDDING_URL:-${OLLAMA_HOST}/api/embed}"
SEMANTIC_MODEL="${NEEDLEX_SEMANTIC_PROVIDER_MODEL:-embeddinggemma:latest}"
SEMANTIC_VECTOR_SPACE="${NEEDLEX_SEMANTIC_VECTOR_SPACE:-ollama-embeddinggemma-v1}"
RENDER_PROVIDER="${NEEDLEX_RENDER_PROVIDER:-exec-dump-dom}"
RENDER_BROWSER_PATH="${NEEDLEX_RENDER_BROWSER_PATH:-}"
RENDER_TIMEOUT_MS="${NEEDLEX_RENDER_TIMEOUT_MS:-30000}"
RENDER_MAX_CONCURRENCY="${NEEDLEX_RENDER_MAX_CONCURRENCY:-1}"
RENDER_NETWORK_IDLE_MS="${NEEDLEX_RENDER_NETWORK_IDLE_MS:-1500}"
RENDER_NETWORK_MAX_BYTES="${NEEDLEX_RENDER_NETWORK_MAX_BYTES:-64000000}"
RENDER_NETWORK_RESOURCE_MAX_BYTES="${NEEDLEX_RENDER_NETWORK_RESOURCE_MAX_BYTES:-64000000}"
RENDER_NETWORK_MAX_RESOURCES="${NEEDLEX_RENDER_NETWORK_MAX_RESOURCES:-32}"
RENDER_NETWORK_MAX_MESSAGES="${NEEDLEX_RENDER_NETWORK_MAX_MESSAGES:-4096}"
RENDER_CHROME_VERSION="${NEEDLEX_RENDER_CHROME_VERSION:-}"
RENDER_PLAYWRIGHT_VERSION="${NEEDLEX_RENDER_PLAYWRIGHT_VERSION:-latest}"
RENDER_NODE_VERSION="${NEEDLEX_RENDER_NODE_VERSION:-latest-v24.x}"

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
    "${STATE_ROOT}/fingerprint_graph" \
    "${STATE_ROOT}/browsers"
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

chrome_for_testing_platform() {
  case "${GOOS}/${GOARCH}" in
    linux/amd64) printf '%s\n' "linux64" ;;
    darwin/amd64) printf '%s\n' "mac-x64" ;;
    darwin/arm64) printf '%s\n' "mac-arm64" ;;
    *)
      echo "Chrome for Testing headless shell is not available as a PAL-local package for ${GOOS}/${GOARCH}." >&2
      echo "Set NEEDLEX_RENDER_BROWSER_PATH to an executable browser path or set NEEDLEX_INSTALL_SKIP_RENDER_PREREQS=1 for controlled packaging tests." >&2
      exit 1
      ;;
  esac
}

chrome_for_testing_version() {
  if [[ -n "${RENDER_CHROME_VERSION}" ]]; then
    printf '%s\n' "${RENDER_CHROME_VERSION}"
    return 0
  fi
  curl -fsSL https://googlechromelabs.github.io/chrome-for-testing/LATEST_RELEASE_STABLE
}

install_chrome_headless_shell() {
  local version platform target_dir target_exec archive_url render_tmp extracted_dir staged_dir
  version="$(chrome_for_testing_version)"
  platform="$(chrome_for_testing_platform)"
  target_dir="${STATE_ROOT}/browsers/chrome-headless-shell/${version}/${platform}"
  target_exec="${target_dir}/chrome-headless-shell"
  if [[ -x "${target_exec}" ]]; then
    RENDER_BROWSER_PATH="${target_exec}"
    return 0
  fi
  if ! command -v unzip >/dev/null 2>&1; then
    echo "unzip is required to install Chrome for Testing into PAL state: ${STATE_ROOT}/browsers" >&2
    exit 1
  fi
  archive_url="https://storage.googleapis.com/chrome-for-testing-public/${version}/${platform}/chrome-headless-shell-${platform}.zip"
  render_tmp="$(mktemp -d)"
  curl -fsSL "${archive_url}" -o "${render_tmp}/chrome-headless-shell.zip"
  unzip -q "${render_tmp}/chrome-headless-shell.zip" -d "${render_tmp}"
  extracted_dir="${render_tmp}/chrome-headless-shell-${platform}"
  if [[ ! -x "${extracted_dir}/chrome-headless-shell" ]]; then
    echo "Chrome for Testing archive did not contain chrome-headless-shell for ${platform}" >&2
    rm -rf "${render_tmp}"
    exit 1
  fi
  mkdir -p "$(dirname "${target_dir}")"
  staged_dir="${target_dir}.tmp.$$"
  rm -rf "${staged_dir}"
  cp -R "${extracted_dir}" "${staged_dir}"
  chmod 0755 "${staged_dir}/chrome-headless-shell"
  rm -rf "${target_dir}"
  mv "${staged_dir}" "${target_dir}"
  rm -rf "${render_tmp}"
  RENDER_BROWSER_PATH="${target_exec}"
}

node_platform() {
  case "${GOOS}/${GOARCH}" in
    linux/arm64) printf '%s\n' "linux-arm64" ;;
    *)
      echo "installer-managed Playwright render fallback is only supported for linux/arm64; got ${GOOS}/${GOARCH}" >&2
      exit 1
      ;;
  esac
}

ensure_node_for_playwright() {
  if command -v node >/dev/null 2>&1 && command -v npm >/dev/null 2>&1; then
    NODE_BIN="$(command -v node)"
    NPM_BIN="$(command -v npm)"
    return 0
  fi

  local platform base sums filename archive_url target_dir staged_dir node_tmp
  platform="$(node_platform)"
  base="https://nodejs.org/dist/${RENDER_NODE_VERSION}"
  sums="$(curl -fsSL "${base}/SHASUMS256.txt")"
  filename="$(printf '%s\n' "${sums}" | awk '{print $2}' | grep -E "^node-v[0-9][^ ]*-${platform}[.]tar[.]xz$" | head -n1)"
  if [[ -z "${filename}" ]]; then
    echo "could not resolve Node.js ${platform} tarball from ${base}" >&2
    exit 1
  fi

  target_dir="${STATE_ROOT}/browsers/node/${filename%.tar.xz}"
  if [[ -x "${target_dir}/bin/node" && -x "${target_dir}/bin/npm" ]]; then
    NODE_BIN="${target_dir}/bin/node"
    NPM_BIN="${target_dir}/bin/npm"
    return 0
  fi

  archive_url="${base}/${filename}"
  node_tmp="$(mktemp -d)"
  curl -fsSL "${archive_url}" -o "${node_tmp}/${filename}"
  mkdir -p "$(dirname "${target_dir}")"
  staged_dir="${target_dir}.tmp.$$"
  rm -rf "${staged_dir}"
  mkdir -p "${staged_dir}"
  tar -xJf "${node_tmp}/${filename}" -C "${staged_dir}" --strip-components=1
  if [[ ! -x "${staged_dir}/bin/node" || ! -x "${staged_dir}/bin/npm" ]]; then
    echo "Node.js archive did not contain node/npm for ${platform}" >&2
    rm -rf "${node_tmp}" "${staged_dir}"
    exit 1
  fi
  rm -rf "${target_dir}"
  mv "${staged_dir}" "${target_dir}"
  rm -rf "${node_tmp}"
  NODE_BIN="${target_dir}/bin/node"
  NPM_BIN="${target_dir}/bin/npm"
}

find_playwright_headless_shell() {
  local root="$1"
  [[ -d "${root}" ]] || return 1
  find "${root}" -type f \( -name headless_shell -o -name chrome-headless-shell \) -perm -111 2>/dev/null | sort | tail -n1
}

install_playwright_headless_shell() {
  local playwright_root npm_cache npm_home existing installed
  playwright_root="${STATE_ROOT}/browsers/playwright"
  existing="$(find_playwright_headless_shell "${playwright_root}" || true)"
  if [[ -n "${existing}" && -x "${existing}" ]]; then
    RENDER_BROWSER_PATH="${existing}"
    return 0
  fi

  ensure_node_for_playwright
  npm_cache="${STATE_ROOT}/browsers/npm-cache"
  npm_home="${STATE_ROOT}/browsers/npm-home"
  mkdir -p "${playwright_root}" "${npm_cache}" "${npm_home}"
  HOME="${npm_home}" \
  NPM_CONFIG_CACHE="${npm_cache}" \
  PLAYWRIGHT_BROWSERS_PATH="${playwright_root}" \
  PATH="$(dirname "${NODE_BIN}"):${PATH}" \
    "${NPM_BIN}" exec --yes --package="playwright@${RENDER_PLAYWRIGHT_VERSION}" -- playwright install --only-shell chromium

  installed="$(find_playwright_headless_shell "${playwright_root}" || true)"
  if [[ -z "${installed}" || ! -x "${installed}" ]]; then
    echo "Playwright did not install a Chromium headless shell under ${playwright_root}" >&2
    exit 1
  fi
  RENDER_BROWSER_PATH="${installed}"
}

install_render_browser() {
  case "${GOOS}/${GOARCH}" in
    linux/arm64)
      install_playwright_headless_shell
      ;;
    *)
      install_chrome_headless_shell
      ;;
  esac
}

verify_render_browser() {
  local profile_dir output browser_base
  if [[ ! -x "${RENDER_BROWSER_PATH}" ]]; then
    echo "render browser is not executable: ${RENDER_BROWSER_PATH}" >&2
    exit 1
  fi
  profile_dir="$(mktemp -d)"
  browser_base="$(basename "${RENDER_BROWSER_PATH}")"
  if [[ "${browser_base}" == *"chrome-headless-shell"* || "${browser_base}" == "headless_shell" ]]; then
    output="$("${RENDER_BROWSER_PATH}" \
      --disable-gpu \
      --no-sandbox \
      --no-first-run \
      --no-default-browser-check \
      --disable-dev-shm-usage \
      --disable-background-networking \
      --disable-sync \
      --user-data-dir="${profile_dir}" \
      --virtual-time-budget=1000 \
      --dump-dom \
      "data:text/html,<main>Needle-X%20render%20readiness%20probe</main>" 2>/dev/null || true)"
  else
    output="$("${RENDER_BROWSER_PATH}" \
      --headless=new \
      --no-sandbox \
      --disable-gpu \
      --no-first-run \
      --no-default-browser-check \
      --disable-dev-shm-usage \
      --disable-background-networking \
      --disable-sync \
      --user-data-dir="${profile_dir}" \
      --virtual-time-budget=1000 \
      --dump-dom \
      "data:text/html,<main>Needle-X%20render%20readiness%20probe</main>" 2>/dev/null || true)"
  fi
  rm -rf "${profile_dir}"
  if [[ "${output}" != *"Needle-X render readiness probe"* ]]; then
    echo "render browser probe failed: ${RENDER_BROWSER_PATH}" >&2
    exit 1
  fi
}

ensure_render_prereqs() {
  if [[ "${SKIP_RENDER_PREREQS}" == "1" ]]; then
    echo "Render prerequisite install skipped by NEEDLEX_INSTALL_SKIP_RENDER_PREREQS=1"
    return 0
  fi
  if [[ "${RENDER_PROVIDER}" != "exec-dump-dom" ]]; then
    echo "unsupported render provider for installer-managed browser: ${RENDER_PROVIDER}" >&2
    exit 1
  fi
  if [[ -n "${RENDER_BROWSER_PATH}" ]]; then
    if [[ ! -x "${RENDER_BROWSER_PATH}" ]]; then
      echo "NEEDLEX_RENDER_BROWSER_PATH is not executable: ${RENDER_BROWSER_PATH}" >&2
      exit 1
    fi
  else
    install_render_browser
  fi
  verify_render_browser
}

configure_render_config() {
  if [[ "${SKIP_RENDER_PREREQS}" == "1" ]]; then
    return 0
  fi
  if [[ -z "${RENDER_BROWSER_PATH}" ]]; then
    echo "render browser path is empty after prerequisite install" >&2
    exit 1
  fi
  NEEDLEX_HOME="${STATE_ROOT}" NEEDLEX_CONFIG="${CONFIG_PATH}" "${REAL_BIN}" config set render.enabled true >/dev/null
  NEEDLEX_HOME="${STATE_ROOT}" NEEDLEX_CONFIG="${CONFIG_PATH}" "${REAL_BIN}" config set render.provider "${RENDER_PROVIDER}" >/dev/null
  NEEDLEX_HOME="${STATE_ROOT}" NEEDLEX_CONFIG="${CONFIG_PATH}" "${REAL_BIN}" config set render.browser_path "${RENDER_BROWSER_PATH}" >/dev/null
  NEEDLEX_HOME="${STATE_ROOT}" NEEDLEX_CONFIG="${CONFIG_PATH}" "${REAL_BIN}" config set render.timeout_ms "${RENDER_TIMEOUT_MS}" >/dev/null
  NEEDLEX_HOME="${STATE_ROOT}" NEEDLEX_CONFIG="${CONFIG_PATH}" "${REAL_BIN}" config set render.max_concurrency "${RENDER_MAX_CONCURRENCY}" >/dev/null
  NEEDLEX_HOME="${STATE_ROOT}" NEEDLEX_CONFIG="${CONFIG_PATH}" "${REAL_BIN}" config set render.network_idle_ms "${RENDER_NETWORK_IDLE_MS}" >/dev/null
  NEEDLEX_HOME="${STATE_ROOT}" NEEDLEX_CONFIG="${CONFIG_PATH}" "${REAL_BIN}" config set render.network_max_bytes "${RENDER_NETWORK_MAX_BYTES}" >/dev/null
  NEEDLEX_HOME="${STATE_ROOT}" NEEDLEX_CONFIG="${CONFIG_PATH}" "${REAL_BIN}" config set render.network_resource_max_bytes "${RENDER_NETWORK_RESOURCE_MAX_BYTES}" >/dev/null
  NEEDLEX_HOME="${STATE_ROOT}" NEEDLEX_CONFIG="${CONFIG_PATH}" "${REAL_BIN}" config set render.network_max_resources "${RENDER_NETWORK_MAX_RESOURCES}" >/dev/null
  NEEDLEX_HOME="${STATE_ROOT}" NEEDLEX_CONFIG="${CONFIG_PATH}" "${REAL_BIN}" config set render.network_max_messages "${RENDER_NETWORK_MAX_MESSAGES}" >/dev/null
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
ensure_render_prereqs
NEEDLEX_HOME="${STATE_ROOT}" \
NEEDLEX_CONFIG="${CONFIG_PATH}" \
NEEDLEX_SEMANTIC_EMBEDDING_URL="${SEMANTIC_EMBEDDING_URL}" \
NEEDLEX_SEMANTIC_PROVIDER_MODEL="${SEMANTIC_MODEL}" \
NEEDLEX_SEMANTIC_VECTOR_SPACE="${SEMANTIC_VECTOR_SPACE}" \
NEEDLEX_RENDER_ENABLED="$([[ "${SKIP_RENDER_PREREQS}" == "1" ]] && printf false || printf true)" \
NEEDLEX_RENDER_PROVIDER="${RENDER_PROVIDER}" \
NEEDLEX_RENDER_BROWSER_PATH="${RENDER_BROWSER_PATH}" \
NEEDLEX_RENDER_TIMEOUT_MS="${RENDER_TIMEOUT_MS}" \
NEEDLEX_RENDER_MAX_CONCURRENCY="${RENDER_MAX_CONCURRENCY}" \
NEEDLEX_RENDER_NETWORK_IDLE_MS="${RENDER_NETWORK_IDLE_MS}" \
NEEDLEX_RENDER_NETWORK_MAX_BYTES="${RENDER_NETWORK_MAX_BYTES}" \
NEEDLEX_RENDER_NETWORK_RESOURCE_MAX_BYTES="${RENDER_NETWORK_RESOURCE_MAX_BYTES}" \
NEEDLEX_RENDER_NETWORK_MAX_RESOURCES="${RENDER_NETWORK_MAX_RESOURCES}" \
NEEDLEX_RENDER_NETWORK_MAX_MESSAGES="${RENDER_NETWORK_MAX_MESSAGES}" \
NEEDLEX_MODELS_BASE_URL="${OLLAMA_HOST}/v1" \
  "${REAL_BIN}" config init
configure_render_config
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
printf 'Render browser: %s\n' "${RENDER_BROWSER_PATH:-<skipped>}"
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
