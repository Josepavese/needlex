# Fetch Resilience Roadmap

This document tracks the hardening work for real-world fetch reliability.

It is intentionally scoped to **resilience and compatibility**, not stealth or anti-bot evasion.

## Boundary

Needle-X should:
1. maximize successful acquisition on legitimate targets
2. remain observable and benchmarkable
3. avoid product behavior that depends on deceptive browser simulation or anti-bot bypass tricks

Needle-X should not:
1. implement human-behavior simulation
2. implement browser rotation intended to defeat site protections
3. hide or spoof identity beyond normal client compatibility settings

## Current State

Today the runtime already uses:
1. `req/v3` browser impersonation for `browser_like`
2. Chrome TLS fingerprinting for `hardened`
3. retry profile escalation on `403`, `429`, `503`
4. fallback to standard `net/http` on incompatible `HTTP/2` / `TLS` paths

## What Req Exposes

Official Req documentation and repository indicate support for:
1. HTTP fingerprint impersonation
2. TLS fingerprint controls
3. retry with capped exponential backoff and jitter
4. request / response middleware
5. forcing HTTP versions

References:
1. https://github.com/imroc/req
2. https://req.cool/docs/tutorial/retry/

## Roadmap

### Stage 1: Polite Fetch Hardening

Goal:
1. reduce repeated collisions with rate limits and soft blocks
2. improve operator control

Work:
1. configurable blocked-retry backoff
2. configurable timeout-retry backoff
3. jitter for retries
4. trace metadata for retry profile and retry count
5. per-host pacing hooks
6. provider health persistence and cooldowns for seedless bootstrap
7. provider ordering based on recent success and cooldown state
8. provider-level circuit breaking inside a single seedless request

### Stage 2: Better Compatibility

Goal:
1. improve fetch success on sites with modern delivery stacks

Work:
1. explicit HTTP version controls per profile
2. optional cookie jar / session continuity
3. redirect policy tuning
4. render fallback only where justified

### Stage 3: Candidate Hygiene

Goal:
1. stop wasting fetch budget on low-value candidates
2. improve both seedless discovery and technical retrieval

Work:
1. resource-class annotation
2. host-family / first-party routing
3. endpoint-specific candidate ordering
4. benchmark gates before broad rollout into general seedless

### Stage 4: Measurement Discipline

Goal:
1. separate provider noise from product regressions

Work:
1. benchmark pacing / backoff
2. multiple-run seedless evaluation
3. error taxonomy:
   - provider_blocked
   - ranking_miss
   - timeout
   - unsupported_content_type
4. median and majority reporting, not single-run conclusions

## First Concrete Change

Started in this iteration:
1. configurable blocked retry backoff
2. configurable timeout retry backoff
3. jitter support for retries
4. per-host minimum pacing gap
5. retry and pacing metadata in fetch trace
6. persistent discovery-provider health store
7. provider cooldowns for blocked, timeout, unavailable, and generic failure outcomes
8. dynamic provider ordering from persisted health state
9. provider-level short-circuiting to avoid burning repeated queries on a degraded bootstrap source

This is a resilience improvement, not a stealth feature.
