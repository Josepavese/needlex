# Code Governance Workflow

Use this workflow when changing the governance platform itself.

## Scope

This workflow covers:
- budget baseline recalibration
- hard vs target gate changes
- linter adoption or removal
- governance CI changes
- release gate changes tied to quality control

## Macrostep Declaration

Before starting, declare:

```text
Macrostep:
- name:
- family:
- hard_signal:
- target_signal:
- stop_condition:
```

## Principles

1. Governance must be honest.
2. Hard gates block real regressions, not aesthetic disagreement.
3. Targets apply pressure without pretending the repo is already smaller than it is.
4. Advisory checks should produce actionable fronts, not generic shame.

## Required Baseline Collection

Always collect:

```bash
bash scripts/check_budget.sh .
find internal -name '*.go' -not -name '*_test.go' -exec wc -l {} + | sort -n | tail -20
python - <<'PY'
import os, collections
pkg=collections.Counter()
for root,_,names in os.walk('internal'):
    total=0
    for n in names:
        if n.endswith('.go') and not n.endswith('_test.go'):
            with open(os.path.join(root,n)) as f:
                total += sum(1 for _ in f)
    if total:
        pkg[root]=total
for k,v in sorted(pkg.items(), key=lambda kv: kv[1], reverse=True):
    print(f"{v:5d} {k}")
PY
```

## Linter Admission Rules

A linter can enter hard gate only if:
1. it catches bugs or concentration risks we care about
2. the repo can pass it honestly in the same macrostep, or the adoption is scoped and justified
3. it does not mainly encode taste

A linter belongs in advisory only if:
1. it is valuable
2. it is still too noisy for hard gate today
3. its output can drive the next reduction burst

## Mandatory Validation

Always finish with:

```bash
bash scripts/check_governance.sh .
```

If governance CI changed, also validate the workflow syntax or run locally equivalent commands.

## Output

A valid governance macrostep should leave behind:
1. updated baseline files and scripts
2. updated docs
3. updated workflow or CI checks if applicable
4. a clear explanation of what is hard, what is target, and what is advisory
