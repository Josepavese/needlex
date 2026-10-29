# Benchmarks

This directory contains the reproducible evaluation harness for Needle-X.

It is intentionally separate from the core runtime so that:
1. product code stays easier to read
2. benchmark logic stays inspectable as a standalone surface
3. anyone can run the same evaluation flows without repo archaeology
4. only active `*-latest` and `*-baseline` artifacts should remain in `improvements/`

## Structure

1. `corpora/`
2. `seeded/runner/`
3. `competitive/runner/`
4. `discovery_memory/runner/`
5. `live_read_eval/runner/`
6. `discovery_eval/runner/`
7. `hard_case_matrix/runner/`

## Main Runs

Seeded benchmark:

```bash
go run ./benchmarks/seeded/runner \
  --cases benchmarks/corpora/seeded-corpus-v2.json \
  --out improvements/seeded-benchmark-latest.json
```

Competitive benchmark:

```bash
go run ./benchmarks/competitive/runner \
  --cases benchmarks/corpora/competitive-corpus-v1.json \
  --out improvements/competitive-benchmark-latest.json
```

Discovery Memory benchmark:

```bash
go run ./benchmarks/discovery_memory/runner \
  --cases benchmarks/corpora/seeded-corpus-v2.json \
  --out improvements/discovery-memory-benchmark-latest.json
```

Unique-source seeded benchmark:

```bash
go run ./benchmarks/seeded/runner \
  --cases benchmarks/corpora/unique-sources-corpus-v1.json \
  --out improvements/unique-sources-seeded-latest.json
```

Live-read evaluation:

```bash
go run ./benchmarks/live_read_eval/runner \
  --cases benchmarks/corpora/live-sites-market-v2.json \
  --out improvements/live-read-latest.json \
  --baseline improvements/live-read-baseline.json
```

Discovery evaluation:

```bash
go test ./benchmarks/discovery_eval/runner -run TestExportDiscoveryEval -count=1 -v
```

The ordinary test writes its report to a temporary directory. Use
`./scripts/run_discovery_eval.sh` to export to `improvements/` explicitly,
or set `NEEDLEX_DISCOVERY_EVAL_OUT` to another destination.

Experimental seedless DDG smoke with explicit profile/provider selection:

```bash
go run ./benchmarks/seedless_ddg/runner \
  --runs 1 \
  --profiles browser_like_semantic \
  --provider-chains 'ddg_bing=https://lite.duckduckgo.com/lite/,https://html.duckduckgo.com/html/,https://www.bing.com/search' \
  --out improvements/seedless-ddg-benchmark-latest.json
```

Hard-case matrix:

```bash
go test ./benchmarks/hard_case_matrix/runner -run TestExportHardCaseMatrix -count=1 -v
```

## Rules

1. benchmark harness code belongs here, not in `internal/`
2. product tests remain near product code
3. reports stay in `improvements/`
4. paid-provider cache remains local in `.needlex/`

## MCP Chromium reuse

`render_reuse/measure.py` runs a deterministic local JS application through the
real MCP executable. It asserts rendered DOM, fetch, XHR, finite SSE/WebSocket,
streamed fetch and repeated same-origin isolation content in the resulting
WebIR. Linux RAM sampling aggregates PSS and RSS over every thread's process
children; the embedding server and fixture driver are outside that tree.

Build before/after executables outside the repository. Supply the same saved
SSOT configuration and browser to both, and run them serially without concurrent
build/test workloads:

```bash
python3 benchmarks/render_reuse/measure.py --binary /tmp/owned/before --config /tmp/owned/config.json --rounds 3 --idle-seconds 31 --out /tmp/owned/before.json
python3 benchmarks/render_reuse/measure.py --binary /tmp/owned/after --config /tmp/owned/config.json --rounds 3 --idle-seconds 31 --out /tmp/owned/after.json
```

