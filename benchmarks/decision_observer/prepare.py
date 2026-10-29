#!/usr/bin/env python3
"""Freeze source-grounded decision controls; labels never come from Julia.

One source title/excerpt produces supported, contradicted and not-established
claim pairs. These are three correlated cases, not independent web sources.
The labels concern only the supplied excerpt, not the complete source page.
"""
import argparse
import hashlib
import json
from pathlib import Path

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
        for verdict, assertion in anchor["claims"].items():
            cases.append(dict(id=anchor["source_id"]+"-"+verdict,
                              input=dict(assertion=assertion, evidence=[dict(
                                  id="ev_"+hashlib.sha256(anchor["quote"].encode()).hexdigest(),
                                  heading=[source["title"]], text=anchor["quote"])]),
                              expected={"support":verdict}, source=dict(url=source["url"],
                                  trace_id=source["trace_id"], text_sha256=source["text_sha256"],
                                  language=anchor["language"], synthetic=False),
                              variant=verdict, family=anchor["family"], partition=anchor["partition"]))
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
