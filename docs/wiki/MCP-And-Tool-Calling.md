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
2. otherwise it falls back to a stable absolute PAL-aware state root
3. diagnostics go to the shared PAL runtime log
4. inspect diagnostics with `needlex logs path` and `needlex logs tail`

Current MCP tool set:
1. `web_read`
2. `web_query`
3. `web_crawl`
4. `web_proof`
5. `web_replay`
6. `web_diff`
7. `web_prune`
8. `memory`
9. `analytics`

`memory` and `analytics` are non-core advanced dispatchers.
Use their `action` parameter instead of exposing many separate MCP tools.

Memory actions:
1. `stats`
2. `search`
3. `prune`
4. `export`
5. `import`
6. `rebuild_index`

Analytics actions:
1. `stats`
2. `recent_runs`
3. `value_report`
4. `hosts`
5. `providers`
6. `failures`
7. `daily`
8. `export`

Canonical query discovery literals:
1. `same_site_links`
2. `web_search`
3. `off`

Agent note:
1. aliases like `same-site` are accepted for compatibility
2. use the canonical literals above in generated tool calls

Retrieval effort:
1. `retrieval_effort` is optional on `web_read` and `web_query`
2. valid values are `minimal`, `light`, `balanced`, `standard`, and `exhaustive`
3. default is `standard`
4. agents should omit it unless explicit semantic extraction or internal retrieval escalation control is needed
5. `retrieval_effort` is not a result count, page count, crawl depth, candidate limit, token budget, or timeout
6. `lane_max` is not part of the public agent-facing schema

Compact-first output rule:
1. MCP `content.text` exposes the compact packet first
2. MCP `structuredContent` keeps the richer diagnostic payload
3. agents should default to the compact packet before opening diagnostics
4. the `analytics` dispatcher follows the same rule: headline numbers first, richer rollups in structured payloads

Tool scope rule:
1. `web_extract` is intentionally not added yet
2. Needle-X should first get better through clearer schema, examples, aliases, and compact-first packets

Agent routing rule:
1. use `web_read` when the exact URL is known and page layout fidelity is not the goal
2. use `web_query` when the agent has a goal and may need same-site routing, local memory, or public bootstrap
3. omit `seed_url` for seedless discovery; Needle-X will consult Discovery Memory first and use public bootstrap only when needed
4. use `memory` with `action="search"` to inspect local recall explicitly before spending public-provider budget
5. use `discovery_mode=off` only after the exact canonical page has already been verified

## Provider Catalogs

Export tool definitions directly from the binary:

```bash
needlex tool-catalog --provider openai
needlex tool-catalog --provider openai --strict
needlex tool-catalog --provider anthropic
```

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

## Next

1. [CLI](./CLI.md)
2. [Benchmarks](./Benchmarks.md)

## Full Reference

1. [Tool Calling](../tool-calling.md)
