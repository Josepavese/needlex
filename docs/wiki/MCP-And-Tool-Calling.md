# MCP And Tool Calling

Needle-X exposes:
1. `MCP` as the canonical interoperable runtime surface
2. provider JSON catalogs for direct tool/function calling

## MCP

Start the stdio server with:

```bash
needlex mcp
```

Transport compatibility:
1. accepts standard `Content-Length` framing
2. accepts raw newline-delimited JSON-RPC
3. replies in the same framing style used by the client

State and logging:
1. MCP uses `NEEDLEX_HOME` when set
2. installed MCP uses `NEEDLEX_CONFIG` from the wrapper to read the PAL SSOT config
3. otherwise it falls back to a stable absolute PAL-aware state root
4. diagnostics go to the shared PAL runtime log
5. inspect diagnostics with `needlex logs path` and `needlex logs tail`

Semantic prerequisite:
1. Semantic retrieval requires dense embeddings
2. default installed backend is local Ollama `embeddinggemma:latest`
3. change it with `needlex config set`, not per-tool MCP arguments

Current MCP tool set:
1. `web_read`
2. `web_query`
3. `web_crawl`
4. `web_proof`
5. `web_replay`
6. `web_diff`
7. `web_prune`
8. `web_fetch`
9. `memory`
10. `analytics`

Authenticated navigation:
1. `web_read`, `web_query`, `web_crawl` and `web_fetch` accept `session_path`, pointing to a navigation-session JSON file.
2. They also accept inline `headers` (an object scoped to the target origin) and `cookies` (an array scoped to the target host) for short-lived caller credentials. Inline cookies are host-only unless `host_only: false` is set.
3. `browser_profile` accepts `{"path": "...", "profile": "Default"}` to import cookies from a local Chromium profile; `path` is required. Imported host-only cookies stay host-only. Imported cookies are the baseline identity and explicit `cookies` override matching profile cookies.
4. `proxy` and `insecure_tls` are available for explicit transport overrides; `session_path` also carries custom CA and client-certificate settings. Browser proxy URLs with embedded credentials are rejected because Chromium does not apply them.
5. Credentials are never echoed in compact packets, receipts or diagnostics. Cross-origin redirects and assets receive only rules scoped to their own origin; cookies remain domain- or host-scoped, and the browser renderer applies the same header rules through CDP interception. A rendered page seeds only cookies scoped to the requested URL, not cookies for unrelated subresources.
6. A session file may carry `browser_profile` (a Chromium user-data or profile directory) to import cookies without copying their values into MCP arguments. Cookie values are resolved server-side and never returned.
7. The MCP server does not read a session from `NEEDLEX_SESSION` if it is running in a different environment; pass `session_path` explicitly or set the variable in the server environment.

Example arguments:

```json
{
  "url": "https://api.github.com/user",
  "headers": {"Authorization": "Bearer token-from-the-agent-host"},
  "cookies": [{"name": "sid", "value": "cookie-value"}]
}
```

`web_prune` is an operator maintenance tool. For disposable PAL embedding cache
cleanup, call it with `embedding_cache=true`; use `dry_run=true` before deletion.

`memory` and `analytics` are non-core advanced dispatchers.
Use their `action` parameter instead of exposing many separate MCP tools.

Memory actions:
1. `stats`
2. `search`
3. `prune`
4. `export`
5. `import`
6. `rebuild_index`

Use `force_embeddings=true` with `action="rebuild_index"` only for explicit
semantic memory maintenance after embedding model or vector-space changes.

Analytics actions:
1. `stats`
2. `recent_runs`
3. `value_report`
4. `hosts`
5. `providers`
6. `failures`
7. `daily`
8. `export`

Canonical query discovery values:
1. `same_site_links`
2. `web_search` (experimental explicit opt-in)
3. `off`

Agent note:
1. only the canonical values above are accepted
2. do not generate alias spellings such as `same-site` or `web-search`

