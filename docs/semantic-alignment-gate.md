# Semantic Alignment Gate

## Purpose

Needle-X keeps the runtime deterministic-first, but meaning-sensitive decisions can no longer depend primarily on lexical overlap.

The semantic alignment gate exists to evaluate:

1. objective-to-chunk alignment
2. semantic dominance of the best candidate
3. multilingual equivalence when the page language and the user objective diverge

The old lexical gate is now considered auxiliary and weak for context decisions.

## Scope

- Input: objective + top selected chunks
- Output: suppress or allow `resolve_ambiguity`
- Backend: optional embeddings provider
- Default: disabled

This is not a retrieval layer, not a vector database, and not a new ranking substrate.
It is a narrow semantic control plane for context decisions.

## Failure Modes Addressed

1. Cross-lingual lexical mismatch
2. Semantic dominance of a single anchor chunk
3. Synonymic mismatch in the same language
4. Abstract objective wording against concrete page wording

## Contract

- The gate must stay bounded and cheap.
- The gate may suppress a model route immediately.
- The gate may later become the primary context gate for meaning-sensitive routing.
- The gate must stay local to context interpretation and ambiguity routing. It must not become a general retrieval stack.
- If the embeddings backend is unavailable, Needle-X falls back to deterministic behavior.

## Backend

Current implementation supports:

- `ollama /api/embed`
- `openai-like /v1/embeddings`

The semantic gate follows SSOT defaults from:
- [model-baseline.json](../internal/config/modelbaseline/model-baseline.json)

Current SSOT semantic baseline:

- backend: `openai-embeddings`
- model: `intfloat/multilingual-e5-small`
- base_url: `http://127.0.0.1:18180`

Reason:

1. multilingual and CPU-friendly enough for a local gate
2. commercial-friendly license
3. adequate dimensionality and quality for local objective-to-chunk alignment
4. much better fit for a small suppression gate than a heavyweight retrieval model

Other candidates worth later evaluation:

- `Alibaba-NLP/gte-multilingual-base`
- `BAAI/bge-m3`
- `jina-embeddings-v5-text-nano` for internal experiments only where license is acceptable

## Decision Rule

Current implementation is asymmetric and conservative:

- top semantic similarity exceeds `semantic.similarity_threshold`
- the gap to the second candidate exceeds `semantic.dominance_delta`

That means the gate currently removes false-positive ambiguity work.

Strategic direction from now on:

1. structural signals remain first-class
2. semantic alignment becomes the primary meaning-sensitive gate
3. lexical overlap becomes auxiliary:
   - debugging
   - cheap fallback
   - simple noise patterns

Needle-X should not trust lexical matching as the main judge of context meaning on a multilingual web.

## Rollout

1. Keep disabled by default until the semantic gate is benchmarked broadly
2. Enable first in evaluation environments
3. Compare live latency and accepted/rejected interventions before promotion
4. Promote semantic alignment to first-class report metric as soon as live evidence is real
5. Replace lexical meaning-gates in macrosteps, not piecemeal
6. Evaluate eventual removal of lexical gating from meaning-sensitive decisions

## Current Position

As of `2026-03-30`:

1. live multilingual evaluation already shows semantic alignment on real Chinese, German, Russian, Japanese, French, and Spanish pages
2. the same cases previously showed `keyword_coverage = 0.0`, which is why that legacy metric was removed from the active live context reports
3. this means lexical overlap is not merely imperfect; it is empirically blind for a core class of real web inputs

So the doctrine is now:

- semantic alignment is the primary context metric for meaning-sensitive evaluation
- keyword coverage has been removed from the active live context layer

## Architectural Position

Correct layering:

1. `structural substrate`
   - DOM
   - WebIR
   - embedded signals
   - heading evidence
2. `semantic context gate`
   - embeddings-based objective alignment
   - multilingual and synonymic equivalence
3. `lexical auxiliary layer`
   - cheap support signal
   - explainability
   - CTA/noise heuristics
4. `generative escalation`
   - only when structure + semantics still do not resolve the case

This is the stronger direction for a product that aims to compile the web rather than pattern-match strings.

## Local CPU Upstream

Needle-X now ships a local CPU upstream for the SSOT semantic baseline:

- [run_semantic_embed_upstream.py](../scripts/run_semantic_embed_upstream.py)

It exposes:

- `/healthz`
- `/v1/models`
- `/v1/embeddings`

Smoke command:

```bash
./scripts/run_semantic_gate_smoke.sh
```