Use an idle observation longer than the effective browser idle timeout. The
harness owns temporary state/profiles, records browser starts and surviving
process identities, and retires its state after the run. Inspect the retained
JSON for complete WebIR and render diagnostics. Consolidate the sufficient
measurement evidence before retiring owned binaries and raw intermediates.
Native regression tests separately cover active streams, truncation, crash,
cleanup, concurrency, cancellation and browser storage isolation.

## Local decision qualification on public pages

`decision_observer/live.py` reads the union of the two existing 100-entry corpora
through the installed MCP executable. The union has 176 distinct expected URLs
on 109 hosts; it is not a 1,000-site corpus. It reads those exact URLs and does
not exercise the original same-site routing tasks. A nonempty packet is a
transport result, not a content-completeness or correctness pass.

```bash
python3 benchmarks/decision_observer/live.py \
  --binary /path/to/installed/needlex-real --config /path/to/private/config.json \
  --corpus benchmarks/corpora/unique-sources-corpus-v1.json \
  --corpus benchmarks/corpora/seeded-corpus-v2.json \
  --model-pid MODEL_PID --out improvements/decision-live-latest.jsonl
```

`--resume` validates input identity and reuses completed URL receipts, including
failures. `--limit` permits an admission smoke followed by a full resume. RAM is
50 ms process-tree PSS plus the shared decision process; the embedding service
and Python driver are excluded. Browser scratch is retired per batch. An actual
binary honors the isolated PAL home; the installed shell wrapper can override
that home and retain useful proof/trace artifacts in canonical user state.

The current `claim-support-corpus-v1.json` preserves 52 verified source excerpts
and their titles, URLs, traces and hashes. Each has supported, contradicted and
not-established assertions: 156 correlated cases in six languages. A frozen
split by registrable site family has 51 development cases and 105 held-out
cases. Labels were authored and reviewed by the agent against the excerpts,
not independently human-reviewed and never generated from Julia output. They
concern the supplied passage, not the entire page or external truth.

```bash
decision_work=$(mktemp -d /tmp/needlex-decision-eval.XXXXXX)
python3 benchmarks/decision_observer/prepare.py \
  --anchors benchmarks/corpora/claim-support-corpus-v1.json \
  --out "$decision_work/cases.json"
GOTMPDIR="$decision_work" go build -o "$decision_work/runner" ./benchmarks/decision_observer/runner
"$decision_work/runner" --config /path/to/private/config.json \
  --cases "$decision_work/cases.json" --partition heldout --with-semantic-reference \
  > improvements/claim-quality-latest.jsonl
python3 benchmarks/decision_observer/summarize.py \
  --live improvements/claim-live-latest.jsonl --cases "$decision_work/cases.json" \
  --quality improvements/claim-quality-latest.jsonl --partition heldout \
  --out improvements/claim-eval-latest.json
rm -r "$decision_work"
```

Warm the shared embedding service separately and record its cold-start time
before collecting the dense reference. An absent vector remains unavailable;
it is never zero similarity. Run classification after live collection to avoid
inference contention. `--encoding json` is a development-only format ablation,
not an alternative production contract. Do not tune on held-out outputs.

Live reads omit assertions by default. Add `--assertions` with the prepared
corpus to request explicit supported-claim checks on matching URLs. Add
`--coverage-anchors benchmarks/corpora/claim-support-corpus-v1.json` to record
exact excerpt presence in the complete compiled text, independently of its
bounded snapshot; such markers are test oracles, never retrieval ranking.
Optional `prepare.py --receipts` checks source identity against frozen receipts.
The report separates availability, confusion, false support, support recall,
not-established judgments, latency and a constant-label reference. The cosine
threshold in the qualification report was fitted on development only and is
not a deployed factual-support classifier.

The earlier 208-case/624-judgment broad audit used the retired generic
`evidence-audit-v1` specification. Its corpus, runner and reproduction procedure
are preserved at commit `55bfaab`; do not feed that format to the current runner
or compare its accuracy directly with the new three-class task. Raw benchmark
outputs remain local under `improvements/`.
