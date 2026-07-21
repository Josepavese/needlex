# Model Baseline

This document defines the current SSOT model configuration for Needle-X.

SSOT file:
- [model-baseline.json](../internal/config/modelbaseline/model-baseline.json)

## Active CPU Baseline

Current selected CPU baseline:
1. candidate id: `gemma3_1b_all`
2. micro solver: `gemma3:1b-it-q8_0`
3. backend: `openai-compatible`
4. active benchmark-proven task: `resolve_ambiguity`

Reason:
1. same hard-case quality as the other Wave 1 candidates
2. materially lower hard-case compare latency
3. lower live compare latency
4. fewer live runtime errors

## Semantic Gate Baseline

Current SSOT semantic gate baseline:
1. semantic gate enabled by default: `true`
2. embedding endpoint: `http://127.0.0.1:11434/api/embed`
3. provider model: `embeddinggemma:latest`
4. vector space: `ollama-embeddinggemma-v1`
5. if semantic config is missing, disabled, or incomplete, Needle-X must fail instead of falling back to lexical ranking

Reason:
1. the product needs multilingual objective-to-chunk alignment
2. semantic context is now the primary meaning signal
3. Needle-X must not emulate meaning with surface or sub-token overlap
4. the endpoint contract is local/no-key friendly and provider-neutral
5. provider model and vector-space identity are separate so memory never mixes incompatible vectors
6. installed commands read the PAL SSOT config at `<state-root>/configs/needlex.json`

## Discovery Baseline

Current SSOT discovery baseline:
1. provider chain: `https://lite.duckduckgo.com/lite/,https://html.duckduckgo.com/html/,https://www.bing.com/search`
2. primary bootstrap provider: provider-health ordered chain
3. fallback provider: next healthy provider in the chain

Reason:
1. public DuckDuckGo HTML can return anti-bot challenge pages
2. provider health memory should route around blocked, timed out, or unavailable providers
3. provider order is product behavior and must not live as an implicit code default

## Discovery Memory Baseline

Current SSOT memory baseline:
1. backend: `sqlite`
2. path: `discovery/discovery.db`
3. enabled by default: `true`
4. vector-space identity: inherited from the active dense embedder

Reason:
1. experimental seedless discovery should improve as the tool is used without entering the stable agent path implicitly
2. local proof-backed memory should be tried before public bootstrap
3. memory must never fork a second embedding identity from semantic config
4. without a valid embedding endpoint, operational use is invalid; memory must not invent vectors

## Override Policy

The baseline is a default, not a lock.

Users can override through `needlex config set`:
1. main runtime backend
2. main runtime base URL
3. main runtime model id
4. semantic embedding endpoint
5. semantic provider model id
6. semantic vector-space id
7. timeouts
8. discovery provider chain

This lets an operator reuse models already present on the machine without changing repo state.

Relevant commands:
```bash
needlex config path
needlex config show
needlex config set semantic.embedding_url http://127.0.0.1:11434/api/embed
needlex config set semantic.provider_model embeddinggemma:latest
needlex config set semantic.vector_space ollama-embeddinggemma-v1
```

The installer wrapper sets `NEEDLEX_CONFIG` to the PAL config path. Operators should prefer `needlex config` over per-shell env vars.

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
