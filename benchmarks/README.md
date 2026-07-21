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
