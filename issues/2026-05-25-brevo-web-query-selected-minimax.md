# Brevo web_query selected unrelated MiniMax docs

Status: closed in v0.1.53; unrelated-entity regression fixed, seedless source precision remains a residual.
Last reviewed: 2026-11-15 (v0.1.53)
State: closed (fixed for the reported unrelated-family regression).

Date: 2026-05-25

## Context

A side analysis was performed to determine whether comprehensive Brevo command-line or agentic tools exist beyond the official Brevo CLI.

The query was specifically about Brevo, including campaign, contact, list, template, report, CLI, and MCP coverage.

## Needlex Call

Tool: `web_query`

Goal:

```text
Find whether there are existing comprehensive Brevo command line tools or agentic/MCP tools for managing Brevo campaigns, contacts, lists, templates, reports beyond the official @getbrevo/cli OAuth app CLI. Include official CLI, npm packages, GitHub projects, MCP servers if relevant.
```

Discovery mode: `web_search`

Retrieval effort: `exhaustive`

## Observed Result

Needlex selected a MiniMax documentation page:

```text
selected_url: https://platform.minimax.io/docs/api-reference/api-overview
title: API Overview - MiniMax API Docs
provider: discovery_memory_same_site
uncertainty.level: medium
uncertainty.reasons: ["moderate_top_chunk_confidence"]
```

The returned chunk text was from MiniMax API documentation and did not mention Brevo:

```text
MiniMax API Docs home page discord x linkedin github Research MiniMax M2.7 MiniMax M2-her MiniMax M2.1 MiniMax Speech 2.8 MiniMax Hailuo 2.3 MiniMax Music 2.6 Product Agent Video Hailuo Audio Talkie API Developer Docs Token Plan Pricing Console Login Developer Program Recommended Model Introduction Text Generation M2.7 for AI Coding Tools Text to Speech Video Generation
```

## Candidate URLs

The candidate URLs returned in the result were:

```text
https://platform.minimax.io/docs/api-reference/api-overview
https://platform.minimax.io/docs/api-reference/text-chat-openai
https://platform.minimax.io/docs/api-reference/api-overview#official-mcp
https://platform.minimax.io/docs/guides/mcp-guide
https://platform.minimax.io/user-center/basic-information
https://platform.minimax.io/contact-us
https://platform.minimax.io/docs/api-reference/text-openai-api
https://platform.minimax.io/docs
```

No Brevo URL was present in the selected URL or candidate URL list.

## Domain Hints

The response included these domain hints:

```text
platform.minimax.io
www.cloudwego.io
docs.z.ai
microsoft.github.io
docs.langchain.com
ai-sdk.dev
```

No Brevo domain was present in the domain hints.

## Trace Details

Trace ID:

```text
trace_08950570cf0c249b
```

Fetched at:

```text
2026-05-25T21:22:58.425454224Z
```

Proof reference:

```text
proof_850e42d504b6df4d
```

## User-Visible Effect

The Needlex result did not provide information about Brevo CLI tools, Brevo MCP tools, Brevo campaign management, Brevo contacts, Brevo lists, Brevo templates, or Brevo reports.

The analysis proceeded with standard web search after the Needlex result returned an unrelated selected URL and unrelated candidate set.

## Evidence Summary

For a Brevo-specific `web_query`, Needlex returned a selected URL, candidate URLs, domain hints, and chunk text centered on MiniMax documentation instead of Brevo-related sources.

## Reproduction (2026-11-15, v0.1.52)

The same goal, run with `--discovery web_search --retrieval-effort light`,
selected `https://www.icloud.com.cn/find` instead of a Brevo source. The
response reported `uncertainty.reasons=["ambiguous_candidate_selection"]` and
provider observations from Bing and DuckDuckGo. This confirms that the issue is
still open and is not obsolete; the benchmark evidence must not be promoted to
a pass from an unrelated result.

## Resolution (2026-11-15, v0.1.53)

The selected MiniMax/iCloud family was caused by local memory candidates being
trusted without semantic grounding, plus an oversized multi-sentence goal being
sent verbatim to a public provider. Both paths were corrected:

1. semantic memory recall is filtered at `semantic.similarity_threshold` before
   it can become a memory shortcut or a seed; ungrounded local history no longer
   bypasses public bootstrap;
2. long seedless goals are sent to providers as a bounded contiguous excerpt
   instead of the full goal, so the provider does not read the leading
   "Find whether..." as the search entity.

A verification run of the same goal with `web_search` and light effort returned
`https://www.brevo.com/it/products/` with a Brevo-only candidate family. The
former iCloud/MiniMax result did not reproduce. One later live attempt hit a
provider `401` rate-limit response, which is a provider failure, not a
regression of this issue.

Residual: seedless discovery may still rank a Brevo marketing, login, or help
surface instead of a CLI-specific documentation page. Seedless discovery is
experimental and this residual is not a stable-contract or release claim.
