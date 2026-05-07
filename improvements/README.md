# Improvements

This folder keeps the current benchmark and evaluation artifacts that scripts write by default.

It is an operational surface, not a research dump.

Keep in root:
1. active `latest` reports
2. active `baseline` reports
3. reports used directly by scripts and docs

Current root reports are intentionally small and operational.

Rules:
1. root files should be either `*-baseline.*` or `*-latest.*`
2. root files may also include a very small number of actively referenced closure reports
3. provider-specific experiments, cache-check runs, one-off competitive captures, old waves, empirical captures, and superseded notes are removed from the active tree
4. scripts should default to root paths only when the artifact is part of the active working surface
