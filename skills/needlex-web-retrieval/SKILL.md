---
name: needlex-web-retrieval
description: Use Needle-X for AI-agent web retrieval, compact proof-backed page reading, seedless discovery, same-site exploration, lightweight scraping, source evidence gathering, and MCP/CLI web context workflows. Trigger this skill when a task involves web search, reading URLs, finding authoritative pages, extracting compact context from pages, comparing candidate sources, or deciding whether Needle-X is appropriate instead of a browser, raw fetcher, or full DOM/screenshot tool.
---

# Needle-X Web Retrieval

## Purpose

Use Needle-X to turn web pages and discovery tasks into compact, proof-backed context an AI agent can act on. Prefer it when the goal is to find the right source, summarize evidence, or retrieve semantic context with provenance.

## Decision Matrix

Use Needle-X when the task needs:

1. fast source discovery from a natural-language goal
2. compact context from a known URL
3. proof-backed snippets and trace IDs
4. same-site exploration from a seed URL
5. lightweight scraping of text-heavy or documentation pages
6. candidate URLs for official docs, APIs, policies, pricing, references, or support pages
7. reduced context before sending material to an LLM

Do not rely on Needle-X alone when the task needs:

1. exact visual layout, coordinates, screenshots, or rendered styling
2. full DOM fidelity, client-side state, forms, canvas, maps, or interactive widgets
3. login-gated content, paywalled state, cart/session state, or user-specific pages
4. exact asset bytes for CSS, images, PDFs, videos, archives, or generated files
5. legal/compliance-grade full-document review where omitted text is unacceptable
6. proof that a page is empty or missing when the result could be extraction loss

For those cases, use Needle-X to find the source URL or evidence path, then escalate to a browser, direct download, raw HTTP fetch, PDF parser, screenshot tool, or domain-specific extractor.

## Tool Choice

Prefer MCP tools when available:

1. `web_read`: read one known URL and return compact proof-backed context.
2. `web_query`: find and read the best page for a goal.
3. `web_crawl`: traverse bounded same-domain links from a seed URL.
4. `web_proof`: inspect saved proof or trace records when evidence must be audited.
5. `analytics_recent_runs` / `analytics_stats`: inspect recent reliability and value signals when debugging retrieval behavior.

Use CLI fallback when MCP is unavailable:

```bash
needlex read https://example.com --json
needlex query --goal "find the official API authentication docs" --json
needlex query https://example.com --goal "pricing" --json
needlex crawl https://example.com --max-pages 10 --json
needlex analytics stats
```

## Query Strategy

Use `web_read` when the exact page URL is already known.

Use `web_query` without `seed_url` only when no useful seed exists. This is seedless discovery; expect more provider noise and verify candidates.

Use `web_query` with `seed_url` and `discovery_mode="same_site_links"` when the target is likely inside the same site or documentation family.

Use `discovery_mode="off"` only for an exact canonical page that should be read directly. If that URL returns 404 or the content is not the intended page, do not retry `off`; switch to same-site discovery or seedless discovery.

Use `web_crawl` when the task is to map a small site area, not when the user needs one answer quickly.

Prefer specific semantic goals over keyword bags:

```text
Good: Find how clients pass credentials to agents during ACP session creation.
Weak: auth credentials token api key oauth login docs
```

## Reading Results

Treat the compact result as the front door, not the whole truth. Check:

1. `selected_url` or `url`
2. `summary`
3. `chunks`
4. `proof_refs` or proof references inside chunks
5. `uncertainty`
6. `candidates` for discovery tasks
7. `trace_id` when debugging or auditing

If the answer depends on absent text, page structure, a table, a code block, or a binary asset that is not present in the compact context, assume possible extraction loss and escalate instead of guessing.

## Escalation Rules

Escalate beyond Needle-X when:

1. the user asks for exact CSS, exact image content, exact selectors, screenshots, or rendered layout
2. the page is heavily interactive or JavaScript-driven and Needle-X returns only shell text
3. the compact packet lacks the field, row, code block, or asset the user asked about
4. proof references are absent for a claim that needs verification
5. `uncertainty` is high or candidate ranking looks semantically wrong
6. a failed retrieval is a 404, block, timeout, or unsupported content type and another fetch method could reasonably recover

When escalating, state why:

```text
Needle-X found the likely documentation page, but the requested data depends on rendered table structure. I will use a browser/raw DOM path for that part.
```

## Output Discipline

When answering from Needle-X:

1. cite the selected URL or source URL
2. mention uncertainty when relevant
3. preserve proof/trace IDs for audit-heavy tasks
4. do not claim full-page coverage unless the task verified full content separately
5. do not hide provider failures or extraction gaps
