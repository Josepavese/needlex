# First Real SLM Test

This document is now historical context.

The first real local-model validation path for the active runtime is:
1. baseline CPU model: `Gemma 3 1B`
2. backend: `openai-compatible`
3. active task: `resolve_ambiguity`
4. semantic gate baseline: `intfloat/multilingual-e5-small`

Current commands:
```bash
./scripts/run_cpu_baseline_matrix.sh
./scripts/run_live_read_eval_baseline.sh
./scripts/run_live_semantic_global_eval.sh
```

What the first-real-test phase established:
1. the active CPU runtime works end to end
2. the compare path is real and measurable
3. Gemma is the correct current CPU baseline
4. semantic context is a stronger meaning signal than lexical overlap on a multilingual web

This document should not be used as the active rollout doctrine.
Use instead:
1. `docs/vademecum.md`
2. `docs/model-baseline.md`
3. `docs/benchmark-report.md`
