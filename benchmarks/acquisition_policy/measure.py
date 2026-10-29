#!/usr/bin/env python3
"""Sample the native routing qualification test binary, excluding its compiler.

Records PSS by case/route and phase, plus the resident local decision service.
Shared Chromium is intentionally included even on a later static route. Native
GET attempts and Chromium request events are different counters, not wire totals.
"""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import threading

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "internal"))
from process_memory import memory

INTERVAL_S = .05


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--binary", required=True, type=Path)
    p.add_argument("--config", required=True, type=Path)
    p.add_argument("--out", required=True, type=Path)
    p.add_argument("--input", type=Path)
    p.add_argument("--resources", required=True, type=Path)
    p.add_argument("--phase", choices=["collect", "development", "heldout"], required=True)
    p.add_argument("--warm", action="store_true", help="qualify inference after real bounded dense-vector cache warmup; retain its cost separately")
    p.add_argument("--model-pid", type=int, required=True)
    a = p.parse_args()
    env = dict(os.environ, NEEDLEX_ACQUISITION_CONFIG=str(a.config.resolve()),
               NEEDLEX_ACQUISITION_OUT=str(a.out.resolve()), NEEDLEX_ACQUISITION_PHASE=a.phase)
    if a.warm:
        env["NEEDLEX_ACQUISITION_WARM"] = "1"
    if a.input:
        env["NEEDLEX_ACQUISITION_INPUT"] = str(a.input.resolve())
    # cwd is fixed to the service package, like `go test`, for its corpus path.
    cwd = Path(__file__).resolve().parents[2] / "internal/core/service"
    process = subprocess.Popen([str(a.binary.resolve()), "-test.run", "^TestExportAcquisitionPolicy$", "-test.v", "-test.timeout=1h"],
                               env=env, cwd=cwd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    lock, stop = threading.Lock(), threading.Event()
    current = {"key": None}
    peaks = {}

    def sample():
        while not stop.is_set():
            own = memory(process.pid)["pss_kib"]
            model = memory(a.model_pid)["pss_kib"]
            with lock:
                for key in ["phase", current["key"]]:
                    if key is None:
                        continue
                    record = peaks.setdefault(key, {})
                    for name, value in [("tool_pss_kib", own), ("model_pss_kib", model), ("aggregate_pss_kib", own + model)]:
                        if value:
                            record[name] = max(record.get(name, 0), value)
            stop.wait(INTERVAL_S)

    watcher = threading.Thread(target=sample)
    watcher.start()
    try:
        with a.resources.open("a") as output:
            for line in process.stdout:
                print(line, end="", flush=True)
                marker = re.match(r"ACQUISITION_SAMPLE (start|end) id=(\S+) action=(\S+)", line)
                if not marker:
                    continue
                mode, case, action = marker.groups()
                key = (case, action)
                with lock:
                    if mode == "start":
                        current["key"] = key
                        peaks[key] = {}
                    else:
                        output.write(json.dumps(dict(phase=a.phase, case=case, action=action, **peaks.get(key, {}))) + "\n")
                        output.flush()
                        current["key"] = None
            result = process.wait()
            with lock:
                output.write(json.dumps(dict(phase=a.phase, action="phase_peak", returncode=result, **peaks.get("phase", {}))) + "\n")
    finally:
        stop.set()
        watcher.join()
    if result:
        raise SystemExit(result)


if __name__ == "__main__":
    main()
