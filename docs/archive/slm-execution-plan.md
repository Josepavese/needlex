# SLM Execution Plan

This document is retained only as historical architecture context.

The active runtime no longer follows the broader multi-task SLM expansion described in the original macrostep-6 plan.

Current reality:
1. only `resolve_ambiguity` remains active
2. the CPU baseline is `Gemma 3 1B`
3. semantic context alignment is the primary meaning-sensitive control layer
4. specialist multi-task SLM expansion is out of the active core

If this area is reopened in the future, it must start from current SSOT and benchmark evidence, not from the earlier speculative task list.

Active references:
1. `docs/vademecum.md`
2. `docs/project-context.md`
3. `docs/model-baseline.md`
4. `docs/semantic-alignment-gate.md`
