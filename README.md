# Needle-X

[![dist](https://github.com/Josepavese/needlex/actions/workflows/dist.yml/badge.svg)](https://github.com/Josepavese/needlex/actions/workflows/dist.yml)
[![installer-smoke](https://github.com/Josepavese/needlex/actions/workflows/installer-smoke.yml/badge.svg)](https://github.com/Josepavese/needlex/actions/workflows/installer-smoke.yml)
[![release](https://img.shields.io/github/v/release/Josepavese/needlex?display_name=tag)](https://github.com/Josepavese/needlex/releases/latest)

> [!WARNING]
> Alpha software. Needle-X is still in active development and test. Install flow, local state layout, CLI details, and output shape may still change.

**Turn messy web pages into compact, proof-carrying context for AI agents.**

**Smaller packets. Fewer hops. Real provenance.**

![Needle-X Hero](docs/assets/readme-hero.png)

## Why It Wins

1. **Smaller output**
   Needle-X returns much less context than extraction-heavy tools.
2. **Source-backed**
   It carries proof, not just extracted text.
3. **Less cleanup**
   A downstream agent does less work before it can act.

## Live Comparison

| Metric | Needle-X | Tavily | Jina | Firecrawl |
| --- | ---: | ---: | ---: | ---: |
| Avg packet bytes | **4436** | 6975 | 30565 | 72166 |
| Claim-to-source steps | **1** | 2 | 2 | 2 |
| Post-processing burden | **0.25** | 1.92 | 1.86 | 2.50 |
| Proof usability | **1.0** | 0 | 0 | 0 |

Needle-X vs `Jina`:
- about **85.5% smaller** packets

This is the current sweet spot:
1. compact context
2. direct verification
3. low-friction agent consumption

![Needle-X Metrics](docs/assets/readme-metrics-2.png)

## Discovery Memory

Needle-X includes local `Discovery Memory` backed by SQLite.

The story is simple:
1. first run observes and compiles
2. later runs reuse local verified evidence
3. repeated use improves local retrieval without hosted infra

Current warm-state result:
1. **30/30** selected-url correctness
2. **30/30** `discovery_memory` provider selection

Guardrail:
1. warm-state local retrieval claim
2. not a cold-state open-web seedless claim

![Needle-X Discovery Memory](docs/assets/readme-memory.png)

## What It Does

1. `read`
2. `query`
3. `crawl`
4. `proof`
5. `replay`
6. `diff`
7. `memory stats/search/prune`

Default output is AI-first:
1. compact packet first
2. proof inline when useful
3. full diagnostics only on demand

## Tiny Demo

```bash
needlex read https://example.com --json
needlex query https://example.com --goal "pricing" --json
needlex proof proof_1 --json
```

## Install

Linux and macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/Josepavese/needlex/main/install/install.sh | bash
```

Windows:

```powershell
irm https://raw.githubusercontent.com/Josepavese/needlex/main/install/install.ps1 | iex
```

Installed command:
1. `needlex`

This installer downloads the right release binary. Full details:
1. [Install Guide](/home/jose/hpdev/Libraries/needlex/docs/install.md)

## What It Is Not

1. browser agent
2. search engine
3. generic scraper
4. LLM-first reader

## Read More

1. [Operator Guide](/home/jose/hpdev/Libraries/needlex/docs/operator-guide.md)
2. [Benchmark Report](/home/jose/hpdev/Libraries/needlex/docs/benchmark-report.md)
3. [Go-To-Market](/home/jose/hpdev/Libraries/needlex/docs/go-to-market.md)
4. [Spec](/home/jose/hpdev/Libraries/needlex/spec.md)
5. [Agent Answer Packet](/home/jose/hpdev/Libraries/needlex/docs/agent-answer-packet.md)
