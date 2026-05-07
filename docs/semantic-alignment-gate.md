# Semantic Alignment Gate

## Purpose

Needle-X keeps the runtime deterministic-first, but meaning-sensitive decisions can no longer depend primarily on surface-form overlap.

The semantic alignment gate exists to evaluate:

1. objective-to-chunk alignment
2. semantic dominance of the best candidate
3. multilingual equivalence when the page language and the user objective diverge

The old surface-form gate is now considered auxiliary and weak for context decisions.

## Scope

- Input: objective + top selected chunks
- Output: suppress or allow `resolve_ambiguity`
- Vectorizer: dense embedding endpoint configured through the PAL SSOT config
- Default: active with local Ollama `embeddinggemma:latest`

This is not a retrieval layer, not a vector database, and not a new ranking substrate.
It is a narrow semantic control plane for context decisions.

## Failure Modes Addressed

1. Cross-lingual surface-form mismatch
2. Semantic dominance of a single anchor chunk
3. Synonymic mismatch in the same language
4. Abstract objective wording against concrete page wording

## Contract

- The gate must stay bounded and cheap.
- The gate may suppress a model route immediately.
- The gate may later become the primary context gate for meaning-sensitive routing.
- The gate must stay local to context interpretation and ambiguity routing. It must not become a general retrieval stack.
- Semantic vectors are produced only by the configured dense embedding endpoint.
- If no endpoint is configured, Needle-X fails instead of falling back to text overlap.

## Backend

Current implementation supports dense HTTP embeddings only.

The semantic gate follows SSOT defaults from:
- [model-baseline.json](../internal/config/modelbaseline/model-baseline.json)

Current SSOT semantic baseline:

- endpoint: `http://127.0.0.1:11434/api/embed`
- provider model: `embeddinggemma:latest`
- vector space: `ollama-embeddinggemma-v1`

Reason:

1. multilingual meaning requires a real embedding space
2. a character or token overlap vector is not acceptable as semantic ranking
3. the endpoint contract is no-key friendly and can be provided by any local or trusted embedding service
4. the default path is bounded and local: no valid endpoint means no valid Needle-X runtime

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
3. surface-form overlap becomes auxiliary:
   - debugging
   - cheap fallback
   - simple noise patterns

Needle-X should not trust surface-form matching as the main judge of context meaning on a multilingual web.

## Rollout

1. Keep dense embeddings mandatory in installed/runtime surfaces
2. Use `needlex config` to change endpoint, model, or vector space
3. Compare live latency and accepted/rejected interventions before changing defaults
4. Promote semantic alignment to first-class report metric as soon as live evidence is real
5. Replace surface-form meaning-gates in macrosteps, not piecemeal
6. Evaluate eventual removal of surface-form gating from meaning-sensitive decisions

## Current Position

As of `2026-03-30`:

1. live multilingual evaluation already shows semantic alignment on real Chinese, German, Russian, Japanese, French, and Spanish pages
2. the same cases previously showed `context_coverage = 0.0`, which is why that retired metric was removed from the active live context reports
3. this means surface-form overlap is not merely imperfect; it is empirically blind for a core class of real web inputs

So the doctrine is now:

- semantic alignment is the primary context metric for meaning-sensitive evaluation
- context coverage has been removed from the active live context layer

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
3. `provenance auxiliary layer`
   - cheap support signal
   - explainability
   - CTA/noise heuristics
4. `generative escalation`
   - only when structure + semantics still do not resolve the case

This is the stronger direction for a product that aims to compile the web rather than pattern-match strings.

## Dense Embedding Smoke

The semantic baseline is exercised directly through core Go tests:

```bash
./scripts/run_semantic_gate_smoke.sh
```
