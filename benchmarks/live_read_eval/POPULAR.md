# Popular web: frozen 1,000-URL retrieval baseline

The corpus uses existing published rankings, not a new popularity model:

- Tranco list **ZJ8PG**, aggregated September 29 through October 28, 2026; the
  downloaded prefix has 10,000 ranked registrable domains.
- The global CrUX top-list research cache at `zakird/crux-top-lists`, commit
  `35ba243941defcd21882b134d9dd354824922482` (October 13, 2026). CrUX provides
  web origins and popularity buckets, not an exact ranking inside each bucket.

Primary descriptions: [Tranco](https://tranco-list.eu/list/ZJ8PG),
[CrUX methodology](https://developer.chrome.com/docs/crux/methodology),
[the research data cache](https://github.com/zakird/crux-top-lists).
Exact download URLs, byte lengths, hashes and ranking configuration are embedded
in `benchmarks/corpora/popular-web-v1.json`.

## Sample and scope

Keep the historical 100 occurrences as 99 distinct URL tasks: the source file
contains an identical repeated `pandas-docs` entry, preserved in `origins`.
Add 901 ranked origins, selecting one per Tranco domain present in CrUX. Prefer
its best published CrUX bucket, HTTPS, then root/www identity. This can select a
popular subdomain such as Scratch under MIT, rather than inventing a root URL.

A predeclared 16-origin reference panel supplies non-software subject areas.
The remaining candidates are sampled in fixed SHA256 order, round-robin across
Tranco ranks 1-1000, 1001-5000 and 5001-10000. Final counts are 307/298/296 plus
99 historical cases. The source rank is never recalculated. Sampling is balanced
by popularity band, not proportional to traffic, source language or industry.
Most new URLs are origin homepages; the historical and separate quality tasks
provide deeper/specific retrieval. This is not a whole-site crawling benchmark.

New questions rotate across six languages. Source language is `und` unless
independently annotated; country suffixes do not determine it. Corporate families
are not all identified, so 1,000 URL tasks are not 1,000 independent companies.
The sample reflects CrUX's opt-in Chrome audience and publicly discoverable sites,
not every website or country. Infrastructure absent from CrUX is filtered before
measurement. No blocked, empty, slow or unsuccessful origin is replaced afterward.

## Reproduce the frozen corpus

Use an owned outside-repository scratch directory. Do not clone the source
ranking repositories or download models. The builder can fetch only missing
pinned ranking files and verifies their exact hashes and lengths:

```sh
python3 benchmarks/live_read_eval/prepare_popular.py \
  --inputs /tmp/owned-popular-inputs \
  --fetch-from benchmarks/corpora/popular-web-v1.json \
  --out /tmp/owned-popular-inputs/reproduced.json
cmp benchmarks/corpora/popular-web-v1.json /tmp/owned-popular-inputs/reproduced.json
```

The existing installed MCP reader is the measurement owner:
`benchmarks/decision_observer/live.py`. Use the installed wrapper and canonical
config/state; do not bypass public entrypoints with a test-only service. Ordinary
reads request neither Julia assertion nor acquisition observation. The code,
actual `/proc/PID/exe`, config, corpus and observation watchdog bind resume
identity. Browser batch directories are temporary; canonical product proofs,
traces and shared embeddings remain useful state and are not globally pruned.

The corpus `execution` object owns this Linux profile: deep/auto, one serial
reader, 2 GiB cgroup memory maximum, no swap, two CPU equivalents, an observer
watchdog of 300 seconds, and a 1 GiB shared-PAL growth allowance. Native fetch,
render and embedding budgets stay unchanged. The collector checks the workstation
SSOT free-space floor plus a next-batch reserve. Report profile/resource-limit
failures as such. A per-request begin record makes interrupted reads reconcilable;
an observer timeout is unknown, never an upstream failure or a success.

Run under an owned `systemd-run --user --scope`, passing the corpus profile to:

```sh
python3 benchmarks/decision_observer/live.py \
  --binary /home/jose/.local/bin/needlex \
  --config /home/jose/.local/share/needlex/configs/needlex.json \
  --corpus benchmarks/corpora/popular-web-v1.json \
  --model-pid MODEL_PID --out /tmp/owned-scratch/full-live.jsonl \
  --scratch-dir /tmp/owned-scratch --rpc-timeout-s 300 \
  --state-dir /home/jose/.local/share/needlex --max-state-growth-mib 1024
```

Use `--resume` only with matching pins and after reconciling any pending operation.
It appends, skips confirmed receipts and never automatically retries a failed URL.
The four admission-pilot URLs are separate, excluded from the final capped run.

## Source-grounded quality

`popular-web-quality-v1.json` contains 60 precise tasks and 62 expected markers:
52 prior independently annotated source facts, six new manually reviewed positive
facts from independent HTTP references, and two dynamic Quotes controls checked
against its public API. No Needle-X or Julia output became a gold label. It is a
small, software-heavy quality panel with new science/culture/media/commerce
examples, not a representative accuracy estimate for all 1,000 URLs.

Some new independent references are partial: observed positive quote presence is
valid, while unobserved content is not proven absent. Preserve blocked, empty and
partial reference outcomes. News facts are volatile. A failed frozen-marker check
requires reference/source-variant diagnosis before calling it an extraction loss.

Run the same collector after broad acquisition, with the quality corpus and a
temporary `--coverage-anchors` projection containing each `source.url` and `quote`.
This projection comes only from `must_contain_facts`, never from collected output.
Markers are tested on the complete compiled text, not the truncated annotation
snapshot. Proof mapping, final-capture diagnostics and factual retention are
reported separately. The raw snapshot explicitly reports whether it is complete.

`summarize_popular.py` refuses missing/duplicate/foreign or mixed-identity broad
receipts, and requires every quality case before publishing a complete report.
Availability, capture completeness, proof mapping, source-marker retention,
transport blocks, native timeouts and observer failures are distinct metrics.
Report latency/PSS by popularity band and historical cohort. Sampling covers the
MCP process and descendants; external embeddings are excluded, resident Julia is
reported separately even though inference is not requested. Cgroup charged-memory
peak is a different quantity from PSS. A shared host and one live pass cannot
prove causal performance gains against older uncapped runs.

Keep complete raw receipts privately in canonical benchmark state, compressed
once after validation. Publish a compact summary, source pins and scoped quality
findings; do not add thousands of copied web pages to the repository. Retire
owned profiles, downloads and scratch after the final consumer, preserving
canonical product proofs, useful evidence, user WIP and shared caches.
