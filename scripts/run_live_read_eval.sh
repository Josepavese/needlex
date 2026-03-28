#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

go run ./scripts/live_read_eval \
  --out improvements/live-read-latest.json \
  --baseline improvements/live-read-baseline.json \
  "$@"
