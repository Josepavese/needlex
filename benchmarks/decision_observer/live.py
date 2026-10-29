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
                selected[url]["origins"].append(case["id"])
                continue
            selected[url] = dict(id=case["id"], url=url, host=urlsplit(url).hostname,
                                 objective=case["goal"], family=case["family"],
                                 language=case.get("language"), origins=[case["id"]])
    return list(selected.values())


class MCP:
    def __init__(self, binary, config, root, errors):
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
            result = self.messages.get(timeout=RPC_WAIT_S)
            if "error" in result:
                raise MCPError(result["error"])
            if result.get("id") == self.number:
                return result["result"]

    def read(self, case):
        try:
            result = self.rpc("tools/call", dict(name="web_read", arguments=dict(
                url=case["url"], objective=case["objective"], profile="deep", render="auto",
                config_path=self.config)))
        except MCPError as error:
            return dict(tool_error=error.details)
        if result.get("isError"):
            return dict(tool_error=result.get("content"))
        full = result["structuredContent"]
        trace = json.loads(Path(full["trace_path"]).read_text())
        proofs = json.loads(Path(full["proof_path"]).read_text())
        chunks = full["agent_context"]["chunks"]
        text = "\n".join(chunk["text"] for chunk in chunks)
        observation = full["agent_context"].get("decision_observation")
        audits = [e for e in trace["events"] if e["stage"] == "decision_observation"]
        render = [s["metadata"] for s in trace["stages"] if s["stage"] == "render"]
        return dict(title=full.get("title"), summary=full.get("summary"),
                    final_url=full.get("url"), trace_id=full["trace_id"],
                    chunk_count=len(chunks), text_characters=len(text),
                    text_sha256=hashlib.sha256(text.encode()).hexdigest(),
                    snapshot=text[:SNAPSHOT_CHARACTERS], snapshot_complete=len(text) <= SNAPSHOT_CHARACTERS,
                    observation=observation, signals=full.get("signals"), render=render,
                    trace_audit_matches=bool(observation and len(audits) == 1 and
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


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--config", required=True, type=Path)
    parser.add_argument("--corpus", required=True, action="append", type=Path)
    parser.add_argument("--model-pid", required=True, type=int)
    parser.add_argument("--out", required=True, type=Path)
    parser.add_argument("--resume", action="store_true")
    parser.add_argument("--limit", type=int, help="optional admission smoke before resuming the full corpus")
    args = parser.parse_args()
    all_cases = cases(args.corpus)
    with tempfile.TemporaryDirectory(prefix="needlex-live-audit.") as temporary:
        with tempfile.TemporaryFile() as errors:
            identity_client = MCP(args.binary.resolve(), args.config.resolve(), Path(temporary), errors)
            try:
                binary_sha256 = identity_client.binary_sha256
            finally:
                identity_client.close()
    identity = hashlib.sha256(bytes.fromhex(binary_sha256) + args.config.read_bytes() +
                              b"".join(p.read_bytes() for p in args.corpus)).hexdigest()
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
            with tempfile.TemporaryDirectory(prefix="needlex-live-audit.") as temporary:
                root = Path(temporary)
                with tempfile.TemporaryFile() as errors:
                    client = MCP(args.binary.resolve(), args.config.resolve(), root, errors)
                    print(json.dumps(dict(batch=offset, server=client.server["serverInfo"])), flush=True)
                    try:
                        if client.binary_sha256 != binary_sha256:
                            raise ValueError("MCP executable changed during the evaluation")
                        for case in all_cases[offset:offset+BATCH_SIZE]:
                            stop = threading.Event()
                            peak = dict(process_pss_kib=None, model_pss_kib=None, aggregate_pss_kib=None)

                            def sample():
                                while not stop.is_set():
                                    own = memory(client.process.pid)["pss_kib"]
                                    model = memory(args.model_pid)["pss_kib"]
                                    if own and model:
                                        for key, value in zip(peak, (own, model, own+model)):
                                            peak[key] = max(peak[key] or 0, value)
                                    stop.wait(SAMPLE_INTERVAL_S)

                            watcher = threading.Thread(target=sample)
                            watcher.start()
                            started = time.monotonic()
                            try:
                                result = client.read(case)
                            finally:
                                stop.set()
                                watcher.join()
                            row = dict(**case, **result, run_identity=identity, binary_sha256=binary_sha256,
                                       elapsed_ms=round((time.monotonic()-started)*1000), **peak)
                            output.write(json.dumps(row, ensure_ascii=False)+"\n")
                            output.flush()
                            print(json.dumps(dict(done=offset+all_cases[offset:offset+BATCH_SIZE].index(case)+1,
                                total=len(all_cases), id=case["id"], ms=row["elapsed_ms"],
                                chunks=row.get("chunk_count"), audit=(row.get("observation") or {}).get("status"),
                                error=row.get("tool_error"))), flush=True)
                    finally:
                        client.close()


if __name__ == "__main__":
    main()
