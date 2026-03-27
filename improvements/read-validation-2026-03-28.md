# Real-World Read Validation

Date: `2026-03-28`

This document captures real `needle read` trials executed on live websites.

It exists for two reasons:
1. preserve concrete evidence of current runtime behavior
2. derive the next improvement backlog from real failures and partial wins

## Scope

Sites tested:
1. `https://carratellire.com/`
2. `https://www.cnfsrl.it/`
3. `https://halfpocket.net/`

## Commands Used

### Default timeout

```bash
go run ./cmd/needle read https://carratellire.com/
go run ./cmd/needle read https://www.cnfsrl.it/
go run ./cmd/needle read https://halfpocket.net/
```

### Extended timeout

```bash
NEEDLEX_RUNTIME_TIMEOUT_MS=10000 go run ./cmd/needle read https://carratellire.com/
NEEDLEX_RUNTIME_TIMEOUT_MS=10000 go run ./cmd/needle read https://www.cnfsrl.it/
```

## Results Summary

| Site | Default | Extended Timeout | Outcome | Main Issue |
| --- | --- | --- | --- | --- |
| `carratellire.com` | fetch timeout | fetch ok, segmentation failed | failed | JS-heavy app shell with content embedded in scripts/JSON |
| `www.cnfsrl.it` | fetch timeout | success | partial success | live content extracted, but spam/SEO contamination leaked into chunks |
| `halfpocket.net` | success | not needed | success | minor chunk duplication / hierarchical repetition |

## Detailed Outcomes

### 1. `https://carratellire.com/`

Default run:

```text
read failed: fetch page: Get "https://carratellire.com/": context deadline exceeded
```

Extended-timeout run:

```text
read failed: no segments produced
```

Observed page shape from raw HTML inspection:
1. HTTP `200 OK`
2. app shell mounted into `<app-root></app-root>`
3. heavy inline CSS and scripts
4. large business and listing content embedded in `window._a2s = {...}`
5. useful text is present, but mostly inside JSON/script payloads, not clean semantic content blocks

Assessment:
1. fetch path is recoverable with higher timeout
2. reducer/segmenter cannot currently promote embedded structured payloads into readable content
3. this is a real blocker for JS-heavy commercial sites with server-delivered bootstrap state

### 2. `https://www.cnfsrl.it/`

Default run:

```text
read failed: fetch page: Get "https://www.cnfsrl.it/": context deadline exceeded
```

Extended-timeout run:

```text
Title: Homepage - Cnf
URL: https://www.cnfsrl.it/
Chunks: 6
Profile: standard
Proof Records: 6
Stages: 4
Latency: 4680ms
Trace ID: trace_07f2fa6aafbb5ad0
Trace Path: .needlex/traces/trace_07f2fa6aafbb5ad0.json
Proof Path: .needlex/proofs/trace_07f2fa6aafbb5ad0.json
Fingerprint Path: .needlex/fingerprints/trace_07f2fa6aafbb5ad0.json
```

Positive extraction signals:
1. title and company positioning captured correctly
2. intralogistics messaging extracted
3. product/service sections extracted
4. runtime completed end-to-end and persisted trace/proof/fingerprint

Negative extraction signals:
1. unrelated gambling/spam text leaked into chunks
2. ranking kept contaminated content in the final pack
3. current pruning does not aggressively suppress injected SEO or compromised page sections

Assessment:
1. runtime is already usable on this site class
2. the main gap is quality, not basic reachability
3. this is the clearest current example for anti-spam pruning work

### 3. `https://halfpocket.net/`

Default run:

```text
Title: Half Pocket - Sviluppo siti web, app, hardware e software per il business
URL: https://www.halfpocket.net/
Chunks: 6
Profile: standard
Proof Records: 6
Stages: 4
Latency: 1349ms
Trace ID: trace_0dc65bb06e9c9634
Trace Path: .needlex/traces/trace_0dc65bb06e9c9634.json
Proof Path: .needlex/proofs/trace_0dc65bb06e9c9634.json
Fingerprint Path: .needlex/fingerprints/trace_0dc65bb06e9c9634.json
```

Positive extraction signals:
1. title captured correctly
2. main proposition extracted
3. service areas extracted
4. no timeout tuning required
5. output is coherent and usable as compact business context

Minor weaknesses:
1. repeated hierarchy prefixes like `> GR > Design emozionale`
2. some repeated semantic content across nearby chunks
3. chunk naming can still be cleaner

Assessment:
1. this is a strong positive case for the current runtime
2. it proves the core path already works on real business websites with mostly semantic content

## What These Tests Prove

The runtime already handles three different realities:
1. standard business landing pages: good
2. slower websites with usable semantic HTML: acceptable with timeout tuning
3. app-like pages with structured payloads in scripts: not solved yet

This means the product is no longer hypothetical.
It also means the main gaps are now specific and testable.

## Priority Improvement Backlog

### P0

1. Embedded structured payload extraction
   - detect high-signal JSON/script payloads like `window.__STATE__`, `window._a2s`, `__NEXT_DATA__`
   - safely extract business descriptions, listings, titles, and article content from embedded state
   - fallback only when semantic HTML segmentation is weak or empty

2. Contamination and spam pruning
   - suppress chunks with gambling/adult/pharma/SEO-spam signatures when they conflict with site topic
   - add domain-topic coherence checks
   - down-rank isolated off-topic sections even if they contain dense text

3. Better timeout strategy
   - keep default timeout lean
   - add adaptive retry policy for slow-but-valid pages
   - avoid forcing the user to manually raise `NEEDLEX_RUNTIME_TIMEOUT_MS` for common cases

### P1

1. Chunk deduplication
   - remove near-duplicate chunks after packing
   - reduce repeated hierarchy prefixes and repeated body text

2. Better section-title shaping
   - normalize breadcrumb-like prefixes
   - shorten heading chains when they do not add retrieval value

3. Failure-mode classification
   - distinguish:
     - `fetch_timeout`
     - `empty_semantic_dom`
     - `script_embedded_content_detected`
     - `contaminated_page_detected`
   - this will make debugging and roadmap prioritization cleaner

### P2

1. Domain genome upgrades from live tests
   - remember slow domains
   - remember contamination-prone domains
   - remember script-heavy domains that require embedded payload extraction

2. Targeted profile selection
   - use more conservative packing when contamination risk is high
   - bias toward deeper inspection when semantic DOM is sparse

## Recommended Engineering Sequence

The next implementation order should be:
1. add embedded payload detection and extraction
2. add contamination pruning and off-topic down-ranking
3. add adaptive retry/timeout behavior
4. add post-pack dedup and heading normalization
5. feed these outcomes into the domain genome

## Product Reading

If judged only on these three live reads:
1. one site is already good
2. one site is usable but noisy
3. one site exposes a real capability gap

This is a useful distribution.
It shows the runtime is already real, but not yet robust enough for broad unattended market usage.
