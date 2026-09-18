---
name: needlex-web-retrieval
version: v0.1.37
description: Use Needle-X to compile known web URLs into compact proof-backed context after an AI agent obtains candidate URLs with its own search tool. Trigger this skill when a task involves reading one or more URLs, extracting token-efficient evidence, comparing candidate sources, same-site exploration from a verified seed, or deciding whether Needle-X is appropriate instead of a browser, raw fetcher, or full DOM/screenshot tool.
---

# Needle-X Web Retrieval

## Purpose

Use Needle-X to turn web pages selected by the host agent into compact, proof-backed context the agent can act on. The host agent owns search, candidate selection, research breadth, and source comparison. Needle-X owns token-efficient reading and evidence compilation for each URL the agent chooses to analyze.

Needle-X must not impose or suggest a numeric candidate limit. If the agent chooses to analyze one URL or one hundred URLs, use Needle-X for every selected URL as allowed by the host's execution and concurrency controls.

Use Needle-X first when the value is compact semantic evidence, proof-backed snippets, and low-token context for agent reasoning. Needle-X also renders client-rendered pages and captures textual application data from fetch/XHR, SSE, and WebSocket payloads, so dynamic pages are in scope before any external escalation. Escalate when the task depends on exact visual bytes, full-document completeness, client-side interaction, or authenticated state.

## Policy Compatibility

Needle-X does not override higher-priority browsing, citation, copyright, official-source, legal, medical, financial, or freshness requirements. If policy or task risk requires current verification, official sources, or explicit citations, Needle-X is sufficient only when its output provides adequate source evidence for that requirement.

## Decision Matrix

Use Needle-X when the task needs:

1. compact context from a known URL
2. proof-backed snippets and trace IDs
3. same-site exploration from a verified seed URL
4. lightweight extraction from text-heavy or documentation pages
5. reduced context before sending material to an LLM
6. consistent evidence packets across all URLs selected by the host agent
7. content that a page builds after load, up to the capture window and budget the runtime reports

Do not rely on Needle-X alone when the task needs:

1. exact visual layout, coordinates, screenshots, or rendered styling
2. interactive widget state: forms, canvas, maps, drag interactions, or anything requiring clicks or scrolling
3. login-gated content, paywalled state, cart/session state, or user-specific pages
4. exact asset bytes for CSS, images, PDFs, videos, archives, or generated files
5. legal/compliance-grade full-document review where omitted text is unacceptable
6. proof that a page is empty or missing when the result could be extraction loss
7. application data the page only requests after the capture window, or an unbounded stream still open when the render stopped

For those cases, keep the URL found by the host agent and escalate from Needle-X to a browser, direct download, raw HTTP fetch, PDF parser, screenshot tool, or domain-specific extractor. For rule 7, check `signals.content_source`, `network_truncated`, and `render_degraded` first, and retry once with `render: "required"` before leaving Needle-X.

## Tool Choice

Prefer MCP tools when available:

1. `web_read`: primary tool; read one known URL and return compact proof-backed context.
2. `web_query`: route within a known site or documentation family only when a verified `seed_url` is supplied.
3. `web_crawl`: traverse bounded same-domain links from a seed URL.
4. `web_proof`: inspect saved proof or trace records when evidence must be audited.
5. `analytics`: advanced diagnostics only. Use `action="recent_runs"`, `action="stats"`, or `action="value_report"` when debugging retrieval behavior or showing value.
6. `memory`: advanced local semantic memory inspection only. Use `action="search"` or `action="stats"` when checking whether Needle-X already knows a source.

Use CLI fallback when MCP is unavailable:

```bash
needlex read https://example.com --json
needlex query https://example.com --goal "pricing" --json
needlex crawl https://example.com --max-pages 10 --json
needlex analytics stats
```

## Examples

1. Multi-source research: use the host agent's search tool to obtain candidate URLs, then call `web_read` with the same objective for every URL the agent chooses to analyze. The agent decides the count and comparison strategy.
2. Known docs URL: use `web_read` on the official page, answer from cited chunks, and keep proof/trace IDs when the answer is implementation-sensitive.
3. Latest release/changelog: search with the host agent's current-information tool, then use `web_read` on every selected release or changelog URL before answering.
4. Same-site docs routing: use `web_query` with `seed_url` and `discovery_mode="same_site_links"` to find a specific concept inside a known documentation family.
5. Layout/table/assets needed: use Needle-X to compact the page first, then switch to browser/raw/PDF/image tooling for exact rendered or binary content.

