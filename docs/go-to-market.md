# Go-To-Market

Needle-X is a local-first web context compiler for AI agents.

Short version:
1. smaller packets
2. proof-carrying output
3. less cleanup for downstream agents
4. strong warm-state local retrieval

## One-Liner

Turn noisy pages into compact, proof-carrying context.

## What We Can Say

These claims are benchmark-backed today:
1. Needle-X is much smaller than extraction-heavy stacks on agent-facing output
2. Needle-X is the only measured runtime in the live comparison with usable proof-carrying output
3. Needle-X reduces claim-to-source distance
4. Needle-X lowers post-processing burden for downstream agents
5. warm-state `Discovery Memory` is strong on the active local benchmark

## Live Advantage Snapshot

| Metric | Needle-X | Tavily | Jina | Firecrawl |
| --- | ---: | ---: | ---: | ---: |
| Avg packet bytes | 4436 | 6975 | 30565 | 72166 |
| Claim-to-source steps | 1 | 2 | 2 | 2 |
| Post-processing burden | 0.25 | 1.92 | 1.86 | 2.50 |
| Proof usability | 1.0 | 0 | 0 | 0 |

Read it like this:
1. Needle-X wins on compactness
2. Needle-X wins on verification leverage
3. Needle-X wins on downstream agent ergonomics

## Discovery Memory Story

This is the right way to say it:
1. first run observes and compiles
2. later runs reuse local verified evidence
3. repeated use improves local retrieval without hosted infra

Current warm-state benchmark:
1. `30/30` selected-url correctness
2. `30/30` `discovery_memory` provider selection

Guardrail:
1. warm-state claim only
2. not a cold-state open-web seedless claim

## What Needle-X Is

1. local-first context compiler
2. proof-carrying retrieval runtime
3. semantic web-context layer for agent systems

## What Needle-X Is Not

1. browser agent
2. search engine
3. universal extractor
4. hosted scraping platform

## Best Early Users

1. agent builders who need traceable web context
2. local-first operators who want auditable outputs
3. teams that care about replay, diff, proof, and debugging
4. multilingual workflows where lexical matching is weak

## Surface We Can Defend

1. `read`
2. `query`
3. `crawl`
4. `proof`
5. `replay`
6. `diff`

Behind that:
1. deterministic substrate
2. semantic context alignment
3. bounded ambiguity solving only when benchmark-backed

## Things We Should Not Say

1. that Needle-X replaces web search
2. that it solves cold-state seedless discovery
3. that it is a browser agent
4. that it has broad market-quality superiority today
5. that model escalation is broadly superior everywhere

## Release Readiness Check

1. `go test ./... -count=1`
2. `bash scripts/check_budget.sh .`
3. README, spec, operator guide, benchmark report, and vademecum aligned
4. baseline commands run on maintainer machine
5. public claim remains narrow and benchmark-backed
