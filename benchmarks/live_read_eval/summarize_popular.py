#!/usr/bin/env python3
"""Availability, capture, proof mapping and source facts are separate scores."""
import argparse
from collections import Counter
import hashlib
import json
from pathlib import Path
import sys

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "decision_observer"))
from summarize import distribution, live_summary


def read_rows(path):
    return [json.loads(line) for line in path.read_text().splitlines() if line]


def summarize(rows):
    base = live_summary(rows)
    packets = [r for r in rows if not r.get("tool_error")]
    nonempty = [r for r in packets if r.get("chunk_count", 0) > 0]
    proof = [r for r in nonempty if r.get("proof_mapping_valid") and r.get("proof_file_nonempty") and r.get("proof_refs")]
    render = [s for r in rows for s in r.get("render", [])]
    base.update(nonempty_with_mapped_proof=len(proof),
                complete_capture_with_mapped_proof=sum(r.get("capture_complete") is True for r in proof),
                capture_unknown=sum(r.get("capture_complete") is None for r in packets),
                capture_partial_or_degraded=sum(r.get("capture_complete") is False for r in packets),
                empty_packets=len(packets)-len(nonempty),
                unexpected_model_audits=sum(bool(r.get("observation") or r.get("acquisition_observation")) for r in rows),
                rendered=sum(s.get("rendered") == "true" for s in render),
                render_attempts=len(render), render_failed=sum(s.get("rendered") != "true" for s in render),
                network_body_missing=sum(int(s["network_body_missing"]) for s in render if "network_body_missing" in s),
                network_stats_unknown=sum("network_body_missing" not in s for s in render),
                browser_http_request_events=sum(int(s.get("network_http_requests", "0")) for s in render),
                tool_pss_kib=distribution(r.get("process_pss_kib") for r in rows))
    return base


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--corpus", type=Path, required=True)
    p.add_argument("--rows", type=Path, required=True)
    p.add_argument("--quality-corpus", type=Path)
    p.add_argument("--quality-rows", type=Path)
    p.add_argument("--out", type=Path, required=True)
    a = p.parse_args()
    corpus = json.loads(a.corpus.read_text())
    rows = read_rows(a.rows)
    expected = {c["expected_url"] for c in corpus["cases"]}
    if len({r["url"] for r in rows}) != len(rows) or {r["url"] for r in rows} != expected:
        raise ValueError("missing, repeated or foreign URL receipts; partial run cannot be published as complete")
    if len({r["run_identity"] for r in rows}) != 1 or len({r["binary_sha256"] for r in rows}) != 1:
        raise ValueError("mixed measurement identities")
    result = dict(specification=corpus["specification"], corpus_sha256=hashlib.sha256(a.corpus.read_bytes()).hexdigest(),
                  receipts_sha256=hashlib.sha256(a.rows.read_bytes()).hexdigest(), installed_binary_sha256=rows[0]["binary_sha256"],
                  execution=corpus["execution"], availability=summarize(rows),
                  by_cohort={v:summarize([r for r in rows if r.get("cohort")==v]) for v in sorted({r.get("cohort", "unspecified") for r in rows})},
                  by_band={v:summarize([r for r in rows if r.get("popularity_band", "historical")==v]) for v in sorted({r.get("popularity_band", "historical") for r in rows})},
                  question_languages=dict(Counter(r["language"] for r in rows)),
                  defects=[dict(id=r["id"],url=r["url"],error=r["tool_error"])for r in rows if r.get("tool_error") and "trace_event.data[title] must not be empty" in json.dumps(r["tool_error"])],
                  scope="A stratified public-origin retrieval baseline, not a traffic-weighted web accuracy estimate. One live pass, native budgets unchanged. Context/proof mapping validity does not establish correctness. Resident external embedding server excluded from PSS; resident Julia separately included though inference is not requested. Shared-host load and cache state are not controlled laboratory conditions.")
    old = [r for r in rows if r.get("cohort") == "historical"]
    result["historical_occurrences"] = dict(original=100, distinct_urls=len(old), nonempty_with_proof=sum(len(r["origins"]) for r in old if r.get("chunk_count",0)>0 and r.get("proof_mapping_valid")))
    if a.quality_corpus and a.quality_rows:
        gold = json.loads(a.quality_corpus.read_text())["cases"]
        quality = read_rows(a.quality_rows)
        if {r["url"]for r in quality}!={c["expected_url"]for c in gold} or len(quality)!=len(gold):
            raise ValueError("quality tasks incomplete")
        checks = []
        by_url = {r["url"]:r for r in quality}
        for case in gold:
            row = by_url[case["expected_url"]]
            retained = row.get("coverage", {})
            hits = sum(retained.get(fact, False) for fact in case["must_contain_facts"])
            checks.append(dict(id=case["id"],url=case["expected_url"],cohort=case["cohort"],hits=hits,total=len(case["must_contain_facts"]),
                               all_markers_retained=hits==len(case["must_contain_facts"]),tool_error=row.get("tool_error"),
                               proof_mapping_valid=row.get("proof_mapping_valid"),capture_complete=row.get("capture_complete"),
                               interpretation="Source marker retention; a miss requires independent source drift/acquisition diagnosis before calling it an omission."))
        result["quality"] = dict(cases=len(checks), expected_markers=sum(c["total"]for c in checks),retained_markers=sum(c["hits"]for c in checks),all_markers_retained=sum(c["all_markers_retained"]for c in checks),
                                 by_cohort={v:dict(cases=sum(c["cohort"]==v for c in checks),all_markers_retained=sum(c["cohort"]==v and c["all_markers_retained"]for c in checks))for v in {c["cohort"]for c in checks}},checks=checks)
    a.out.write_text(json.dumps(result,indent=2,ensure_ascii=False)+"\n")
    print(json.dumps(result["availability"],indent=2))


if __name__ == "__main__":
    main()
