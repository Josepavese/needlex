# Seedless Discovery Strategy

Status: future capability strategy  
Version: `0.1`  
Scope: Needle-X-compatible bootstrap discovery without mandatory infrastructure

Primary references:
- [project-context.md](/home/jose/hpdev/Libraries/needlex/docs/project-context.md)
- [vademecum.md](/home/jose/hpdev/Libraries/needlex/docs/vademecum.md)
- [discovery-memory-spec.md](/home/jose/hpdev/Libraries/needlex/docs/experimental/discovery-memory-spec.md)
- [model-baseline.md](/home/jose/hpdev/Libraries/needlex/docs/model-baseline.md)

## 1. Problem

`query` without `seed_url` requires a bootstrap source of candidate URLs.

For a repo-only, local-first product, this layer is structurally weak because:
1. public search providers block automation
2. hosted APIs create strategic dependence
3. mandatory self-hosting violates product distribution constraints
4. full per-client web indexing is too heavy

The problem is not page compilation.
The problem is initial candidate acquisition.

## 2. Product Position

Needle-X must not define itself as:
1. a search engine
2. a browser automation vendor
3. an anti-bot bypass toolkit

Needle-X should define seedless discovery as:
1. a best-effort capability
2. an opportunistic bootstrap layer
3. a progressively improving local recall problem

The product identity remains:
1. local-first context compilation
2. proof-carrying output
3. semantic reranking and validation

## 3. Runtime Thesis

The correct long-range flow is:

`Goal -> QueryRewrite -> DiscoveryMemory -> PublicBootstrap(best effort) -> SemanticRerank -> Read -> Proof`

Implications:
1. public web search is fallback, not product identity
2. `query_rewrite` is valuable because it improves bootstrap quality without hardcoded lexical rules
3. `Discovery Memory` is the only path that can improve over time without mandatory infrastructure

## 4. What Needle-X Should Not Do

### 4.1 Not Core Strategy

The following may be useful for research, but must not become product identity:
1. reverse engineering unstable private search endpoints
2. anti-bot evasion arms races
3. browser stealth stacks
4. dependence on one public HTML SERP provider

### 4.2 Not Acceptable as Default

The following violate the intended product shape:
1. mandatory SaaS search providers
2. mandatory self-hosted search servers
3. per-client full-web crawling/indexing
4. browser farm assumptions

## 5. Provider Policy

Public bootstrap providers are allowed only under this framing:
1. optional or best-effort
2. SSOT-configured
3. replaceable
4. failure-classified explicitly
5. never the primary moat

Evaluation criteria:
1. no mandatory payment
2. no mandatory infrastructure
3. usable recall
4. tolerable blocking behavior
5. structured output preferred over HTML parsing

Operational stance:
1. public providers may be used
2. public providers may fail
3. provider failure must not be confused with runtime intelligence failure

## 6. Query Rewrite Role

`query_rewrite` is part of the retrieval moat.

Its role is:
1. preserve entity identity
2. generate bounded, higher-signal search variants
3. improve seedless recall without language-specific heuristic trees

It should not:
1. invent entities
2. run unbounded search planning
3. replace candidate validation

Success criteria:
1. activates only when bootstrap is weak or ambiguous
2. preserves entity fidelity
3. degrades conservatively to original bootstrap

## 7. Discovery Memory Role

`Discovery Memory` is the strategic answer to repo-only seedless discovery.

It is:
1. local
2. cumulative
3. proof-adjacent
4. semantic-first

It is not:
1. a web-scale search engine
2. a server requirement
3. a replacement for `read/query/crawl`

Strategic expectation:
1. cold start remains weak
2. warm local state becomes increasingly strong
3. public bootstrap usage should decline as memory grows

## 8. Recommended Direction

### 8.1 Immediate

Keep seedless discovery framed as best-effort.

Do:
1. keep provider chain SSOT-backed
2. keep provider failures explicit
3. keep `query_rewrite`
4. benchmark bootstrap quality separately from page compilation quality

### 8.2 Intermediate

Add `Discovery Memory` as the first seedless substrate.

Do:
1. populate from successful `read/query/crawl`
2. retrieve locally before public bootstrap
3. bias toward previously validated pages and hosts

### 8.3 Long-Term

Make public search secondary.

Target state:
1. most repeated usage patterns resolve from local memory
2. public bootstrap is needed mainly for cold-start and novelty

## 9. Benchmark Policy

Seedless discovery must be measured on its own axis.

Required metrics:
1. selected URL correctness
2. provider failure rate
3. provider blocked rate
4. rewrite activation rate
5. rewrite win rate
6. local-memory hit rate
7. bootstrap dependence rate

Important:
Do not mix:
1. provider anti-bot failure
2. ranking failure
3. query rewrite failure
4. page compilation failure

## 10. Practical Product Contract

For a repo-only distribution model, the defensible contract is:

1. `read` and seed-based `query` are the strong path
2. seedless `query` is best-effort today
3. quality improves through local memory and bounded semantic retrieval
4. the moat is not public search access
5. the moat is compilation, proof, semantic reranking, and local discovery reuse

## 11. Exit Criteria

This strategy becomes implementation-ready when:
1. `Discovery Memory v1` is active locally
2. public bootstrap is clearly secondary in warm runs
3. `query_rewrite` has benchmark-backed win rate on hard seedless cases
4. product docs explicitly separate guaranteed paths from best-effort discovery
