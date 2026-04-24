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
8. `needlex analytics stats|recent|value-report|hosts|providers|failures|daily|export`
9. `needlex logs path|stats|tail`
10. `needlex support bundle --out DIR`
11. `needlex doctor [--json]`

## Minimal Examples

```bash
needlex read https://example.com --json
needlex query https://example.com --goal "pricing" --json
needlex proof proof_1 --json
needlex analytics stats
needlex analytics value-report
needlex analytics failures
needlex analytics daily --limit 30
needlex logs stats
needlex support bundle --out /tmp/needlex-support
needlex doctor
```

## Output Rule

Default JSON is compact and AI-first:
1. less noise
2. proof-aware
3. diagnostics only when needed
4. `read` and `query` include a compact `analytics` footer so the value delivered by Needle-X is visible inline
5. `analytics stats` surfaces the fast headline view: runs, saved chars, estimated tokens, estimated cost

## Runtime Logs

Use `logs` when stderr gives a `diagnostic_id`:

```bash
needlex logs path
needlex logs stats
needlex logs tail --limit 20 --json
```

Runtime diagnostics live in the PAL state root at `<state-root>/logs/needlex.jsonl` and rotate automatically.

## Support Bundle

Use `support bundle` when a maintainer needs a portable diagnostic snapshot:

```bash
needlex support bundle --out /tmp/needlex-support
needlex support bundle --out /tmp/needlex-support --json
```

It exports doctor output, analytics JSON, log stats, log tail, and copied runtime log files.

## Doctor

Use `doctor` when install, MCP, analytics, or local state behavior looks inconsistent:

```bash
needlex doctor
needlex doctor --json
```

It reports:
1. installed version and executable path
2. `NEEDLEX_HOME` and effective state root
3. analytics and discovery database paths
4. local state subdirectories
5. runtime log path and event count
6. active MCP processes when detectable

## Next

1. [MCP And Tool Calling](./MCP-And-Tool-Calling.md)
2. [Discovery Memory](./Discovery-Memory.md)

## Full Reference

1. [Operator Guide](../operator-guide.md)
2. [Agent Answer Packet](../agent-answer-packet.md)
3. [Fetch Profiles](../fetch-profiles.md)
