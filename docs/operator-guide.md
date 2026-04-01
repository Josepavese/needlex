# Operator Guide

This is the shortest practical path to operate Needle-X without repo archaeology.

## What Needle-X Gives You

Every successful `read` and `query` run gives you four things:
1. a compact context payload
2. `web_ir` summary for fast structural inspection
3. proof access for chunk provenance
4. a stored trace for replay and diff

For agent-facing consumption, prefer the compact agent-facing fields first:
1. inline chunk text
2. inline `source_url`
3. inline `source_selector`
4. inline `proof_ref`
5. candidate URLs for seedless discovery

In CLI compact JSON these show up primarily as:
1. `chunks`
2. `candidates`

The internal `agent_context` object still matters at runtime and in transports that expose it directly.

Think of the artifacts like this:
1. `result` answers: what context did Needle-X keep?
2. `web_ir` answers: what structure did Needle-X see?
3. `proof` answers: where did each chunk come from?
4. `trace` answers: what did the runtime do stage by stage?

Operational rule:
1. default CLI JSON is compact by design
2. use full diagnostic envelopes only when debugging, auditing, or comparing runs
3. do not pay token cost for proof/trace detail unless the question requires it
4. the default packet is ordered for AI consumption: locator, summary, evidence, alternatives, signals, cost
5. `uncertainty` is part of the default packet and should be read before opening full diagnostics

Canonical reference:
- [agent-answer-packet.md](/home/jose/hpdev/Libraries/needlex/docs/agent-answer-packet.md)

## Core Commands

### Read

Use `read` when you want one page compiled into compact context.

```bash
go run ./cmd/needle read https://example.com --json
```

Use full diagnostics only when needed:

```bash
go run ./cmd/needle read https://example.com --json --json-mode full
```

Use `--profile tiny` when you want a tighter pack:

```bash
go run ./cmd/needle read https://example.com --profile tiny --json
```

### Query

Use `query` when you have a goal and optionally a seed URL.

Seeded query:

```bash
go run ./cmd/needle query https://example.com --goal "company profile" --json
```

Full diagnostic query payload:

```bash
go run ./cmd/needle query https://example.com --goal "company profile" --json --json-mode full
```

Unseeded query with bootstrap discovery:

```bash
go run ./cmd/needle query --goal "company profile" --json
```

Bootstrap discovery provider order is SSOT-driven. Current default chain:
1. `lite.duckduckgo.com`
2. `html.duckduckgo.com`

Operator override:
```bash
export NEEDLEX_DISCOVERY_PROVIDER_CHAIN='https://lite.duckduckgo.com/lite/,https://html.duckduckgo.com/html/'
```

Strict single-page query:

```bash
go run ./cmd/needle query https://example.com --goal "company profile" --discovery off --json
```

### Crawl

Use `crawl` when you want bounded exploration from a seed page.

```bash
go run ./cmd/needle crawl https://example.com --max-pages 5 --max-depth 1 --same-domain --json
```

Full diagnostic crawl payload:

```bash
go run ./cmd/needle crawl https://example.com --max-pages 5 --max-depth 1 --same-domain --json --json-mode full
```

## How To Read The Output

### 1. Result Pack

Look at compact JSON first:
1. `summary`
2. `uncertainty`
3. `chunks`
4. `outline`
5. `links`
6. `web_ir_summary`
7. `cost_report`

This tells you what Needle-X decided to keep and how expensive the run was.

### 1b. Agent Context

Look at compact `chunks` and `candidates` first when the caller is an AI agent and you want less join work.

It gives you:
1. `summary`
2. `uncertainty`
3. `kind`
4. `selected_url` when applicable
5. `selection_why` on query runs
6. `chunks[].text`
7. `chunks[].source_url`
8. `chunks[].source_selector`
9. `chunks[].proof_ref`
10. `candidates`

This is the fastest path when the agent needs evidence-backed context without separately joining `chunks`, `sources`, and `proof`.

Operator note:
1. compact `chunks` are diversity-biased
2. fewer chunks is often better than spending tokens on near-duplicates
3. compact output also de-prioritizes structurally weak tail fragments when a stronger explanatory chunk already exists

### 2. WebIR

Look at `web_ir_summary` first. Use full `web_ir` only when you need to understand page structure rather than final selection.

Use it to answer:
1. did the runtime see heading-backed structure?
2. did embedded or app-shell evidence dominate?
3. was the page mostly noise or usable content?

### 3. Proof

