#!/usr/bin/env python3
"""Score frozen routing receipts; fit a dense threshold on development only.

Facts are an offline oracle, never runtime features. Missing captures, failed
routes and missing model/dense scores remain unknown. Latency is a counterfactual
estimate from measured routes plus observer work, not an active-policy trial.
"""
import argparse
from collections import Counter
import hashlib
import json
import math
from pathlib import Path
import statistics

SERVE, DECLARED, RENDER, INSUFFICIENT = "serve_static", "read_declared", "render", "insufficient"


def rows(path):
    return [json.loads(line) for line in path.read_text().splitlines() if line]


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def route(row, action):
    return next((r for r in row["routes"] if r["action"] == action), None)


def success(r):
    return bool(r and not r.get("error") and r["facts_found"] and r["capture_complete"] and r.get("witnesses"))


def oracle(row):
    legal = [r for r in row["routes"] if r["action"] in (row["input"].get("allowed_actions") or [])]
    good = [r for r in legal if success(r)]
    if good:
        cost = lambda r: (r["browser_uses"], r["fetch_attempts"])
        best = min(map(cost, good))
        return [r["action"] for r in good if cost(r) == best]
    # Only the controlled, known-empty fixture supports exhaustive insufficiency.
    # A changed public source or degraded capture cannot establish absence.
    if row["case"]["url"] == "fixture://unavailable" and legal and all(not r.get("error") and r["capture_complete"] and not r["facts_found"] for r in legal):
        return [INSUFFICIENT]
    return []


def dense_choice(row, threshold):
    score = row.get("reference_similarity")
    if score is None or not math.isfinite(score):
        return ""
    allowed = row["input"]["allowed_actions"]
    if score >= threshold and SERVE in allowed:
        return SERVE
    for action in [DECLARED, RENDER, INSUFFICIENT]:
        if action in allowed:
            return action
    return ""


def prediction(row, policy, threshold):
    if policy == "baseline":
        return "baseline"
    if policy == "dense":
        return dense_choice(row, threshold)
    o = row.get("observation") or {}
    return o.get("proposed_action", "") if o.get("status") == "observed" and not row.get("observation_error") else ""


def summary(data, policy, threshold):
    counts, choices, errors = Counter(), Counter(), Counter()
    by_action = {}
    latency, requests, browser_events, observer_ms = [], [], [], []
    for row in data:
        gold = oracle(row)
        choice = prediction(row, policy, threshold)
        choices[choice or "abstain"] += 1
        counts["cases"] += 1
        counts["oracle_known"] += bool(gold)
        counts["proposals"] += bool(choice)
        counts["optimal_choice"] += bool(gold and choice in gold)
        selected = route(row, choice)
        effective = selected.get("actual_action", "") if choice == "baseline" and selected else choice
        baseline_optimal = False
        if choice == "baseline" and gold != [INSUFFICIENT] and success(selected):
            alternatives = [r for r in row["routes"] if r["action"] in gold]
            baseline_optimal = bool(alternatives and (selected["browser_uses"], selected["fetch_attempts"]) <= min((r["browser_uses"], r["fetch_attempts"]) for r in alternatives))
        if gold:
            for action in gold:
                group = by_action.setdefault(action, dict(cases=0, correct=0))
                group["cases"] += 1
                group["correct"] += baseline_optimal if choice == "baseline" else effective in gold
        if choice == "baseline":
            counts["optimal_choice"] += baseline_optimal
        if not choice:
            counts["abstentions"] += 1
            continue
        counts["verified_fact_delivery"] += success(selected)
        counts["known_insufficient"] += bool(choice == INSUFFICIENT and gold == [INSUFFICIENT])
        counts["false_insufficient"] += bool(choice == INSUFFICIENT and gold and gold != [INSUFFICIENT])
        counts["static_miss_when_recoverable"] += bool(choice == SERVE and gold and SERVE not in gold)
        counts["capture_unknown_or_failed"] += bool(selected and (selected.get("error") or not selected["capture_complete"]))
        if selected and selected.get("error"):
            errors[selected["error"]] += 1
        if choice == INSUFFICIENT:
            # Initial fetch is still necessary to construct the state. Serving
            # static is its measured native path, not a fabricated zero cost.
            selected = route(row, SERVE)
        if not selected:
            counts["unmeasured_route"] += 1
            continue
        baseline = route(row, "baseline") or {}
        overhead = 0
        if policy == "julia":
            overhead = baseline.get("snapshot_ms", 0) + row.get("model_wall_ms", 0)
        if policy == "dense":
            native_ms = (row.get("observation") or {}).get("runtime_ms", 0)
            overhead = baseline.get("snapshot_ms", 0) + max(row.get("model_wall_ms", 0)-native_ms, 0) + row.get("reference_ms", 0)
        observer_ms.append(overhead)
        latency.append(selected["latency_ms"] + overhead)
        requests.append(selected["fetch_attempts"])
        counts["browser_uses"] += selected["browser_uses"]
        if selected["browser_http_events"] is not None:
            browser_events.append(selected["browser_http_events"])
    return dict(counts=dict(counts), choices=dict(choices), optimal_by_action=by_action, macro_optimal_accuracy=statistics.mean(g["correct"]/g["cases"] for g in by_action.values()) if by_action else None, transport_errors=dict(errors),
                median_estimated_total_ms=statistics.median(latency) if latency else None,
                median_observer_overhead_ms=statistics.median(observer_ms) if observer_ms else None,
                measured_native_get_attempts=sum(requests), measured_browser_http_events=sum(browser_events),
                browser_http_counter_known=len(browser_events))


