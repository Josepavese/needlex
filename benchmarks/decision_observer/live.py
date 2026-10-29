#!/usr/bin/env python3
"""Installed MCP audit on existing known-URL corpora; no discovery or gold invention.

Snapshots are bounded excerpts for independent annotation, not complete pages.
Each URL is read once. Runtime budgets and Julia policy come from the supplied
config. Browser scratch is retired after each batch; shared models stay. A shell
installer wrapper can select canonical PAL state, whose useful proofs are kept.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import queue
import shutil
import subprocess
import sys
import tempfile
import threading
import time
from urllib.parse import urlsplit

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "internal"))
from process_memory import memory

BATCH_SIZE = 12
SAMPLE_INTERVAL_S = 0.05
RPC_WAIT_S = 90
SNAPSHOT_CHARACTERS = 12000
PROTOCOL_VERSION = "2024-11-05"


class MCPError(RuntimeError):
    def __init__(self, details):
        super().__init__(details)
        self.details = details


def cases(paths):
    selected = {}
    for path in paths:
        data = json.loads(path.read_text())
        for case in data["cases"]:
            url = case["expected_url"]
            if url in selected:
                selected[url]["origins"].extend(case.get("origins", [case["id"]]))
                continue
            selected[url] = dict(id=case["id"], url=url, host=urlsplit(url).hostname,
                                 objective=case["goal"], family=case["family"],
                                 language=case.get("language"), origins=list(case.get("origins", [case["id"]])))
            for key in ("cohort", "source_language", "tranco_rank", "crux_bucket", "popularity_band", "reference_panel"):
                if key in case:
                    selected[url][key] = case[key]
    return list(selected.values())


class MCP:
    def __init__(self, binary, config, root, errors, rpc_wait_s=RPC_WAIT_S):
        self.rpc_wait_s = rpc_wait_s
        self.process = subprocess.Popen(
            [str(binary), "mcp"], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
            stderr=errors, text=True, env=dict(os.environ, NEEDLEX_HOME=str(root),
                                             NEEDLEX_CONFIG=str(config), TMPDIR=str(root)))
        self.messages = queue.Queue()
        self.config = str(config)
        self.number = 0
        self.reader = threading.Thread(target=self.receive, daemon=True)
        self.reader.start()
        self.server = self.rpc("initialize", dict(protocolVersion=PROTOCOL_VERSION,
                               capabilities={}, clientInfo=dict(name="needlex-live-audit", version="1")))
        self.binary_sha256 = hashlib.sha256(Path(f"/proc/{self.process.pid}/exe").read_bytes()).hexdigest()
        self.send(dict(jsonrpc="2.0", method="notifications/initialized"))

    def receive(self):
        for line in self.process.stdout:
            try:
                self.messages.put(json.loads(line))
            except json.JSONDecodeError as error:
                self.messages.put(dict(error=dict(message=str(error))))
        self.messages.put(dict(error=dict(message="MCP output closed")))

    def send(self, payload):
        self.process.stdin.write(json.dumps(payload) + "\n")
        self.process.stdin.flush()

    def rpc(self, method, params):
        self.number += 1
        self.send(dict(jsonrpc="2.0", id=self.number, method=method, params=params))
        while True:
            result = self.messages.get(timeout=self.rpc_wait_s)
            if "error" in result:
                raise MCPError(result["error"])
            if result.get("id") == self.number:
                return result["result"]

    def read(self, case):
        try:
            result = self.rpc("tools/call", dict(name="web_read", arguments=dict(
                url=case["url"], objective=case["objective"], profile="deep", render="auto",
                config_path=self.config, assertion=case.get("assertion", ""))))
        except MCPError as error:
            return dict(tool_error=error.details)
        except queue.Empty:
            return dict(tool_error=dict(kind="observer_timeout", message="MCP observation deadline reached; operation result unknown"), observer_timeout=True)
        if result.get("isError"):
            return dict(tool_error=result.get("content"))
        full = result["structuredContent"]
        trace = json.loads(Path(full["trace_path"]).read_text())
        proofs = json.loads(Path(full["proof_path"]).read_text())
        chunks = full["agent_context"]["chunks"]
        text = "\n".join(chunk["text"] for chunk in chunks)
        observation = full["agent_context"].get("decision_observation")
        acquisition = full["agent_context"].get("acquisition_observation")
        audits = [e for e in trace["events"] if e["stage"] == "decision_observation"]
        render = [s["metadata"] for s in trace["stages"] if s["stage"] == "render"]
        complete = capture_complete(trace["stages"])
        records = {r["id"]: r for r in proofs}
        proof_errors = []
        for chunk in chunks:
            record = records.get(chunk.get("proof_ref"), {})
            if record.get("proof", {}).get("chunk_id") != chunk["id"]:
                proof_errors.append("missing_or_foreign_chunk_proof:" + chunk["id"])
        signals = full.get("signals") or full.get("compact", {}).get("signals")
        return dict(title=full.get("title"), summary=full.get("summary"),
                    final_url=full.get("url"), trace_id=full["trace_id"],
                    chunk_count=len(chunks), text_characters=len(text),
                    text_sha256=hashlib.sha256(text.encode()).hexdigest(),
                    snapshot=text[:SNAPSHOT_CHARACTERS], snapshot_complete=len(text) <= SNAPSHOT_CHARACTERS,
                    observation=observation, acquisition_observation=acquisition, signals=signals, render=render,
                    capture_complete=complete, proof_mapping_valid=not proof_errors and bool(chunks),
                    proof_validation_errors=proof_errors[:3],
                    trace_sha256=hashlib.sha256(Path(full["trace_path"]).read_bytes()).hexdigest(),
                    proof_sha256=hashlib.sha256(Path(full["proof_path"]).read_bytes()).hexdigest(),
                    coverage={marker: marker in text for marker in case.get("coverage_markers", [])},
                    trace_audit_matches=(observation is None and not audits) or bool(observation and len(audits) == 1 and
                        json.loads(audits[0]["data"]["observation"]) == observation),
                    proof_file_nonempty=bool(proofs), proof_refs=full.get("proof_refs"),
                    chunk_ids=[c["id"] for c in chunks])

    def close(self):
        self.process.stdin.close()
        try:
            self.process.wait(timeout=RPC_WAIT_S)
        except subprocess.TimeoutExpired:
            self.process.terminate()
            self.process.wait(timeout=RPC_WAIT_S)
        self.reader.join(timeout=1)
        if self.process.returncode:
            raise RuntimeError(f"MCP exited {self.process.returncode}")


def capture_complete(stages):
    complete = None
    for stage in stages:
        m = stage.get("metadata", {})
        if stage["stage"] == "acquire":
            complete = m.get("partial_body") == "false" if "partial_body" in m else None
        if stage["stage"] == "agent_readable" and m.get("selected") == "true":
            complete = m.get("selected_partial") == "false" if "selected_partial" in m else None
        if stage["stage"] == "render" and m.get("rendered") == "true":
            complete = (m.get("render_path") == "cdp" and m.get("partial") == "false"
                        and m.get("network_truncated") == "false" and m.get("network_body_missing") == "0"
                        and m.get("render_degraded") != "true")
    return complete


def state_bytes(path):
    return sum(p.stat().st_size for p in path.rglob("*") if p.is_file() and not p.is_symlink())


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--config", required=True, type=Path)
    parser.add_argument("--corpus", required=True, action="append", type=Path)
    parser.add_argument("--model-pid", required=True, type=int)
    parser.add_argument("--out", required=True, type=Path)
    parser.add_argument("--assertions", type=Path, help="prepared claim corpus; request its supported assertions on matching URLs")
    parser.add_argument("--coverage-anchors", type=Path, help="frozen source excerpts; record exact presence in the complete compiled text, not just its bounded snapshot")
    parser.add_argument("--resume", action="store_true")
    parser.add_argument("--limit", type=int, help="optional admission smoke before resuming the full corpus")
    parser.add_argument("--scratch-dir", type=Path, help="owned outside-repository parent for bounded browser batch scratch")
    parser.add_argument("--rpc-timeout-s", type=float, default=RPC_WAIT_S, help="observer watchdog only; does not narrow native fetch/render budgets")
    parser.add_argument("--state-dir", type=Path, help="actual installed PAL state root for growth guarding")
    parser.add_argument("--max-state-growth-mib", type=int, help="stop before the next batch when the shared-state growth allowance is exhausted")
    args = parser.parse_args()
    config_bytes = args.config.read_bytes()
    cfg = json.loads(config_bytes)
    baseline_state = state_bytes(args.state_dir) if args.state_dir else 0
    space_policy = Path(__file__).resolve().parents[2] / "governance/workstation.env"
    hard_gib = int(next(line.split("=", 1)[1] for line in space_policy.read_text().splitlines() if line.startswith("MIN_FREE_GB_HARD=")))
    all_cases = cases(args.corpus)
    if args.coverage_anchors:
        anchors = json.loads(args.coverage_anchors.read_text())
        by_url = {}
        for anchor in anchors:
            by_url.setdefault(anchor["source"]["url"], []).append(anchor["quote"])
        for case in all_cases:
            case["coverage_markers"] = by_url.get(case["url"], [])
    if args.assertions:
        checks = json.loads(args.assertions.read_text())
        by_url = {case["source"]["url"]: case["input"]["assertion"] for case in checks if case["expected"]["support"] == "supported"}
        for case in all_cases:
            if case["url"] in by_url:
                case["assertion"] = by_url[case["url"]]
    with tempfile.TemporaryDirectory(prefix="needlex-live-audit.", dir=args.scratch_dir) as temporary:
        with tempfile.TemporaryFile() as errors:
            identity_client = MCP(args.binary.resolve(), args.config.resolve(), Path(temporary), errors, args.rpc_timeout_s)
            try:
                binary_sha256 = identity_client.binary_sha256
            finally:
                identity_client.close()
    identity = hashlib.sha256(Path(__file__).read_bytes() + str(args.rpc_timeout_s).encode() +
                              bytes.fromhex(binary_sha256) + config_bytes +
                              b"".join(p.read_bytes() for p in args.corpus) +
                              (args.assertions.read_bytes() if args.assertions else b"") +
                              (args.coverage_anchors.read_bytes() if args.coverage_anchors else b"")).hexdigest()
    completed = []
    if args.resume:
        completed = [json.loads(line) for line in args.out.read_text().splitlines()]
        if any(row["run_identity"] != identity for row in completed):
            raise ValueError("resume inputs differ from the completed receipts")
    done = {row["url"] for row in completed}
    all_cases = [case for case in all_cases if case["url"] not in done]
    if args.limit is not None:
        all_cases = all_cases[:args.limit]
    # Exclusive creation protects evidence; explicit resume reuses completed URLs.
    with args.out.open("a" if args.resume else "x") as output:
        for offset in range(0, len(all_cases), BATCH_SIZE):
            reserve = BATCH_SIZE * cfg["runtime"]["max_bytes"]
            if shutil.disk_usage(args.out.parent).free < hard_gib * 1024**3 + reserve:
                raise RuntimeError("workstation space below SSOT floor plus next-batch reserve; receipts retained")
            if args.state_dir and args.max_state_growth_mib and state_bytes(args.state_dir) - baseline_state > args.max_state_growth_mib * 1024**2:
                raise RuntimeError("shared PAL growth allowance exhausted; receipts retained, no user state pruned")
            with tempfile.TemporaryDirectory(prefix="needlex-live-audit.", dir=args.scratch_dir) as temporary:
                root = Path(temporary)
                with tempfile.TemporaryFile() as errors:
                    client = MCP(args.binary.resolve(), args.config.resolve(), root, errors, args.rpc_timeout_s)
                    print(json.dumps(dict(batch=offset, server=client.server["serverInfo"])), flush=True)
                    try:
                        if client.binary_sha256 != binary_sha256:
                            raise ValueError("MCP executable changed during the evaluation")
                        for case in all_cases[offset:offset+BATCH_SIZE]:
                            if args.config.read_bytes() != config_bytes:
                                raise ValueError("configuration changed during evaluation; prior receipts retained")
                            stop = threading.Event()
                            peak = dict(process_pss_kib=None, model_pss_kib=None, aggregate_pss_kib=None)

                            def sample():
                                while not stop.is_set():
                                    own = memory(client.process.pid)["pss_kib"]
                                    model = memory(args.model_pid)["pss_kib"]
                                    for key, value in (("process_pss_kib", own), ("model_pss_kib", model), ("aggregate_pss_kib", own+model if own and model else None)):
                                        if value:
                                            peak[key] = max(peak[key] or 0, value)
                                    stop.wait(SAMPLE_INTERVAL_S)

                            watcher = threading.Thread(target=sample)
                            watcher.start()
                            started = time.monotonic()
                            print(json.dumps(dict(begin=case["id"], url=case["url"], request_id=client.number+1)), flush=True)
                            try:
                                result = client.read(case)
                            finally:
                                stop.set()
                                watcher.join()
                            row = dict(**case, **result, run_identity=identity, binary_sha256=binary_sha256,
                                       elapsed_ms=round((time.monotonic()-started)*1000), **peak)
                            output.write(json.dumps(row, ensure_ascii=False)+"\n")
                            output.flush()
                            if row.get("observer_timeout"):
                                raise RuntimeError("observer timeout receipt retained; closing owned MCP before continuation; do not repeat this URL")
                            print(json.dumps(dict(done=offset+all_cases[offset:offset+BATCH_SIZE].index(case)+1,
                                total=len(all_cases), id=case["id"], ms=row["elapsed_ms"],
                                chunks=row.get("chunk_count"), audit=(row.get("observation") or {}).get("status"),
                                error=row.get("tool_error"))), flush=True)
                    finally:
                        client.close()


if __name__ == "__main__":
    main()
