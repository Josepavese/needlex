#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
source scripts/lib/model_baseline.sh

BASE_URL="${NEEDLEX_MODELS_BASE_URL:-$(needlex_model_baseline_jq '.recommended_base_url')}"
MODEL_ROUTER="${NEEDLEX_MODELS_ROUTER:-$(needlex_model_baseline_jq '.models.router')}"
MODEL_JUDGE="${NEEDLEX_MODELS_JUDGE:-$(needlex_model_baseline_jq '.models.judge')}"
MODEL_EXTRACTOR="${NEEDLEX_MODELS_EXTRACTOR:-$(needlex_model_baseline_jq '.models.extractor')}"
MODEL_FORMATTER="${NEEDLEX_MODELS_FORMATTER:-$(needlex_model_baseline_jq '.models.formatter')}"
MICRO_TIMEOUT="${NEEDLEX_MODELS_MICRO_TIMEOUT_MS:-$(needlex_model_baseline_jq '.timeouts.micro_timeout_ms')}"
STRUCTURED_TIMEOUT="${NEEDLEX_MODELS_STRUCTURED_TIMEOUT_MS:-$(needlex_model_baseline_jq '.timeouts.structured_timeout_ms')}"
SPECIALIST_TIMEOUT="${NEEDLEX_MODELS_SPECIALIST_TIMEOUT_MS:-$(needlex_model_baseline_jq '.timeouts.specialist_timeout_ms')}"

SEMANTIC_BACKEND="${NEEDLEX_SEMANTIC_BACKEND:-$(needlex_model_baseline_jq '.semantic.recommended_backend')}"
SEMANTIC_BASE_URL="${NEEDLEX_SEMANTIC_BASE_URL:-$(needlex_model_baseline_jq '.semantic.recommended_base_url')}"
SEMANTIC_MODEL="${NEEDLEX_SEMANTIC_MODEL:-$(needlex_model_baseline_jq '.semantic.model')}"
SEMANTIC_TIMEOUT="${NEEDLEX_SEMANTIC_TIMEOUT_MS:-$(needlex_model_baseline_jq '.semantic.timeout_ms')}"
SEMANTIC_LOG="${NEEDLEX_SEMANTIC_LOG:-/tmp/needlex-semantic-embed.log}"

export NEEDLEX_MODELS_BACKEND="${NEEDLEX_MODELS_BACKEND:-openai-compatible}"
export NEEDLEX_MODELS_BASE_URL="$BASE_URL"
export NEEDLEX_MODELS_ROUTER="$MODEL_ROUTER"
export NEEDLEX_MODELS_JUDGE="$MODEL_JUDGE"
export NEEDLEX_MODELS_EXTRACTOR="$MODEL_EXTRACTOR"
export NEEDLEX_MODELS_FORMATTER="$MODEL_FORMATTER"
export NEEDLEX_MODELS_MICRO_TIMEOUT_MS="$MICRO_TIMEOUT"
export NEEDLEX_MODELS_STRUCTURED_TIMEOUT_MS="$STRUCTURED_TIMEOUT"
export NEEDLEX_MODELS_SPECIALIST_TIMEOUT_MS="$SPECIALIST_TIMEOUT"

export NEEDLEX_SEMANTIC_ENABLED="${NEEDLEX_SEMANTIC_ENABLED:-true}"
export NEEDLEX_SEMANTIC_BACKEND="$SEMANTIC_BACKEND"
export NEEDLEX_SEMANTIC_BASE_URL="$SEMANTIC_BASE_URL"
export NEEDLEX_SEMANTIC_MODEL="$SEMANTIC_MODEL"
export NEEDLEX_SEMANTIC_TIMEOUT_MS="$SEMANTIC_TIMEOUT"
export NEEDLEX_LIVE_READ_USE_COMPARE=1

python3 "$ROOT/scripts/run_semantic_embed_upstream.py" >"$SEMANTIC_LOG" 2>&1 &
SEMANTIC_PID=$!
cleanup() {
  kill "$SEMANTIC_PID" >/dev/null 2>&1 || true
}
trap cleanup EXIT

HEALTHZ="$SEMANTIC_BASE_URL/healthz"
for _ in $(seq 1 120); do
  if curl -fsS "$HEALTHZ" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
if ! curl -fsS "$HEALTHZ" >/dev/null 2>&1; then
  echo "semantic upstream did not become ready" >&2
  exit 1
fi

go run ./benchmarks/live_read_eval/runner \
  --cases "${NEEDLEX_LIVE_READ_CASES:-benchmarks/corpora/live-sites-semantic-eval-v1.json}" \
  --out "${NEEDLEX_LIVE_READ_OUT:-improvements/live-semantic-eval-latest.json}" \
  --baseline improvements/live-read-baseline.json \
  "$@"
