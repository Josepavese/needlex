#!/usr/bin/env bash
set -euo pipefail

needlex_model_baseline_file() {
  printf '%s\n' "${NEEDLEX_MODEL_BASELINE_FILE:-internal/config/modelbaseline/model-baseline.json}"
}

needlex_model_baseline_jq() {
  local expr="$1"
  jq -r "$expr" "$(needlex_model_baseline_file)"
}