def fit(data):
    scores = sorted(set(r["reference_similarity"] for r in data if r.get("reference_similarity") is not None and math.isfinite(r["reference_similarity"])))
    if not scores:
        raise ValueError("No development dense scores; threshold cannot be qualified")
    thresholds = [-1.000001] + [(a+b)/2 for a, b in zip(scores, scores[1:])] + [1.000001]
    def objective(threshold):
        s = summary(data, "dense", threshold)
        c = s["counts"]
        # Prefer factual delivery, then verified absence; only then cost/choice.
        return (-c.get("verified_fact_delivery", 0), -c.get("known_insufficient", 0),
                c.get("browser_uses", 0), s["measured_native_get_attempts"], -c.get("optimal_choice", 0), threshold)
    return min(thresholds, key=objective)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--development", required=True, type=Path)
    p.add_argument("--heldout", type=Path)
    p.add_argument("--threshold", required=True, type=Path)
    p.add_argument("--out", required=True, type=Path)
    p.add_argument("--resources", type=Path)
    a = p.parse_args()
    dev = rows(a.development)
    if a.threshold.exists():
        frozen = json.loads(a.threshold.read_text())
        if frozen["development_sha256"] != digest(a.development):
            raise ValueError("development changed after threshold freeze")
        threshold = frozen["threshold"]
    else:
        if a.heldout:
            raise ValueError("freeze development threshold before reading heldout")
        threshold = fit(dev)
        a.threshold.write_text(json.dumps(dict(threshold=threshold, development_sha256=digest(a.development)), indent=2)+"\n")
    partitions = dict(development=dev)
    if a.heldout:
        partitions["heldout"] = rows(a.heldout)
    result = dict(failure_pacing_ms=sum(r.get("failure_pacing_ms",0) for data in partitions.values() for r in data), specification="acquisition-route-v1", threshold=threshold,
                  qualification="observation_only_pending_review", partitions={})
    for name, data in partitions.items():
        result["partitions"][name] = dict(
            cases=len(data), families=len({r["case"]["family"] for r in data}),
            statuses=dict(Counter((r.get("observation") or {}).get("status", "unavailable") for r in data)),
            oracle_unknown=[r["case"]["id"] for r in data if not oracle(r)],
            baseline_indirect_source_paths=[r["case"]["id"] for r in data if (route(r, "baseline") or {}).get("actual_action") == "read_conventional"],
            policies={policy: summary(data, policy, threshold) for policy in ["baseline", "dense", "julia"]})
    if a.resources:
        samples = rows(a.resources)
        result["resource_phase_peaks"] = [r for r in samples if r["action"] == "phase_peak"]
        result["resource_scope"] = "50ms PSS samples; tool/test binary plus owned Chromium; resident Julia separately and aggregate; external embedding service excluded consistently. Shared warm browser may remain resident on static routes. No active-policy RAM claim."
    result["evidence_sha256"] = {"development": digest(a.development)}
    if a.heldout:
        result["evidence_sha256"]["heldout"] = digest(a.heldout)
    a.out.write_text(json.dumps(result, indent=2)+"\n")
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
