# System One decision models in Needle-X

Assessment and implementation: 2026-10-28 to 2026-10-29. Baseline: `63eb2aa` / v0.1.40. Candidate: v0.1.41.

## Decision

Introduce a small local typed decision provider beside dense embeddings, initially as an observer of objective coverage and ambiguous selections. Keep Chromium as the JavaScript/application-data executor. Prioritize deterministic correctness repairs before allowing a model to skip work or discard evidence. Most current decisions use code or embeddings, and the default model-task runtime is noop: a decision provider adds cost unless it saves an expensive operation, improves evidence coverage or prevents downstream agent retries.

The initial research reproduced three contract-level defects with Go overlays. The implementation below fixes them and adds a real local Julia observer. The observer is **not qualified for active decisions**: the measured domain accuracy is too low. It does not select sources, discard content, replace dense embeddings or certify factual truth.

## What the research establishes

The relevant family is Jev/System One, distinct from JEPA. These models judge a supplied state against supplied alternatives rather than producing an explanatory answer. Choice returns an option distribution; Noul judges a Boolean condition; Score distributes probability over a rubric. The code still owns actions, arithmetic, permissions, deadlines and verification. TypeSafe's official documentation lists Jev 1.14 as the newest release from October 14. The official product is documented as an API; the sources consulted do not provide a local release of its weights.

