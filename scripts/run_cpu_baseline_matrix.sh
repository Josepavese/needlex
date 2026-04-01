#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
source scripts/lib/model_baseline.sh
needlex_apply_model_baseline_env
export NEEDLEX_HARD_CASE_MATRIX_GO_TEST_TIMEOUT="${NEEDLEX_HARD_CASE_MATRIX_GO_TEST_TIMEOUT:-30m}"
bash ./scripts/run_hard_case_matrix.sh "$@"
