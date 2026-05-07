# Core Embeddings Runtime

## Status

Implemented in the working tree.

Validation evidence:

1. production `semantic.enabled` and `NEEDLEX_SEMANTIC_ENABLED` controls removed
2. production noop semantic runtime removed
3. PAL SSOT config added for CLI and installed wrappers
4. installers prepare and probe the default Ollama embedding substrate
5. `needlex config` manages substrate fields and rejects semantic mode toggles
6. `needlex doctor` reports semantic readiness
7. memory observation no longer stores vectorless semantic documents

## Decision

Needle-X is an embeddings-first retrieval runtime.

Dense semantic retrieval is not an optional feature, not a profile, and not a user-facing mode.
It is the normal core operating model.

Therefore Needle-X must not support a production/runtime path where semantic embeddings are disabled.

If the local embedding substrate is missing, broken, or misconfigured, Needle-X must fail clearly and guide the operator toward repair.
It must not silently degrade into lexical, structural-only, or surface-form retrieval.

## Product Rule

The valid installed product state is:

1. one PAL home
2. one PAL SSOT config
3. local no-key embeddings available
4. Ollama as the default current embedding substrate
5. `embeddinggemma:latest` as the default current embedding model
6. durable vector-space identity
7. CLI and MCP sharing the same config and embedding substrate

The invalid product state is:

1. `semantic.enabled=false`
2. empty embedding endpoint
3. empty embedding model
4. empty vector-space identity
5. runtime fallback to token overlap, string similarity, n-grams, language lists, or lexical scoring

## Why

The project doctrine is semantic-first, multilingual, and zero-literal as a ranking strategy.

If embeddings are optional, the system can accidentally run in a structurally plausible but philosophically invalid mode.
That creates misleading benchmark results and makes regressions hard to interpret.

Needle-X should have one honest runtime contract:

1. real dense embeddings exist
2. semantic context is available
3. local memory stores real vectors
4. ranking and reranking may use semantic alignment
5. if this cannot be satisfied, the operator must fix the runtime

## Current Problem

The codebase still exposes semantic activation as a configurable switch in several places:

1. `semantic.enabled`
2. `NEEDLEX_SEMANTIC_ENABLED`
3. docs that describe semantic as inactive until configured
4. tests that explicitly disable semantic
5. logic that returns `NoopSemanticAligner`
6. benchmark profiles that treat semantic as a conditional add-on

This is inconsistent with the product direction.

The only valid distinction should be:

1. embeddings runtime ready
2. embeddings runtime not ready

Not:

1. semantic mode enabled
2. semantic mode disabled

## Target Architecture

### Config

The PAL SSOT config remains useful, but not as a switch to choose whether semantics exist.

It should define only the concrete local embedding substrate:

```json
{
  "semantic": {
    "embedding_url": "http://127.0.0.1:11434/api/embed",
    "provider_model": "embeddinggemma:latest",
    "vector_space": "ollama-embeddinggemma-v1",
    "timeout_ms": 1200,
    "failure_cooldown_ms": 5000,
    "similarity_threshold": 0.55,
    "dominance_delta": 0.08,
    "max_candidates": 4
  }
}
```

`enabled` must be removed from public config and schemas.

If the field remains temporarily for migration, it must be ignored or rejected, not honored.
Preferred final state: strict config rejects `semantic.enabled`.

### Runtime

The runtime should construct a real dense embedder as part of normal service initialization.

Expected behavior:

1. config validates semantic endpoint/model/vector-space
2. service initialization creates the embedder
3. operational commands fail if the embedding substrate is unavailable when required
4. memory observation writes real embeddings
5. memory search uses real embeddings
6. candidate intelligence and reranking use real embeddings
7. no `NoopSemanticAligner` in production path

Test doubles may still exist, but only in tests.

### Installer

Installers must prepare the default semantic substrate.

Linux:

1. detect `ollama`
2. install Ollama if missing through the official install path
3. start/probe the local API
4. pull `embeddinggemma:latest`
5. write PAL SSOT config
6. probe `/api/embed`

macOS:

1. detect `ollama`
2. install through Homebrew when available
3. otherwise stop with clear instructions
4. start/probe the local API
5. pull `embeddinggemma:latest`
6. write PAL SSOT config
7. probe `/api/embed`

Windows:

1. detect `ollama.exe`
2. install through `winget install Ollama.Ollama` when available
3. otherwise stop with clear instructions
4. start/probe the local API
5. pull `embeddinggemma:latest`
6. write PAL SSOT config
7. probe `/api/embed`

