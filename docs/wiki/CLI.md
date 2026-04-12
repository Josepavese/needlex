# CLI

The public command is:

```bash
needlex
```

## Core Commands

1. `needlex read <url> --json`
2. `needlex query [seed-url] --goal "<goal>" --json`
3. `needlex crawl <seed-url> --json`
4. `needlex proof <trace-id|proof-id|chunk-id> --json`
5. `needlex replay <trace-id> --json`
6. `needlex diff <trace-a> <trace-b> --json`
7. `needlex memory stats|search|prune|export|import|rebuild-index`

## Minimal Examples

```bash
needlex read https://example.com --json
needlex query https://example.com --goal "pricing" --json
needlex proof proof_1 --json
```

## Output Rule

Default JSON is compact and AI-first:
1. less noise
2. proof-aware
3. diagnostics only when needed

## Next

1. [MCP And Tool Calling](./MCP-And-Tool-Calling.md)
2. [Discovery Memory](./Discovery-Memory.md)

## Full Reference

1. [Operator Guide](../operator-guide.md)
2. [Agent Answer Packet](../agent-answer-packet.md)
3. [Fetch Profiles](../fetch-profiles.md)
