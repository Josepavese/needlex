# Issue 03: Trace-Driven Semantic Calibration

Status: implemented as guarded semantic feature registry and static calibrator; trace-trained artifact remains pending
Date: 2026-05-04
Priority: P1, after diagnostics and graph evidence are stable
Surface: ranking calibration, analytics, benchmarks, regression prevention

## Objective

Use real Needle-X traces, benchmark outcomes, and analytics to train or calibrate a small local ranking model that improves candidate selection without relying on literal lexical heuristics.

This is not an LLM reasoning feature. It is a local learning-to-rank calibration layer over semantic and structural evidence.

## Semantic Contract

The learned model may use:

1. dense similarity scores
2. late-interaction scores
3. semantic role confidence
4. entity-family graph activation
5. provider semantic quorum signals
6. resource class embeddings
7. proof usability signals
8. historical success/failure labels

The learned model must not use primary lexical features such as:

1. keyword overlap counts
2. language-specific query terms
3. exact surface-form matches
4. provider-specific boosts
5. domain-specific target labels

If a feature cannot be expressed as semantic, structural, provenance, or outcome evidence, it should not be used.

## Research Basis

Primary references:

1. [Unbiased Learning-to-Rank with Biased Feedback](https://arxiv.org/abs/1608.04468)
2. [Counterfactual Online Learning to Rank](https://arvinzhuang.github.io/files/arvin2020counterfactual.pdf)
3. [Adversarial Retriever-Ranker for Dense Text Retrieval](https://arxiv.org/abs/2110.03611)
4. [BEIR: A Heterogeneous Benchmark for Zero-shot Evaluation of Information Retrieval Models](https://arxiv.org/abs/2104.08663)
5. [What are the limits of cross-lingual dense passage retrieval for low-resource languages?](https://arxiv.org/abs/2408.11942)
6. [Hubness Reduction Improves Sentence-BERT Semantic Spaces](https://arxiv.org/abs/2311.18364)

Interpretation for Needle-X:

1. Trace data is biased by the current ranker, so naive training can reinforce mistakes.
2. Pairwise positive/negative candidate data is more useful than isolated success counts.
3. The calibrator should learn score composition and confidence, not memorize words or domains.
4. Hubness and low-resource language weaknesses must be measured, not ignored.

## Product Hypothesis

Needle-X already records enough trace and analytics data to learn which semantic signals are trustworthy. A local calibrator can improve final selection and reduce future regressions.

Target impact:

1. reduce seedless regressions after ranking changes
2. improve selected pass by `+5` to `+10` points after graph and late-interaction signals exist
3. improve confidence calibration for MCP output
4. expose measurable feature contributions for maintainers
5. detect when a new ranking change is overfitting

## Training Data Design

Positive labels:

1. benchmark expected URL or accepted canonical equivalent
2. successful proof-backed selected URL
3. user-verified selected resource if a future confirmation signal exists
4. memory record that later produced a benchmark pass

Negative labels:

1. benchmark selected URL when expected family was available but not selected
2. candidates in wrong family
3. derivative/context/distribution surfaces selected when custodian record was expected
4. runtime-successful but proof-unusable pages
5. previously contradicted graph edges

Weak labels:

1. candidate was in same activated family but not selected
2. provider returned candidate but no proof was usable
3. page was too noisy or low-content
4. final answer had high uncertainty

Holdout rules:

1. split by entity family, not by individual URL
2. split by domain/host where possible
3. preserve multilingual and low-resource cases
4. never train and test on the same benchmark family
5. maintain a no-learning baseline lane

## Feature Set

Allowed feature groups:

1. dense query-candidate similarity
2. late-interaction score and confidence
3. semantic role confidence
4. graph activation score
5. graph edge confidence and contradiction count
6. provider semantic cluster support
7. proof usability probability
8. resource class compatibility embedding
9. acquisition reliability class
10. freshness/decay metadata

Explicitly excluded:

1. exact keyword overlap
2. query token frequency
3. domain allowlists
4. literal URL path term boosts
5. language-specific dictionaries
6. provider-specific boosts not mediated by measured reliability

## Model Strategy

Start with a deliberately small, inspectable model:

1. pairwise logistic ranker
2. isotonic or Platt-style confidence calibration
3. monotonic constraints where useful
4. JSON-serializable model file in PAL
5. no runtime training during normal retrieval
6. shadow evaluation before active use

Avoid:

1. black-box large models
2. hosted training
3. training on raw page text
4. unbounded online learning
5. opaque feature importances

## Implementation Plan

Phase 0: trace dataset builder

1. add internal dataset export from analytics/traces
2. include candidate diagnostics, failure taxonomy, expected URL/family where available
3. create positive, negative, and weak-label rows
4. store model training artifacts outside committed benchmark JSON unless intentionally curated

Phase 1: offline trainer

1. implement Go-native pairwise trainer or use a build-time research script outside product runtime
2. output small model artifact
3. include feature schema version
4. include training corpus fingerprint
5. include validation metrics

Phase 2: shadow calibrator

1. load model in query pipeline only when configured
2. score candidates in shadow mode
3. record would-have-selected URL
4. compare against live selection in analytics
5. emit no extra MCP noise unless debug requested

Phase 3: active calibrator

1. activate only after benchmark gate
2. keep fallback to uncalibrated ranker
3. add release workflow check for model feature schema compatibility
4. add governance check preventing lexical feature names from entering the calibrator schema

## Tests

Unit tests:

1. feature extraction is deterministic
2. excluded lexical features cannot be registered
3. model load validates schema version
4. calibration score is stable
5. missing features fall back safely

Integration tests:

1. trace export includes positives and negatives
2. shadow model records would-have-selected candidate
3. active model changes selection only when confidence margin is high
4. graph contradictions reduce score

Benchmark tests:

1. train on old corpus, test on held-out entity families
2. multilingual holdout
3. unique-domain holdout
4. regression replay of prior ranking failures

## Metrics

Primary:

1. selected pass delta
2. right-family-selected delta
3. wrong-family-selected delta
4. calibration error
5. active override success rate

Safety:

1. overfit gap between train and holdout
2. per-language degradation
3. hubness concentration
4. false positive confidence
5. shadow/live disagreement rate

## Devil's Advocate

Objection: trace-driven learning will learn the current bugs.

Response: train from benchmark outcomes and contradicted candidates, not only successful selections. Use family-level holdouts and counterfactual caution.

Objection: learning-to-rank is a backdoor for lexical heuristics.

Response: make the feature registry explicit and guarded. If a feature encodes literal overlap or language-specific terms, it fails governance.

Objection: local model artifacts make releases harder.

Response: the first calibrator should be tiny, JSON-based, optional, and versioned. Large models belong to Issue 01, not this issue.

Objection: analytics data may contain private or sensitive URLs.

Response: training artifacts stay local by default. Public corpora must be curated and scrubbed before commit.

Objection: low-resource language behavior may degrade silently.

Response: require multilingual holdouts and per-language failure taxonomy before enabling active mode.

## Acceptance Criteria

1. Trace dataset builder exists and exports semantic-only feature rows.
2. Feature registry explicitly blocks lexical-primary features.
3. Calibrator runs in shadow mode and records would-have-selected candidates.
4. Holdout benchmark proves improvement before active mode.
5. Active mode is configurable and safely falls back.
6. No API key, hosted model, or site-specific feature is introduced.