The installer should be convergent:

1. never overwrite user config unless explicitly requested
2. keep existing valid vector-space identity
3. avoid duplicate PATH entries
4. preserve local memory DBs
5. print the config path and semantic readiness result

### CLI

`needlex config` should configure the substrate, not the doctrine.

Allowed:

```bash
needlex config show
needlex config path
needlex config set semantic.embedding_url http://127.0.0.1:11434/api/embed
needlex config set semantic.provider_model embeddinggemma:latest
needlex config set semantic.vector_space ollama-embeddinggemma-v1
```

Forbidden:

```bash
needlex config set semantic.enabled false
```

That command should fail because semantic is not a mode.

### Doctor

`needlex doctor` should report semantic readiness as a first-class health signal:

1. config path
2. endpoint
3. provider model
4. vector space
5. API reachable
6. embedding probe succeeded
7. vector dimension
8. active MCP processes that may need restart

If not ready, the output should be actionable:

1. Ollama missing
2. Ollama API not running
3. model missing
4. embedding endpoint returned invalid response
5. config invalid

### Repair

Consider adding:

```bash
needlex repair semantic
```

This should be a later step if installer and doctor are not enough.

Possible behavior:

1. install or locate Ollama
2. start Ollama if safe
3. pull default embedding model
4. rewrite only missing semantic config fields
5. probe endpoint

Do not block the core refactor on this command.

## Implementation Plan

### Phase 1: Remove Semantic Optionality

1. Remove `Enabled bool` from `config.SemanticConfig`.
2. Remove `NEEDLEX_SEMANTIC_ENABLED` handling.
3. Remove `semantic.enabled` from `model-baseline.json`.
4. Update strict config tests so `semantic.enabled` is rejected.
5. Change validation so semantic endpoint/model/vector-space are always required.
6. Remove docs/examples that imply semantic can be disabled.

Acceptance:

1. `config.Defaults().Validate()` passes with default Ollama embedding config.
2. config containing `{"semantic":{"enabled":false}}` fails strict decode.
3. no production code branches on `cfg.Semantic.Enabled`.

### Phase 2: Make Embedder Mandatory

1. Replace `SemanticActive(cfg)` with `SemanticConfigured(cfg)` or remove it entirely.
2. Make `NewTextEmbedder` return a real `DenseHTTPTextEmbedder` for valid config.
3. Keep no-op or fake embedders only as test helpers.
4. Ensure `NewSemanticAligner` does not silently return no-op for invalid runtime config.
5. Decide whether embedder reachability is checked at service init or at first semantic operation.

Recommendation:

1. config validity at load/init
2. network readiness in `doctor`
3. operation failure on actual embedding call if endpoint is down

This avoids making every command start with a network probe while still preventing semantic-disabled operation.

Acceptance:

1. operational semantic failures surface as clear diagnostic errors
2. no silent lexical fallback
3. no production no-op semantic aligner

### Phase 3: PAL SSOT Config

1. Ensure installed wrapper exports `NEEDLEX_HOME`.
2. Ensure installed wrapper exports `NEEDLEX_CONFIG=<PAL_HOME>/configs/needlex.json`.
3. Make `config.Load("")` read `NEEDLEX_CONFIG` when set.
4. Add `needlex config path`.
5. Add `needlex config show`.
6. Add `needlex config init`.
7. Add `needlex config set` for allowed substrate fields.
8. Reject unsupported keys and semantic disable attempts.

Acceptance:

1. user can run `needlex read ...` without exporting semantic env vars
2. CLI and MCP use the same PAL config
3. config mutation is explicit and inspectable

### Phase 4: Installer Semantic Substrate

1. Update Unix installer.
2. Update Windows installer.
3. Create config directory in PAL home.
4. Write default config only if missing.
5. Install/probe Ollama.
6. Pull/probe `embeddinggemma:latest`.
7. Print semantic readiness.
8. Add CI installer smoke with semantic prereq skip only where external install is too slow/flaky.

Acceptance:

1. fresh install creates config
2. rerun preserves config
3. wrapper exports config path
4. `needlex config show` works after install
5. `needlex doctor` reports semantic readiness

### Phase 5: Tests

Add or update tests for:

