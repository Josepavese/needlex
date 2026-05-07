# Issue 05: Semantic Quorum Provider Fusion

Status: implemented for provider observation metadata, health memory, and semantic cluster quorum
Date: 2026-05-04
Priority: P1, P0 if public bootstrap volatility returns as top failure class
Surface: seedless discovery, provider PAL, no-key acquisition resilience

## Objective

Build a provider fusion layer where multiple no-key candidate sources contribute evidence to semantic clusters, and the final selection is made by semantic family confidence rather than provider order or literal overlap.

This is not an anti-bot/stealth feature. It is production-grade resilience through provider diversity, pacing, caching, health memory, and semantic consensus.

## Semantic Contract

The fusion layer must be:

1. semantic cluster first
2. no-key provider only
3. provider-agnostic
4. multilingual
5. embeddings-first
6. failure-taxonomy aware
7. compact in MCP output

It must not become:

1. provider-specific boosts
2. exact URL voting as primary selection
3. same-word quorum
4. stealth/evasion architecture
5. site-specific fallback recipes

Provider observations are evidence. The selected entity/family is chosen by semantic agreement, graph memory, and reranking.

## Research Basis

Primary references:

1. [Reciprocal Rank Fusion outperforms Condorcet and individual rank learning methods](https://plg.uwaterloo.ca/~gvcormac/cormacksigir09-rrf.pdf)
2. [Sparse Meets Dense: A Hybrid Approach to Enhance Scientific Document Retrieval](https://arxiv.org/abs/2401.04055)
3. [Efficient and Effective Retrieval of Dense-Sparse Hybrid Vectors using Graph-based ANN](https://arxiv.org/abs/2410.20381)
4. [BEIR: A Heterogeneous Benchmark for Zero-shot Evaluation of IR Models](https://arxiv.org/abs/2104.08663)
5. [BGE M3-Embedding](https://arxiv.org/abs/2402.03216)
6. [DS@GT at TREC TOT 2025: Fusion Retrieval and Learned Reranking](https://arxiv.org/abs/2601.15518)

Interpretation for Needle-X:

1. Rank fusion is useful when independent retrievers have complementary errors.
2. Naive rank fusion is not enough for Needle-X because it can amplify popular wrong surfaces.
3. The fusion unit should be a semantic family cluster, not a literal URL string.
4. Provider ranks are weak priors. Semantic reranking and graph memory must make the final decision.

## Product Hypothesis

A semantic quorum layer will improve seedless reliability by reducing dependence on any one public no-key provider and by detecting when the top provider result is a derivative, mirror, or context surface.

Target impact:

1. runtime success remains `>= 0.98`
2. seedless pass improves by `+5` to `+10` points before late interaction, higher when combined
3. provider-blocked and unavailable-upstream failures decrease
4. wrong-family selections decrease when providers disagree
5. analytics show which provider families contributed useful evidence

## Provider PAL

Define provider output as observations, not final candidates:

```go
type ProviderObservation struct {
    ProviderID string
    URL string
    Title string
    Snippet string
    Rank int
    RetrievedAt time.Time
    FailureClass string
    HealthScore float64
}
```

Normalize into semantic candidate records:

```go
type SemanticObservation struct {
    Observation ProviderObservation
    CandidateEmbedding []float32
    SourceContextEmbedding []float32
    RoleScores map[string]float64
    ResourceClass string
    FamilyClusterID string
}
```

Fusion output:

```go
type SemanticClusterVote struct {
    ClusterID string
    FamilyConfidence float64
    ProviderDiversity int
    IndependentEvidence float64
    BestRepresentativeURL string
    Reasons []SemanticReason
}
```

## Fusion Algorithm

Step 1: collect observations

1. query no-key providers with pacing and cooldown
2. include Discovery Memory as a provider-like source
3. include graph memory as a provider-like source
4. include cached recent observations if fresh

Step 2: embed observations

1. embed title/snippet/source context as semantic evidence
2. infer resource role
3. infer resource class
4. attach provenance metadata

Step 3: cluster semantically

1. cluster by embedding similarity and role compatibility
2. keep URL/host as provenance, not cluster identity
3. prevent hub candidates from merging unrelated clusters
4. preserve singleton clusters when confidence is low

Step 4: vote on clusters

1. provider diversity increases confidence only if semantic cluster agreement exists
2. rank position is a weak prior
3. repeated provider failures reduce provider health, not candidate meaning
4. graph memory and late interaction can override provider rank

Step 5: choose representative

1. choose the best semantic representative inside the winning family
2. prefer proof-usable, custodian-record-like resources when intent requires records
3. keep assets/resources valid when the query semantically asks for them
4. fetch selected representative

## Implementation Plan

Phase 0: provider observation abstraction

1. standardize provider outputs into `ProviderObservation`
2. preserve current DDG/Bing behavior behind adapters
3. add provider health memory to PAL
4. add failure taxonomy per provider
5. add cache keys based on semantic query fingerprint, not literal query string alone

Phase 1: semantic cluster fusion shadow mode

1. embed observations
2. build clusters
3. compute cluster votes
4. record would-have-selected cluster
5. compare against existing selection

Phase 2: active fusion

1. use cluster vote before candidate limiting
2. rerank representatives with Issue 01 when available
3. consult graph memory from Issue 02
4. emit compact diagnostics in query plan
5. add analytics for provider contribution and cluster confidence

Phase 3: resilience layer

1. provider cooldown on blocked/unavailable failures
2. jitter/backoff/pacing without stealth claims
3. cache fallback when public providers are unavailable
4. majority/median benchmark runs for noisy providers
5. provider chain selection by health, not hardcoded preference

## Tests

Unit tests:

1. provider observations normalize consistently
2. semantic clustering does not require identical URLs
3. provider rank cannot override strong semantic mismatch
4. provider health updates on failure classes
5. singleton clusters are preserved

Integration tests:

1. DDG/Bing observations fuse into one semantic family
2. memory candidate competes with public provider candidates
3. provider blocked triggers cooldown and fallback
4. derivative/context cluster loses to custodian-record cluster when semantics support it
5. asset/resource classes are retained when semantically requested

Benchmark tests:

1. 100-case seedless with current chain
2. 100-case seedless with provider simulated outage
3. 100-case seedless with noisy derivative candidates injected
4. multi-run majority/median evaluation
5. multilingual seedless smoke

## Metrics

Primary:

1. seedless selected pass rate
2. runtime success rate
3. provider-blocked rate
4. unavailable-upstream rate
5. semantic cluster winner accuracy

Provider metrics:

1. provider contribution rate
2. provider useful-observation rate
3. provider health score
4. cooldown count
5. cache fallback success

Semantic safety:

1. wrong-family-selected rate
2. derivative-surface-selected rate
3. context-surface-selected rate
4. distribution-surface-selected rate
5. hub cluster rate

## Devil's Advocate

Objection: rank fusion is not semantic-first.

Response: raw RRF is not the product decision layer. It may be a weak prior inside semantic cluster scoring. The primary unit is the semantic family cluster.

Objection: providers are correlated and can all be wrong.

Response: provider diversity does not guarantee truth. Graph memory, late-interaction reranking, role classification, and proof usability must validate the winner.

Objection: consensus can favor popular mirrors over official records.

Response: semantic role and entity-family graph evidence must distinguish custodian origin, derivative representation, distribution, and social context.

Objection: provider diversity increases network volume.

Response: use health memory, caching, time budgets, and adaptive provider chains. Do not query every provider every time.

Objection: no-key providers are unstable.

Response: that is precisely why the PAL must track health, cooldowns, failure taxonomy, and cache fallback.

## Acceptance Criteria

1. Provider observations are normalized behind adapters.
2. Semantic cluster fusion runs in shadow mode.
3. Cluster diagnostics are compact and inspectable.
4. Active fusion improves seedless pass or runtime reliability without deterministic regression.
5. All providers remain no-key.
6. No provider-specific, site-specific, or lexical-primary rule is introduced.
