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

## Agent Retrieval Workflow

Needle-X is the compact reading layer after search:
1. the host agent uses its own search tool to obtain candidate URLs
2. the agent decides which URLs to analyze and how many; Needle-X imposes no candidate limit
3. the agent calls `web_read` for every selected URL, passing the research objective
4. Needle-X returns compact, proof-carrying context for comparison and synthesis
5. the agent escalates to browser, raw fetch, PDF, or domain-specific tooling only when exact layout, bytes, or missing content matter

This keeps responsibilities explicit: the agent controls discovery strategy and breadth; Needle-X optimizes the evidence read from each chosen source.

Current verified seeded result on `seeded-corpus-v2`:
1. **100/100** selected-url correctness
2. **100/100** proof usability
3. **100/100** runtime success

## What It Does

1. `read`
2. seeded `query`
3. `crawl`
4. `proof`
5. `replay`
6. `diff`
7. `memory stats/search/prune/export/import/rebuild-index`
8. `analytics stats/recent/value-report/hosts/providers/failures/daily/export`
9. `logs path/stats/tail`
10. `support bundle`
11. `doctor`
12. `config path/show/init/set`

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
needlex read https://example.com --objective "pricing and cancellation policy" --json
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

The installer also prepares semantic prerequisites:
1. installs or verifies Ollama where the platform supports automated install
2. pulls the default embedding model `embeddinggemma:latest`
3. writes the PAL-home SSOT config
4. wires the `needlex` wrapper to that config so users do not export env vars per command
5. installs a PAL-local headless render browser
6. enables render in the PAL config for JavaScript-rendered sites

Change defaults with:

```bash
needlex config show
needlex config set semantic.provider_model nomic-embed-text:latest
```

## Agent Skill

Needle-X also ships an optional Codex skill that tells agents to obtain candidate URLs with their own search tool, compile every URL they choose with Needle-X, escalate when needed, and avoid treating compact context as full DOM coverage.

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
5. [Benchmarks](docs/wiki/Benchmarks.md)
