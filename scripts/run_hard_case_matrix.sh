#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

export NEEDLEX_HARD_CASE_MATRIX_OUT="${NEEDLEX_HARD_CASE_MATRIX_OUT:-improvements/hard-case-matrix-latest.json}"
export NEEDLEX_HARD_CASE_MATRIX_BASELINE="${NEEDLEX_HARD_CASE_MATRIX_BASELINE:-improvements/hard-case-matrix-baseline.json}"
export NEEDLEX_HARD_CASE_MATRIX_CORPUS="${NEEDLEX_HARD_CASE_MATRIX_CORPUS:-benchmarks/corpora/hard-case-corpus-v2.json}"
export NEEDLEX_HARD_CASE_MATRIX_UPDATE_BASELINE="${NEEDLEX_HARD_CASE_MATRIX_UPDATE_BASELINE:-0}"
export NEEDLEX_HARD_CASE_MATRIX_USE_LIVE_BACKEND="${NEEDLEX_HARD_CASE_MATRIX_USE_LIVE_BACKEND:-0}"
export NEEDLEX_HARD_CASE_MATRIX_GO_TEST_TIMEOUT="${NEEDLEX_HARD_CASE_MATRIX_GO_TEST_TIMEOUT:-10m}"

for arg in "$@"; do
  if [[ "$arg" == "--update-baseline" ]]; then
    export NEEDLEX_HARD_CASE_MATRIX_UPDATE_BASELINE=1
  fi
done

go test ./benchmarks/hard_case_matrix/runner -run TestExportHardCaseMatrix -count=1 -timeout "${NEEDLEX_HARD_CASE_MATRIX_GO_TEST_TIMEOUT}" -v
