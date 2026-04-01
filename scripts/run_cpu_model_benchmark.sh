#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

CANDIDATES_FILE="${NEEDLEX_MODEL_BENCHMARK_CANDIDATES:-benchmarks/corpora/model-candidates-cpu-v1.json}"
HARD_CASE_CORPUS="${NEEDLEX_MODEL_BENCHMARK_HARD_CASE_CORPUS:-benchmarks/corpora/hard-case-corpus-benchmark-v1.json}"
LIVE_CASES_FILE="${NEEDLEX_MODEL_BENCHMARK_LIVE_CASES:-benchmarks/corpora/live-sites-cpu-v1.json}"
OUT_DIR="${NEEDLEX_MODEL_BENCHMARK_OUT_DIR:-improvements/model-benchmark-cpu}"
SUMMARY_OUT="${NEEDLEX_MODEL_BENCHMARK_OUT:-improvements/model-benchmark-cpu-latest.json}"
BASE_URL="${NEEDLEX_MODELS_BASE_URL:-http://127.0.0.1:11434/v1}"
OLLAMA_BIN="${OLLAMA_BIN:-ollama}"
SELECT_IDS="${NEEDLEX_MODEL_BENCHMARK_IDS:-}"
GO_TEST_TIMEOUT="${NEEDLEX_MODEL_BENCHMARK_GO_TEST_TIMEOUT:-45m}"
LIVE_TIMEOUT_MS="${NEEDLEX_MODEL_BENCHMARK_LIVE_TIMEOUT_MS:-25000}"

mkdir -p "$OUT_DIR"

command -v jq >/dev/null 2>&1 || { echo "jq is required" >&2; exit 1; }
command -v "$OLLAMA_BIN" >/dev/null 2>&1 || { echo "ollama binary not found: $OLLAMA_BIN" >&2; exit 1; }

if ! curl -fsS "${BASE_URL%/v1}/api/tags" >/dev/null 2>&1; then
	echo "starting ollama serve on background session"
	"$OLLAMA_BIN" serve >/tmp/needlex-ollama.log 2>&1 &
	OLLAMA_PID=$!
	trap 'kill ${OLLAMA_PID:-0} >/dev/null 2>&1 || true' EXIT
	for _ in $(seq 1 30); do
		curl -fsS "${BASE_URL%/v1}/api/tags" >/dev/null 2>&1 && break
		sleep 1
	done
fi

if [[ -n "$SELECT_IDS" ]]; then
	mapfile -t IDS < <(printf '%s\n' $SELECT_IDS)
else
	mapfile -t IDS < <(jq -r '.candidates[].id' "$CANDIDATES_FILE")
fi

rows_file="$(mktemp)"
trap 'rm -f "$rows_file"' EXIT

