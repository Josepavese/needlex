# Operator Guide

This is the shortest practical path to operate Needle-X without repo archaeology.

## What Needle-X Gives You

Every successful `read` and `query` run gives you four things:
1. a compact context payload
2. `web_ir` summary for fast structural inspection
3. proof access for chunk provenance
4. a stored trace for replay and diff

Every successful `read` and `query` run now also carries a compact analytics footer:
1. chars saved
2. compression ratio
3. proof-backed yes/no
4. local-memory vs public-bootstrap use
5. topic-node usage when applicable

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
- [agent-answer-packet.md](agent-answer-packet.md)

## Core Commands

### Read

Use `read` when you want one page compiled into compact context.

```bash
needlex read https://example.com --json
```

Use full diagnostics only when needed:

```bash
needlex read https://example.com --json --json-mode full
```

Use `--profile tiny` when you want a tighter pack:

```bash
needlex read https://example.com --profile tiny --json
```

Fetch profile rule:
1. product default is `browser_like`
2. retry default is `hardened`
3. use `standard` only for benchmark/debug comparability

Example benchmark override:

```bash
NEEDLEX_FETCH_PROFILE=standard NEEDLEX_FETCH_RETRY_PROFILE=standard needlex read https://example.com --json
```

### Query

Use `query` when you have a goal and optionally a seed URL.

Seeded query:

```bash
needlex query https://example.com --goal "company profile" --json
```

Full diagnostic query payload:

```bash
needlex query https://example.com --goal "company profile" --json --json-mode full
```

Unseeded query with bootstrap discovery:

```bash
needlex query --goal "company profile" --json
```

Bootstrap discovery provider order is SSOT-driven. Current default chain:
1. `lite.duckduckgo.com`
2. `html.duckduckgo.com`
3. `www.bing.com`

Operator override:
```bash
export NEEDLEX_DISCOVERY_PROVIDER_CHAIN='https://lite.duckduckgo.com/lite/,https://html.duckduckgo.com/html/,https://www.bing.com/search'
```

Strict single-page query:

```bash
needlex query https://example.com --goal "company profile" --discovery off --json
```

### Crawl

Use `crawl` when you want bounded exploration from a seed page.

```bash
needlex crawl https://example.com --max-pages 5 --max-depth 1 --same-domain --json
```

Full diagnostic crawl payload:

```bash
needlex crawl https://example.com --max-pages 5 --max-depth 1 --same-domain --json --json-mode full
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
8. `analytics`

This tells you what Needle-X decided to keep and how expensive the run was.
The `analytics` footer tells you how much work Needle-X avoided for the agent and whether the result came from public bootstrap or local-first recovery.

## Analytics

Use Analytics PAL when you want product-visible value numbers first and maintainer diagnostics second.

Core commands:

```bash
needlex analytics stats
needlex analytics recent --limit 20
needlex analytics value-report
needlex analytics hosts --limit 20
needlex analytics providers --limit 20
needlex analytics failures --limit 20
needlex analytics daily --limit 30
needlex analytics export --out /tmp/needlex-analytics-export
needlex doctor
needlex support bundle --out /tmp/needlex-support
```

Interpretation rule:
1. `stats` is the quick health/value snapshot: run counts, saved chars/tokens, DB path
2. `value-report` is front-of-house and demo-friendly
3. `hosts` tells you where Needle-X wins or struggles on real target families
4. `providers` tells you whether the value came from local-first recovery, same-site expansion, or public bootstrap
5. `failures` shows the failure-class mix: blocks, timeouts, missing pages, unsupported content, empty candidates
6. `daily` tells you whether those numbers are improving or regressing over time
7. `export` makes the substrate portable for dashboards, audit, and offline analysis
8. `doctor` verifies the effective local home, DB paths, runtime log health, binary path, and active MCP processes
9. `support bundle` exports doctor, analytics, runtime log stats/tail, and redacted runtime log files into one diagnostic directory

Token and cost rule:
1. Analytics stores canonical character counts in SQLite
2. token counts are derived retroactively with the explicit `chars_per_token_estimate` policy
3. cost savings are scenario values at fixed `$ / million tokens`, not provider-price claims
4. use tokenizer-exact pricing only when a future provider-specific tokenizer layer is wired

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
needlex proof trace_123 --json
needlex proof proof_123 --json
needlex proof chk_123 --json
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
needlex replay trace_123 --json
```

Diff two runs:

```bash
needlex diff trace_a trace_b --json
```

Use trace when you need to answer:
1. what changed between runs?
2. did the runtime stay deterministic?
3. where did a model-assisted escalation happen?

## Storage Layout

Needle-X persists local state under the PAL state root by default.

Installed setups can override the state root with `NEEDLEX_HOME`.

Resolution order:
1. `NEEDLEX_HOME` when set
2. otherwise the platform application data directory

