#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

MODEL="${NEEDLEX_QWEN_MODEL:-qwen3.5:0.8b}"
BASE_URL="${NEEDLEX_MODELS_BASE_URL:-http://127.0.0.1:11434/v1}"
OLLAMA_BIN="${OLLAMA_BIN:-ollama}"

if ! command -v "${OLLAMA_BIN}" >/dev/null 2>&1; then
  echo "ollama binary not found: ${OLLAMA_BIN}" >&2
  exit 1
fi

if ! curl -fsS "${BASE_URL%/v1}/api/tags" >/dev/null 2>&1; then
  echo "starting ollama serve on background session"
  "${OLLAMA_BIN}" serve >/tmp/needlex-ollama.log 2>&1 &
  OLLAMA_PID=$!
  trap 'kill ${OLLAMA_PID:-0} >/dev/null 2>&1 || true' EXIT
  for _ in $(seq 1 30); do
    if curl -fsS "${BASE_URL%/v1}/api/tags" >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
fi

echo "ensuring model ${MODEL} is available"
"${OLLAMA_BIN}" pull "${MODEL}"

export NEEDLEX_LIVE_READ_USE_COMPARE=1
export NEEDLEX_MODELS_BACKEND=openai-compatible
export NEEDLEX_MODELS_BASE_URL="${BASE_URL}"
export NEEDLEX_MODELS_ROUTER="${NEEDLEX_MODELS_ROUTER:-$MODEL}"
export NEEDLEX_MODELS_EXTRACTOR="${NEEDLEX_MODELS_EXTRACTOR:-$MODEL}"
export NEEDLEX_MODELS_FORMATTER="${NEEDLEX_MODELS_FORMATTER:-$MODEL}"
export NEEDLEX_LIVE_READ_TIMEOUT_MS="${NEEDLEX_LIVE_READ_TIMEOUT_MS:-25000}"
export NEEDLEX_BUDGET_MAX_LATENCY_MS="${NEEDLEX_BUDGET_MAX_LATENCY_MS:-25000}"
export NEEDLEX_MODELS_MICRO_TIMEOUT_MS="${NEEDLEX_MODELS_MICRO_TIMEOUT_MS:-8000}"
export NEEDLEX_MODELS_STRUCTURED_TIMEOUT_MS="${NEEDLEX_MODELS_STRUCTURED_TIMEOUT_MS:-20000}"
export NEEDLEX_MODELS_SPECIALIST_TIMEOUT_MS="${NEEDLEX_MODELS_SPECIALIST_TIMEOUT_MS:-6000}"

go run ./benchmarks/live_read_eval/runner \
  --out improvements/live-read-qwen35-compare.json \
  --baseline improvements/live-read-baseline.json \
  "$@"
