# Intelligence Failure Classes

This map defines the blocking failure classes for measured-intelligence acceptance.

These classes are consumed by the hard-case matrix acceptance gate and tied to active runtime rollout decisions.

## Classes

1. `FC01_PASS_RATE`
   - Meaning: global hard-case pass rate is below acceptance threshold.
2. `FC02_LANE_LIFT`
   - Meaning: higher lanes are not producing measurable lift versus baseline.
3. `FC03_OBJECTIVE_REGRESSION`
   - Meaning: average objective/context lift is below required threshold.
4. `FC04_LOSSINESS_RISK`
   - Meaning: medium/high lossiness risk exceeds the configured cap.
5. `FC05_COVERAGE_GAP`
   - Meaning: one or more required benchmark families are missing.
6. `FC06_BACKEND_COVERAGE`
   - Meaning: backend coverage or intervention rate is below the required floor.
7. `FC07_BACKEND_ACCEPTANCE`
   - Meaning: accepted backend interventions are below the required quality floor.
8. `FC08_BACKEND_TASK_COVERAGE`
   - Meaning: required benchmark-proven backend task coverage is missing.
9. `FC00_UNCLASSIFIED`
   - Meaning: a failure message did not match any configured class.

## Policy

1. If any blocking class is present in acceptance output, active model rollout is gated.
2. The failure-class map must be updated in the benchmark corpus before introducing new acceptance failure messages.
3. Changes to thresholds or class mapping require updating both:
   - `benchmarks/corpora/hard-case-corpus-v2.json`
   - this document