Load proof by trace id, proof ref, or chunk id:

```bash
go run ./cmd/needle proof trace_123 --json
go run ./cmd/needle proof proof_123 --json
go run ./cmd/needle proof chk_123 --json
```

Look at:
1. `source_span`
2. `transform_chain`
3. `lane`
4. `risk_flags`

Use proof when you need provenance and trust.

### 4. Trace

Replay a run:

```bash
go run ./cmd/needle replay trace_123 --json
```

Diff two runs:

```bash
go run ./cmd/needle diff trace_a trace_b --json
```

Use trace when you need to answer:
1. what changed between runs?
2. did the runtime stay deterministic?
3. where did a model-assisted escalation happen?

## Storage Layout

Needle-X persists local state under `.needlex/` by default.

Installed setups can override the state root with `NEEDLEX_HOME`.

Important paths:
1. `.needlex/traces/`
2. `.needlex/proofs/`
3. `.needlex/fingerprints/`
4. `.needlex/genome/`
5. `.needlex/discovery/discovery.db`

This is local-first product state, not disposable cache.

## Discovery Memory

Use `memory` when you want to inspect or control the local discovery store.

Stats:

```bash
go run ./cmd/needle memory stats --json
```

Search local memory directly:

```bash
go run ./cmd/needle memory search "playwright installation" --json
```

Prune the local discovery store using configured limits:

```bash
go run ./cmd/needle memory prune --json
```

Operator rule:
1. `memory search` is local recall, not public web search
2. `memory prune` keeps the store bounded using configured document, edge, and embedding limits
3. the canonical store is SQLite under `.needlex/discovery/`

Evaluation artifacts live under `improvements/`.

Operator rule:
1. treat `improvements/` root as the active working surface
2. expect only `baseline` and `latest` style reports there
3. look under `improvements/archive/` for historical waves, provider experiments, and empirical one-offs

## Active Runtime Contract

The active runtime today is intentionally narrow:
1. deterministic substrate is primary
2. semantic context alignment is the primary meaning signal
3. only `resolve_ambiguity` is active as a benchmark-proven model task
4. CPU baseline is `Gemma 3 1B`

This matters operationally:
1. most runs stay deterministic
2. model activation is bounded and visible in proof/trace
3. lexical overlap is not the primary meaning judge
4. seedless web discovery provider order is part of config, not a hidden default

## Baseline Commands

Hard-case baseline:

```bash
./scripts/run_cpu_baseline_matrix.sh
```

Live compare baseline:

```bash
NEEDLEX_LIVE_READ_USE_BASELINE_MODELS=1 \
NEEDLEX_LIVE_READ_OUT=improvements/live-read-baseline-cpu-compare.json \
./scripts/run_live_read_eval.sh
```

Multilingual semantic evaluation:

```bash
NEEDLEX_LIVE_READ_CASES=benchmarks/corpora/live-sites-semantic-global-v1.json \
NEEDLEX_LIVE_READ_OUT=improvements/live-semantic-global-eval-latest.json \
./scripts/run_live_semantic_eval.sh
```

Active reports usually land in:
1. [live-read-latest.json](/home/jose/hpdev/Libraries/needlex/improvements/live-read-latest.json)
2. [hard-case-matrix-latest.json](/home/jose/hpdev/Libraries/needlex/improvements/hard-case-matrix-latest.json)
3. [discovery-eval-latest.json](/home/jose/hpdev/Libraries/needlex/improvements/discovery-eval-latest.json)

## Recommended Operator Workflow

For a new integration, use this order:
1. `read --json` on representative URLs
2. inspect `chunks`, `web_ir_summary`, and `cost_report`
3. inspect `proof` on one good chunk and one doubtful chunk
4. use `query` only after page-level trust is understood
5. use `replay` and `diff` when changes appear between runs

## Failure Triage

If output quality looks wrong:
1. inspect compact `chunks` and `web_ir_summary` first
2. inspect `proof` second
3. inspect `trace` third
4. only then change discovery, profile, or model configuration

If latency looks wrong:
1. compare deterministic run versus compare run
2. inspect `cost_report`
3. inspect trace for escalation stages

## Integration Rule

Do not treat Needle-X as a raw HTML reader.
Treat it as a context compiler that gives your agent:
1. compact context
2. provenance
3. replayability
4. structural inspection

Default transport philosophy:
1. compact output is the product surface
2. full diagnostics are opt-in
3. rapid navigation and low token cost take precedence over exposing every internal artifact by default