Retrieval effort:
1. `retrieval_effort` is optional on `web_read` and `web_query`
2. valid values are `minimal`, `light`, `balanced`, `standard`, and `exhaustive`
3. default is `standard`
4. agents should omit it unless explicit semantic extraction or internal retrieval escalation control is needed
5. `retrieval_effort` is not a result count, page count, crawl depth, candidate limit, token budget, or timeout
6. `lane_max` is not part of the public agent-facing schema

Compact-first output rule:
1. `content.text` exposes the compact packet first
2. `structuredContent` is the same compact packet by default, plus only `trace_path`, `proof_path`, and `fingerprint_path`
3. full diagnostics are explicit opt-in: `include_diagnostics: true` on `web_read` or `web_query` adds a nested `diagnostics` object with `document`, `document_extraction` when the source is a PDF, `web_ir`, `result_pack`, `agent_context` (and `plan` for `web_query`); the default response never duplicates those payloads
4. `web_crawl` returns one compact crawl shape (`kind`, `summary`, `documents`, `stored_runs`) without a duplicate `compact` copy
5. agents should default to the compact packet before opening diagnostics
6. the `analytics` dispatcher follows the same compact-first rule: headline numbers first, richer rollups in structured payloads

Evidence coverage rule:
1. the compact packet adds `evidence` only when extraction is limited: `{"coverage":"title_only"|"thin"|"none","reason":"..."}`
2. `title_only` means the delivered chunks carry only page title or heading text; `thin` means the delivered body is shorter than a sentence or two; `none` means no usable body text
3. limited coverage also adds an uncertainty reason (`title_only_extraction`, `thin_extraction`, or `no_extraction`) and raises `uncertainty.level`
4. `analytics.proof_backed` means proof references exist for the delivered chunks; it is provenance, not answer adequacy
5. treat a present `evidence` field as a signal to read a broader source or request diagnostics, not as a normal packet annotation

PDF document rule:
1. `web_read` accepts public PDFs with embedded text; there is no separate document tool and no new MCP tool name
2. the compact packet carries `document_extraction` (`schema: "needlex.document.read.v1"`) with `raw_sha256`, `source_bytes`, `pages_total`, `pages_processed`, `pages_with_text`, `truncated`, `coverage`, `limits`, and `pages_failed[]` when applicable
3. compact chunks carry `page_start`/`page_end`; proof source spans carry `page` and use `/pdf/page[N]/block[M]` selectors; the top-level `url` is the final URL
4. `raw_sha256` is the full SHA-256 of the received PDF bytes; `limits` states the exact byte, page, output-character and timeout budgets that applied
5. extraction is static and page-aware: no JavaScript, no browser launch for PDFs, no system `pdftotext`, and no OCR; image-only PDFs return `document_no_text`
6. `render="required"` is strict and fails with `render_not_applicable` for PDFs; omit `render` or use `render="off"` for PDFs
7. PDFs use the same `NEEDLEX_NETWORK_POLICY` and privacy rules as HTML; the policy is never relaxed for document reads
8. distinct document failure classes are `document_encrypted`, `document_malformed`, `document_no_text`, `document_unsupported`, `document_limit_exceeded`, and `document_timeout`; do not retry a deterministic failure as if it were a transient network error
9. a truncated or partial extraction is reported through `truncated`/`coverage`/`pages_processed`; never infer completeness from a present text chunk alone
10. inspect full `document` and `document_extraction` only with `include_diagnostics: true`; the compact packet is the normal answer surface

Tool scope rule:
1. `web_extract` is intentionally not added yet
2. Needle-X should first get better through clearer schema, examples, and compact-first packets

Agent routing rule:
1. use `web_read` when the exact URL is known and page layout fidelity is not the goal
2. use the host agent's search tool to obtain candidate URLs, then call `web_read` for every URL the agent chooses to analyze
3. do not impose a candidate limit; breadth and concurrency belong to the host agent
4. use `web_query` with a verified `seed_url` for same-site routing
5. use `memory` with `action="search"` only for explicit local recall inspection
6. use `discovery_mode=off` only after the exact canonical page has already been verified
7. omit `seed_url` only for explicit experimental testing with `discovery_mode="web_search"`; this mode is never an automatic fallback

