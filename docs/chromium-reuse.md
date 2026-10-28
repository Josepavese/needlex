# MCP Chromium reuse qualification

This is a local source candidate, not an update to the installed release.
Measurements and native qualification were performed on Linux amd64 using the
existing Chrome Headless Shell 153.0.8010.52 and Go 1.27.1. Native PAL test
binaries also compile for Windows amd64 and macOS amd64; those are build checks,
not native lifecycle/performance qualification on those operating systems.

## Lifetime and resource contract

The MCP dispatch loop owns a lazy renderer shared across read, query and crawl.
Each render opens a separate CDP connection, creates a fresh browser context
with `disposeOnDetach`, disposes it, then releases the browser lease. Cookies,
local/session storage, IndexedDB, CacheStorage and service workers are isolated.
User-Agent overrides apply to the target rather than the shared browser.

`config.DefaultRenderConfig` is the lifecycle default owner. The current policy
allows one active context, retires the browser after 30 seconds without leases,
and recycles it after 32 acquisitions. Positive overrides are available through
`render.max_concurrency`, `render.browser_idle_timeout_ms` and
`render.browser_max_uses`, their existing config environment resolver, and
`needlex config set`. A configuration with zero concurrency is rejected rather
than interpreted as unlimited. These are process/context/lifetime bounds, not a
cgroup or other hard total-RAM ceiling.

At most one browser process is owned by the reuse runtime. Degraded dump-DOM
capture obtains that same process slot exclusively. Existing deadlines,
network/body/message budgets and explicit degradation/truncation reporting
remain in force. A changed render configuration retires the previous runtime
between MCP operations. CLI services use the same capture code with per-render
lifetime and their shared service concurrency gate.

The PAL owns process launch, private temporary profiles, endpoint discovery,
bounded process output, idle scheduling and native cleanup. On Unix, owned
process groups are killed/reaped and stdin is borrowed through an interruptible
descriptor whose flags are restored at shutdown. EOF, transport errors and
SIGTERM close the MCP renderer. Arbitrary SIGKILL of the MCP process is outside
this graceful-cleanup qualification.

## Measurement method

The source is [measure.py](../benchmarks/render_reuse/measure.py). Before/after
executables are built from the actual worktrees, including prior consolidation
WIP; the baseline is not an unmodified release or merely the Git HEAD. Their
binary hashes and effective input configuration are retained with the results.
Both are configured for the same browser and existing local embedding service; model backend is
noop and semantic discovery memory is disabled in the fixture configuration.

The deterministic local application exercises DOM text generated after load,
fetch, XHR, finite SSE/WebSocket and streamed fetch, plus repeated same-origin
storage isolation checks. Three rounds contain twelve MCP reads per executable.
The final comparison uses the production network-idle setting of 1,500 ms.
Runs are serial after owned builds/tests finish. End-to-end latency includes
MCP, acquisition, rendering, embedding, evidence compilation and persistence.

RAM is sampled every 50 ms from `/proc/*/smaps_rollup`, aggregating the MCP
process and descendants from every OS thread's children. PSS apportions shared
pages; the accompanying summed RSS double-counts shared pages. The fixture
driver, embedding server and unrelated processes are excluded. Idle RAM is
observed both after the final response and after 31 seconds. Peaks are sampled,
not an assertion of an absolute maximum.

An early observer missed non-main-thread children and an early fixture was too
thin for existing retrieval pruning. Those preliminary numbers are not used in
the final comparison. The checked fixture asserts all expected content markers
in WebIR; it does not count a successful RPC as evidence completeness.


## Before/after results

Full compact evidence, binary/browser hashes, input render configuration and
per-read comparisons are in [the measurement JSON](measurements/chromium-reuse-20261027.json).

| Measurement | Before | Reuse candidate |
| --- | ---: | ---: |
| First read, browser cold (one observation) | 2.157 s | 1.901 s |
| Median of subsequent 11 reads | 2.154 s | 1.790 s |
| Median of all 12 reads | 2.154 s | 1.846 s |
| Sampled peak process-tree PSS | 206.0 MiB | 207.8 MiB |
| Idle process-tree PSS, immediately after last read | 21.7 MiB | 186.2 MiB |
| Idle process-tree PSS after 31 seconds | 21.7 MiB | 21.9 MiB |
| Browser process starts | 12 | 1 |
| Live descendants after MCP exit | 0 | 0 |