1. config rejects `semantic.enabled`
2. config requires endpoint/model/vector-space
3. config env path resolves from `NEEDLEX_CONFIG`
4. `needlex config init` writes valid default config
5. `needlex config set semantic.provider_model ...` persists
6. `needlex config set semantic.enabled false` fails
7. doctor includes semantic readiness fields
8. installer scripts parse successfully
9. service tests use embedding test servers instead of disabling semantic
10. semantic guard rejects reintroduction of `NEEDLEX_SEMANTIC_ENABLED`

Test rule:

Tests may use fake embedding endpoints.
Tests must not depend on local Ollama unless explicitly marked as integration/manual.

### Phase 6: Documentation

Update:

1. `README.md`
2. `docs/install.md`
3. `docs/model-baseline.md`
4. `docs/semantic-alignment-gate.md`
5. `docs/operator-guide.md`
6. `docs/wiki/Install.md`
7. `docs/wiki/CLI.md`
8. `docs/wiki/MCP-And-Tool-Calling.md`
9. `AGENTS.md`

Required doctrine:

1. embeddings are core runtime
2. semantic cannot be disabled
3. Ollama is the current default substrate
4. PAL config is the SSOT
5. use `needlex config` to change substrate
6. failure is preferable to fake semantic fallback

## Non-Goals

1. Do not add API-key providers as default prerequisites.
2. Do not support cloud-only embedding backends as the default product path.
3. Do not preserve legacy `semantic.enabled` compatibility.
4. Do not reintroduce Ollama-specific code outside the substrate/install boundary unless needed.
5. Do not make lexical fallback a recovery mechanism.
6. Do not force tests to require a real local Ollama daemon.

## Open Design Questions

### Should Ollama Be Hardcoded Or Adapter-Abstracted?

Current answer:

Use Ollama as the default current core substrate, but keep the code boundary provider-neutral enough that a future local embedding engine can replace it.

Product language:

1. embeddings are non-configurable doctrine
2. Ollama is the current bundled/default substrate
3. adapter replacement is an internal platform concern

### Should Runtime Probe At Startup?

Recommended answer:

No global network probe for every command.

Reason:

1. `needlex analytics stats` and `needlex logs tail` should not need Ollama
2. operational retrieval commands will fail naturally on embed call if Ollama is down
3. `doctor` is the explicit readiness probe

However, retrieval commands must not silently proceed without embeddings.

### Should Memory Store Documents Without Embeddings?

Recommended answer:

No for normal operations.

If embedding write fails, memory observation should report a diagnostic warning or failure rather than storing misleading vectorless semantic state.

If a maintenance command needs to inspect existing state without embeddings, it may do so, but it must not be treated as retrieval success.

## Acceptance Criteria

The issue is complete only when:

1. `rg "semantic.enabled|NEEDLEX_SEMANTIC_ENABLED|Semantic.Enabled|cfg.Semantic.Enabled" internal benchmarks scripts docs README.md AGENTS.md` returns no production/runtime references.
2. `go test ./...` passes.
3. semantic guard passes.
4. installer scripts parse.
5. fresh local install creates PAL config.
6. `needlex config show` shows Ollama embedding defaults.
7. `needlex doctor` reports semantic readiness.
8. MCP and CLI share the same config.
9. an invalid config missing semantic endpoint/model/vector-space fails clearly.
10. docs state that semantic embeddings are the core runtime, not an optional mode.

## Risk Register

### Risk: Installer Becomes Too Heavy

Installing Ollama and pulling an embedding model can be slow.

Mitigation:

1. print progress clearly
2. keep reruns convergent
3. skip only in CI with explicit env
4. avoid downloading if model already exists

### Risk: Existing Users Have Old Config

Old config may contain `semantic.enabled`.

Mitigation:

1. fail with clear strict config error
2. document `needlex config init --force` or manual migration
3. consider one-time migration only if public installs already contain the field

### Risk: Tests Become Flaky

Tests must not require local Ollama.

Mitigation:

1. use `httptest` embedding endpoints
2. keep local Ollama checks in manual/integration tests only
3. doctor probe tests can use fake clients or fake endpoints

### Risk: Future Non-Ollama Local Engines

Hardcoding Ollama everywhere would make replacement expensive.

Mitigation:

1. keep Ollama-specific code in installers and default config
2. keep runtime embedder HTTP/provider-neutral
3. preserve vector-space identity

## Summary

This refactor makes the product contract honest:

Needle-X does not optionally use semantics.
Needle-X is semantics.

Ollama is the current local no-key substrate.
The PAL config tells Needle-X where that substrate is.
If the substrate is unavailable, the correct behavior is a clear failure and repair path, not lexical degradation.
