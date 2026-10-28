"""Linux process-tree PSS sampling shared by resource benchmarks."""
from pathlib import Path


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
