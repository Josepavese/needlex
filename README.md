# Needle-X

Needle-X is a local-first web context compiler for AI agents.

It turns noisy web pages into compact, auditable, proof-carrying context with:
1. deterministic substrate reduction
2. semantic context alignment on a multilingual web
3. bounded model escalation only for benchmark-backed ambiguity solving

## Why Needle-X

What stands out in the current benchmarked runtime:
1. much smaller agent-facing packets than extraction-heavy baselines
2. proof-carrying output instead of raw extracted text only
3. lower post-processing burden for downstream agents
4. strong warm-state local retrieval through `Discovery Memory`

Current benchmark-backed highlights:
1. average packet size on the live competitive run:
   - `Needle-X`: `4436` bytes
   - `Tavily`: `6975`
   - `Jina`: `30565`
   - `Firecrawl`: `72166`
2. packet reduction versus `Jina` baseline:
   - `Needle-X`: about `85.5%` smaller
3. average claim-to-source steps on the live competitive run:
   - `Needle-X`: `1`
   - `Jina`: `2`
   - `Tavily`: `2`
   - `Firecrawl`: `2`
4. average post-processing burden on the live competitive run:
   - `Needle-X`: `0.25`
   - `Jina`: `1.86`
   - `Tavily`: `1.92`
   - `Firecrawl`: `2.5`
5. proof usability on the live competitive run:
   - `Needle-X`: `1.0`
   - `Jina`, `Tavily`, `Firecrawl`: `0`

Interpretation rule:
1. these are advantage metrics, not broad quality-superiority claims
2. they show where Needle-X is strongest today:
   - compactness
   - verification leverage
   - low-friction agent consumption

## Output Philosophy

The primary product surface is compact compiled context, not the full diagnostic envelope.

Default CLI/JSON behavior is optimized for:
1. fast navigation
2. low token consumption
3. immediate agent use

This means:
1. `read --json`, `query --json`, and `crawl --json` return compact payloads by default
2. proof, trace, and full `web_ir` detail remain available, but only through explicit commands or `--json-mode full`
3. auditability is first-class, but it is not the default payload shape

The default packet is AI-first:
1. `kind`
2. primary locator (`url`, `goal`, `selected_url`)
3. `summary`
4. `uncertainty`
5. `chunks`
6. `candidates` when selection matters
7. minimal provenance inline (`source_url`, `source_selector`, `proof_ref`)

Compact packet rule:
1. chunk count is capped, not fixed
2. the packet prefers diverse evidence over redundant near-duplicate chunks

Reference:
- [agent-answer-packet.md](/home/jose/hpdev/Libraries/needlex/docs/agent-answer-packet.md)

## Product Contract

Needle-X is not:
1. a generic scraper
2. a browser agent
3. a search engine
4. an LLM-first web reader

Needle-X is:
1. a local-first runtime for `read`, `query`, and `crawl`
2. a proof-carrying context packer for agents
3. a semantic context compiler that prefers meaning over lexical overlap

## Supported Use Cases

Needle-X is currently meant for:
1. page-level `read` on real web pages
2. seeded and unseeded `query` with deterministic selection and bootstrap discovery
3. bounded `crawl` on linked pages
4. local proof, replay, and diff for agent debugging
5. multilingual context compilation where lexical overlap is too weak to judge meaning

## Non-Goals

Needle-X is not currently meant for:
1. broad web search competition
2. browser automation or authenticated flows
3. form submission or side-effectful navigation
4. heavy multi-model orchestration in the active runtime
5. claiming semantic superiority beyond the measured benchmark scope

## Status

This repository is in active implementation mode, but the active runtime path is real and benchmark-backed.

## Discovery Memory

Needle-X now includes a local SQLite-backed `Discovery Memory`.

What it does:
1. stores previously observed pages, links, proofs, and embeddings locally
2. reuses that evidence before public bootstrap search on later seedless queries
3. turns repeated use into local retrieval leverage

What the current benchmark supports:
1. `cold` seedless state remains weak and best-effort
2. `warm` discovery-memory state is strong on the current benchmark corpus
3. active artifact:
   [discovery-memory-benchmark-latest.json](/home/jose/hpdev/Libraries/needlex/improvements/discovery-memory-benchmark-latest.json)

Current measured warm-state result:
1. `30/30` selected-url correctness on the active discovery-memory benchmark
2. `discovery_memory` is the selected provider in `30/30` warm-state cases

