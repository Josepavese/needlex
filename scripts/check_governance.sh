#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-.}"
cd "$ROOT"

TOOLS_BIN="${TOOLS_BIN:-$(go env GOPATH 2>/dev/null)/bin}"
if [ -n "$TOOLS_BIN" ] && [ -d "$TOOLS_BIN" ]; then
  export PATH="$PATH:$TOOLS_BIN"
fi

echo '== Governance Check =='
echo '-- go test ./... -count=1'
go test ./... -count=1

echo '-- bash scripts/check_budget.sh .'
bash scripts/check_budget.sh .
