#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

export NEEDLEX_HARD_CASE_MATRIX_CORPUS="${NEEDLEX_HARD_CASE_MATRIX_CORPUS:-benchmarks/corpora/hard-case-corpus-smoke-v1.json}"
export NEEDLEX_HARD_CASE_MATRIX_OUT="${NEEDLEX_HARD_CASE_MATRIX_OUT:-improvements/hard-case-matrix-smoke-qwen35-cpu.json}"

bash ./scripts/run_qwen35_cpu_matrix.sh "$@"