Interpretation rule:
1. this is a warm-state local retrieval claim
2. it is not a claim that Needle-X solves open-web seedless discovery in general

Current active baseline:
1. CPU model baseline SSOT: `Gemma 3 1B` via [model-baseline.md](/home/jose/hpdev/Libraries/needlex/docs/model-baseline.md)
2. active model backend: `openai-compatible`
3. active benchmark-proven model task: `resolve_ambiguity`
4. active semantic context baseline: `intfloat/multilingual-e5-small`

Current implemented baseline includes:
1. deterministic `Acquire`, `Reduce`, and `Segment` in `internal/pipeline`
2. canonical `WebIR` emitted by `read` and `query`
3. end-to-end `read`, `query`, and `crawl` orchestration in `internal/core/service`
4. local proof, trace, replay, and diff in `internal/proof`
5. local state persistence in `.needlex/{traces,proofs,fingerprints,genome}` via `internal/store`
6. CLI and MCP on the same core path in `cmd/needle` and `internal/transport`
7. semantic-first context evaluation and routing support for meaning-sensitive decisions
8. end-to-end tests and budget enforcement passing
9. local `Discovery Memory` in SQLite for reusable seeded/seedless recall

## Current CLI

```bash
go run ./cmd/needle crawl https://example.com --max-pages 3 --max-depth 1 --same-domain
go run ./cmd/needle query https://example.com --goal "proof replay deterministic"
go run ./cmd/needle query --goal "proof replay deterministic"
go run ./cmd/needle query https://example.com --goal "proof replay deterministic" --discovery off
go run ./cmd/needle query https://example.com --goal "proof replay deterministic" --discovery web_search
go run ./cmd/needle read https://example.com
go run ./cmd/needle read https://example.com --json
go run ./cmd/needle read https://example.com --json --json-mode full
go run ./cmd/needle read https://example.com --profile tiny
go run ./cmd/needle read https://example.com --profile deep --json
go run ./cmd/needle replay trace_1
go run ./cmd/needle diff trace_a trace_b --json
go run ./cmd/needle proof trace_1 --json
go run ./cmd/needle proof proof_1 --json
go run ./cmd/needle proof chk_1
go run ./cmd/needle memory stats --json
go run ./cmd/needle memory search "playwright installation" --json
go run ./cmd/needle memory prune --json
go run ./cmd/needle prune --older-than-hours 24
go run ./cmd/needle mcp
```

## Working Documents

- [idea.md](/home/jose/hpdev/Libraries/needlex/idea.md)
- [spec.md](/home/jose/hpdev/Libraries/needlex/spec.md)
- [project-context.md](/home/jose/hpdev/Libraries/needlex/docs/project-context.md)
- [benchmark-report.md](/home/jose/hpdev/Libraries/needlex/docs/benchmark-report.md)
- [model-baseline.md](/home/jose/hpdev/Libraries/needlex/docs/model-baseline.md)
- [semantic-alignment-gate.md](/home/jose/hpdev/Libraries/needlex/docs/semantic-alignment-gate.md)
- [operator-guide.md](/home/jose/hpdev/Libraries/needlex/docs/operator-guide.md)
- [go-to-market.md](/home/jose/hpdev/Libraries/needlex/docs/go-to-market.md)

## Repo Rules

1. No empty architectural stubs.
2. No duplicate logic between CLI and MCP.
3. No feature lands without proof, replay, and budget awareness.
4. No merge if code budget or determinism gates fail.
5. No product claim wider than benchmark evidence.
6. Default transport output must expose compact compiled context before diagnostic detail.

## Budget Check

Use:

```bash
bash scripts/check_budget.sh .
```

## Baseline Commands

Hard-case baseline run:
```bash
./scripts/run_cpu_baseline_matrix.sh
```

Live-read baseline compare:
```bash
NEEDLEX_LIVE_READ_USE_BASELINE_MODELS=1 \
NEEDLEX_LIVE_READ_OUT=improvements/live-read-baseline-cpu-compare.json \
./scripts/run_live_read_eval.sh
```

Multilingual semantic eval:
```bash
NEEDLEX_LIVE_READ_CASES=benchmarks/corpora/live-sites-semantic-global-v1.json \
NEEDLEX_LIVE_READ_OUT=improvements/live-semantic-global-eval-latest.json \
./scripts/run_live_semantic_eval.sh
```