for id in "${IDS[@]}"; do
	candidate_json="$(jq -c --arg id "$id" '.candidates[] | select(.id == $id)' "$CANDIDATES_FILE")"
	[[ -z "$candidate_json" ]] && { echo "skipping unknown candidate: $id" >&2; continue; }

	label="$(jq -r '.label' <<<"$candidate_json")"
	router="$(jq -r '.router' <<<"$candidate_json")"
	extractor="$(jq -r '.extractor' <<<"$candidate_json")"
	formatter="$(jq -r '.formatter' <<<"$candidate_json")"
	micro_timeout_ms="$(jq -r '.micro_timeout_ms' <<<"$candidate_json")"
	structured_timeout_ms="$(jq -r '.structured_timeout_ms' <<<"$candidate_json")"
	specialist_timeout_ms="$(jq -r '.specialist_timeout_ms' <<<"$candidate_json")"

	candidate_dir="$OUT_DIR/$id"
	mkdir -p "$candidate_dir"
	hard_case_out="$candidate_dir/hard-case.json"
	live_out="$candidate_dir/live.json"
	hard_case_log="$candidate_dir/hard-case.log"
	live_log="$candidate_dir/live.log"
	rm -f "$hard_case_out" "$live_out" "$hard_case_log" "$live_log"

	echo
	echo "== Benchmarking $id =="
	echo "label=$label"
	declare -A seen_models=()
	for model in "$router" "$extractor" "$formatter"; do
		[[ -z "$model" ]] && continue
		[[ -n "${seen_models[$model]:-}" ]] && continue
		seen_models[$model]=1
		echo "ensuring model $model is available"
		"$OLLAMA_BIN" pull "$model"
	done

	hard_status=0
	NEEDLEX_HARD_CASE_MATRIX_USE_LIVE_BACKEND=1 \
	NEEDLEX_HARD_CASE_MATRIX_CORPUS="$HARD_CASE_CORPUS" \
	NEEDLEX_HARD_CASE_MATRIX_OUT="$hard_case_out" \
	NEEDLEX_HARD_CASE_MATRIX_GO_TEST_TIMEOUT="$GO_TEST_TIMEOUT" \
	NEEDLEX_HARD_CASE_MATRIX_PROGRESS=1 \
	NEEDLEX_MODELS_BACKEND=openai-compatible \
	NEEDLEX_MODELS_BASE_URL="$BASE_URL" \
	NEEDLEX_MODELS_ROUTER="$router" \
	NEEDLEX_MODELS_EXTRACTOR="$extractor" \
	NEEDLEX_MODELS_FORMATTER="$formatter" \
	NEEDLEX_BUDGET_MAX_LATENCY_MS="$LIVE_TIMEOUT_MS" \
	NEEDLEX_MODELS_MICRO_TIMEOUT_MS="$micro_timeout_ms" \
	NEEDLEX_MODELS_STRUCTURED_TIMEOUT_MS="$structured_timeout_ms" \
	NEEDLEX_MODELS_SPECIALIST_TIMEOUT_MS="$specialist_timeout_ms" \
	bash ./scripts/run_hard_case_matrix.sh 2>&1 | tee "$hard_case_log" >/tmp/needlex-bench-hardcase-"$id".log || hard_status=$?
	hard_case_report_exists=0
	[[ -f "$hard_case_out" ]] && hard_case_report_exists=1

	live_status=0
	NEEDLEX_LIVE_READ_USE_COMPARE=1 \
	NEEDLEX_LIVE_READ_CASES="$LIVE_CASES_FILE" \
	NEEDLEX_MODELS_BACKEND=openai-compatible \
	NEEDLEX_MODELS_BASE_URL="$BASE_URL" \
	NEEDLEX_MODELS_ROUTER="$router" \
	NEEDLEX_MODELS_EXTRACTOR="$extractor" \
	NEEDLEX_MODELS_FORMATTER="$formatter" \
	NEEDLEX_BUDGET_MAX_LATENCY_MS="$LIVE_TIMEOUT_MS" \
	NEEDLEX_MODELS_MICRO_TIMEOUT_MS="$micro_timeout_ms" \
	NEEDLEX_MODELS_STRUCTURED_TIMEOUT_MS="$structured_timeout_ms" \
	NEEDLEX_MODELS_SPECIALIST_TIMEOUT_MS="$specialist_timeout_ms" \
	go run ./benchmarks/live_read_eval/runner --cases "$LIVE_CASES_FILE" --out "$live_out" --baseline improvements/live-read-baseline.json 2>&1 | tee "$live_log" >/tmp/needlex-bench-live-"$id".log || live_status=$?
	live_report_exists=0
	[[ -f "$live_out" ]] && live_report_exists=1

	jq -cn \
		--arg id "$id" \
		--arg label "$label" \
		--arg router "$router" \
		--arg extractor "$extractor" \
		--arg formatter "$formatter" \
		--arg hard_case_report "$hard_case_out" \
		--arg live_report "$live_out" \
		--arg hard_case_log "$hard_case_log" \
		--arg live_log "$live_log" \
		--argjson hard_case_status "$hard_status" \
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
		}' >>"$rows_file"
done

jq -s \
	--arg generated_at_utc "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
	--arg candidates_version "$(jq -r '.version' "$CANDIDATES_FILE")" \
	--arg hard_case_corpus "$HARD_CASE_CORPUS" \
	--arg live_cases_file "$LIVE_CASES_FILE" \
	'{}' >/dev/null

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
	--arg candidates_version "$(jq -r '.version' "$CANDIDATES_FILE")" \
	--arg hard_case_corpus "$HARD_CASE_CORPUS" \
	--arg live_cases_file "$LIVE_CASES_FILE" \
	'{
		generated_at_utc: $generated_at_utc,
		candidates_version: $candidates_version,
		hard_case_corpus: $hard_case_corpus,
		live_cases_file: $live_cases_file,
		runs: .
	}' >"$SUMMARY_OUT"

PATH=$PATH:/home/jose/go/bin bash scripts/check_budget.sh . >/dev/null

echo
echo "Benchmark summary written to $SUMMARY_OUT"
