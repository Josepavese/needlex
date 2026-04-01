#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HOST="${NEEDLEX_SEMANTIC_HOST:-127.0.0.1}"
PORT="${NEEDLEX_SEMANTIC_PORT:-18180}"
BASE_URL="http://${HOST}:${PORT}"
MODEL_ID="${NEEDLEX_SEMANTIC_MODEL_ID:-intfloat/multilingual-e5-small}"
LOG_PATH="${NEEDLEX_SEMANTIC_LOG:-/tmp/needlex-semantic-embed.log}"
OUT_PATH="${NEEDLEX_SEMANTIC_SMOKE_OUT:-/tmp/needlex-semantic-smoke.json}"

python3 "$ROOT/scripts/run_semantic_embed_upstream.py" >"$LOG_PATH" 2>&1 &
UPSTREAM_PID=$!
cleanup() {
  kill "$UPSTREAM_PID" >/dev/null 2>&1 || true
}
trap cleanup EXIT

for _ in $(seq 1 120); do
  if curl -fsS "$BASE_URL/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

curl -fsS "$BASE_URL/v1/embeddings" \
  -H 'Content-Type: application/json' \
  -d "{\"model\":\"$MODEL_ID\",\"input\":[\"service offering\",\"sviluppo siti web e marketing digitale\",\"contattaci ora\"]}" \
  | tee "$OUT_PATH"

echo
echo "semantic smoke saved to $OUT_PATH"
