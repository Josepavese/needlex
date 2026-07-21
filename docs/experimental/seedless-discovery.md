# Experimental Seedless Discovery

Status: experimental, explicit opt-in, outside the stable agent workflow.

## Stable Product Boundary

The stable retrieval workflow is:

```text
Host agent search -> agent-selected URLs -> web_read per selected URL -> compact proof-carrying evidence
```

The host agent owns:
1. search provider and search strategy
2. candidate selection
3. the number of URLs to analyze
4. concurrency and research budget

Needle-X must not impose or suggest a numeric candidate limit. It compiles every URL the agent chooses to analyze.

## Experimental Access

Seedless execution remains available for development and benchmarking only through explicit selection:

```bash
needlex query --goal "company profile" --discovery web_search --json
```

MCP callers must omit `seed_url` only when they explicitly pass:

```json
{"goal":"company profile","discovery_mode":"web_search"}
```

A missing `seed_url` must never silently resolve to `web_search`. Stable schemas, examples, errors, README content, and the shipped agent skill must direct agents to their host search tool followed by `web_read`.

## Experimental Responsibilities

The implementation remains:
1. embeddings-first and multilingual
2. inspectable through candidate metadata and reason codes
3. covered by deterministic tests
4. evaluated with multi-run live benchmarks and failure taxonomy
5. isolated from stable product claims

## Promotion Gate

Seedless discovery can return to the stable agent workflow only after fresh evidence demonstrates:
1. repeatable multi-run selected-source quality
2. production-grade runtime success and timeout behavior
3. acceptable median and p95 latency
4. stability across languages, entities, source families, and providers
5. a failure mix that does not require provider-specific or surface-form ranking hacks
6. clear advantage over the host-search-plus-`web_read` workflow

Until that gate is explicitly accepted, seedless remains a research lane and never an automatic fallback.