Subsequent-read median latency improves by **16.89%** in this
corpus. Peak PSS is essentially unchanged; reuse retains roughly 164 MiB more
idle memory until retirement. This is a latency/idle-memory tradeoff, not a
RAM-reduction claim. Twelve reads are not a general web performance guarantee.

All twelve before/after pairs have identical complete WebIR text multisets
(normalizing only the fixture's ephemeral origin) and identical compact capture
signals. Required DOM and every fetch/XHR/SSE/WebSocket/streamed-fetch marker
are present; finite captures report neither truncation nor degradation.

## Reliability evidence and limits

- Native Chromium tests passed for isolated cookies, local/session storage,
  IndexedDB, CacheStorage, service workers and target User-Agent overrides;
  no browser contexts survived completed renders.
- Real configured use/idle budgets, native browser crash recovery, cancellation
  and closed-runtime refusal passed. Pool tests cover concurrent lease limits,
  cancelled waiters/startup and retirement without killing active borrowers.
- Existing real-browser capture regressions pass for DOM, fetch/XHR, SSE,
  WebSocket, streaming bodies, slow/open streams and truncation diagnostics.
- Actual candidate MCP subprocesses terminate on EOF, malformed transport input
  and SIGTERM while idle or rendering, without live descendants or profiles.
  The signal probe first exposed blocking stdin; the PAL descriptor regression
  now verifies interruption and preservation of the original input flags.
- Scoped race tests and the complete governance gate pass. Advisory complexity,
  LOC/package/dependency/concentration targets and shared-cache warnings remain.

A public smoke on <https://quotes.toscrape.com/js/> did not qualify content:
both executables produced the same 44-character, one-node evidence without the
expected quotes. A direct Chromium DOM capture contained the expected author
and 8,987 bytes of HTML. This identifies a pre-existing reduction/content gap,
not a successful public-site acceptance test. Browser reuse does not repair
that independent reducer issue. The controlled corpus and native tests do not
establish universal open-web, anti-bot or cross-platform acceptance.

### Reducer follow-up, 2026-10-27

The independent reduction gap is now fixed in the source candidate. The reducer
retains text nodes outside its recognized text blocks, in DOM order, without
emitting them again when an ancestor already owns their text. Source selectors
include exact text-node indices. Recovered text respects subtree exclusions,
including hidden content and inert templates, also inside recognized blocks.
No site-specific production selectors, ranking rules or rendering exceptions
were added.

An independent Chromium capture of the first `/js/` page provides ten complete
quotes and ten author occurrences as the oracle. Before the fix, both MCP render
modes retained none of them. After the fix, `auto` and `required` retain all ten
pairs in WebIR, agent context and compact MCP output (two chunks). Every chunk's
proof reference resolves through `web_proof`, with matching source selector and
text span. Actual CLI reads also retain all ten pairs in compact and full JSON.
Capture reports neither degradation nor truncation.

[Reducer measurement evidence](measurements/reducer-20261027.json) retains binary
hashes, effective render settings and coverage counts. Synthetic multilingual
regressions cover generic/custom elements, mixed inline/block text, order,
visibility, inert templates and proof propagation. The earlier reuse measurements
remain historical results from before this fix; their RAM/latency comparison was
not repeated. This follow-up qualifies that page, not arbitrary web content or
pagination, and does not update the installed release.

The complete governance gate passes with existing advisory warnings: full suite
with native Chromium available, vet, staticcheck, zero structural issues and
35,004 runtime Go lines within the unchanged 35,100 hard ceiling.

## Code budget

The runtime grows from 34,390 to 34,995 physical Go lines, including PAL lifetime
and interruptible-input cleanup. Top-level internal areas and runtime
dependencies remain at twelve and five. The strategic recalibration admits a
35,100 hard LOC ceiling while retaining the 34,500 target and zero structural
issues; it tightens the hard count of files over 300 lines from 29 to 27
(26 observed). File/package concentration guards remain active.
