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

Discovery Memory is enabled by default and stored in the PAL state root. If an external embeddings service is unavailable, Needle-X falls back to a native local semantic vectorizer so memory still accumulates and remains searchable.

Current verified seeded result on `seeded-corpus-v2`:
1. **100/100** selected-url correctness
2. **100/100** proof usability
3. **100/100** runtime success

Guardrail:
1. seeded-runtime claim
2. not a blanket cold-state open-web seedless claim
3. Discovery Memory warm-state stress is tracked separately from the seeded runtime score

![Needle-X Discovery Memory](docs/assets/readme-memory.png)

## What It Does

1. `read`
2. `query`
3. `crawl`
4. `proof`
5. `replay`
6. `diff`
7. `memory stats/search/prune/export/import/rebuild-index`
8. `analytics stats/recent/value-report/hosts/providers/failures/daily/export`
9. `logs path/stats/tail`
10. `support bundle`
11. `doctor`

Default output is AI-first:
1. compact packet first
2. proof inline when useful
3. full diagnostics only on demand
4. browser-like fetch by default for real-world targets
5. local memory is populated automatically by successful `read`, `query`, and `crawl` runs
6. MCP server accepts both standard `Content-Length` framing and raw newline-delimited JSON

MCP advertises 9 tools: 7 core `web_*` tools plus `memory` and `analytics`.
The non-core `memory` and `analytics` surfaces use an explicit `action` parameter to avoid bloating agent tool lists with maintenance and observability operations.

## Tiny Demo

```bash
needlex read https://example.com --json
needlex query https://example.com --goal "pricing" --json
needlex proof proof_1 --json
needlex analytics stats
needlex analytics value-report
needlex logs stats
needlex support bundle --out /tmp/needlex-support
needlex doctor
```

`analytics stats` gives quick operational counters plus saved chars/tokens. `analytics value-report` is the fuller value view with estimated cost scenarios.
`logs stats` shows the PAL runtime log state used for clean CLI/MCP diagnostics.
`support bundle` exports a maintainer-friendly diagnostic directory with doctor, analytics, and runtime logs.

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
1. [Install](docs/wiki/Install.md)

## Agent Skill

Needle-X also ships an optional Codex skill that tells agents when to use Needle-X for web retrieval, when to escalate to browser/raw fetch tools, and how to avoid treating compact context as full DOM coverage.

Skill path:
1. [skills/needlex-web-retrieval](skills/needlex-web-retrieval)

Codex install helper:

```bash
python3 ~/.codex/skills/.system/skill-installer/scripts/install-skill-from-github.py --repo Josepavese/needlex --path skills/needlex-web-retrieval
```

After installing the skill, restart Codex so it can discover it.

## What It Is Not

1. browser agent
2. search engine
3. generic scraper
4. LLM-first reader

## Read More

1. [Wiki Home](docs/wiki/README.md)
2. [Install](docs/wiki/Install.md)
3. [CLI](docs/wiki/CLI.md)
4. [MCP And Tool Calling](docs/wiki/MCP-And-Tool-Calling.md)
5. [Discovery Memory](docs/wiki/Discovery-Memory.md)
6. [Benchmarks](docs/wiki/Benchmarks.md)
