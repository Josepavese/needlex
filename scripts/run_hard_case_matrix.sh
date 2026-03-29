#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

export NEEDLEX_HARD_CASE_MATRIX_OUT="${NEEDLEX_HARD_CASE_MATRIX_OUT:-improvements/hard-case-matrix-latest.json}"
export NEEDLEX_HARD_CASE_MATRIX_BASELINE="${NEEDLEX_HARD_CASE_MATRIX_BASELINE:-improvements/hard-case-matrix-baseline.json}"
export NEEDLEX_HARD_CASE_MATRIX_CORPUS="${NEEDLEX_HARD_CASE_MATRIX_CORPUS:-testdata/benchmark/hard-case-corpus-v2.json}"
export NEEDLEX_HARD_CASE_MATRIX_UPDATE_BASELINE="${NEEDLEX_HARD_CASE_MATRIX_UPDATE_BASELINE:-0}"

for arg in "$@"; do
  if [[ "$arg" == "--update-baseline" ]]; then
    export NEEDLEX_HARD_CASE_MATRIX_UPDATE_BASELINE=1
  fi
done

go test ./scripts/hard_case_matrix -run TestExportHardCaseMatrix -count=1 -v
