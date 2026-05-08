# PAL Embedding Cache

## Status

Closed in the current worktree.

Implemented:

1. PAL layout exposes `$NEEDLEX_HOME/data/embeddings/cache`.
2. Runtime services and CLI/MCP memory paths use the PAL cache directory through the store root.
3. Dense HTTP embeddings use an in-process cache plus a persistent file cache.
4. Cache filenames are SHA-256 hashes, not raw text.
5. Cache writes use same-directory temp files and atomic rename with `0600` records.
6. Cache hits are validated by schema, key hash, dimensions, and finite vector values.
7. In-process and persistent cache entries honor TTL.
8. Bounded eviction by max entries, max bytes, positive TTL, and negative TTL.
9. Negative cache prevents short-lived provider retry storms.
10. Context-canceled calls are not negative-cached.
11. Stale vectors are returned on provider error when policy allows.
12. Concurrent identical misses singleflight to one provider call.
13. Discovery memory refresh reuses unchanged vectors without re-embedding.
14. `needlex doctor` surfaces embedding cache health.
15. `needlex prune --embedding-cache [--dry-run]` cleans disposable PAL cache files.
16. MCP `web_prune` supports `embedding_cache=true` for operator cleanup.
17. Env policy supports enable/disable, cache dir, TTL, negative TTL, size, entries, and stale-if-error.
18. PAL config exposes embedding cache policy under `semantic.embedding_cache`.
19. Analytics stats roll up cache hits, misses, writes, negative hits, stale hits, evictions, and evicted bytes.
20. Memory maintenance can force semantic re-embedding via `needlex memory rebuild-index --force-embeddings` and MCP `memory` with `force_embeddings=true`.
21. Persistent provider-cache records now carry an explicit provider identity, endpoint digest, options digest, input digest, modality, normalization, schema version, and dimensions; records are self-validating and do not store raw endpoint or input text.

This issue captures cache design lessons from a production-oriented Go embedding
runtime. Scope is Needle-X PAL embeddings only. Do not treat this as a request
to change retrieval philosophy: Needle-X remains semantic-first, multilingual,
embeddings-first, and zero-literal as ranking strategy.

## Decision

Needle-X should add a PAL-backed embedding cache with two separate layers:

1. Provider cache: avoids repeated calls to the embedding provider for identical semantic input under the same provider identity.
2. Refresh skip: avoids re-indexing already-current vector records when source text and target identity did not change.

These are different concerns and should not be collapsed into one mechanism.
Provider cache saves provider calls. Refresh skip protects index maintenance.

## Product Rule

The cache must preserve vector-space truth.

Valid cache identity must include:

1. schema version
2. provider
3. model
4. vector space id
5. dimensions
6. normalization
7. modality
8. provider options digest
9. input digest

Invalid cache identity:

1. raw target id only
2. filename only
3. URL only
4. query text without provider/model/space
5. any key that can mix models or dimensions

## PAL Layout

Recommended default:

```text
$NEEDLEX_HOME/data/embeddings/cache/
```

Vector snapshots or indexes should stay separate:

```text
$NEEDLEX_HOME/data/embeddings/vector.snapshot.json
```

Rules:

1. PAL home is the single source of truth.
2. Cache files are generated data, not repo artifacts.
3. Cache keys are hashed filenames, never raw text.
4. Cache writes are atomic: temp file in same dir, fsync or close, chmod, rename.
5. Cache files should use `0600`.
6. Directories should use `0755`.
7. Cache cleanup must be bounded by entries, bytes, and TTL.

## Cache Key

Recommended key material:

```text
schema_version
provider
model
space_id
dimensions
normalization
modality
options_digest
input_digest
```

Then:

```text
key = sha256(join(parts, "\0"))
```

Input digest:

```text
input_digest = "sha256:" + sha256(input_text)
```

The input target id must not be part of the provider-cache key. Same text under
different target ids should reuse the same vector. On cache hit, the stored
vector must be rebound to the current input id and current metadata.

## Record Shape

Minimum cached record:

```json
{
  "schema": "needlex.embedding-cache.v2",
  "key": "sha256...",
  "identity": {
    "provider": "ollama",
    "model": "embeddinggemma:latest",
    "space_id": "ollama:embeddinggemma:latest",
    "dimensions": 768,
    "normalization": "provider_default",
    "modality": "text",
    "schema_version": "needlex.embedding-cache.v2",
    "options_digest": "sha256:..."
  },
  "input_digest": "sha256:...",
  "record": {
    "id": "current-input-id",
    "provider": "ollama",
    "model": "embeddinggemma:latest",
    "space_id": "ollama:embeddinggemma:latest",
    "dimensions": 768,
    "normalization": "provider_default",
    "input_digest": "sha256:...",
    "vector": [0.1, 0.2],
    "metadata": {}
  },
  "created_at": "2026-05-08T00:00:00Z"
}
```

On hit, return:

1. cached vector
2. current input id
3. current metadata
4. current input digest
5. same provider/model/space/dimensions

