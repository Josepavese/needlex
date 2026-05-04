# Tool Calling

Needle-X should be treated as a tool-calling runtime with two current catalog surfaces:
1. `MCP` as the primary interoperable transport
2. provider-specific JSON Schema catalogs for direct tool/function calling

## What Standard Exists

There is no single universal tool-calling standard across every AI platform.

What exists in practice:
1. `JSON Schema` tool definitions for provider APIs
2. `MCP` for interoperable tool discovery and invocation

That means the correct Needle-X strategy is:
1. keep MCP as the canonical runtime tool surface
2. derive provider-facing tool catalogs from the same tool set

## Canonical Tool Surface

Needle-X currently exposes these MCP tools:
1. `web_read`
2. `web_query`
3. `web_crawl`
4. `web_proof`
5. `web_replay`
6. `web_diff`
7. `web_prune`
8. `memory`
9. `analytics`

The core retrieval surface is the `web_*` group.
`memory` and `analytics` are advanced dispatch tools with an explicit `action` parameter.
They keep non-core observability and maintenance operations out of the primary retrieval tool list.

Runtime reference:
- [mcp_tools.go](../internal/transport/mcp_tools.go)

Protocol reference:
- [mcp.go](../internal/transport/mcp.go)

## Provider Catalogs

Machine-readable catalogs live in:
1. [needlex-tools.openai.json](../schemas/needlex-tools.openai.json)
2. [needlex-tools.anthropic.json](../schemas/needlex-tools.anthropic.json)

Installed binary export:

```bash
needlex tool-catalog --provider openai
needlex tool-catalog --provider openai --strict
needlex tool-catalog --provider anthropic
```

These files are not a second source of truth.
They are generated catalog artifacts that must stay aligned with the MCP tool set.

That alignment is enforced by tests.

## Integration Rules

If you wire Needle-X into an AI tool-calling stack:
1. prefer `web_read` for single-page compilation
2. prefer `web_query` for goal-oriented retrieval
3. use `web_proof` to convert claims into source-backed evidence
4. keep `web_replay` and `web_diff` for audit/debug flows, not default agent loops
5. keep `web_prune` as an operator tool, not a model-default tool
6. use `memory` only for advanced local semantic memory inspection or maintenance
7. use `analytics` only for diagnostics, value reporting, or maintainer rollups

`memory` and `analytics` are dispatch tools with an `action` parameter.
They intentionally replace many narrower MCP tools to reduce provider-side tool-list context.

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



### Query Discovery Mode

For `web_query`, use the canonical `discovery_mode` values:
1. `same_site_links` to expand from the seed site
2. `web_search` to bootstrap with search
3. `off` for strict seeded mode

Strict mode note:
1. Needle-X accepts only the canonical values above
2. non-canonical spellings such as `same-site` or `web-search` are rejected

### Retrieval Effort

`retrieval_effort` is optional on `web_read` and `web_query`.
Agents should omit it unless they need explicit control over semantic extraction and internal retrieval escalation.

Valid values:
1. `minimal`: fastest, least internal escalation
2. `light`: low-latency retrieval effort
3. `balanced`: moderate retrieval effort
4. `standard`: default production behavior
5. `exhaustive`: highest retrieval effort

`retrieval_effort` is not a result count, page count, crawl depth, candidate limit, token budget, or timeout.
Use `web_crawl.max_pages` and `web_crawl.max_depth` when page traversal limits matter.
`lane_max` is not part of the public agent-facing schema.

## Design Constraints

The provider-facing contracts should stay:
1. small
2. strict
3. JSON-Schema-based
4. version-conscious
5. proof-aware

Needle-X should not collapse core retrieval into one oversized “do everything” tool.
Keep the retrieval tools narrow and composable.
Use dispatchers only for non-core maintenance and observability surfaces such as `memory` and `analytics`.

## Recommended Mapping

### OpenAI

Use the OpenAI-compatible catalog when the host expects:
1. `type=function`
2. `function.name`
3. `function.description`
4. `function.parameters` with JSON Schema
5. optionally `strict: true` for tighter argument conformance

### Anthropic

Use the Anthropic-compatible catalog when the host expects:
1. `name`
2. `description`
3. `input_schema`

### MCP

Use MCP when the client can speak:
1. `initialize`
2. `tools/list`
3. `tools/call`

Transport note:
1. `needlex mcp` accepts both standard `Content-Length` framed stdio and raw newline-delimited JSON-RPC
2. responses follow the same framing style used by the client
3. this is intentional for compatibility with Claude Desktop-style framed clients and simpler raw-JSON clients

This remains the best standard integration surface for Needle-X itself.

## Compact-first MCP Rule

For MCP agent-facing calls:
1. `content.text` should expose the compact packet first
2. `structuredContent` should retain the richer diagnostic envelope
3. agents should read the compact packet first and open diagnostics only when needed

Tool expansion rule:
1. do not add narrower tools like `web_extract` until repeated agent misuse shows that `web_read` and `web_query` cannot be made clear enough
2. improve schema, examples, and compact output first

## Practical Recommendation