## Raw downloads

Call `web_fetch` when the user needs the original HTTP body, including a
binary file. It works independently of semantic retrieval.

```json
{"url":"https://example.com","assets":true,"asset_origins":["https://cdn.example.com"]}
```

The tool saves files on the MCP host and returns their absolute paths under
`structuredContent.download` with HTTP metadata and a `sha256` digest. The
download object is a versioned receipt identified by
`"schema": "needlex.fetch.receipt.v1"` (see
[needlex-fetch-receipt.v1.json](../../schemas/needlex-fetch-receipt.v1.json)).
Use a client with access to that host filesystem to open the files and verify
the digest. Successful downloads stay in a fresh directory under
`NEEDLEX_HOME/downloads`. Set `max_bytes` to override the total byte budget.
The [CLI raw download contract](./CLI.md#raw-downloads) describes asset
discovery, permissions, the complete receipt, stable error codes and origin
restrictions.

## Provider Catalogs

Export tool definitions directly from the binary:

```bash
needlex tool-catalog --provider openai
needlex tool-catalog --provider openai --strict
needlex tool-catalog --provider anthropic
```

## Render Mode And Content Provenance

`web_read`, `web_query`, and `web_crawl` accept a `render` argument with `auto`, `off`, or `required`.

1. `auto` reads declared agent-readable sources first, then renders when the static surface is thin, client-rendered, or does not cover the objective, capturing rendered DOM plus textual application data from fetch/XHR, SSE, and received WebSocket frames
2. `off` forbids browser rendering
3. `required` forces a browser read even for non-HTML content and fails if the rendered DOM cannot be obtained

Render waiting is adaptive: it may wait up to `render.timeout_ms` and never past the remaining operation deadline, and it stops as soon as the page and its streams go idle.

Read `signals` in the compact packet to know what was delivered:

1. `content_source` is `dom` when content comes from the rendered DOM, and `dom+network` when captured application data also contributed
2. `network_truncated` means more application data exists upstream than was captured
3. `render_degraded` means the browser path failed and application data was not observed

Before escalating to an external browser, retry the same URL once with `render: "required"`.

## Mapping

1. `web_read` -> `needlex read`
2. `web_query` -> `needlex query`
3. `web_crawl` -> `needlex crawl`
4. `web_proof` -> `needlex proof`
5. `web_replay` -> `needlex replay`
6. `web_diff` -> `needlex diff`
7. `web_prune` -> `needlex prune`
8. `memory` with `action="stats"` -> `needlex memory stats`
9. `memory` with `action="search"` -> `needlex memory search`
10. `memory` with `action="prune"` -> `needlex memory prune`
11. `memory` with `action="export"` -> `needlex memory export`
12. `memory` with `action="import"` -> `needlex memory import`
13. `memory` with `action="rebuild_index"` -> `needlex memory rebuild-index`
14. `analytics` with `action="stats"` -> `needlex analytics stats`
15. `analytics` with `action="recent_runs"` -> `needlex analytics recent`
16. `analytics` with `action="value_report"` -> `needlex analytics value-report`
17. `analytics` with `action="hosts"` -> `needlex analytics hosts`
18. `analytics` with `action="providers"` -> `needlex analytics providers`
19. `analytics` with `action="failures"` -> `needlex analytics failures`
20. `analytics` with `action="daily"` -> `needlex analytics daily`
21. `analytics` with `action="export"` -> `needlex analytics export`
22. `web_fetch` -> `needlex fetch`

## Next

1. [CLI](./CLI.md)
2. [Benchmarks](./Benchmarks.md)

## Full Reference

1. [Tool Calling](../tool-calling.md)
