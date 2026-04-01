#!/usr/bin/env bash
set -euo pipefail

needlex_model_baseline_file() {
  printf '%s\n' "${NEEDLEX_MODEL_BASELINE_FILE:-internal/config/modelbaseline/model-baseline.json}"
}

needlex_model_baseline_jq() {
  local expr="$1"
  jq -r "$expr" "$(needlex_model_baseline_file)"
}

needlex_apply_model_baseline_env() {
  local base_url="${NEEDLEX_MODELS_BASE_URL:-$(needlex_model_baseline_jq '.recommended_base_url')}"
  local model_router="${NEEDLEX_MODELS_ROUTER:-$(needlex_model_baseline_jq '.models.router')}"
  local model_judge="${NEEDLEX_MODELS_JUDGE:-$(needlex_model_baseline_jq '.models.judge')}"
  local model_extractor="${NEEDLEX_MODELS_EXTRACTOR:-$(needlex_model_baseline_jq '.models.extractor')}"
  local model_formatter="${NEEDLEX_MODELS_FORMATTER:-$(needlex_model_baseline_jq '.models.formatter')}"
  local micro_timeout="${NEEDLEX_MODELS_MICRO_TIMEOUT_MS:-$(needlex_model_baseline_jq '.timeouts.micro_timeout_ms')}"
  local structured_timeout="${NEEDLEX_MODELS_STRUCTURED_TIMEOUT_MS:-$(needlex_model_baseline_jq '.timeouts.structured_timeout_ms')}"
  local specialist_timeout="${NEEDLEX_MODELS_SPECIALIST_TIMEOUT_MS:-$(needlex_model_baseline_jq '.timeouts.specialist_timeout_ms')}"

  export NEEDLEX_MODELS_BACKEND="${NEEDLEX_MODELS_BACKEND:-openai-compatible}"
  export NEEDLEX_MODELS_BASE_URL="$base_url"
  export NEEDLEX_MODELS_ROUTER="$model_router"
  export NEEDLEX_MODELS_JUDGE="$model_judge"
  export NEEDLEX_MODELS_EXTRACTOR="$model_extractor"
  export NEEDLEX_MODELS_FORMATTER="$model_formatter"
  export NEEDLEX_MODELS_MICRO_TIMEOUT_MS="$micro_timeout"
  export NEEDLEX_MODELS_STRUCTURED_TIMEOUT_MS="$structured_timeout"
  export NEEDLEX_MODELS_SPECIALIST_TIMEOUT_MS="$specialist_timeout"
}

needlex_apply_semantic_baseline_env() {
  local semantic_backend="${NEEDLEX_SEMANTIC_BACKEND:-$(needlex_model_baseline_jq '.semantic.recommended_backend')}"
  local semantic_base_url="${NEEDLEX_SEMANTIC_BASE_URL:-$(needlex_model_baseline_jq '.semantic.recommended_base_url')}"
  local semantic_model="${NEEDLEX_SEMANTIC_MODEL:-$(needlex_model_baseline_jq '.semantic.model')}"
  local semantic_timeout="${NEEDLEX_SEMANTIC_TIMEOUT_MS:-$(needlex_model_baseline_jq '.semantic.timeout_ms')}"

  export NEEDLEX_SEMANTIC_ENABLED="${NEEDLEX_SEMANTIC_ENABLED:-true}"
  export NEEDLEX_SEMANTIC_BACKEND="$semantic_backend"
  export NEEDLEX_SEMANTIC_BASE_URL="$semantic_base_url"
  export NEEDLEX_SEMANTIC_MODEL="$semantic_model"
  export NEEDLEX_SEMANTIC_TIMEOUT_MS="$semantic_timeout"
}
