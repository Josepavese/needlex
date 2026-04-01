#!/usr/bin/env bash
set -euo pipefail

needlex_model_benchmark_candidate_ids() {
  local candidates_file="$1"
  local selected_ids="${2:-}"

  if [[ -n "$selected_ids" ]]; then
    printf '%s\n' $selected_ids
    return 0
  fi

  jq -r '.candidates[].id' "$candidates_file"
}

needlex_model_benchmark_candidate_json() {
  local candidates_file="$1"
  local id="$2"
  jq -c --arg id "$id" '.candidates[] | select(.id == $id)' "$candidates_file"
}

needlex_model_benchmark_candidate_field() {
  local candidate_json="$1"
  local field="$2"
  jq -r --arg field "$field" '.[$field]' <<<"$candidate_json"
}

needlex_model_benchmark_write_row() {
  local id="$1"
  local label="$2"
  local router="$3"
  local extractor="$4"
  local formatter="$5"
  local hard_case_report="$6"
  local live_report="$7"
  local hard_case_log="$8"
  local live_log="$9"
  local hard_case_status="${10}"
  local live_status="${11}"
  local hard_case_report_exists="${12}"
  local live_report_exists="${13}"

  jq -cn \
    --arg id "$id" \
    --arg label "$label" \
    --arg router "$router" \
    --arg extractor "$extractor" \
    --arg formatter "$formatter" \
    --arg hard_case_report "$hard_case_report" \
    --arg live_report "$live_report" \
    --arg hard_case_log "$hard_case_log" \
    --arg live_log "$live_log" \
    --argjson hard_case_status "$hard_case_status" \
    --argjson live_status "$live_status" \
    --argjson hard_case_report_exists "$hard_case_report_exists" \
    --argjson live_report_exists "$live_report_exists" \
    '{
      id: $id,
      label: $label,
      router: $router,
      extractor: $extractor,
      formatter: $formatter,
      hard_case_status: $hard_case_status,
      live_status: $live_status,
      hard_case_report: $hard_case_report,
      live_report: $live_report,
      hard_case_log: $hard_case_log,
      live_log: $live_log,
      hard_case_report_exists: $hard_case_report_exists,
      live_report_exists: $live_report_exists
    }'
}

needlex_model_benchmark_summarize_runs() {
  local rows_file="$1"
  local summary_out="$2"
  local candidates_file="$3"
  local hard_case_corpus="$4"
  local live_cases_file="$5"

  while IFS= read -r row; do
    hard_case_report="$(jq -r '.hard_case_report' <<<"$row")"
    live_report="$(jq -r '.live_report' <<<"$row")"
    hard_case_summary="$(jq -c '
      {
        pass_rate: .acceptance.pass_rate,
        lane_lift_rate: .acceptance.lane_lift_rate,
        objective_lift_avg: .acceptance.objective_lift_avg,
        backend_acceptance_rate: .acceptance.backend_acceptance_rate,
        case_count: (.rows | length),
        avg_compare_latency_ms: ((.rows | map(.compare.latency_ms // 0) | add) / ((.rows | length) | if . == 0 then 1 else . end))
      }
    ' "$hard_case_report" 2>/dev/null || echo null)"
    live_summary="$(jq -c '
      {
        case_count: (.results | length),
        avg_compare_latency_ms: ((.results | map(.compare.latency_ms // 0) | add) / ((.results | length) | if . == 0 then 1 else . end)),
        avg_compare_context_alignment: ((.results | map(.compare.context_alignment // 0) | add) / ((.results | length) | if . == 0 then 1 else . end)),
        runtime_error_sites: (.results | map(select((.compare.patch_effects // []) | index("runtime_error"))) | map(.name))
      }
    ' "$live_report" 2>/dev/null || echo null)"
    jq -n \
      --argjson row "$row" \
      --argjson hard_case_summary "$hard_case_summary" \
      --argjson live_summary "$live_summary" \
      '$row + {hard_case_summary: $hard_case_summary, live_summary: $live_summary}'
  done <"$rows_file" | jq -s \
    --arg generated_at_utc "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --arg candidates_version "$(jq -r '.version' "$candidates_file")" \
    --arg hard_case_corpus "$hard_case_corpus" \
    --arg live_cases_file "$live_cases_file" \
    '{
      generated_at_utc: $generated_at_utc,
      candidates_version: $candidates_version,
      hard_case_corpus: $hard_case_corpus,
      live_cases_file: $live_cases_file,
      runs: .
    }' >"$summary_out"
}