Do not return old metadata from the cached record.

## Provider Identity

Provider identity must be explicit and versioned.

For Ollama:

```text
provider=ollama
model=embeddinggemma:latest
space_id=ollama:embeddinggemma:latest
normalization=provider_default
modality=text
```

Options digest should include only vector-affecting options:

1. base URL if endpoint can change model behavior
2. dimensions
3. truncate
4. normalization
5. provider options map

Do not include operational options that do not alter vector math, unless the
provider semantics require it:

1. keep_alive
2. timeout
3. retry count

## Default Policy

Recommended defaults:

```text
enabled=true for local Ollama
enabled=false for unknown remote providers unless explicitly enabled
max_entries=200000
max_bytes=2GiB
ttl=30d
negative_ttl=2m
stale_if_provider_error=true
```

Environment:

```text
NEEDLEX_EMBEDDING_CACHE=0|1
NEEDLEX_EMBEDDING_CACHE_DIR=/path
NEEDLEX_EMBEDDING_CACHE_MAX_ENTRIES=200000
NEEDLEX_EMBEDDING_CACHE_MAX_BYTES=2147483648
NEEDLEX_EMBEDDING_CACHE_TTL=720h
NEEDLEX_EMBEDDING_CACHE_NEGATIVE_TTL=2m
NEEDLEX_EMBEDDING_CACHE_STALE_IF_ERROR=1
```

Precedence:

1. explicit code policy
2. env overrides
3. PAL defaults

Important rule:

`NEEDLEX_EMBEDDING_CACHE=0` must disable cache even when `NEEDLEX_EMBEDDING_CACHE_DIR` is set.

## Positive Cache

Behavior:

1. skip empty input ids
2. compute key from provider identity plus input digest
3. lookup file cache
4. if valid and not expired, return rebound record
5. update file mtime on hit for LRU eviction
6. on miss, call provider
7. validate provider returned one record per requested input id
8. write cache atomically
9. evict after write

## Negative Cache

Negative cache avoids provider retry storms when local embedding runtime is down.

Behavior:

1. cache provider errors for a short TTL
2. return a typed negative-cache hit error during TTL
3. do not negative-cache `context.Canceled`
4. do not negative-cache `context.DeadlineExceeded`
5. delete negative cache when a positive record is written

Recommended typed error:

```go
var ErrNegativeCacheHit = errors.New("embedding cache negative hit")
```

## Stale If Error

If provider call fails and stale cache exists, return stale when policy allows.

Why:

1. local Ollama may be restarting
2. laptop may be resource constrained
3. retrieval quality with stale vectors is usually better than total outage
4. operator still sees provider health failure through logs/doctor

Do not hide the event. Emit metric/audit note:

```text
embedding_cache_stale_hit=true
embedding_provider_error=...
```

## Singleflight

Concurrent identical misses must produce one provider call.

Implementation:

1. package-level in-flight map keyed by cache key
2. first goroutine owns provider call
3. other goroutines wait on done channel
4. owner stores result or error
5. all waiters receive rebound record for their own input id/metadata

This matters for batch retrieval, MCP parallel calls, and startup bursts.

## Refresh Skip

Provider cache is not enough. Index refresh also needs unchanged-input skip.

For each index target:

1. build embedding input from canonical semantic fields
2. compute input digest
3. find existing vector record by embedding record id
4. if existing record has same input digest, target id, and target type, skip
5. if `force=true`, bypass skip

Refresh result should report:

```json
{
  "input_count": 120,
  "embedded_count": 7,
  "upserted_count": 7,
  "cached_target_ids": ["tag:abc"],
  "skipped_target_ids": ["wiki_document:old"],
  "space_ids": ["ollama:embeddinggemma:latest"]
}
```

Terminology:

1. `cached_target_ids`: unchanged vectors reused from index snapshot
2. `skipped_target_ids`: not eligible for indexing because stale/tombstoned/out of scope/limit
3. `embedded_count`: provider returned fresh vectors

## Scope And Lifecycle

Embedding refresh must respect memory lifecycle:

1. do not embed tombstoned records
2. do not embed stale records
3. do not embed superseded records
4. do not embed scope-incompatible records
5. do not retrieve vectors from a different `space_id`

Scope compatibility can be structured. It must not rely on keywords or language.

## Vector Store Notes

For a JSON vector snapshot:

1. store records by stable embedding record id
2. clone vectors on read/write
3. save sorted records for deterministic diffs
4. write temp snapshot then rename
5. query only matching `space_id`
6. filter by structured metadata
7. use bounded top-k candidate selection
8. sort by score desc, id asc for deterministic tie-break

Do not mix unversioned vectors with provider-versioned vectors.

## Observability

Every refresh should emit structured evidence:

```text
operation=embedding.refresh
status=completed|cached|failed
duration_ms
inputs
embedded
upserted
cached
skipped
space_ids
provider
model
cache_hits
cache_misses
negative_hits
stale_hits
evicted_entries
evicted_bytes
```

