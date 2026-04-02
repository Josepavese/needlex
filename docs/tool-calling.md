# Tool Calling

Needle-X should be treated as a tool-calling runtime with two compatibility layers:
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
They are compatibility artifacts that must stay aligned with the MCP tool set.

That alignment is enforced by tests.

## Integration Rules

If you wire Needle-X into an AI tool-calling stack:
1. prefer `web_read` for single-page compilation
2. prefer `web_query` for goal-oriented retrieval
3. use `web_proof` to convert claims into source-backed evidence
4. keep `web_replay` and `web_diff` for audit/debug flows, not default agent loops
5. keep `web_prune` as an operator tool, not a model-default tool

## Design Constraints

The provider-facing contracts should stay:
1. small
2. strict
3. JSON-Schema-based
4. backward-conscious
5. proof-aware

Needle-X should not expose oversized “do everything” tools.
It is better to keep tools narrow and composable.

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

This remains the best standard integration surface for Needle-X itself.

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

    tool_result = subprocess.check_output(
        ["needlex", tool_name.replace("web_", ""), "--json", *sum(([f"--{k}", str(v)] for k, v in tool_args.items()), [])],
        text=True,
    )

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

    tool_result = subprocess.check_output(
        ["needlex", tool_name.replace("web_", ""), "--json", *sum(([f"--{k}", str(v)] for k, v in tool_input.items()), [])],
        text=True,
    )

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

### Minimal MCP Client

If your host speaks MCP, use `needlex mcp` directly.

Minimal request sequence:

1. `initialize`
2. `tools/list`
3. `tools/call`

Example JSON-RPC payloads:

```json
{"jsonrpc":"2.0","id":1,"method":"initialize"}
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

MCP framing uses `Content-Length` headers because the transport is stdio JSON-RPC.

### Mapping Rule

Needle-X CLI names and tool names map like this:
1. `web_read` -> `needlex read`
2. `web_query` -> `needlex query`
3. `web_crawl` -> `needlex crawl`
4. `web_proof` -> `needlex proof`
5. `web_replay` -> `needlex replay`
6. `web_diff` -> `needlex diff`
7. `web_prune` -> `needlex prune`
