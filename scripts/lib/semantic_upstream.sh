#!/usr/bin/env bash
set -euo pipefail

needlex_semantic_upstream_start() {
  local root="$1"
  local log_path="$2"
  local healthz="$3"

  python3 "$root/scripts/run_semantic_embed_upstream.py" >"$log_path" 2>&1 &
  NEEDLEX_SEMANTIC_PID=$!

  for _ in $(seq 1 120); do
    if curl -fsS "$healthz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "semantic upstream did not become ready" >&2
  exit 1
}

needlex_semantic_upstream_cleanup() {
  kill "${NEEDLEX_SEMANTIC_PID:-0}" >/dev/null 2>&1 || true
}
