#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
source scripts/lib/model_baseline.sh

needlex_apply_model_baseline_env
needlex_apply_semantic_baseline_env
export NEEDLEX_LIVE_READ_USE_COMPARE=1

go run ./benchmarks/live_read_eval/runner \
  --cases "${NEEDLEX_LIVE_READ_CASES:-benchmarks/corpora/live-sites-semantic-eval-v1.json}" \
  --out "${NEEDLEX_LIVE_READ_OUT:-improvements/live-semantic-eval-latest.json}" \
  --baseline improvements/live-read-baseline.json \
  "$@"
