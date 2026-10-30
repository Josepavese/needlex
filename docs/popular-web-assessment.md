# Popular-web 1,000-URL assessment

The corpus now contains **1,000 distinct URLs/hosts** and has one complete live
installed-MCP baseline. This broadens availability testing; it does not establish
1,000-site factual accuracy. The separate 60-question quality panel is still
small and predominantly software documentation.

## Existing rankings and reproducibility

Use [Tranco ZJ8PG](https://tranco-list.eu/list/ZJ8PG), the published September 29
to October 28, 2026 aggregate, and the
[CrUX research cache](https://github.com/zakird/crux-top-lists/tree/35ba243941defcd21882b134d9dd354824922482)
from October 13. [CrUX methodology](https://developer.chrome.com/docs/crux/methodology)
describes its opt-in Chrome audience; its buckets are not exact origin ranks.
Download URLs, bytes and hashes are frozen in
[popular-web-v1.json](../benchmarks/corpora/popular-web-v1.json).

Preserve 100 historical occurrences as 99 unique URL tasks: the repeated Pandas
entry remains represented in `origins`. Add 901 origins present in the existing
rankings, with fixed-hash sampling across Tranco's top 10,000 domains. The bands
1-1,000 / 1,001-5,000 / 5,001-10,000 contribute 307 / 298 / 296 new cases.
A predeclared 16-origin subject panel adds science, culture, commerce and media;
the full sample is not category-balanced or traffic-weighted. Most new tasks
are origin homepages. Corporate families are not all independent.

Questions rotate across six languages; source language is not inferred from
country suffixes. No failed, blocked, empty or slow origin was replaced.
The builder reproduced the frozen broad manifest byte-for-byte. Commands and
limits live in [POPULAR.md](../benchmarks/live_read_eval/POPULAR.md).

## Measured baseline: installed v0.1.43

The actual executable remained pinned to
`7e52c95d76a2efb4181d4f360a705fa54a5323d4dcffceca2f5f39f5b42664b5` throughout both baseline runs.
The broad acquisition window was 2026-10-29T22:21:09.880020228Z to
2026-10-29T23:44:22.026428663Z (UTC, October 29; local completion
October 30). All 1,000 receipts share one measurement identity.

| Measurement | Result |
| --- | ---: |
| URL tasks | 1,000 |
| Nonempty packets with mapped proofs | 818 / 1,000 |
| Complete capture with mapped proofs, per diagnostics | 762 / 1,000 |
| Partial/degraded returned captures | 56 |
| Tool errors, retained | 182 |
| Returned packets with failed proof mapping | 0 / 818 |
| Rendered reads | 352 |
| Truncated rendered captures | 7 |
| Unexpected decision-model observations | 0 |
| All-attempt latency p50 / p95 / max | 2.511 / 16.371 / 38.454 s |
| MCP + descendant peak-PSS samples p50 / p95 / max | 256.8 / 500.0 / 760.0 MiB |
| Observed cgroup charged-memory peak | 838.8 MiB |
| Cgroup OOM events | 0 |

Capture completeness is a transport diagnostic, not proof that the question was
answered. The 134 unreadable-body events are resource counts, not 134 failed
pages. Seven open/truncated captures remain explicitly visible.

Historical tasks produced 97/99 packets (98/100 original occurrences) and 93/99
complete captures. The 901 ranked additions produced 721 packets and 669 complete
captures. Their median latency was 3.219 s versus 0.973 s for the historical group;
these are different samples with different cache/source conditions, not a
performance regression experiment.

Every run used one serial reader, deep/auto, browser batches of 12, unchanged
native budgets and a 300 s observation watchdog. Owned MCP/browser processes
were capped at 2 GiB, no swap and two CPU equivalents. External embedding-service
RAM is excluded; resident Julia PSS was sampled separately, without inference.
Cgroup charged memory and process PSS are different quantities. This shared-host,
one-pass experiment cannot prove multi-run stability or causal speed/RAM gains.

| Error taxonomy | Count |
| --- | ---: |
| `authentication_required` | 6 |
| `empty_content` | 5 |
| `provider_blocked` | 63 |
| `redirect_limit` | 2 |
| `runtime_error` | 40 |
| `tls_error` | 2 |
| `unavailable_resource` | 26 |
| `unavailable_upstream` | 18 |
| `unsupported_content_type` | 2 |
| `upstream_http_rejected` | 7 |
| `upstream_timeout` | 11 |

Of 40 runtime-contract errors, 33 were the optional-title producer defect fixed
below; seven were empty WebIR validations. These seven remain unresolved cases,
not evidence that useful source content was necessarily available. The much
larger transport/path failure group must not be diagnosed as a ranking miss.

## Quality: keep regression and independent evidence distinct

[popular-web-quality-v1.json](../benchmarks/corpora/popular-web-quality-v1.json)
contains 60 questions and 62 frozen literal markers. It yielded **58/60 tasks**
with all markers and **60/62 markers** overall, with valid proof mapping.

- Historical excerpts: 52/52 tasks retained. These were originally agent-reviewed
  Needle-X captures; independent HTTP before this quality run reconfirmed 39,
  while 13 differed by representation, locale or source variant. Retain those
  13 as regressions, not an entirely independent current-source oracle.
- Six new HTTP-reference questions: 4/6 exact-marker tasks retained.
- Two Quotes JS/API controls: 2/2 tasks, all four markers retained.
- The pre-run independently positive subset contains 47 questions/49 markers:
  45/47 complete literal tasks and 47/49 markers retained.

No current-run output or Julia judgment generated a new gold label. Literal
retention is not an answer-accuracy metric, and 52 software cases dominate this
small panel. News references are volatile; partial independent fetches establish
positive presence only, not full-page absence.

The two literal misses were reviewed without rewriting their scores:

1. **Open Library:** the compiled source was Chinese and explicitly stated its
   Internet Archive affiliation. The English marker fails across that locale
   variant. Agent review establishes factual presence in the excerpt, not a
   new independently human-reviewed accuracy score.
2. **Library of Congress:** the original full-text marker check failed. Its
   saved excerpt is bounded, contains SVG geometry and identifier-only XHR
   evidence, and cannot alone establish semantic absence across the full packet.
   Independent HTTP still contains the phrase. A fresh diagnostic read with a
   full packet export returns it, with zero captured application bodies versus
   three originally. Upstream/capture variation prevents attributing that result
   to the title patch. Objective retention amid noisy network payloads remains
   a concrete follow-up; causality has not been demonstrated.

## Delivered correction: v0.1.44

The broad run exposed 33 reads rejected because reduction always emitted
an empty `title` trace value. The document title is optional. Its producer now
omits absent/blank title metadata, without inventing titles, changing ranking
or adding site rules. Off/required regressions retain facts, proofs and replay.

The [published v0.1.44](https://github.com/Josepavese/needlex/releases/tag/v0.1.44)
was installed only after both v0.1.43 baseline runs ended. A recheck of exactly
those 33 URLs produced **zero title errors, 30 mapped-proof packets and 24
complete captures**. Three still fail empty WebIR validation. The targeted
latency median/p95 is 8.714/26.583 s: these requests now perform work previously
aborted, and changing upstream/cache conditions rule out a causal speed claim.
Do not add these results to the original 818/1,000 baseline or label them a full
v0.1.44 thousand-site run.

Full governance/tests/vet/staticcheck and release/main CI passed; six public
archives were verified by digest, binary-only membership and architecture.
Native installed CLI/MCP qualified the untitled JS fixture with explicit
`render=required`, seeded strict query, proof lookup and deterministic replay.
The public real `/scroll` smoke retained all ten quote texts and their author
name markers, with CDP/XHR capture, `content_source=dom+network`, no missing body
or truncation, valid proof mapping and deterministic replay.

An initial `/js/` smoke had already retained the quote/author markers but failed
an inappropriate assertion requiring application-network bodies from inline JS.
That failed observer log is preserved. `/scroll` exercises the actual XHR
transport without weakening the transport gate.

The canonical config remained byte-identical. `doctor` reports v0.1.44, semantic
and render readiness and guidance `stale=false`; its PATH/active-client warnings
are retained. Fresh MCP execution matches the public CI binary. Existing agent
clients were not restarted and are not claimed refreshed.

## Evidence custody and next use

[Machine-readable evidence](evidence/popular-web-2026-10-29.json) binds inputs,
executable/code hashes, all metrics, source diagnostics, release checks and raw
receipt hashes. Complete raw receipts/references are kept once in private
PAL-home benchmark state as `benchmarks/popular-web-v1/2026-10-29-v0.1.43/receipts.tar.gz`
(4,175,045 bytes), verified by decompression/readback. Canonical product
traces and proofs remain in their native store. Copied pages are not committed.
Owned browser profiles, build archives, source downloads and temporary install
are retired after their final consumer; shared models and useful state are kept.

Use this corpus for reproducible availability/failure comparisons. Next quality
work should add independently annotated non-software tasks and locale-equivalent
facts, then isolate DOM/network packing under matched inputs. Julia remains
advisory and unqualified for active decisions; this corpus did not re-evaluate it.
