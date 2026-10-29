#!/usr/bin/env python3
"""Separate live packet availability, model admission and source-scoped accuracy."""
import argparse
from collections import Counter, defaultdict
import json
import math
from pathlib import Path
import statistics
from urllib.parse import urlsplit

OVERCONFIDENCE_REPORT_THRESHOLD = 0.9


def distribution(values):
    values = sorted(value for value in values if value is not None)
    if not values:
        return dict(samples=0, p50=None, p95=None, maximum=None)
    return dict(samples=len(values), p50=statistics.median(values),
                p95=values[math.ceil(len(values)*0.95)-1], maximum=max(values))


def error_class(error):
    text = json.dumps(error).lower()
    if any(code in text for code in ("403", "429")):
        return "provider_blocked"
    if any(code in text for code in ("502", "503", "no such host", "connection refused")):
        return "unavailable_upstream"
    if "deadline exceeded" in text or "timeout" in text:
        return "benchmark_timeout"
    if "unsupported" in text:
        return "unsupported_content_type"
    return "runtime_error"


def live_summary(rows):
    packets = [row for row in rows if not row.get("tool_error")]
    audit = Counter("tool_error" if row.get("tool_error") else (row.get("observation") or {}).get("status", "not_requested") for row in rows)
    render = [meta for row in packets for meta in row.get("render", [])]
    return dict(urls=len(rows), hosts=len({row["host"] for row in rows}),
                nonempty_packets=sum(bool(row.get("snapshot")) for row in packets),
                transport_errors=dict(Counter(error_class(row["tool_error"]) for row in rows if row.get("tool_error"))),
                audit_statuses=dict(audit),
                audit_trace_matches=sum(row.get("trace_audit_matches", False) for row in packets),
                proof_files_nonempty=sum(row.get("proof_file_nonempty", False) for row in packets),
                rendered=len(render), reused=sum(meta.get("browser_reused") == "true" for meta in render),
                network_truncated=sum(meta.get("network_truncated") == "true" for meta in render),
                render_degraded=sum(meta.get("render_path") != "cdp" for meta in render),
                latency_ms=distribution(row.get("elapsed_ms") for row in rows),
                aggregate_pss_kib=distribution(row.get("aggregate_pss_kib") for row in rows),
                model_pss_kib=distribution(row.get("model_pss_kib") for row in rows),
                families=dict(Counter(row["family"] for row in rows)),
                declared_languages=dict(Counter(row["language"] for row in rows)))


def quality_summary(cases, rows):
    case_map = {case["id"]: case for case in cases}
    if len(case_map) != len(cases) or len(rows) != len(cases):
        raise ValueError("quality requires exactly one receipt per distinct case")
    if {row["case"] for row in rows} != set(case_map):
        raise ValueError("quality receipts have missing or foreign cases")
    questions = defaultdict(lambda: dict(correct=0, total=0, confusion=Counter(), expected=Counter()))
    variants = defaultdict(lambda: dict(correct=0, total=0, cases=0, all_correct=0))
    wrong_confident = abstentions = observed = 0
    support = Counter()
    for row in rows:
        case = case_map[row["case"]]
        observation = row["observation"]
        is_observed = observation["status"] == "observed"
        observed += is_observed
        variant = variants[case["variant"]]
        variant["cases"] += 1
        variant["all_correct"] += row["correct"] == row["total"]
        for question, expected in case["expected"].items():
            answer = observation.get("answers", {}).get(question, {})
            selected = answer.get("choice", observation["status"])
            correct = selected == expected
            support["true_positive"] += selected == "supported" and expected == "supported"
            support["false_positive"] += selected == "supported" and expected != "supported"
            support["false_negative"] += selected != "supported" and expected == "supported"
            support["true_negative"] += selected != "supported" and expected != "supported"
            bucket = questions[question]
            bucket["total"] += 1
            bucket["correct"] += correct
            bucket["confusion"][expected+" -> "+selected] += 1
            bucket["expected"][expected] += 1
            variant["total"] += 1
            variant["correct"] += correct
            abstentions += selected == "not_established"
            wrong_confident += not correct and answer.get("probabilities", {}).get(selected, 0) > OVERCONFIDENCE_REPORT_THRESHOLD
    support["precision"] = support["true_positive"] / (support["true_positive"] + support["false_positive"]) if support["true_positive"] + support["false_positive"] else None
    support["recall"] = support["true_positive"] / (support["true_positive"] + support["false_negative"]) if support["true_positive"] + support["false_negative"] else None
    support["false_support_rate"] = support["false_positive"] / (support["false_positive"] + support["true_negative"]) if support["false_positive"] + support["true_negative"] else None
    for bucket in questions.values():
        bucket["constant_label_correct"] = max(bucket["expected"].values())
        bucket["balanced_recall"] = statistics.mean(
            bucket["confusion"][label+" -> "+label] / count
            for label, count in bucket["expected"].items())
    return dict(cases=len(cases), source_excerpts=len(cases)//len(variants),
                source_hosts=len({urlsplit(case["source"]["url"]).hostname for case in cases}),
                source_families=len({case["family"] for case in cases}),
                observed_cases=observed, statuses=dict(Counter(row["observation"]["status"] for row in rows)),
                correct=sum(bucket["correct"] for bucket in questions.values()),
                total=sum(bucket["total"] for bucket in questions.values()),
                constant_label_correct=sum(bucket["constant_label_correct"] for bucket in questions.values()),
                support_detection=dict(support),
                abstentions=abstentions, wrong_above_probability_threshold=wrong_confident,
                probability_threshold=OVERCONFIDENCE_REPORT_THRESHOLD,
                by_question=dict(questions), by_variant=dict(variants),
                wall_ms=distribution(row["wall_ms"] for row in rows),
                scope="Three correlated source-scoped assertion pairs per anchor; agent-reviewed labels, not Julia judgments or independently human-reviewed open-web truth")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--live", type=Path, required=True)
    parser.add_argument("--cases", type=Path, required=True)
    parser.add_argument("--quality", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--partition", help="score only this frozen family partition")
    args = parser.parse_args()
    live = [json.loads(line) for line in args.live.read_text().splitlines()]
    quality = [json.loads(line) for line in args.quality.read_text().splitlines()]
    cases = json.loads(args.cases.read_text())
    if args.partition:
        cases = [case for case in cases if case["partition"] == args.partition]
    result = dict(live=live_summary(live), quality=quality_summary(cases, quality))
    args.out.write_text(json.dumps(result, ensure_ascii=False, indent=2)+"\n")
    print(json.dumps(result, ensure_ascii=False))