If you are integrating Needle-X into an agent stack:
1. use MCP first when supported
2. otherwise use the provider JSON catalog that matches your host
3. do not hand-write a third tool definition set unless you must

That is the lowest-drift setup.

## Ready Examples

### OpenAI Responses API

Use the exported tool catalog directly:

```bash
needlex tool-catalog --provider openai --strict > /tmp/needlex-openai-tools.json
```

Python example:

```python
import json
import subprocess
from openai import OpenAI

client = OpenAI()

tools = json.loads(
    subprocess.check_output(
        ["needlex", "tool-catalog", "--provider", "openai", "--strict"],
        text=True,
    )
)["tools"]

def add(cmd, flag, value):
    if value not in (None, "", False):
        cmd.extend([flag, str(value)])

def run_needlex_tool(name, args):
    if name == "web_read":
        cmd = ["needlex", "read", args["url"], "--json"]
        add(cmd, "--objective", args.get("objective"))
        add(cmd, "--profile", args.get("profile"))
        add(cmd, "--user-agent", args.get("user_agent"))
    elif name == "web_query":
        cmd = ["needlex", "query"]
        add(cmd, "", args.get("seed_url"))
        add(cmd, "--goal", args.get("goal"))
        add(cmd, "--discovery", args.get("discovery_mode"))
        add(cmd, "--profile", args.get("profile"))
        add(cmd, "--user-agent", args.get("user_agent"))
        cmd.append("--json")
    elif name == "web_crawl":
        cmd = ["needlex", "crawl", args["seed_url"], "--json"]
        add(cmd, "--max-pages", args.get("max_pages"))
        add(cmd, "--max-depth", args.get("max_depth"))
        add(cmd, "--profile", args.get("profile"))
        if args.get("same_domain"):
            cmd.append("--same-domain")
    elif name == "web_replay":
        cmd = ["needlex", "replay", args["trace_id"], "--json"]
    elif name == "web_diff":
        cmd = ["needlex", "diff", args["trace_a"], args["trace_b"], "--json"]
    elif name == "web_proof":
        proof_key = args.get("proof_id") or args.get("chunk_id") or args.get("trace_id")
        cmd = ["needlex", "proof", proof_key, "--json"]
    elif name == "web_prune":
        cmd = ["needlex", "prune", "--json"]
        if args.get("all"):
            cmd.append("--all")
        add(cmd, "--older-than-hours", args.get("older_than_hours"))
    elif name == "memory":
        action = {"rebuild_index": "rebuild-index"}.get(args["action"], args["action"])
        cmd = ["needlex", "memory", action, "--json"]
        add(cmd, "--config", args.get("config_path"))
        add(cmd, "--limit", args.get("limit"))
        add(cmd, "--domain-hints", args.get("domain_hints"))
        add(cmd, "--out", args.get("out_dir"))
        add(cmd, "--in", args.get("in_dir"))
        if action == "search":
            add(cmd, "", args.get("query"))
    elif name == "analytics":
        action = {"recent_runs": "recent", "value_report": "value-report"}.get(args["action"], args["action"])
        cmd = ["needlex", "analytics", action, "--json"]
        add(cmd, "--limit", args.get("limit"))
        add(cmd, "--out", args.get("out_dir"))
    else:
        raise ValueError(f"unknown Needle-X tool: {name}")
    return subprocess.check_output([part for part in cmd if part], text=True)

response = client.responses.create(
    model="gpt-5",
    input="Read https://example.com and give me compact context.",
    tools=tools,
)

for item in response.output:
    if getattr(item, "type", None) != "function_call":
        continue

    tool_name = item.name
    tool_args = json.loads(item.arguments)

    tool_result = run_needlex_tool(tool_name, tool_args)

    followup = client.responses.create(
        model="gpt-5",
        previous_response_id=response.id,
        input=[{
            "type": "function_call_output",
            "call_id": item.call_id,
            "output": tool_result,
        }],
    )

    print(followup.output_text)
```

Operational note:
1. in production, do not shell out by naive key/value flattening for every tool
2. route tool names to explicit handlers
3. validate arguments before execution
4. route `memory` and `analytics` through their `action` field instead of using `web_` name stripping

### Anthropic Tool Use

Use the Anthropic-compatible catalog:

```bash
needlex tool-catalog --provider anthropic > /tmp/needlex-anthropic-tools.json
```

Python example:

