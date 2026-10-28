#!/usr/bin/env python3
"""Paired CLI content/PSS/latency qualification, using one config and local server.

The embedding service is external and excluded consistently. A model PID is
included separately and in aggregate PSS for the observation phase. Source facts,
not RPC success, determine coverage. No raw responses or state survive cleanup.
"""
import argparse
import hashlib
import http.server
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import time
import urllib.request

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "internal"))
from process_memory import memory


class Fixture(http.server.BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_GET(self):
        if self.path == "/stream":
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.end_headers()
            try:
                self.wfile.write(b"data: end\n\n")
                self.wfile.flush()
                time.sleep(3)
                self.wfile.write(b'data: {"report":"Late payload contains the warranty of 37 months."}\n\n')
                self.wfile.flush()
                time.sleep(12)
            except (BrokenPipeError, ConnectionResetError):
                pass
            return
        body = "<html><title>Research notes</title><body>"
        if self.path == "/sse":
            body += '<h1>Live report</h1><p>Waiting for application data</p><script>new EventSource("/stream")</script>'
        else:
            body += "<h1>Ancient astronomy</h1>" + ("<p>Ancient astronomers documented movements of stars, mapped constellations and studied the seasonal night sky across the Mediterranean. This historical account describes celestial observation and archival records.</p>" * 15)
            if self.path == "/js":
                body += '<script>setTimeout(()=>{document.body.innerHTML="<h1>Product conditions</h1><p>Prezzo: 47 euro. Garanzia: 37 mesi. Reso: 29 giorni.</p>"},250)</script>'
        body += "</body></html>"
        self.send_response(200)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.end_headers()
        self.wfile.write(body.encode())


CASES = [
    ("short", "/js", "Prezzi", "auto", ["47 euro", "37 mesi", "29 giorni"]),
    ("zh", "/js", "请查找这个产品在意大利的保修条件及退货政策", "auto", ["47 euro", "37 mesi", "29 giorni"]),
    ("sse", "/sse", "Retrieve the warranty conditions in the live application report", "required", ["37 months"]),
    ("static", "/static", "Ancient astronomy and the historical observation of constellations", "off", ["Ancient astronomers"]),
]


def run(binary, label, cfg, server, root, rounds, model_pid):
    conf = root / (label + "-config.json")
    conf.write_text(json.dumps(cfg))
    env = dict(os.environ, NEEDLEX_HOME=str(root / label), TMPDIR=str(root))
    rows = []
    for repeat in range(rounds):
        for name, path, objective, render, facts in CASES:
            started = time.monotonic()
            process = subprocess.Popen([binary, "read", f"http://127.0.0.1:{server.server_port}" + path, "--objective", objective, "--render", render, "--profile", "deep", "--config", str(conf), "--json", "--json-mode", "full"], stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)
            stop = threading.Event()
            peak = {"pss_kib": None, "model_pss_kib": None, "aggregate_pss_kib": None}

            def sample():
                while not stop.is_set():
                    own = memory(process.pid)["pss_kib"]
                    model = memory(model_pid)["pss_kib"] if model_pid else 0
                    for key, value in [("pss_kib", own), ("model_pss_kib", model), ("aggregate_pss_kib", own + model)]:
                        if value:
                            peak[key] = max(peak[key] or 0, value)
                    stop.wait(.005)

            watcher = threading.Thread(target=sample)
            watcher.start()
            try:
                output, errors = process.communicate(timeout=60)
            except subprocess.TimeoutExpired:
                process.kill()
                process.communicate()
                raise
            finally:
                stop.set()
                watcher.join()
            if process.returncode:
                raise RuntimeError(errors.decode()[:1000])
            response = json.loads(output)
            text = "\n".join(c["text"] for c in response["agent_context"]["chunks"])
            observation = response["agent_context"].get("decision_observation")
            row = dict(phase=label, case=name, repeat=repeat, ms=round((time.monotonic() - started) * 1000, 1), hits=sum(fact in text for fact in facts), total=len(facts), text_sha256=hashlib.sha256(text.encode()).hexdigest(), observation=observation, **peak)
            rows.append(row)
            print(json.dumps({k: row[k] for k in ["phase", "case", "repeat", "hits", "total", "ms", "aggregate_pss_kib"]}), flush=True)
    return rows


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--before", required=True)
    parser.add_argument("--after", required=True)
    parser.add_argument("--config", required=True)
    parser.add_argument("--decision", required=True)
    parser.add_argument("--model-pid", type=int, required=True)
    parser.add_argument("--out", required=True)
    parser.add_argument("--rounds", type=int, default=3)
    parser.add_argument("--warm-embedding", action="store_true", help="verify a warm local embedding dependency before each phase; record cold-load time separately")
    parser.add_argument("--phases", default="before,fixed,observation", help="phases to measure; retained baselines need not be rerun")
    args = parser.parse_args()
    cfg = json.loads(Path(args.config).read_text())
    decision = json.loads(Path(args.decision).read_text())
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Fixture)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    rows = []
    warmups = []
    try:
        with tempfile.TemporaryDirectory(prefix="needlex-decision-measure.") as temporary:
            root = Path(temporary)
            cfg.pop("decision", None)
            for binary, label, model_pid in [(args.before, "before", None), (args.after, "fixed", None), (args.after, "observation", args.model_pid)]:
                if label not in args.phases.split(","):
                    continue
                if label == "observation":
                    cfg["decision"] = decision
                if args.warm_embedding:
                    started = time.monotonic()
                    request = urllib.request.Request(cfg["semantic"]["embedding_url"], data=json.dumps({"model": cfg["semantic"]["provider_model"], "input": ["Needle-X qualification warmup"]}).encode(), headers={"Content-Type": "application/json"})
                    with urllib.request.urlopen(request, timeout=30) as response:
                        embedding = json.load(response)
                    vectors = embedding.get("embeddings", [])
                    if not vectors or not vectors[0] or not any(vectors[0]):
                        raise RuntimeError("embedding warmup returned no dense vector")
                    warmups.append(dict(phase=label, ms=round((time.monotonic()-started)*1000, 1), dimensions=len(vectors[0])))
                rows.extend(run(binary, label, cfg, server, root, args.rounds, model_pid))
    finally:
        server.shutdown()
        server.server_close()
    Path(args.out).write_text(json.dumps({"rows": rows, "warmups": warmups, "rounds": args.rounds, "embedding_ram": "shared service excluded from all phases", "memory": "5ms process-tree PSS, including all task children; missing samples are null"}, indent=2) + "\n")


if __name__ == "__main__":
    main()
