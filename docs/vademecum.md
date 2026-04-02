# Needle-X Vademecum

Questo documento è la dottrina operativa da qui all'uscita pubblica su GitHub.

Non è una nota di visione.
Non è una backlog infinita.
È il contratto di esecuzione che seguiamo fino al rilascio.

Se un task locale confligge con questo file, il task locale è sbagliato finché questo file non viene aggiornato.

## Dashboard

- Last updated: `2026-03-31`
- Current phase: `release hardening`
- Execution mode: `macrostep bursts only`
- Product identity: `local-first web context compiler for AI agents`
- Primary output contract: `agent answer packet`
- Active CPU baseline: `Gemma 3 1B`
- Active semantic baseline: `intfloat/multilingual-e5-small`
- Active semantic backend: `openai-embeddings`
- Active model task: `resolve_ambiguity`
- Default philosophy: `AI-first compact output, diagnostics explicit`
- Search status: `seed-first strong, seedless best-effort`

## Product Definition

Needle-X is not:
1. a generic scraper
2. a browser agent
3. a search engine
4. an LLM-first reader
5. an infra-heavy hosted pipeline

Needle-X is:
1. a local-first web context compiler
2. a deterministic substrate reducer
3. a semantic context layer for meaning-sensitive decisions
4. a proof-carrying context packer for agents
5. an AI-first runtime whose default payload is directly consumable by another AI

## Non-Negotiables

1. Deterministic first.
2. Local-first state is part of the product, not an implementation detail.
3. Compact compiled output is the default surface.
4. Proof, trace, replay, and diff stay first-class, but not default.
5. Semantic signals arbitrate meaning; lexical overlap does not.
6. No linguistic heuristics as primary decision logic.
7. Models are bounded solvers, not the substrate.
8. No new active model capability without direct evidence.
9. No widening of the market claim while behavior is narrower.
10. No infra requirement as a precondition for using the repo.
11. Seedless discovery is allowed only as best-effort; it is not the product core.
12. Every macrostep must improve one of:
   - user value
   - output quality
   - trust/provenance
   - operator ergonomics
   - architectural coherence

## What Is Already Closed

These fronts are not the active focus anymore unless a real regression reopens them:

1. Runtime trust baseline
2. Semantic-first context doctrine
3. Compact-by-default CLI/JSON
4. Product contract narrowing
5. Operator guide baseline
6. Go-to-market narrative baseline
7. Core package concentration recovery
8. Docs cleanup: active / experimental / archive split

## What Is Not The Focus Now

From now to GitHub release we do **not** spend cycles primarily on:

1. speculative multi-agent runtime expansion
2. broad search-engine replacement claims
3. browser automation
4. anti-bot reverse-engineering as product identity
5. deep budget-only work unless it blocks shipping quality
6. new specialist model tasks
7. infrastructure-dependent features
8. benchmark theater with non-comparable competitors

## Active Phase

## Release Hardening

Goal:
Make Needle-X convincing in real use by an AI agent on seeded workflows, then package that behavior cleanly for public release.

The active axis is:
1. real workflows
2. packet quality
3. proof ergonomics
4. docs and demos aligned with actual behavior

## Macrostep Board

### 1. Real Agent Workflows

Status: `[in_progress]`

Goal:
Run Needle-X on real pages and real goals the way an AI agent would use it.

Definition of done:
1. at least `5-10` real seeded workflows exercised
2. cases include:
   - docs pages
   - corporate sites
   - multilingual pages
   - seeded same-site query routing
3. each case is reviewed from the perspective of:
   - answer quality
   - chunk quality
   - proof usefulness
   - uncertainty usefulness
   - token cost
4. each real failure produces either:
   - a code fix
   - a contract clarification
   - an explicit non-goal

Hard rule:
No “looks good in theory” closure. Only behavior observed in real runs counts.

### 2. Agent Answer Packet Refinement

Status: `[in_progress]`

Goal:
Make the default returned format the one an AI actually wants without extra parameters.

Definition of done:
1. default output stays ordered like this:
   - `kind`
   - primary locator
   - `summary`
   - `uncertainty`
   - `chunks`
   - `candidates` / `selection_why` when relevant
   - `signals`
   - `cost_report`
2. the default packet is enough for common agent reasoning without opening full diagnostics
3. proof lookup remains simple when the agent needs verification
4. no default field exists only for internal implementation nostalgia

Hard rule:
The default output is an answer packet, not a runtime dump.

### 3. Seed-First Product Excellence

Status: `[in_progress]`

Goal:
Make `read` and seeded `query` clearly strong and reliable.

Definition of done:
1. seeded `query` can route to the correct same-site page in real docs/corporate cases
2. `read` returns compressed context that is meaningfully smaller and more useful than the raw page
3. common structural noise classes are demoted:
   - boilerplate
   - index-like link hubs
   - code/constants when explanatory text exists
4. provenance is inline enough for an agent

Hard rule:
We optimize the strongest path first:
`seeded page -> compiled context -> proof-backed answer`

### 4. Release Surface Cleanup

Status: `[pending]`

Goal:
Remove or archive anything that weakens the public repo story.