Primary sources: [TypeSafe introduction](https://docs.typesafe.ai/introduction.md), [primitives](https://docs.typesafe.ai/primitives.md), [models](https://docs.typesafe.ai/models.md), [known limitations of 1.13](https://docs.typesafe.ai/model-jaggedness/jev-1.13.md).

Three different quantities must remain distinct: embedding similarity, probability assigned to an answer, and confidence derived from the answer distribution. TypeSafe's Choice confidence is normalized by the number of alternatives. With two alternatives, confidence 0.8 corresponds to maximum option probability 0.9; neither number alone establishes accuracy on Needle-X pages. Noul does not carry the same confidence field. Source: [official confidence contract](https://docs.typesafe.ai/confidence.md).

An independent preprint evaluates Jev 1.13 on 37 datasets and 346,009 requests. Choice calibration is encouraging, while Boolean decisions can require task-specific thresholds; the paper reports a substantial improvement when tuning thresholds instead of universally using 0.5. Prompt templates, domain shift and unknown pretraining contamination limit transfer of leaderboard results. Its measurements are not an evaluation of Jev 1.14 or of Needle-X. Source: [Deußer et al., full paper](https://arxiv.org/html/2609.37647v1).

## Local candidates

| Candidate | Relevant properties | Initial role | Important limit |
| --- | --- | --- | --- |
| Julia 1 | 144.3M, multilingual, Apache-2.0; native finite-choice interface | First CPU candidate for short coverage/routing questions | 2–20 options; 8,192 combined tokens supported, but historical accuracy was measured at 1,024 and 8k task accuracy is not established |
| Laya multilingual | Encoder-based, open weights, CPU/GPU and multiple runtime paths | Candidate for a Needlex-specific classifier after calibration | Default short context; base and task-tuned checkpoints differ greatly; raw probabilities can be overconfident |
| Kev 0.8B / 4B | Open decision models with pointer heads and a System One-compatible server | Quality/size comparison when questions become harder | Larger resident memory; small and large variants have different transfer accuracy |
| AnyJev / Tacit | Decision readout over open LLMs, with debiasing/calibration or trained models | Useful when a compatible LLM is already resident | Repeated option rotations and an additional LLM can erase latency/RAM savings |
| Official Jev | Hosted reference implementation | Optional external reference on public evaluation data | Not a local/offline default; no silent cloud fallback |

Sources: [Julia model card](https://huggingface.co/SupersonicLabs/Julia-1), [Laya model card](https://huggingface.co/convaiinnovations/laya), [Kev implementation and model cards](https://github.com/jaredpalmer/kev), [AnyJev implementation](https://github.com/nokia-applied-research/AnyJev).

Julia's supplied Jev comparisons are references, not a simultaneous independent Jev run. The Banking77 pilot uses a shortlist; it does not establish native high-cardinality performance. Laya's own card warns that its base checkpoints are near chance on one typed-decision benchmark and that the much higher task-tuned result uses that benchmark's training split. It also documents label bias, ineffective action-probability signals and a need for held-out temperature fitting. These are reasons to compare exact checkpoints, prompts and runtimes, not to select from a headline accuracy number.

The maintained llama.cpp server now exposes `/v1/systemone`, including Julia, Laya and Kev support. Its [maintainer announcement](https://huggingface.co/blog/ggml-org/decision-models-in-llamacpp) links the [merged implementation PR](https://github.com/ggml-org/llama.cpp/pull/29818). This permits a Go HTTP adapter without embedding Python or a tensor stack into every MCP process. The announcement's 3 ms Julia figure is measured on an RTX PRO 6000, not on this workstation's CPU. Backend parity still needs verification for the selected checkpoint and quantization. The server documentation also says a single-loaded-model server ignores the request model field: verify the effective loaded model, not merely the name sent by the client.

Artifact metadata checked without downloading weights: Julia Q8_0 is 168,166,496 bytes; the root English Laya Q8_0 is 449,397,600 bytes. These are file sizes, not resident RAM. Sources: [Julia GGUF files](https://huggingface.co/ggml-org/Julia-1-GGUF/tree/main), [Laya GGUF files](https://huggingface.co/ggml-org/Laya-GGUF/tree/main).

The workstation currently has an i7-1165G7, 8 logical CPUs, about 19 GiB total RAM, around 4.8 GiB available at observation, and nearly full swap. Prefer a single small resident model for the first experiment. Measure actual process-tree PSS, CPU contention, cold loading and long-input cost. An automatically switching multi-checkpoint router can add memory or expensive reloads; a decision model need not reduce total peak RAM even when it saves browser calls.

A promising later direction is a small model specialized on Needlex's own decisions. The recent [Narrow Decision Models preprint](https://arxiv.org/html/2610.25553v1) studies question isolation, option-order invariance, calibration and abstention. Its validation is preliminary, and agreement with another classifier is not independent ground-truth accuracy. It motivates an experiment, not an immediate production dependency.

## Three reproduced correctness defects

Temporary tests were injected through `go test -overlay` into the original packages. They intentionally failed on the current baseline. The existing test suite was not edited or replaced; these are contrastive probes, not a claim that the released suite is red.

1. **Meaningful short or non-whitespace-language objectives bypass semantic coverage.** `internal/core/sourceresolution/render_policy.go:106` requires at least 16 runes and three whitespace-separated fields. `Prezzi` and `请查找这个产品在意大利的保修条件及退货政策` are rejected before embeddings are consulted. Structural rendering can still run; the specific defect is exclusion from objective-based escalation. Replace the linguistic length test with explicit origin/availability of the objective, so internal crawl placeholders are distinguished structurally.
2. **Valid zero/negative similarity is treated as no coverage gap.** In the same file, `best` starts at zero and `best <= 0` returns false. A stub implementing the actual semantic scoring contract returned 0 and -0.25; both incorrectly reported no gap. The shared cosine implementation explicitly preserves negative similarities. Separate absent/unavailable scores from legitimate low scores, and preserve the failure/unknown reason instead of presenting it as coverage.
3. **An ordinary SSE payload closes the transport.** `internal/rendering/network_event_source.go:42` recognizes the exact payload `end` as a close marker. A valid EventSource response followed by that payload, without a transport-close event, sets `Finished=true` and removes the stream from the active set. A later delayed event can consequently be missed. Existing fixtures use this application convention, but it is not a generic SSE close signal. Track transport/subscription lifetime explicitly; a decision model must not decide closure from message wording.

Probe results: objective eligibility failed for both examples; coverage failed for both valid scores; SSE state became `active=false, finished=true` without a close event. These are unit/contract observations, not a measured count of affected public sites. Native delayed-stream and complete CLI/MCP/proof regressions are necessary when implementing the repairs.

## Integration into existing ownership

The current `intel.ModelRuntime` is a task/JSON-output interface with noop and OpenAI-compatible implementations. A System One endpoint is not a chat-completions endpoint. Add one narrow typed decision contract in `internal/intel`, one HTTP adapter, and composition in the existing service root. Keep `SemanticAligner` and the durable embedding vector-space identity intact. The existing embedding model is not automatically a compatible causal decision backend for AnyJev.

Proposed flow:

`acquire -> reduce/dense semantic selection -> typed coverage decision -> deterministic policy -> optional Chromium -> segment/pack -> coverage audit -> proof/trace`

The decision provider returns permitted option IDs and probability distributions, model/revision, token usage/admission, truncation, latency and abstention/error status. It does not return replacement source text or execute a tool. Decisions reference existing document/chunk IDs; validators reject foreign IDs or invalid probability vectors.

The first questions should be narrow:

- Does the supplied evidence support the caller's exact objective, partially support it, or leave it unknown?
- Which supplied evidence bundle directly supports this assertion, including its negation, time scope and conjunctions?
- Does a low-cost next step have likely value: static result, browser escalation, or a request for another verified source?

Count captured resources, close streams, enforce strict render modes and handle numeric budgets in deterministic code. Keep `render=required` unconditional; preserve `render=off`; keep non-HTML auto-mode rules. Unknown judgments or unavailable decision services must remain visible and follow a conservative policy. Dense embeddings remain mandatory; an optional decision provider is not a lexical fallback.

Use one externally owned, shared local inference server with a borrowed HTTP client rather than one model process per MCP request. Configuration, templates, semantic thresholds and model pins have one owner. Reserve deadline for actual capture before optional inference, respect cancellation and admission, and keep model data in the existing managed cache/runtime boundary. No automatic cloud fallback.

Cache decisions by evidence hash, exact question/option layout, model revision, tokenizer/quantization and calibration artifact. A confidence threshold cannot be copied across models or question cardinalities. Keep code-derived evidence confidence separate from calibrated decision probability: `segmentConfidence` currently adds fixed heuristic increments and is not a measured correctness probability.

## User-visible improvements to qualify

- Compact output states whether it supports the requested objective, which evidence IDs support it, and what remains unknown.
- Escalations are purposeful and explainable, with exact model/policy reasons retained in trace rather than a generic confidence label.
- Better evidence packing preserves the relevant conditions, dates, units and relationships instead of merely retaining semantically related paragraphs.
- Low-confidence or out-of-domain cases abstain; source text, raw captures and proofs remain inspectable.

Start with observation-only decisions. In particular, do not immediately let a new classifier skip browser work. It first has to demonstrate low omission risk against independent rendered/application-data evidence. Remove superseded policy paths only after an accepted replacement exists; maintain one current contract and legitimate deterministic safeguards.

## Acceptance experiment

Compare the current pipeline, a simple calibrated semantic-feature baseline, and the selected local decision model on the same budgets and evidence. Split by site/document family, not random near-duplicate chunks. Include Italian/English, languages without spaces, cross-language objectives, contradictory or irrelevant evidence, long input, changed option orders, malformed provider output and provider outages.

Measure actual objective coverage and false omission of necessary facts, along with p50/p95 end-to-end latency, browser escalations, request count, peak/idle process-tree PSS, CPU and cold-load time. Verify actual quotations/fields, app-data payloads and `web_proof` references. Do not substitute classification accuracy or zero generated tokens for retrieval completeness.

Test nominal context limits against actual task accuracy; reject or explicitly report input truncation. Calibrate on held-out in-domain examples and freeze the question specification, checkpoint and quantization. Model answers must not become their own ground-truth labels.

Recommended delivery order: repair the three deterministic defects; add a typed local provider and trace-only coverage evaluation with Julia as the first lightweight candidate; compare Laya multilingual and Kev only where the measured failure cases justify them; activate decisions only after content/latency/resource acceptance. The measurements below supersede the original proposal; they establish content recovery and advisory cost, not an inference-driven throughput or RAM gain.


## Implemented boundaries

- Every nonempty caller objective can reach the dense semantic render policy,
  including a single Italian word or Chinese without whitespace. Crawl passes
  an absent objective rather than an invented `crawl` query.
- A valid zero or negative cosine is a coverage gap. Missing, foreign-ID and
  nonfinite scores remain distinct from zero. The signed score and calibrated
  threshold are recorded with the render reason.
- SSE data such as `end` never closes a transport. Later messages remain
  eligible for capture. An observed transport failure retires that request;
  an open subscription cut at idle/deadline remains reported as truncated.
- `intel.DecisionObserver` is a typed public contract with a local HTTP adapter,
  assembled at the existing service composition root. One externally owned
  llama.cpp process serves all MCP operations; it is not launched per request.
- The adapter verifies the effective model ID, question/option IDs, finite
  normalized distributions and winning choice. It refuses redirects and uses
  the local runtime's tokenizer to reject head/context overflow before inference.
  Input is never silently shortened to make the audit succeed.
- Observations happen after the public read or the query's final source
  selection. Internal query reads and crawl traversal cannot spend their
  operational budgets on advisory inference. A failed audit returns diagnostics
  while preserving content, sources and proofs; regression tests compare them.
- `agent_context.decision_observation` exposes status, pinned model/revision,
  specification, evidence IDs, distributions, calibration status and elapsed
  time. The same typed payload is saved in the existing trace. Core confidence
  and cosine values retain their previous meaning.

## Local deployment

The local runtime uses Julia Q8 and llama.cpp b11968, listening only on
`127.0.0.1:11941`. The owned user service is `needlex-decision.service`, with one
slot, two inference threads, CPU quota 100%, MemoryHigh 512 MiB, MemoryMax 768 MiB
and TasksMax 32. Its model and runtime live under the PAL state root's
`decision/` directory. Chromium retains its existing isolated contexts and
resource policy. The optional observer is disabled in default installations.

The canonical Needle-X JSON config accepts:

```json
"decision": {
  "mode": "observe",
  "base_url": "http://127.0.0.1:11941",
  "model": "Julia-1-Q8_0",
  "revision": "ggml-org/Julia-1-GGUF@1e6b48a6983d7467f8eedc2cb0d21267b5fc052d;sha256:1ea6a7e87156eeeda88cb7a36a61265b37ba7b993897b7289b99aea5b5e47069;llama.cpp:b11968;Q8_0",
  "timeout_ms": 2000,
  "max_input_bytes": 2000,
  "max_question_tokens": 256
}
```

`max_question_tokens` matches this pinned GGUF's trained head budget, verified
from model metadata. It is not a general knob to enlarge model capacity. The
runtime context is 2,048 tokens; the adapter also checks that actual limit.
The initial 6 KB / 5 second experiment produced long waits and a timeout, so the
local observation configuration now bounds caller input to 2 KB and time to
2 seconds. A large evidence packet can be served completely while its optional
audit reports `input_budget_exceeded`. Dense embeddings remain mandatory.
Changing the model, quantization, questions or budgets requires new evaluation.
The revision field declares the deployment pin; the adapter verifies the loaded
model ID, while artifact SHA-256 was verified during local installation.

Older running binaries reject the new JSON field. Upgrade/restart MCP clients
before enabling it; do not keep a second compatibility configuration.
## Measured capability and cost

The [retained evidence](evidence/decision-observer-2026-10-28.json) contains
per-operation results, raw advisory distributions, configuration and rejected
experiments. Each phase uses the same four controlled pages, profile and budgets
with three repetitions. Eight expected facts per round cover short/multilingual
semantic escalation, JavaScript materialization, an SSE payload arriving three
seconds after ordinary data `end`, and a static control. The baseline delivers
**3/24 facts**; fixes and fixes plus observation both deliver **24/24**. These
are known fixture facts, not a claim of universal page completeness.

The principal comparison explicitly warms the shared embedding service before
**each** phase, verifies a dense 768-dimensional vector, and records warmup time
separately (99-119 ms). A cold run is retained too: its first 1.2-second embedding
timeout and two-minute negative cache prevented semantic JS escalation. The
existing static fallback then omitted requested JS facts. This is a remaining
cold/dependency-availability limitation, not evidence of successful retrieval;
Julia does not fix it. No runtime timeout or content gate was relaxed for the
warm comparison.

Three-run median CLI latency and sampled aggregate peak PSS:

| Case | Before ms | Fixes ms | Fixes + observation ms | Before / fixes / observation MiB |
| --- | ---: | ---: | ---: | --- |
| Short Italian objective | 437 | 2,233 | 2,762 | 9 / 237 / 360 |
| Chinese objective | 338 | 2,001 | 2,701 | 9 / 237 / 360 |
| Late SSE payload | 2,526 | 9,458 | 10,209 | 226 / 233 / 356 |
| Static control | 339 | 59 | 1,407 | 9 / 17 / 140 |

The model process adds roughly 123 MiB sampled PSS in this final configuration.
The successful audit median is about 0.7 seconds; larger eligible inputs cost
more. Eleven of twelve controlled requests produce an audit and one exceeds the
input budget. All twelve still deliver their expected facts. An idle/open SSE
subscription is reported as truncated even when its known fixture fact arrived.

The shared embedding service is excluded consistently from RAM. Process-tree
PSS includes Chromium children from every task. Observation PSS also includes
the separately owned model process. Sampling is 5 ms; missing samples are null,
not zero. Latency is wall time through CLI JSON output, not model-only time.
The host has concurrent workloads, so three-run medians are indicative rather
than SLOs. The before/fixed latency difference pays for work the baseline omitted.

The separate manually labelled corpus has 14 unique examples and 42 unique
question judgements, across six languages plus cross-language cases, missing
facts, partial coverage and conflicting evidence. Three repeated runs give
**54/126 correct judgements**, including 33 abstentions: relevance 30/42,
coverage 3/42, source support 21/42. There are no inference transport errors on
that corpus; the empty-evidence control reports `insufficient_input`. Repetitions
are not independent examples. Forty-five wrong judgements still assign more
than 90% probability to their selected option: probabilities must not be treated
as calibrated correctness.

An official native FP32 comparison, using strict encoding and the full head,
returns 13/42 correct judgements on the same unique examples. Its sampled PSS
is approximately 1,109 MiB. It does not resolve the observed domain failures;
all numerical/runtime differences have not been isolated. That temporary model,
venv and process were retired after the comparison, preserving shared Python
packages and caches. The selected Q8 runtime is substantially smaller; neither
runtime qualifies this question specification for active use.

The improvement in tool ability is therefore concrete retrieval completeness
and inspectable local decision experiments. Julia adds an evaluation surface
and resource cost; it has not demonstrated a reliable semantic judge for this
workflow. Active render skipping, evidence rejection, similarity replacement or
truth confirmation remain disabled. Any future activation needs a separately
labelled domain evaluation and calibration, not a threshold on these raw scores.

## Reproduction

Run the paired resource/content harness with baseline and candidate binaries
built outside the repository, the same private config and managed browser:

```bash
python3 benchmarks/decision_observer/measure.py \
  --before /tmp/needlex-before --after /tmp/needlex-after \
  --config /tmp/read-config.json --decision /tmp/decision-config.json \
  --model-pid MODEL_PID --rounds 3 --warm-embedding --out /tmp/decision-measure.json
```

The harness owns and removes its temporary state, serves public fixture data on
loopback and writes only the requested result artifact. `--phases` can reuse a
still-valid baseline when only observation parameters changed. Its Linux PSS
sampler is shared with the existing Chromium-reuse harness. Omit
`--warm-embedding` to include the dependency cold-start path; do not mix that
condition with the warm comparison.

Build `./benchmarks/decision_observer/runner` outside the repository and run it
with `--config`, `--cases benchmarks/corpora/decision-observer-v1.json` and
`--repeats 3` to reproduce the advisory evaluation. The labels are human-defined,
not derived from model output. The corpus is small and synthetic; it does not
establish open-web accuracy or factual truth.


## Installed-candidate path proof

The v0.1.41 candidate was packaged for Linux, macOS and Windows on amd64/arm64.
All six archives contain only the platform binary. The normal Unix installer
was exercised in owned private PAL state using the existing browser and embedding
service. Real MCP `web_read` and strict seeded `web_query` recover all three
controlled JS facts, call Julia once after selection, and retain its full audit
in the saved trace. The second render reuses Chromium with its independent
context. The full governance gate passes with 35,370 runtime lines, unchanged
five Go runtime dependencies and zero structural issues. The service package
remains at its unchanged 6,700-line ceiling after consolidating audit ownership.

A separate Chromium DOM oracle for `https://quotes.toscrape.com/js/` confirms ten
quotes and ten author occurrences in the MCP output. Three proof records link
all returned chunks to valid WebIR paths and UTF-8 spans; `web_replay` reports a
deterministic six-stage trace. The compact packet identifies rendered DOM.
That page's optional Julia audit exceeds the two-second budget and explicitly
reports `unavailable`; it does not remove quotes, authors or proofs. The content
proof and audit reliability are separate results.

## Expanded corpus qualification, 2026-10-29

The broader experiment **does not qualify the current Julia integration for
active decisions**. On 208 short controlled inputs it returns 231/624 correct
judgements (37.0%), below a constant-most-frequent-label reference of 364/624
(58.3%) on this set. In real page reads it produces an audit for only 9/176 URLs.
Making an input fit the model does not by itself make this question specification
reliable. These measurements concern the pinned integration, not every possible
Julia prompt, adaptation or deployment.

No 1,000-site corpus was found in this checkout or its existing benchmark
reports. The [seeded specification](seeded-benchmark-spec.md) describes
1,000 cases as a later expansion gate. `unique-sources-corpus-v1.json` contains
100 entries but 99 distinct URLs/hosts, including a duplicate pandas entry.
Combined with the 100 seeded tasks, there are **176 distinct expected URLs on
109 hosts**, preserving all 200 task origins. This experiment reads those exact
URLs; it does not rerun the original same-site routing or seedless tasks. Old
benchmark pass rates cannot be treated as current content-quality results.

### Real installed MCP results

All selected URLs were read serially through the installed v0.1.41 MCP runtime,
profile `deep`, `render=auto`, with the existing 8-second runtime, 30-second render
and 2-second/2-KB observer budgets. No production policy, model, prompt, threshold
or shared service was changed. Temporary browser state was retired per batch;
the installer wrapper selects canonical PAL state and useful traces/proofs
remain there. Completed per-URL receipts were reused after an HTTP failure;
failures were retained without repeated requests.

| Measurement | Result |
| --- | ---: |
| Distinct URLs / hosts | 176 / 109 |
| Returned nonempty packets | 173 |
| Retrieval errors | 1 HTTP 503, 1 HTTP 403, 1 DNS failure |
| Julia observed | 9 |
| Julia input budget exceeded | 134 |
| Julia inference timeout | 30 |
| Persisted typed audit matches / nonempty proof files | 173 / 173 |
| Browser renders / reused browser | 60 / 45 |
| Reported degraded / truncated captures | 0 / 0 |

Packet availability, a parsed proof file and a clean capture diagnostic are not
content-completeness passes. Every successful read's typed audit, including
skips and timeouts, matches the audit saved in its trace. Model coverage is 5.1%
of selected URLs, or 5.2% of returned packets. All 30 unavailable audits reached
the existing two-second inference limit.

Wall latency p50/p95 is **3,986/9,756 ms**. Sampled aggregate process-tree PSS
p50/p95/maximum is approximately **382/543/1,535 MiB**, including Chromium children
and the shared decision process. Model PSS p50 is approximately **151 MiB**.
Sampling is 50 ms; the common embedding service and Python driver are excluded.
One recovered HTTP-503 receipt has unknown latency/RAM and remains null, leaving
175 measured rows. These are current one-pass workload measurements, not a
paired performance comparison with v0.1.40 or the earlier four-page fixture.

### Source-scoped decision accuracy

The frozen [anchor corpus](../benchmarks/corpora/decision-live-anchors-v1.json)
contains 52 exact-verified excerpts from 52 hosts, with source title, URL, trace
and content hash. Questions use English, Italian, Spanish, German, French and
Chinese, including cross-language pairs. Each excerpt produces four controls:
an explicitly answered question, a question absent from that excerpt, their
partial conjunction, and unrelated synthetic evidence. There are **208 distinct
inputs and 624 question judgements**, not 624 independent sources. The 52
synthetic-unrelated controls deliberately reuse the same irrelevant statement.

Labels were independently authored by the agent from the supplied source
excerpt and were frozen before inference. They have not received human review;
Julia's answers were never used as labels. Absence refers to the selected
excerpt, not the whole page. Excerpts were selected from returned packets, so
this classifier experiment cannot detect everything retrieval omitted. It is
a convenience sample dominated by software documentation, not a representative
random sample of the web or a whole-page retrieval gold set.

The existing typed Go `DecisionObserver` runner evaluated each input once after
live collection ended. All 208 completed within the unchanged two-second budget;
there were no transport errors, input skips or prompt overflows. Wall p50/p95
was **1,129.5/1,314 ms**, maximum 1,699 ms. The installed executable is unchanged;
the benchmark driver was rebuilt from the same core commit with Go 1.27.3, while
the installed release used Go 1.27.1. Source and executable hashes are retained.

| Judge | Correct | Accuracy | Constant-label reference |
| --- | ---: | ---: | ---: |
| Relevance | 146/208 | 70.2% | 75.0% |
| Coverage | 29/208 | 13.9% | 50.0% |
| Source support | 56/208 | 26.9% | 50.0% |
| Combined | 231/624 | 37.0% | 58.3% |

There are 165 abstentions and 161 wrong judgements with selected-option
probability above 90%. Only 9/208 cases get all three judgements correct.
The constant reference simply selects the most frequent gold label for each
question type on this set; it is a class-prior sanity check, not a trained
retrieval competitor. Repeated variants are correlated, and these results do
not constitute independent calibration or prove a general accuracy difference
from the older 14-example corpus. No questions were tuned against the results.

### Independent content-loss checks

The large live run also exposes content omissions independent of Julia. A
separate ordinary HTTP/HTML-text oracle finds Python's programming-language
description and Vue's JavaScript/UI-framework description in their source
pages, while both complete returned packet texts omit them. Retrying the same
objectives with `render=required` and the observer explicitly disabled still
omits those facts. Both renders report CDP, no missing network body, no open
streams and no truncation/degradation. Thus clean transport diagnostics do not
prove that the useful page text reached the final packet.

| Case | Original read trace | Required-render trace |
| --- | --- | --- |
| Python homepage | `trace_1b5661d76d1469a8` | `trace_117a4850ee0c0744` |
| Vue introduction | `trace_cb022103f202c59f` | `trace_47b03468a4c8b7fd` |

This targeted oracle checked six pages selected for suspicious outputs. Four
markers were present upstream and absent from packets; the raw Flask oracle
was blocked with HTTP 403, and the proposed React marker was not found upstream,
so neither is counted as a confirmed omission. Python and Vue are the two
stronger cases reproduced with required rendering. This is failure evidence,
not an omission-rate estimate for the 176 pages, nor yet a diagnosis of which
capture/reduction/selection stage lost the information. The oracle stores source
hashes and short supporting excerpts; it does not infer truth from Julia.

### Evidence and reproduction

The local current artifacts are `improvements/decision-live-latest.jsonl`,
`decision-quality-latest.jsonl`, `decision-eval-latest.json` and
`decision-content-oracle-latest.json`. The summary retains distributions,
confusion matrices, input/model pins, sampling scope, artifact hashes and failed
oracle checks. These benchmark artifacts are intentionally local. The compact
anchor corpus preserves enough title/excerpt/provenance to reproduce the
classifier offline after temporary builds are retired; see the
[benchmark instructions](../benchmarks/README.md#local-decision-qualification-on-public-pages).

The supported conclusion is narrower than "answers improved across the web":
the previous deterministic fixes remain verified on their regressions, while
this broader qualification rejects active use of the current observer and
identifies remaining content-delivery defects. Repairing those omissions and
then evaluating a revised decision specification on a separate labelled set
are the next useful engineering steps. Increasing the observation budget alone
does not address the measured judgement errors. Julia stays advisory.
