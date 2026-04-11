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
3. session logs go to `${NEEDLEX_MCP_LOG:-/tmp/needlex-mcp.log}`

Current MCP tool set:
1. `web_read`
2. `web_query`
3. `web_crawl`
4. `web_proof`
5. `web_replay`
6. `web_diff`
7. `web_prune`

Canonical query discovery literals:
1. `same_site_links`
2. `web_search`
3. `off`

Agent note:
1. aliases like `same-site` are accepted for compatibility
2. use the canonical literals above in generated tool calls

Compact-first output rule:
1. MCP `content.text` exposes the compact packet first
2. MCP `structuredContent` keeps the richer diagnostic payload
3. agents should default to the compact packet before opening diagnostics

Tool scope rule:
1. `web_extract` is intentionally not added yet
2. Needle-X should first get better through clearer schema, examples, aliases, and compact-first packets

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

## Next

1. [CLI](./CLI.md)
2. [Benchmarks](./Benchmarks.md)

## Full Reference

1. [Tool Calling](../tool-calling.md)
