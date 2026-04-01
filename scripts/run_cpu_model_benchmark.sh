#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
source scripts/lib/ollama.sh
source scripts/lib/model_benchmark.sh

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

needlex_require_command jq "jq is required"
needlex_ollama_ensure_ready "$OLLAMA_BIN" "$BASE_URL"
trap 'needlex_ollama_cleanup; rm -f "$rows_file"' EXIT

mapfile -t IDS < <(needlex_model_benchmark_candidate_ids "$CANDIDATES_FILE" "$SELECT_IDS")

rows_file="$(mktemp)"

for id in "${IDS[@]}"; do
	candidate_json="$(needlex_model_benchmark_candidate_json "$CANDIDATES_FILE" "$id")"
	[[ -z "$candidate_json" ]] && { echo "skipping unknown candidate: $id" >&2; continue; }

	label="$(needlex_model_benchmark_candidate_field "$candidate_json" label)"
	router="$(needlex_model_benchmark_candidate_field "$candidate_json" router)"
	extractor="$(needlex_model_benchmark_candidate_field "$candidate_json" extractor)"
	formatter="$(needlex_model_benchmark_candidate_field "$candidate_json" formatter)"
	micro_timeout_ms="$(needlex_model_benchmark_candidate_field "$candidate_json" micro_timeout_ms)"
	structured_timeout_ms="$(needlex_model_benchmark_candidate_field "$candidate_json" structured_timeout_ms)"
	specialist_timeout_ms="$(needlex_model_benchmark_candidate_field "$candidate_json" specialist_timeout_ms)"

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
	needlex_ollama_pull_unique "$OLLAMA_BIN" "$router" "$extractor" "$formatter"

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

	needlex_model_benchmark_write_row \
		"$id" \
		"$label" \
		"$router" \
		"$extractor" \
		"$formatter" \
		"$hard_case_out" \
		"$live_out" \
		"$hard_case_log" \
		"$live_log" \
		"$hard_status" \
		"$live_status" \
		"$hard_case_report_exists" \
		"$live_report_exists" >>"$rows_file"
done

needlex_model_benchmark_summarize_runs \
	"$rows_file" \
	"$SUMMARY_OUT" \
	"$CANDIDATES_FILE" \
	"$HARD_CASE_CORPUS" \
	"$LIVE_CASES_FILE"

PATH=$PATH:/home/jose/go/bin bash scripts/check_budget.sh . >/dev/null

echo
echo "Benchmark summary written to $SUMMARY_OUT"
