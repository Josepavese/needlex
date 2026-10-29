#!/usr/bin/env python3
"""Freeze source-grounded decision controls; labels never come from Julia.

One source title/excerpt produces complete/absent/partial and synthetic-unrelated
controls. Those are four correlated cases, not four independent web sources.
The labels concern only the supplied excerpt, not the complete source page.
"""
import argparse
import hashlib
import json
from pathlib import Path

UNRELATED_CONTROL = "A blue ceramic bowl weighs 450 grams."
LABELS = {
    "complete": dict(relevance="related", coverage="complete", support="supported"),
    "absent": dict(relevance="related", coverage="absent", support="unsupported"),
    "partial": dict(relevance="related", coverage="partial", support="supported"),
    "unrelated": dict(relevance="unrelated", coverage="absent", support="unsupported"),
}


def materialize(anchors, receipts=None):
    sources = {row["id"]: row for row in receipts} if receipts is not None else None
    cases = []
    for anchor in anchors:
        source = anchor["source"]
        if sources is not None:
            receipt = sources[anchor["source_id"]]
            if anchor["quote"] not in receipt["snapshot"] or any(
                    source[key] != receipt[key] for key in source):
                raise ValueError(f"excerpt/provenance differs from frozen source: {anchor['source_id']}")
        for variant, objective in [("complete", anchor["question"]), ("absent", anchor["missing"]),
                                   ("partial", anchor["question"]+" "+anchor["missing"]),
                                   ("unrelated", anchor["question"])]:
            synthetic = variant == "unrelated"
            text = UNRELATED_CONTROL if synthetic else source["title"]+"\n"+anchor["quote"]
            cases.append(dict(id=anchor["source_id"]+"-"+variant,
                              input=dict(objective=objective, evidence=[dict(
                                  id="ev_"+hashlib.sha256(text.encode()).hexdigest(), text=text)]),
                              expected=LABELS[variant], source=dict(url=source["url"],
                                  trace_id=source["trace_id"], text_sha256=source["text_sha256"],
                                  language=anchor["language"], synthetic=synthetic),
                              variant=variant))
    return cases


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--anchors", required=True, type=Path)
    parser.add_argument("--receipts", type=Path, help="optional captured-source integrity verification")
    parser.add_argument("--out", required=True, type=Path)
    args = parser.parse_args()
    anchors = json.loads(args.anchors.read_text())
    receipts = [json.loads(line) for line in args.receipts.read_text().splitlines()] if args.receipts else None
    prepared = materialize(anchors, receipts)
    args.out.write_text(json.dumps(prepared, ensure_ascii=False, indent=2)+"\n")
    print(json.dumps(dict(sources=len(anchors), controlled_cases=len(prepared))))
