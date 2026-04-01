#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
source scripts/lib/model_baseline.sh
source scripts/lib/semantic_upstream.sh

SEMANTIC_LOG="${NEEDLEX_SEMANTIC_LOG:-/tmp/needlex-semantic-embed.log}"
needlex_apply_model_baseline_env
needlex_apply_semantic_baseline_env
export NEEDLEX_LIVE_READ_USE_COMPARE=1

trap needlex_semantic_upstream_cleanup EXIT
needlex_semantic_upstream_start "$ROOT" "$SEMANTIC_LOG" "$NEEDLEX_SEMANTIC_BASE_URL/healthz"

go run ./benchmarks/live_read_eval/runner \
  --cases "${NEEDLEX_LIVE_READ_CASES:-benchmarks/corpora/live-sites-semantic-eval-v1.json}" \
  --out "${NEEDLEX_LIVE_READ_OUT:-improvements/live-semantic-eval-latest.json}" \
  --baseline improvements/live-read-baseline.json \
  "$@"
