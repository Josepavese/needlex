#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

export NEEDLEX_DISCOVERY_EVAL_OUT="${NEEDLEX_DISCOVERY_EVAL_OUT:-improvements/discovery-eval-latest.json}"
export NEEDLEX_DISCOVERY_EVAL_BASELINE="${NEEDLEX_DISCOVERY_EVAL_BASELINE:-improvements/discovery-eval-baseline.json}"
export NEEDLEX_DISCOVERY_EVAL_CORPUS="${NEEDLEX_DISCOVERY_EVAL_CORPUS:-testdata/benchmark/discovery-corpus-v1.json}"
export NEEDLEX_DISCOVERY_EVAL_UPDATE_BASELINE="${NEEDLEX_DISCOVERY_EVAL_UPDATE_BASELINE:-0}"

for arg in "$@"; do
  if [[ "$arg" == "--update-baseline" ]]; then
    export NEEDLEX_DISCOVERY_EVAL_UPDATE_BASELINE=1
  fi
done

go test ./scripts/discovery_eval -run TestExportDiscoveryEval -count=1 -v