```python
import json
import subprocess
from anthropic import Anthropic

client = Anthropic()

tools = json.loads(
    subprocess.check_output(
        ["needlex", "tool-catalog", "--provider", "anthropic"],
        text=True,
    )
)["tools"]

def add(cmd, flag, value):
    if value not in (None, "", False):
        cmd.extend([flag, str(value)])

def run_needlex_tool(name, args):
    if name == "web_read":
        cmd = ["needlex", "read", args["url"], "--json"]
        add(cmd, "--objective", args.get("objective"))
        add(cmd, "--profile", args.get("profile"))
        add(cmd, "--user-agent", args.get("user_agent"))
    elif name == "web_query":
        cmd = ["needlex", "query"]
        add(cmd, "", args.get("seed_url"))
        add(cmd, "--goal", args.get("goal"))
        add(cmd, "--discovery", args.get("discovery_mode"))
        add(cmd, "--profile", args.get("profile"))
        add(cmd, "--user-agent", args.get("user_agent"))
        cmd.append("--json")
    elif name == "web_crawl":
        cmd = ["needlex", "crawl", args["seed_url"], "--json"]
        add(cmd, "--max-pages", args.get("max_pages"))
        add(cmd, "--max-depth", args.get("max_depth"))
        add(cmd, "--profile", args.get("profile"))
        if args.get("same_domain"):
            cmd.append("--same-domain")
    elif name == "web_replay":
        cmd = ["needlex", "replay", args["trace_id"], "--json"]
    elif name == "web_diff":
        cmd = ["needlex", "diff", args["trace_a"], args["trace_b"], "--json"]
    elif name == "web_proof":
        proof_key = args.get("proof_id") or args.get("chunk_id") or args.get("trace_id")
        cmd = ["needlex", "proof", proof_key, "--json"]
    elif name == "web_prune":
        cmd = ["needlex", "prune", "--json"]
        if args.get("all"):
            cmd.append("--all")
        add(cmd, "--older-than-hours", args.get("older_than_hours"))
    elif name == "memory":
        action = {"rebuild_index": "rebuild-index"}.get(args["action"], args["action"])
        cmd = ["needlex", "memory", action, "--json"]
        add(cmd, "--config", args.get("config_path"))
        add(cmd, "--limit", args.get("limit"))
        add(cmd, "--domain-hints", args.get("domain_hints"))
        add(cmd, "--out", args.get("out_dir"))
        add(cmd, "--in", args.get("in_dir"))
        if action == "search":
            add(cmd, "", args.get("query"))
    elif name == "analytics":
        action = {"recent_runs": "recent", "value_report": "value-report"}.get(args["action"], args["action"])
        cmd = ["needlex", "analytics", action, "--json"]
        add(cmd, "--limit", args.get("limit"))
        add(cmd, "--out", args.get("out_dir"))
    else:
        raise ValueError(f"unknown Needle-X tool: {name}")
    return subprocess.check_output([part for part in cmd if part], text=True)

response = client.messages.create(
    model="claude-sonnet-4-5",
    max_tokens=1200,
    messages=[
        {"role": "user", "content": "Query https://example.com for pricing and keep the answer source-backed."}
    ],
    tools=tools,
)

for block in response.content:
    if block.type != "tool_use":
        continue

    tool_name = block.name
    tool_input = block.input

    tool_result = run_needlex_tool(tool_name, tool_input)

    followup = client.messages.create(
        model="claude-sonnet-4-5",
        max_tokens=1200,
        tools=tools,
        messages=[
            {"role": "user", "content": "Query https://example.com for pricing and keep the answer source-backed."},
            {"role": "assistant", "content": response.content},
            {
                "role": "user",
                "content": [{
                    "type": "tool_result",
                    "tool_use_id": block.id,
                    "content": tool_result,
                }],
            },
        ],
    )

    print(followup.content)
```

Operational note:
1. keep the original assistant `tool_use` block in the follow-up turn
2. send the `tool_result` as a user content block
3. keep tool execution deterministic and side-effect-light
4. route `memory` and `analytics` through their `action` field instead of using `web_` name stripping

### Minimal MCP Client

If your host speaks MCP, use `needlex mcp` directly.

Minimal request sequence:

1. `initialize`
2. `tools/list`
3. `tools/call`

Example raw JSON payloads:

```json
{"jsonrpc":"2.0","id":1,"method":"initialize"}
```

Raw stdio example:

```bash
printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{},"protocolVersion":"2024-11-05","clientInfo":{"name":"test","version":"1.0"}}}\n{"jsonrpc":"2.0","id":2,"method":"tools/list"}\n' | needlex mcp
```

Framed stdio example:

```text
Content-Length: 58

{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
```

```json
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
```

```json
{
  "jsonrpc":"2.0",
  "id":3,
  "method":"tools/call",
  "params":{
    "name":"web_read",
    "arguments":{
      "url":"https://example.com"
    }
  }
}
```

```json
{
  "jsonrpc":"2.0",
  "id":4,
  "method":"tools/call",
  "params":{
    "name":"analytics",
    "arguments":{
      "action":"value_report"
    }
  }
}
```

```json
{
  "jsonrpc":"2.0",
  "id":5,
  "method":"tools/call",
  "params":{
    "name":"memory",
    "arguments":{
      "action":"search",
      "query":"playwright installation",
      "limit":5
    }
  }
}
```

Operational note:
1. if `NEEDLEX_HOME` is unset, MCP falls back to a stable PAL-aware absolute state root
2. diagnostics go to the shared PAL runtime log under `<state-root>/logs/needlex.jsonl`
3. do not mix human-readable wrapper output into stdout while the MCP server is running
4. inspect diagnostics with `needlex logs path` and `needlex logs tail`

### Mapping Rule

Needle-X CLI names and tool names map like this:
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
