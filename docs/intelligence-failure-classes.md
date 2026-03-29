# Intelligence Failure Classes

This map defines the blocking failure classes for measured-intelligence acceptance.

These classes are consumed by the hard-case matrix acceptance gate and are intentionally tied to future real-SLM rollout decisions.

## Classes

1. `FC01_PASS_RATE`
   - Meaning: global hard-case pass rate is below acceptance threshold.
   - Integration gate: `block_slm_rollout`.

2. `FC02_LANE_LIFT`
   - Meaning: higher lanes are not producing measurable lift versus baseline.
   - Integration gate: `block_slm_rollout`.

3. `FC03_OBJECTIVE_REGRESSION`
   - Meaning: average objective-focus lift is below required threshold.
   - Integration gate: `block_slm_rollout`.

4. `FC04_LOSSINESS_RISK`
   - Meaning: medium/high lossiness risk exceeds the configured cap.
   - Integration gate: `block_slm_rollout`.

5. `FC05_COVERAGE_GAP`
   - Meaning: one or more required benchmark families are missing.
   - Integration gate: `block_slm_rollout`.

6. `FC00_UNCLASSIFIED`
   - Meaning: a failure message did not match any configured class.
   - Integration gate: `block_slm_rollout` when `allow_unclassified=false`.

## Policy

1. If any blocking class is present in acceptance output, real-SLM integration work is considered gated.
2. The failure-class map must be updated in the benchmark corpus before introducing new acceptance failure messages.
3. Changes to thresholds or class mapping require updating both:
   - `testdata/benchmark/hard-case-corpus-v2.json`
   - this document
