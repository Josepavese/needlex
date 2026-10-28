#!/usr/bin/env python3
"""Linux MCP rendering comparison. Stdlib only; isolated state and bounded traces.

Run the same command with the before/after binaries and the same browser/config.
The external embedding service is deliberately excluded from process-tree RAM.
"""
import argparse
import base64
import hashlib
import http.server
import json
import os
from pathlib import Path
import select
import signal
import statistics
import subprocess
import tempfile
import threading
import time


class Fixture(http.server.BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_GET(self):
        if self.path == "/socket":
            accept = base64.b64encode(hashlib.sha1(
                (self.headers["Sec-WebSocket-Key"] +
                 "258EAFA5-E914-47DA-95CA-C5AB0DC85B11").encode()).digest())
            self.send_response(101)
            self.send_header("Upgrade", "websocket")
            self.send_header("Connection", "Upgrade")
            self.send_header("Sec-WebSocket-Accept", accept.decode())
            self.end_headers()
            body = b"WebSocket evidence: atlas socket delivery confirmed."
            self.wfile.write(bytes([0x81, len(body)]) + body)
            self.wfile.flush()
            time.sleep(0.15)
            self.close_connection = True
            return
        if self.path in ("/data", "/xhr", "/events", "/stream"):
            kind = {"/data": "application/json", "/xhr": "application/json",
                    "/events": "text/event-stream", "/stream": "text/plain"}[self.path]
            self.send_response(200)
            self.send_header("Content-Type", kind)
            self.end_headers()
            if self.path in ("/data", "/xhr"):
                self.wfile.write(json.dumps({"fact": self.path[1:] +
                    " evidence: atlas retrieval verified."}).encode())
            else:
                for n in range(3):
                    body = f"atlas {self.path[1:]} delivery {n} confirmed.\n"
                    if self.path == "/events":
                        body = "data: " + body + "\n"
                    self.wfile.write(body.encode())
                    self.wfile.flush()
                    time.sleep(0.08)
            self.close_connection = True
            return
        script = {
            "/dom": "setTimeout(()=>document.querySelector('#content').textContent="
                    "'Atlas DOM delivery confirmed: 42 reliable records. This paragraph is populated by JavaScript after load and must reach the evidence packet. The measurement compares identical content across isolated browser operations.',80);",
            "/network": "fetch('/data').then(r=>r.json());"
                        "let x=new XMLHttpRequest();x.open('GET','/xhr');x.send();"
                        "let e=new EventSource('/events');let count=0;e.onmessage=()=>{if(++count===3)e.close()};"
                        "let w=new WebSocket('ws://'+location.host+'/socket');w.onmessage=()=>w.close();"
                        "fetch('/stream').then(async r=>{for await(const c of r.body){}});",
            "/isolation": "let fresh=!localStorage.getItem('atlas')&&!document.cookie;"
                          "document.querySelector('#content').textContent=fresh?"
                          "'Atlas isolation fresh confirmed. No cookie or local storage from a previous request was visible. This independent operation writes private state after checking isolation, so the next read must start clean.':'Atlas isolation LEAK detected.';"
                          "localStorage.setItem('atlas','private');document.cookie='atlas=private;path=/';",
        }.get(self.path, "")
        self.send_response(200)
        self.send_header("Content-Type", "text/html")
        self.end_headers()
        self.wfile.write(("<!doctype html><title>Atlas rendering fixture</title>"
            "<main><p id=content>Application loading</p></main><script>" + script + "</script>").encode())


def descendants(pid):
    found = {pid}
    todo = [pid]
    while todo:
        p = todo.pop()
        try:
            children = []
            for child_file in Path(f"/proc/{p}/task").glob("*/children"):
                try:
                    children.extend(child_file.read_text().split())
                except FileNotFoundError:
                    pass
        except FileNotFoundError:
            continue
        for c in map(int, children):
            if c not in found:
                found.add(c)
                todo.append(c)
    return found


def memory(pid):
    rss = pss = 0
    processes = descendants(pid)
    for p in processes:
        try:
            for line in Path(f"/proc/{p}/smaps_rollup").read_text().splitlines():
                if line.startswith("Rss:"):
                    rss += int(line.split()[1])
                elif line.startswith("Pss:"):
                    pss += int(line.split()[1])
        except (FileNotFoundError, ProcessLookupError, PermissionError):
            pass
    return {"rss_kib": rss, "pss_kib": pss, "processes": len(processes), "pids": sorted(processes)}


def live_residue(seen):
    live = []
    for pid, birth in seen.items():
        try:
            stat = Path(f"/proc/{pid}/stat").read_text().rsplit(")", 1)[1].split()
            if stat[19] == birth and stat[0] != "Z":
                live.append(pid)
        except (FileNotFoundError, ProcessLookupError):
            pass
    return live


def run(binary, config, rounds, idle):
    with tempfile.TemporaryDirectory(prefix="needlex-render-measure.") as root:
        env = dict(os.environ, NEEDLEX_HOME=root, NEEDLEX_CONFIG=config, TMPDIR=root)
        with tempfile.TemporaryFile() as errors:
            p = subprocess.Popen([binary, "mcp"], stdin=subprocess.PIPE,
                                 stdout=subprocess.PIPE, stderr=errors, env=env,
                                 start_new_session=True)
            samples, rows, stop = [], [], threading.Event()
            browser_starts, seen = set(), {}

            def sample():
                while not stop.is_set():
                    measurement = memory(p.pid)
                    samples.append(measurement)
                    for child in measurement["pids"]:
                        try:
                            status = Path(f"/proc/{child}/stat").read_text().rsplit(")", 1)[1].split()
                            seen[child] = status[19]
                            command = Path(f"/proc/{child}/cmdline").read_bytes()
                            if b"--remote-debugging-port=" in command and b"--type=" not in command:
                                browser_starts.add((child, status[19]))
                        except (FileNotFoundError, ProcessLookupError):
                            pass
                    stop.wait(0.05)

            watcher = threading.Thread(target=sample)
            watcher.start()

            def rpc(request):
                p.stdin.write(json.dumps(request).encode() + b"\n")
                p.stdin.flush()
                if not select.select([p.stdout], [], [], 45)[0]:
                    raise TimeoutError("MCP response exceeded fixture deadline")
                response = json.loads(p.stdout.readline())
                if "error" in response:
                    raise RuntimeError(response["error"])
                return response["result"]

            try:
                rpc({"jsonrpc": "2.0", "id": 1, "method": "initialize"})
                for n in range(rounds):
                    for case in ("dom", "network", "isolation", "isolation"):
                        samples.clear()
                        start = time.monotonic()
                        result = rpc({"jsonrpc": "2.0", "id": len(rows) + 2,
                            "method": "tools/call", "params": {"name": "web_read",
                            "arguments": {"url": fixture_url + "/" + case,
                                          "render": "required"}}})
                        elapsed = time.monotonic() - start
                        full = result["structuredContent"]
                        encoded = json.dumps(full, sort_keys=True)
                        evidence = " ".join(node["text"] for node in full["web_ir"]["nodes"])
                        expected = {
                            "dom": ["Atlas DOM delivery confirmed"],
                            "isolation": ["Atlas isolation fresh confirmed"],
                            "network": ["data evidence", "xhr evidence", "atlas socket delivery confirmed"] +
                                [f"atlas {kind} delivery {i}" for kind in ("events", "stream") for i in range(3)],
                        }
                        for marker in expected[case]:
                            if marker not in evidence:
                                raise AssertionError(f"{case}: evidence missing {marker}")
                        trace = json.loads(Path(full["trace_path"]).read_text())
                        render_stage = next(s["metadata"] for s in trace["stages"] if s["stage"] == "render")
                        rows.append({"round": n, "case": case, "latency_ms": round(elapsed * 1000, 2),
                            "peak_pss_kib": max((s["pss_kib"] for s in samples), default=0),
                            "peak_rss_kib": max((s["rss_kib"] for s in samples), default=0),
                            "peak_processes": max((s["processes"] for s in samples), default=0),
                            "after": memory(p.pid), "render": render_stage, "content": full})
                        if "LEAK detected" in encoded:
                            raise AssertionError("browser state leaked between reads")
                immediately_idle = memory(p.pid)
                time.sleep(idle)
                after_idle = memory(p.pid)
                p.stdin.close()
                p.wait(timeout=10)
                assert p.returncode == 0, p.returncode
                errors.seek(0)
                stderr = errors.read()
                assert not stderr, stderr[:500]
                return {"rows": rows, "idle_immediate": immediately_idle,
                        "idle_later": after_idle, "idle_seconds": idle, "browser_starts": len(browser_starts),
                        "live_residue": live_residue(seen)}
            finally:
                if p.poll() is None:
                    try:
                        os.killpg(p.pid, signal.SIGTERM)
                    except ProcessLookupError:
                        pass
                    try:
                        p.wait(timeout=10)
                    except subprocess.TimeoutExpired:
                        try:
                            os.killpg(p.pid, signal.SIGKILL)
                        except ProcessLookupError:
                            pass
                        p.wait()
                stop.set()
                watcher.join()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--config", required=True)
    parser.add_argument("--out", required=True)
    parser.add_argument("--rounds", type=int, default=3)
    parser.add_argument("--idle-seconds", type=float, default=2)
    args = parser.parse_args()
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Fixture)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    fixture_url = f"http://127.0.0.1:{server.server_port}"
    try:
        result = run(str(Path(args.binary).resolve()), str(Path(args.config).resolve()),
                     args.rounds, args.idle_seconds)
        result["binary_sha256"] = hashlib.sha256(Path(args.binary).read_bytes()).hexdigest()
        result["config"] = json.loads(Path(args.config).read_text())
        Path(args.out).write_text(json.dumps(result, indent=2) + "\n")
        print(json.dumps({"reads": len(result["rows"]), "median_ms": statistics.median(
            r["latency_ms"] for r in result["rows"]), "peak_pss_kib": max(
            r["peak_pss_kib"] for r in result["rows"]), "idle": result["idle_later"]}))
    finally:
        server.shutdown()
        server.server_close()
