# Acquisition policy qualification

This is a passive experiment, not an active acquisition engine. The native
service harness captures the original HTTP surface and then measures the
current baseline and each permitted static/declared/browser route. Julia sees
only that original surface, the objective and the permitted menu, after every
route receipt is frozen. Later content, source-fact markers and baseline choices
never become model features. No fixed candidate count is imposed on agents.

The frozen corpus has 75 tasks: 54 public-page tasks and 21 fixtures, grouped
into 59 families. Its 20 development and 55 heldout tasks do not share families.
Source facts are agent-reviewed source-scoped markers, not human-reviewed truth
labels or a completeness claim for whole websites. Fixture questions use English,
Italian and Chinese. Public pages retain their own source-language markers.

Build outside the repository, with the already configured local model and dense
endpoint available:

```sh
go test -c -o /tmp/owned-scratch/acquisition.test ./internal/core/service
python3 benchmarks/acquisition_policy/measure.py \
  --binary /tmp/owned-scratch/acquisition.test --config /path/to/needlex.json \
  --out /tmp/owned-scratch/routes.jsonl --phase collect \
  --resources /tmp/owned-scratch/resources.jsonl --model-pid MODEL_PID
```

Run `development`, then `heldout`, with the same command and
`--input /tmp/owned-scratch/routes.jsonl`, separate `--out` files, and the matching
`--phase`. Both native phases use append-only receipts with identity checks.
Freeze the dense threshold before starting heldout inference:

```sh
python3 benchmarks/acquisition_policy/summarize.py \
  --development /tmp/owned-scratch/development.jsonl \
  --threshold /tmp/owned-scratch/threshold.json --out /tmp/owned-scratch/dev.json
# Then run heldout inference, and score without fitting again:
python3 benchmarks/acquisition_policy/summarize.py \
  --development /tmp/owned-scratch/development.jsonl \
  --heldout /tmp/owned-scratch/heldout.jsonl \
  --threshold /tmp/owned-scratch/threshold.json \
  --resources /tmp/owned-scratch/resources.jsonl --out /tmp/owned-scratch/final.json
```

Successful delivery requires the requested marker, attributable proof and a
complete capture. A failed or truncated route stays unknown. Only the controlled
empty fixture supports an exhaustive `insufficient` oracle; a missing fact on a
changing public page does not. Among successful routes, fewer browser uses and
then fewer native GET attempts define the least-work action. Latency is reported
separately, avoiding optimization against one volatile timing sample.

The dense comparator uses the same selected whole passage as Julia. Its signed
similarity threshold is fitted only on development: maximize verified delivery,
then verified absence, then minimize browser uses and GET attempts. With a low
score it tries an available declared source, then rendering, then insufficiency.
Missing scores cause an abstention, not a zero similarity. The original baseline
also probes conventional resources. Its diagnostic `read_conventional` bucket
records non-direct final source declarations, including indirect linked
resources; it does not prove the root declaration was conventional. Baseline
optimality is compared by verified delivery and measured work, not by this
ambiguous provenance label.

Reports retain abstentions and unknown captures. Proposed-policy costs are
counterfactual estimates: measured selected-route time plus snapshot construction
and model/dense work. They are not active-policy latency or RAM results. Native
GET transport attempts exclude internal redirect hops; Chromium HTTP(S) request
events include redirects/cache events. These are distinct counters, not wire
request totals. PSS sampling is 50 ms over the test binary and owned Chromium,
with resident Julia separately and in aggregate. The external embedding service
is excluded consistently. A shared warm browser may remain resident on static
routes. No model RAM is claimed to disappear when inference is omitted.

Activation requires improved verified delivery or reduced cost without new
incorrect stops, plus an advantage over the same-evidence dense comparator on
heldout families. A negative result keeps the model advisory. Ordinary reads do
not invoke this experiment. Preserve receipts in a compact evidence artifact,
then retire the owned build/state/scratch; do not duplicate models or caches.


For independent corpus inference, a failed observation retains its failure and
waits the configured semantic cooldown before the next case. `failure_pacing_ms`
is accounted separately, not folded into or erased from request latency. The
unpaced operational batch remains evidence of throughput degradation.

Use `--warm` on the development and heldout **model phases** to qualify the
separate warm-cache regime. It fills real dense-vector cache entries in batches
bounded by the configured candidate width and unchanged embedding deadlines,
then runs the unchanged observer. `warmup_ms` and `warmup_error` remain per-case
receipts. Keep cold and warm outputs and threshold files separate. This does not
optimize the cold production path or allow a larger decision budget. Warm
classifier quality must not be presented as cold admission reliability.