Doctor should report:

1. cache enabled
2. cache dir
3. file count
4. byte size
5. max entries
6. max bytes
7. TTL
8. negative TTL
9. stale-if-error
10. current provider identity
11. vector space id

## Cleanup

Needle-X should expose operator cleanup:

```bash
needlex prune --embedding-cache --dry-run
needlex prune --embedding-cache
```

Dry-run should report:

1. files matched
2. bytes reclaimable
3. oldest file
4. newest file
5. policy reason

This matters because local embedding cache can grow quickly on disk-constrained
machines.

## Security And Privacy

Rules:

1. never put raw text in filename
2. avoid storing raw input text in cache entry
3. store only digest, vector, provider identity, and metadata needed for retrieval
4. use `0600` for cache files
5. keep cache under PAL data, not source repo
6. allow full cache deletion without corrupting primary memory
7. treat cache as disposable generated data

Metadata caution:

Current input metadata may contain URLs, scopes, or titles. Keep only fields
needed by downstream retrieval. Avoid saving secrets, auth headers, request
payloads, cookies, or raw page bodies.

## Non-Goals

Do not implement:

1. lexical fallback when cache/provider unavailable
2. cross-provider vector reuse
3. automatic vector-space migration without explicit rebuild
4. raw prompt/body storage in cache
5. cache files inside repo
6. cache correctness based on filenames or URLs
7. retrieval decisions based on keywords, regex, language, or token overlap

## Failure Modes

Known risks:

1. Missing `space_id` mixes incompatible vectors.
2. Cache key includes target id and loses reuse.
3. Cache hit returns stale metadata from old target.
4. Negative cache TTL too long hides recovered provider.
5. No singleflight causes burst calls to Ollama.
6. No max bytes fills disk.
7. No force rebuild makes provider upgrade hard.
8. Remote provider default cache may store sensitive inputs unexpectedly.
9. Unbounded JSON vector query becomes slow.
10. Provider response count mismatch silently corrupts records.

## Acceptance Criteria

Implementation is ready when:

1. same text with different ids calls provider once and returns current ids
2. TTL expiration forces new provider call
3. provider error returns stale record when policy allows
4. provider error writes short negative cache
5. context cancellation is not negative-cached
6. concurrent identical misses call provider once
7. max entries evicts oldest records
8. max bytes evicts oldest records until under budget
9. `NEEDLEX_EMBEDDING_CACHE=0` disables cache despite cache dir
10. local Ollama default enables cache
11. remote/unknown provider default does not silently enable cache
12. refresh skip avoids re-embedding unchanged vector records
13. `force=true` bypasses refresh skip
14. vector query refuses wrong space id
15. doctor reports cache health
16. prune dry-run reports reclaimable cache bytes

## Suggested Package Shape

```text
internal/embeddingcache
  cache.go
  file_store.go
  identity.go
  policy.go
  provider.go
  pal.go
```

Suggested interfaces:

```go
type Provider interface {
	Embed(context.Context, []Input) ([]Record, error)
}

type Store interface {
	Get(context.Context, string, Input, time.Time, Policy) (Record, bool, error)
	GetStale(context.Context, string, Input, time.Time) (Record, bool, error)
	Put(context.Context, string, Input, Record, Identity, time.Time, Policy) error
	PutNegative(context.Context, string, error, time.Time, Policy) error
}

type CachedProvider struct {
	Provider Provider
	Store    Store
	Policy   Policy
	Identity Identity
	Now      func() time.Time
}
```

## Open Design Questions

1. Should cache default be enabled only for `localhost:11434`, or also for any PAL-configured local endpoint?
2. Should Needle-X cache query embeddings and document embeddings in one cache dir or separate namespaces?
3. Should cache policy live in PAL config, env only, or both?
4. Should `needlex doctor` warn when vector snapshot contains unversioned records?
5. Should cache cleanup run opportunistically on write only, or also through a background scheduler?

## Implementation Order

Phase 1 done:

1. Add PAL path and file-backed positive cache.
2. Wire runtime services and CLI/MCP memory paths to the PAL cache dir.
3. Add persistent cache tests.

Phase 2 done:

1. Add identity/policy structs and explicit env policy.
2. Add bounded eviction by max entries, max bytes, and TTL.
3. Add singleflight for concurrent identical misses.
4. Add negative cache and stale-if-provider-error.
5. Add refresh-skip logic in vector maintenance.
6. Surface cache health in doctor.
7. Add CLI and MCP cleanup support.
8. Expand tests listed above.
9. Document cache as disposable PAL data.

Phase 3 done:

1. Add PAL config-backed cache policy fields if env-only control proves insufficient.
2. Add analytics rollups for cache hits, misses, stale hits, negative hits, and evictions.
3. Add an explicit force-refresh path for memory embedding maintenance.

Phase 4 done:

1. Added a deeper provider-options digest and self-validating provider identity records.
