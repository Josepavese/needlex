# Improvements Status

Reviewed: 2026-11-15 against **v0.1.52**. This manifest marks every file in
`improvements/`; it does not delete benchmark evidence.

A file is **fresh** only when its `generated_at_utc` belongs to the current
release cycle. No tracked report qualifies for v0.1.52. The newest tracked
report is from 2026-09-18, and the tracked baselines are intentionally older.

Status vocabulary:

- **active baseline** — canonical comparison point used by a current script; keep.
- **stale operational** — current script output surface, but the file predates v0.1.52; regenerate before citing it.
- **analysis-only** — local evidence for a past/current analysis, not a release benchmark.
- **closed** — historical closure report; no current consumer.
- **obsolete** — superseded one-off or provider-specific capture; retained for provenance only.
- **fresh** — generated in the current release cycle (none at this review).

## Active baselines

| File | State | Generated | Note |
| --- | --- | --- | --- |
| [live-read-baseline.json](live-read-baseline.json) | active baseline | 2026-03-28 | Used by live-read and live-semantic runners. |
| [hard-case-matrix-baseline.json](hard-case-matrix-baseline.json) | active baseline | 2026-03-29 | Used by the hard-case matrix runner. |

## Stale operational reports

These are still named `latest`, but none is a v0.1.52 measurement. Keep them
for comparison and regenerate before using them as current evidence.

| File | State | Generated | Note |
| --- | --- | --- | --- |
| [discovery-eval-latest.json](discovery-eval-latest.json) | stale operational | 2026-09-18 | Newest tracked report; also a local dirty file, left uncommitted. |
| [live-read-latest.json](live-read-latest.json) | stale operational | 2026-03-29 | Ignored/regenerable output. |
| [hard-case-matrix-latest.json](hard-case-matrix-latest.json) | stale operational | 2026-03-29 | Output paired with the baseline. |
| [live-semantic-eval-latest.json](live-semantic-eval-latest.json) | stale operational | 2026-03-30 | Referenced by evaluation docs. |
| [live-semantic-global-eval-latest.json](live-semantic-global-eval-latest.json) | stale operational | 2026-03-30 | Multilingual evaluation snapshot. |
| [seeded-benchmark-latest.json](seeded-benchmark-latest.json) | stale operational | 2026-03-31 | Referenced by benchmark docs. |
| [discovery-memory-benchmark-latest.json](discovery-memory-benchmark-latest.json) | stale operational | 2026-03-31 | Referenced by benchmark docs. |
| [competitive-benchmark-latest.json](competitive-benchmark-latest.json) | stale operational | 2026-03-31 | Referenced by the competitive protocol. |
| [seedless-ddg-benchmark-latest.json](seedless-ddg-benchmark-latest.json) | stale operational | 2026-05-07 | Seedless lane snapshot. |

## Analysis-only evidence

| File | State | Generated | Note |
| --- | --- | --- | --- |
| [decision-live-latest.jsonl](decision-live-latest.jsonl) | analysis-only | 2026-10-29 mtime | Cited by `docs/system-one-assessment.md`; not a release benchmark. |
| [decision-quality-latest.jsonl](decision-quality-latest.jsonl) | analysis-only | 2026-10-29 mtime | Decision quality capture. |
| [decision-eval-latest.json](decision-eval-latest.json) | analysis-only | 2026-10-29 mtime | Decision evaluation capture. |
| [decision-refine-latest.json](decision-refine-latest.json) | analysis-only | 2026-10-29 mtime | Refinement capture. |
| [decision-content-oracle-latest.json](decision-content-oracle-latest.json) | analysis-only | 2026-10-29 mtime | Content-oracle capture. |
| [claim-live-latest.jsonl](claim-live-latest.jsonl) | analysis-only | 2026-10-29 mtime | Claim experiment evidence. |
| [claim-anchor-latest.jsonl](claim-anchor-latest.jsonl) | analysis-only | 2026-10-29 mtime | Claim anchor evidence. |
| [claim-quality-latest.jsonl](claim-quality-latest.jsonl) | analysis-only | 2026-10-29 mtime | Claim quality capture. |
| [claim-measure-latest.json](claim-measure-latest.json) | analysis-only | 2026-10-29 mtime | Claim measurement capture. |

## Closed historical

| File | State | Generated | Note |
| --- | --- | --- | --- |
| [live-validation-closure-latest.json](live-validation-closure-latest.json) | closed | 2026-03-30 | Historical closure report; no current script default. |

## Obsolete or superseded

| File | State | Generated | Note |
| --- | --- | --- | --- |
| [seedless-ddg-benchmark-before-bootstrap-profile-fix.json](seedless-ddg-benchmark-before-bootstrap-profile-fix.json) | obsolete | 2026-05-07 | Explicit pre-fix capture. |
| [seedless-ddg-benchmark-before-identity-window-fix.json](seedless-ddg-benchmark-before-identity-window-fix.json) | obsolete | 2026-05-07 | Explicit pre-fix capture. |
| [native-api-endpoint-benchmark-gemma3-1b.json](native-api-endpoint-benchmark-gemma3-1b.json) | obsolete | 2026-04-03 | Provider/model-specific one-off. |
| [native-api-endpoint-benchmark-qwen35-08b.json](native-api-endpoint-benchmark-qwen35-08b.json) | obsolete | 2026-04-04 | Provider/model-specific one-off. |
| [native-api-endpoint-benchmark-latest.json](native-api-endpoint-benchmark-latest.json) | obsolete | 2026-04-03 | Superseded one-off baseline. |
| [acquisition-policy-latest.json](acquisition-policy-latest.json) | obsolete | 2026-10-29 mtime | No active consumer found in the current tree. |
| [benchmark-100-source-analysis-latest.md](benchmark-100-source-analysis-latest.md) | obsolete | 2026-05-04 mtime | One-off source analysis, no active reference. |
| [needlex-web-retrieval-skill-review-latest.md](needlex-web-retrieval-skill-review-latest.md) | obsolete | 2026-04-28 mtime | Superseded skill review. |

## Index files

| File | State | Note |
| --- | --- | --- |
| [README.md](README.md) | active index | Policy, freshness review, and maintenance rules. |
| [STATUS.md](STATUS.md) | active manifest | This file. |

## Rule for future maintenance

Keep a coherent artifact when it is the only copy of a measurement, a
reproduction input, or a documented closure. Mark it here instead of deleting
it. A one-off capture may be removed only after its owner confirms that no
docs, runner, issue, or release evidence still consumes it.
