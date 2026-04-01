#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

export NEEDLEX_LIVE_READ_CASES="${NEEDLEX_LIVE_READ_CASES:-benchmarks/corpora/live-sites-semantic-global-v1.json}"
export NEEDLEX_LIVE_READ_OUT="${NEEDLEX_LIVE_READ_OUT:-improvements/live-semantic-global-eval-latest.json}"

exec ./scripts/run_live_semantic_eval.sh "$@"
