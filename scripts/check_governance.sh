#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-.}"
cd "$ROOT"

TOOLS_BIN="${TOOLS_BIN:-$(go env GOPATH 2>/dev/null)/bin}"
if [ -n "$TOOLS_BIN" ] && [ -d "$TOOLS_BIN" ]; then
  export PATH="$PATH:$TOOLS_BIN"
fi

if [ -z "${NEEDLEX_HOME:-}" ]; then
  NEEDLEX_GOVERNANCE_HOME="$(mktemp -d)"
  export NEEDLEX_HOME="$NEEDLEX_GOVERNANCE_HOME"
  trap 'rm -rf "$NEEDLEX_GOVERNANCE_HOME"' EXIT
fi

echo '== Governance Check =='
echo "-- NEEDLEX_HOME=${NEEDLEX_HOME}"
echo '-- go test ./... -count=1'
go test ./... -count=1

echo '-- bash scripts/check_budget.sh .'
bash scripts/check_budget.sh .

echo '-- bash scripts/check_skills.sh .'
bash scripts/check_skills.sh .
