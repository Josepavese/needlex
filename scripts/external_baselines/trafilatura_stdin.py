#!/usr/bin/env python3

import sys

try:
    import trafilatura
except Exception as exc:
    sys.stderr.write(f"trafilatura import failed: {exc}\n")
    sys.exit(2)


def main() -> int:
    html = sys.stdin.read()
    if not html:
        sys.stderr.write("no HTML provided on stdin\n")
        return 2
    text = trafilatura.extract(html) or ""
    sys.stdout.write(text.strip())
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
