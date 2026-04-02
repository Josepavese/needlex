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
1. backend: `openai-embeddings`
2. model: `intfloat/multilingual-e5-small`
3. base URL: `http://127.0.0.1:18180`
4. enabled by default: `false`

Reason:
1. the product needs multilingual objective-to-chunk alignment
2. semantic context is now the primary meaning signal
3. the model is CPU-practical for a narrow gate role
4. provider and model are SSOT-defined, then overrideable by env

## Discovery Baseline

Current SSOT discovery baseline:
1. provider chain: `https://lite.duckduckgo.com/lite/,https://html.duckduckgo.com/html/`
2. primary bootstrap provider: `lite.duckduckgo.com`
3. fallback provider: `html.duckduckgo.com`

Reason:
1. public DuckDuckGo HTML can return anti-bot challenge pages
2. `lite` has been empirically more stable for bootstrap result extraction
3. provider order is product behavior and must not live as an implicit code default

## Override Policy

The baseline is a default, not a lock.

Users can override:
1. main runtime backend
2. main runtime base URL
3. main runtime model id
4. semantic backend
5. semantic base URL
6. semantic model id
7. timeouts
8. discovery provider chain

This lets an operator reuse models already present on the machine without changing repo state.

Relevant override env:
1. `NEEDLEX_MODELS_BACKEND`
2. `NEEDLEX_MODELS_BASE_URL`
3. `NEEDLEX_MODELS_ROUTER`
4. `NEEDLEX_SEMANTIC_BACKEND`
5. `NEEDLEX_SEMANTIC_BASE_URL`
6. `NEEDLEX_SEMANTIC_MODEL`
7. `NEEDLEX_DISCOVERY_PROVIDER_CHAIN`

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
