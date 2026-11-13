#!/usr/bin/env python3
"""Generate THIRD_PARTY_NOTICES.txt for Needle-X release archives.

The notice covers the Go modules linked into cmd/needle and reproduces their
license and notice files. Run from the repository root after `go mod download`:

    python3 scripts/release/generate_third_party_notices.py

The output is deterministic for a given go.mod/module cache and is committed
so release builds do not need network access to reproduce it.
"""

from __future__ import annotations

import hashlib
import os
import re
import subprocess
import sys

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
OUTPUT = os.path.join(REPO_ROOT, "THIRD_PARTY_NOTICES.txt")
LICENSE_FILE_RE = re.compile(r"(?i)^(LICEN[CS]E|COPYING|NOTICE|COPYRIGHT)([._-].*)?$")
EXCLUDED_DIRS = {"testdata"}


def go(*args: str) -> str:
    return subprocess.run(
        ["go", *args],
        cwd=REPO_ROOT,
        check=True,
        capture_output=True,
        text=True,
    ).stdout


def linked_modules() -> list[tuple[str, str]]:
    output = go(
        "list",
        "-deps",
        "-f",
        "{{with .Module}}{{if not .Main}}{{.Path}}\t{{.Version}}{{end}}{{end}}",
        "./cmd/needle",
    )
    modules = set()
    for line in output.splitlines():
        if not line.strip():
            continue
        path, version = line.split("\t", 1)
        modules.add((path, version))
    return sorted(modules)


def module_dir(path: str, version: str) -> str:
    import json

    data = json.loads(go("mod", "download", "-json", f"{path}@{version}"))
    return data["Dir"]


def license_files(root: str) -> list[str]:
    found = []
    for current, dirs, files in os.walk(root):
        dirs[:] = sorted(d for d in dirs if d not in EXCLUDED_DIRS)
        relative_dir = os.path.relpath(current, root)
        for name in sorted(files):
            if LICENSE_FILE_RE.match(name):
                found.append(os.path.normpath(os.path.join(relative_dir, name)))
    return sorted(found)


def classify(text: str, relative: str = "") -> str:
    lower = text.lower()
    if "3rd-party" in relative.lower() or "third-party" in relative.lower():
        return "Multiple (see reproduced text)"
    if "gnu affero general public license" in lower:
        return "AGPL-3.0"
    if "gnu lesser general public license" in lower:
        return "LGPL"
    if "gnu general public license" in lower:
        return "GPL"
    if "mozilla public license" in lower and "2.0" in lower:
        return "MPL-2.0"
    if "apache license" in lower and "version 2.0" in lower:
        return "Apache-2.0"
    if "mit license" in lower or "permission is hereby granted, free of charge" in lower:
        return "MIT"
    if "redistribution and use in source and binary forms" in lower:
        if "neither the name" in lower:
            return "BSD-3-Clause"
        return "BSD-2-Clause"
    if "isc license" in lower or "permission to use, copy, modify, and distribute" in lower or "permission to use, copy, modify, and/or distribute" in lower:
        return "ISC"
    if "this is free and unencumbered software" in lower:
        return "Unlicense"
    if "public domain" in lower:
        return "Public-Domain"
    return "See text"


def main() -> int:
    modules = linked_modules()
    if not modules:
        print("no linked modules found", file=sys.stderr)
        return 1

    entries: list[tuple[str, str, str, str]] = []
    for path, version in modules:
        root = module_dir(path, version)
        for relative in license_files(root):
            absolute = os.path.join(root, relative)
            with open(absolute, "r", encoding="utf-8", errors="replace") as handle:
                text = handle.read().strip()
            if not text:
                continue
            entries.append((path, version, relative, text))

    grouped: dict[str, dict[str, object]] = {}
    for path, version, relative, text in entries:
        digest = hashlib.sha256(text.encode("utf-8")).hexdigest()
        group = grouped.setdefault(
            digest,
            {"text": text, "label": classify(text, relative), "references": []},
        )
        group["references"].append(f"{path}@{version} :: {relative}")

    lines = [
        "Needle-X third-party notices",
        "============================",
        "",
        "Needle-X itself is released under the MIT License; see LICENSE.",
        "",
        "Each native Needle-X archive includes the project license plus this file.",
        "The binary links the Go modules listed below. Their license and notice",
        "texts are reproduced here so redistributors can preserve them.",
        "",
        "This file is generated from the modules linked into ./cmd/needle:",
        "",
        "    python3 scripts/release/generate_third_party_notices.py",
        "",
        "Modules:",
        "",
    ]
    for path, version in modules:
        lines.append(f"- {path} {version}")
    lines.extend(["", "License texts follow, grouped by identical content.", ""])

    for digest in sorted(grouped, key=lambda d: str(grouped[d]["label"]) + "|" + grouped[d]["references"][0]):
        group = grouped[digest]
        lines.append("=" * 78)
        lines.append(f"License group: {group['label']} (sha256 {digest[:16]})")
        lines.append("=" * 78)
        lines.append("")
        lines.append("Applies to:")
        for reference in sorted(group["references"]):
            lines.append(f"  - {reference}")
        lines.extend(["", str(group["text"]), ""])

    with open(OUTPUT, "w", encoding="utf-8", newline="\n") as handle:
        handle.write("\n".join(lines).rstrip() + "\n")

    print(f"wrote {OUTPUT} ({len(entries)} license/notice files, {len(grouped)} unique texts)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