Default roots:
1. Linux: `$XDG_DATA_HOME/needlex` or `~/.local/share/needlex`
2. macOS: `~/Library/Application Support/NeedleX`
3. Windows: `%LOCALAPPDATA%\NeedleX`

For `needlex mcp`, the runtime uses this same stable absolute state root and never a relative cwd-dependent root during session handling.

Important paths:
1. `<state-root>/traces/`
2. `<state-root>/proofs/`
3. `<state-root>/fingerprints/`
4. `<state-root>/genome/`
5. `<state-root>/analytics/analytics.db`
6. `<state-root>/discovery/discovery.db`
7. `<state-root>/discovery/providers/`
8. `<state-root>/logs/needlex.jsonl`

This is local-first product state, not disposable cache.

## Runtime Logs

Needle-X writes runtime diagnostics to one PAL-owned JSONL log:

```bash
needlex logs path
needlex logs stats
needlex logs tail --limit 20
needlex logs tail --limit 20 --json
```

Behavior:
1. stdout remains reserved for command output or MCP protocol payloads
2. stderr gives a short failure class, `diagnostic_id`, and log path
3. detailed errors, stack traces, unexpected provider responses, and noisy seedless warnings go to `<state-root>/logs/needlex.jsonl`
4. logs rotate automatically instead of growing without bound
5. secrets in common token/API-key/password forms are redacted before persistence

Fetch/provider diagnostics:
1. successful reads and query reads emit `fetch.completed`
2. query discovery emits `discovery.completed`
3. crawl emits `crawl.completed` plus per-page fetch events
4. fetch/discovery failures emit failure-classed runtime errors before the compact stderr pointer is printed

Support bundle:

```bash
needlex support bundle --out /tmp/needlex-support
needlex support bundle --out /tmp/needlex-support --json
```

The bundle intentionally excludes traces, proofs, fingerprints, and source files by default.

## MCP Transport

Start the server with:

```bash
needlex mcp
```

Transport behavior:
1. accepts standard MCP `Content-Length` framing
2. also accepts raw newline-delimited JSON-RPC
3. replies in the same framing style used by the client

Advertised tool surface:
1. `tools/list` exposes 7 core `web_*` tools: `web_read`, `web_query`, `web_crawl`, `web_proof`, `web_replay`, `web_diff`, `web_prune`
2. `tools/list` also exposes `memory` and `analytics` as advanced dispatchers
3. `memory` actions are `stats`, `search`, `prune`, `export`, `import`, and `rebuild_index`
4. `analytics` actions are `stats`, `recent_runs`, `value_report`, `hosts`, `providers`, `failures`, `daily`, and `export`
5. old narrow memory/analytics tool names are retired and are not valid MCP tools

Operational rules:
1. use raw JSON for clients like OpenCode or simple local wrappers
2. use framed stdio for strict MCP clients
3. do not write wrapper noise to stdout around `needlex mcp`

Example raw initialize:

```bash
printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{},"protocolVersion":"2024-11-05","clientInfo":{"name":"test","version":"1.0"}}}\n' | needlex mcp
```

Logging:
1. MCP diagnostics use the same PAL runtime log as the CLI
2. inspect it with `needlex logs path` and `needlex logs tail`
3. stdout is reserved for protocol output
4. stderr should stay quiet except for fatal startup failures

## Discovery Memory

Use `memory` when you want to inspect or control the local discovery store.

Discovery Memory is enabled by default. Successful `read`, `query`, and `crawl` runs feed it automatically under the PAL state root. If the configured embedding backend is unavailable, Needle-X uses a native local semantic fallback so the store still grows and remains useful for seedless discovery.

Stats:

```bash
needlex memory stats --json
```

Search local memory directly:

```bash
needlex memory search "playwright installation" --json
```

Export the store to JSONL:

```bash
needlex memory export --out /tmp/needlex-memory
```

Import a previously exported store:

```bash
needlex memory import --in /tmp/needlex-memory
```

Rebuild vector/search indexes after import or maintenance:

```bash
needlex memory rebuild-index --json
```

Prune the local discovery store using configured limits:

```bash
needlex memory prune --json
```

Operator rule:
1. `memory search` is local recall, not public web search
2. `memory prune` keeps the store bounded using configured document, edge, and embedding limits
3. `memory export` and `memory import` are operator tools for portability, backup, and warm-state seeding
4. `memory rebuild-index` is the maintenance path after bulk import or index drift
5. the canonical store is SQLite under `<state-root>/discovery/`
6. for seedless `query`, proof-backed memory can short-circuit public bootstrap

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
3. surface-form overlap is not the primary meaning judge
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
1. [live-read-latest.json](../improvements/live-read-latest.json)
2. [hard-case-matrix-latest.json](../improvements/hard-case-matrix-latest.json)
3. [discovery-eval-latest.json](../improvements/discovery-eval-latest.json)

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