Definition of done:
1. docs root contains only active product documents
2. experimental ideas stay in `docs/experimental`
3. historical material stays in `docs/archive`
4. `improvements/` root contains only active operational artifacts
5. dead scripts and dead reports are removed or archived
6. examples shown in README and operator docs actually work

Hard rule:
The repo must not ask a new user to perform archaeology.

### 5. GitHub Release Assembly

Status: `[pending]`

Goal:
Produce a GitHub-facing release that is honest, narrow, and usable.

Definition of done:
1. README is aligned with actual runtime behavior
2. operator guide is enough to run the product end-to-end
3. one or more real workflows are documented as demos
4. release notes can state clearly:
   - what Needle-X does
   - what it does not do
   - what path is strong today
5. the public claim is narrow enough to survive scrutiny

Hard rule:
Do not ship a broader story than the runtime can support.

### 6. Seeded Benchmark Closure

Status: `[pending]`

Goal:
Build the first serious seeded benchmark that can later support a competitive comparison.

Definition of done:
1. a seeded benchmark spec exists and is followed
2. a `30-50` case seeded corpus exists with explicit ground truth
3. protocol distinguishes:
   - internal validation
   - scaled evaluation
   - competitive comparison
4. no market comparison happens before internal seeded quality is measured clearly

Reference:
1. [seeded-benchmark-spec.md](seeded-benchmark-spec.md)

### 7. Competitive Benchmark Discipline

Status: `[pending]`

Goal:
Compare Needle-X against the market without collapsing different product categories into a fake leaderboard.

Definition of done:
1. direct references are kept separate from adjacent references
2. the initial direct references are:
   - `Firecrawl`
   - `Tavily`
   - `Exa`
   - `Brave Search API`
3. the initial simple baseline is:
   - `Jina Reader` or equivalent raw-page baseline
4. the initial adjacent browser reference is:
   - `Vercel Browser Agent`
5. reports explicitly declare when a comparison is:
   - direct
   - baseline
   - adjacent
6. reports explicitly separate:
   - quality metrics
   - advantage metrics
7. advantage metrics include where relevant:
   - packet size / token savings
   - hop count to target
   - tool calls to target
   - time to verifiable claim

Hard rule:
Vercel Browser Agent is not treated as a fully isomorphic Needle-X competitor.

Implementation note:
In this repo, Vercel Browser Agent is benchmarked through a deployable bridge endpoint, not through a single official public API equivalent to Firecrawl or Tavily.

Marketing rule:
Needle-X is allowed to tell the story of where it saves agent work.
That means we should measure and publish:
1. less text to read for the same outcome
2. fewer hops to the right page
3. faster path from answer to proof

But:
1. these claims must stay anchored to real benchmark runs
2. they do not replace quality failures

## Release Gate

GitHub release is allowed only if all of these are true:

1. `go test ./... -count=1` passes
2. the default CLI output is AI-first and compact
3. at least a small real workflow set has been exercised successfully
4. README, spec, operator guide, and this vademecum agree
5. seeded `read/query` demos are stable enough to show publicly
6. no dead or misleading product surface remains in active docs

Soft gate:
1. structural budget should keep improving
2. but release hardening is not blocked on chasing budget purity alone if product quality is the higher-value frontier

## Real Workflow Policy

From now to release, work should prefer this loop:

1. choose a real page or site
2. run `read`
3. run seeded `query` when relevant
4. inspect compact output as an AI consumer would
5. inspect `proof` only when needed
6. fix the product if the result is not immediately useful
7. update docs if the contract changed

A workflow burst is successful only if it ends with one of:
1. product improved
2. docs clarified
3. a non-goal made explicit

## Search Policy

Search is now governed by this rule:

1. `seed-first` is core
2. `seedless` is best-effort
3. no infra is required
4. no public search provider is treated as strategic foundation
5. `Discovery Memory` remains a future strategic front, not a release blocker

References:
1. [seedless-discovery-strategy.md](experimental/seedless-discovery-strategy.md)
2. [discovery-memory-spec.md](experimental/discovery-memory-spec.md)

## Output Policy

Default output must optimize for:
1. low token cost
2. fast navigation
3. direct AI consumption

Default output must not optimize for:
1. maximal debug detail
2. schema nostalgia
3. internal implementation completeness

Diagnostics remain available through:
1. `proof`
2. `replay`
3. `diff`
4. `--json-mode full`

Reference:
1. [agent-answer-packet.md](agent-answer-packet.md)

## Working Order

Until release, prefer work in this order:

1. real workflow failures
2. default packet quality
3. proof / trust ergonomics
4. release docs and demos
5. cleanup of dead or misleading surfaces
6. structural polish

## Anti-Drift Rules

Stop and correct course if we start doing any of these:

1. broadening seedless discovery claims
2. adding model behavior without evidence
3. optimizing internals with no product effect
4. shipping docs that describe optional diagnostics as the primary product
5. spending multiple bursts on infra-shaped ideas for a repo-only product
6. reintroducing lexical heuristics as context arbiters

## Primary References

1. [README.md](../README.md)
2. [spec.md](../spec.md)
3. [project-context.md](project-context.md)
4. [operator-guide.md](operator-guide.md)
5. [agent-answer-packet.md](agent-answer-packet.md)
6. [benchmark-report.md](benchmark-report.md)
