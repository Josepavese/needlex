#!/usr/bin/env bash
set -euo pipefail

needlex_require_command() {
  local cmd="$1"
  local message="${2:-$1 is required}"
  command -v "$cmd" >/dev/null 2>&1 || {
    echo "$message" >&2
    exit 1
  }
}

needlex_ollama_ensure_ready() {
  local ollama_bin="$1"
  local base_url="$2"

  needlex_require_command "$ollama_bin" "ollama binary not found: $ollama_bin"

  if curl -fsS "${base_url%/v1}/api/tags" >/dev/null 2>&1; then
    return 0
  fi

  echo "starting ollama serve on background session"
  "$ollama_bin" serve >/tmp/needlex-ollama.log 2>&1 &
  NEEDLEX_OLLAMA_PID=$!

  for _ in $(seq 1 30); do
    if curl -fsS "${base_url%/v1}/api/tags" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "ollama did not become ready: ${base_url%/v1}/api/tags" >&2
  exit 1
}

needlex_ollama_cleanup() {
  kill "${NEEDLEX_OLLAMA_PID:-0}" >/dev/null 2>&1 || true
}

needlex_ollama_pull_unique() {
  local ollama_bin="$1"
  shift

  local model
  declare -A seen_models=()
  for model in "$@"; do
    [[ -z "$model" ]] && continue
    [[ -n "${seen_models[$model]:-}" ]] && continue
    seen_models[$model]=1
    echo "ensuring model $model is available"
    "$ollama_bin" pull "$model"
  done
}
