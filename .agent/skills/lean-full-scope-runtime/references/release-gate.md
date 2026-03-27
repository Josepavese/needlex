# Release Gate (Day 1 Complete)

A release is valid only if all checks pass.

## Completeness

1. All strategic features are present (no deferred placeholders).
2. CLI and MCP provide equivalent core operations.
3. Replay, diff, and proof endpoints are operational.

## Reliability

1. Determinism suite passes on golden inputs.
2. Extraction golden tests pass.
3. Error model is stable and schema-valid.

## Budget and Complexity

1. `bash scripts/check_budget.sh .` returns PASS.
2. New dependencies are justified and minimal.
3. Largest production file remains within limit.

## Quality of Intelligence Use

1. SLM calls occur only on configured triggers.
2. Escalation reasons are logged and inspectable.
3. Token/cost increase has measurable fidelity benefit.

## Merge Decision

Approve only if every section is PASS.