## Query Strategy

Use `web_read` when the exact page URL is already known.

For research that starts from a natural-language question, use the host agent's own search tool first. Pass every URL the agent elects to analyze to a separate `web_read` call with the research objective. Run those calls concurrently when the host supports concurrency and the agent considers that appropriate.

Use `web_query` with `seed_url` and `discovery_mode="same_site_links"` when the target is likely inside the same site or documentation family.

Use `discovery_mode="off"` only for an exact canonical page that should be read directly. If that URL returns 404 or is not the intended page, use same-site routing from a valid seed or return to the host agent's search tool for a verified replacement URL.

Use `web_crawl` when the task is to map a small site area, not when the user needs one answer quickly.

Prefer specific semantic goals over search-term bags:

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
6. `signals.content_source` when the page loads content dynamically
7. `trace_id` when debugging or auditing

`signals.content_source` reports where the compiled content came from:

1. absent: the page was read without browser rendering
2. `dom`: the page was rendered and the content comes from the rendered DOM
3. `dom+network`: the page was rendered and application data captured from fetch, XHR, SSE, or WebSocket payloads also contributed content
4. `signals.network_truncated`: captured application data was cut by budget or a stream was still open, so more data exists upstream
5. `signals.render_degraded`: the browser path failed and only a DOM dump was available, so application data was not captured

If the answer depends on absent text, page structure, a table, a code block, or a binary asset that is not present in the compact context, assume possible extraction loss and escalate instead of guessing.

Anti-overclaim checklist:

1. selected URL is authoritative enough for the task
2. chunks or proof coverage contain the claim being made
3. uncertainty does not undermine the answer
4. missing tables, code, assets, or layout are not silently inferred
5. fallback or escalation is used when compact context is insufficient
6. `content_source` is checked before claiming a dynamic page has no more data

## Escalation Rules

Escalate beyond Needle-X when:

1. the user asks for exact CSS, exact image content, exact selectors, screenshots, or rendered layout
2. the page is heavily interactive or JavaScript-driven and Needle-X returns only shell text
3. the compact packet lacks the field, row, code block, or asset the user asked about
4. proof references are absent for a claim that needs verification
5. `uncertainty` is high or the extracted evidence does not support the task
6. a failed retrieval is a 404, block, timeout, or unsupported content type and another fetch method could reasonably recover

Before leaving Needle-X for rule 2 or 3, retry the same URL once with `render: "required"`. That forces browser rendering with application-data capture and fails loudly when the rendered DOM cannot be obtained. Only escalate to an external browser path if the forced render still misses the data or reports `network_truncated` for the field you need.

When escalating, state why:

```text
Needle-X compacted the selected documentation page, but the requested data depends on rendered table structure. I will use a browser/raw DOM path for that part.
```

## Output Discipline

When answering from Needle-X:

1. cite the selected URL or source URL
2. mention uncertainty when relevant
3. preserve proof/trace IDs for audit-heavy tasks
4. do not claim full-page coverage unless the task verified full content separately
5. do not hide provider failures or extraction gaps

## Keeping This Skill Current

This skill ships with the Needle-X release and carries a `version:` marker in its frontmatter naming the release it documents.

An installed copy is a snapshot: host agents read it from disk and never refresh it on their own, so a copy installed at `vX.Y.Z` stays at `vX.Y.Z` even after the binary is upgraded. `needlex doctor` reports the installed copy against the running build and prints a refresh command when they differ.

If `needlex doctor` reports installed agent skill drift, refresh the copy before trusting this guidance: an older copy can describe tool behavior that no longer matches the running binary. The installer refreshes an existing copy automatically and keeps a backup of the previous one.

The installer prints the exact command for your host. For a Codex-style install layout it is:

```bash
python3 ~/.codex/skills/.system/skill-installer/scripts/install-skill-from-github.py --repo Josepavese/needlex --path skills/needlex-web-retrieval
```

Reinstalling that way requires removing or renaming the existing copy first, since the host installer refuses to overwrite; keep the old directory as a backup.
