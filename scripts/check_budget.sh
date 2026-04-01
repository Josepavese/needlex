#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-.}"
MAX_PROD_LOC="${MAX_PROD_LOC:-10550}"
MAX_INTERNAL_PACKAGES="${MAX_INTERNAL_PACKAGES:-10}"
MAX_RUNTIME_DEPS="${MAX_RUNTIME_DEPS:-8}"
MAX_FILE_LOC="${MAX_FILE_LOC:-400}"
MAX_AVG_FILE_LOC="${MAX_AVG_FILE_LOC:-175}"
MAX_FILES_OVER_300="${MAX_FILES_OVER_300:-10}"
MAX_FILES_OVER_350="${MAX_FILES_OVER_350:-5}"
MAX_PACKAGE_LOC="${MAX_PACKAGE_LOC:-4000}"
MAX_STRUCTURE_ISSUES="${MAX_STRUCTURE_ISSUES:-8}"
REQUIRE_GOFUMPT="${REQUIRE_GOFUMPT:-1}"
REQUIRE_GOVET="${REQUIRE_GOVET:-1}"
REQUIRE_STATICCHECK="${REQUIRE_STATICCHECK:-1}"
REQUIRE_STRUCTURE_LINT="${REQUIRE_STRUCTURE_LINT:-1}"

cd "$ROOT"

TOOLS_BIN="${TOOLS_BIN:-$(go env GOPATH 2>/dev/null)/bin}"
if [ -n "$TOOLS_BIN" ] && [ -d "$TOOLS_BIN" ]; then
  export PATH="$PATH:$TOOLS_BIN"
fi

mapfile -t GO_FILES < <(find . -type f -name '*.go' \
  ! -name '*_test.go' \
  ! -path './vendor/*' \
  ! -path './.git/*' \
  ! -path './testdata/*' \
  ! -path './.agent/*' \
  ! -path './docs/*' \
  ! -path './scripts/*' \
  | sort)

PROD_LOC=0
MAX_FILE_LOC_OBSERVED=0
MAX_FILE_PATH=""
FILES_OVER_300=0
FILES_OVER_350=0

for f in "${GO_FILES[@]}"; do
  lines=$(wc -l < "$f")
  PROD_LOC=$((PROD_LOC + lines))
  if [ "$lines" -gt 300 ]; then
    FILES_OVER_300=$((FILES_OVER_300 + 1))
  fi
  if [ "$lines" -gt 350 ]; then
    FILES_OVER_350=$((FILES_OVER_350 + 1))
  fi
  if [ "$lines" -gt "$MAX_FILE_LOC_OBSERVED" ]; then
    MAX_FILE_LOC_OBSERVED="$lines"
    MAX_FILE_PATH="$f"
  fi
done

GO_FILE_COUNT="${#GO_FILES[@]}"
AVG_FILE_LOC=0
if [ "$GO_FILE_COUNT" -gt 0 ]; then
  AVG_FILE_LOC=$((PROD_LOC / GO_FILE_COUNT))
fi

if [ -d internal ]; then
  INTERNAL_PACKAGES=$(find internal -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ')
else
  INTERNAL_PACKAGES=0
fi

MAX_PACKAGE_LOC_OBSERVED=0
MAX_PACKAGE_PATH=""
if [ -d internal ]; then
  while IFS='|' read -r pkg lines; do
    [ -z "$pkg" ] && continue
    if [ "$lines" -gt "$MAX_PACKAGE_LOC_OBSERVED" ]; then
      MAX_PACKAGE_LOC_OBSERVED="$lines"
      MAX_PACKAGE_PATH="$pkg"
    fi
  done < <(
    while IFS= read -r -d '' dir; do
      total=$(find "$dir" -maxdepth 1 -type f -name '*.go' ! -name '*_test.go' -exec wc -l {} + | awk '$2 != "total" {sum+=$1} END{print sum+0}')
      [ "$total" -gt 0 ] && printf '%s|%s\n' "$dir" "$total"
    done < <(find internal -type d -print0)
  )
fi

if [ -f go.mod ]; then
  RUNTIME_DEPS=$(awk '
    /^require \(/ {in_block=1; next}
    in_block && /^\)/ {in_block=0; next}
    in_block {
      if ($0 !~ /\/\/ indirect/) count++
      next
    }
    /^require / {
      if ($0 !~ /\/\/ indirect/) count++
    }
    END {print count+0}
  ' go.mod)
else
  RUNTIME_DEPS=0
fi

GOFUMPT_PENDING=0
GO_VET_STATUS="skipped"
STATICCHECK_STATUS="skipped"
STRUCTURE_ISSUES=0

TMPDIR_BUDGET="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_BUDGET"' EXIT

if [ "$REQUIRE_GOFUMPT" -eq 1 ]; then
  if command -v gofumpt >/dev/null 2>&1; then
    mapfile -t GOFMT_PENDING_FILES < <(gofumpt -l $(find . -type f -name '*.go' ! -path './vendor/*' ! -path './.git/*'))
    GOFUMPT_PENDING="${#GOFMT_PENDING_FILES[@]}"
  else
    GOFUMPT_PENDING=-1
  fi
fi

if [ "$REQUIRE_GOVET" -eq 1 ]; then
  if go vet ./... >"$TMPDIR_BUDGET/go_vet.out" 2>&1; then
    GO_VET_STATUS="pass"
  else
    GO_VET_STATUS="fail"
  fi
fi

if [ "$REQUIRE_STATICCHECK" -eq 1 ]; then
  if command -v staticcheck >/dev/null 2>&1; then
    if staticcheck ./... >"$TMPDIR_BUDGET/staticcheck.out" 2>&1; then
      STATICCHECK_STATUS="pass"
    else
      STATICCHECK_STATUS="fail"
    fi
  else
    STATICCHECK_STATUS="missing"
  fi
fi

if [ "$REQUIRE_STRUCTURE_LINT" -eq 1 ]; then
  if command -v golangci-lint >/dev/null 2>&1; then
    golangci-lint run ./internal/... --out-format line-number >"$TMPDIR_BUDGET/structure.out" 2>&1 || true
    if [ -s "$TMPDIR_BUDGET/structure.out" ]; then
      STRUCTURE_ISSUES=$(grep -c '^[^[:space:]].*:[0-9]\+:' "$TMPDIR_BUDGET/structure.out" || true)
    else
      STRUCTURE_ISSUES=0
    fi
  else
    STRUCTURE_ISSUES=-1
  fi
fi

FAIL=0

echo "== Lean Budget Report =="
echo "ROOT=$PWD"
echo "PROD_LOC=$PROD_LOC (limit $MAX_PROD_LOC)"
echo "GO_FILE_COUNT=$GO_FILE_COUNT"
echo "AVG_FILE_LOC=$AVG_FILE_LOC (limit $MAX_AVG_FILE_LOC)"
echo "INTERNAL_PACKAGES=$INTERNAL_PACKAGES (limit $MAX_INTERNAL_PACKAGES)"
echo "RUNTIME_DEPS=$RUNTIME_DEPS (limit $MAX_RUNTIME_DEPS)"
echo "FILES_OVER_300=$FILES_OVER_300 (limit $MAX_FILES_OVER_300)"
echo "FILES_OVER_350=$FILES_OVER_350 (limit $MAX_FILES_OVER_350)"
echo "MAX_FILE_LOC_OBSERVED=$MAX_FILE_LOC_OBSERVED (limit $MAX_FILE_LOC)"
if [ -n "$MAX_FILE_PATH" ]; then
  echo "MAX_FILE_PATH=$MAX_FILE_PATH"
fi
echo "MAX_PACKAGE_LOC_OBSERVED=$MAX_PACKAGE_LOC_OBSERVED (limit $MAX_PACKAGE_LOC)"
if [ -n "$MAX_PACKAGE_PATH" ]; then
  echo "MAX_PACKAGE_PATH=$MAX_PACKAGE_PATH"
fi
if [ "$REQUIRE_GOFUMPT" -eq 1 ]; then
  if [ "$GOFUMPT_PENDING" -ge 0 ]; then
    echo "GOFUMPT_PENDING=$GOFUMPT_PENDING (limit 0)"
  else
    echo "GOFUMPT_PENDING=missing_tool"
  fi
fi
if [ "$REQUIRE_GOVET" -eq 1 ]; then
  echo "GO_VET_STATUS=$GO_VET_STATUS"
fi
if [ "$REQUIRE_STATICCHECK" -eq 1 ]; then
  echo "STATICCHECK_STATUS=$STATICCHECK_STATUS"
fi
if [ "$REQUIRE_STRUCTURE_LINT" -eq 1 ]; then
  if [ "$STRUCTURE_ISSUES" -ge 0 ]; then
    echo "STRUCTURE_ISSUES=$STRUCTURE_ISSUES (limit $MAX_STRUCTURE_ISSUES)"
  else
    echo "STRUCTURE_ISSUES=missing_tool"
  fi
fi

if [ "$PROD_LOC" -gt "$MAX_PROD_LOC" ]; then
  echo "FAIL: production LOC exceeds limit"
  FAIL=1
fi
if [ "$AVG_FILE_LOC" -gt "$MAX_AVG_FILE_LOC" ]; then
  echo "FAIL: average file size exceeds limit"
  FAIL=1
fi
if [ "$INTERNAL_PACKAGES" -gt "$MAX_INTERNAL_PACKAGES" ]; then
  echo "FAIL: internal package count exceeds limit"
  FAIL=1
fi
if [ "$RUNTIME_DEPS" -gt "$MAX_RUNTIME_DEPS" ]; then
  echo "FAIL: runtime dependency count exceeds limit"
  FAIL=1
fi
if [ "$FILES_OVER_300" -gt "$MAX_FILES_OVER_300" ]; then
  echo "FAIL: too many files exceed 300 LOC"
  FAIL=1
fi
if [ "$FILES_OVER_350" -gt "$MAX_FILES_OVER_350" ]; then
  echo "FAIL: too many files exceed 350 LOC"
  FAIL=1
fi
if [ "$MAX_FILE_LOC_OBSERVED" -gt "$MAX_FILE_LOC" ]; then
  echo "FAIL: largest file exceeds LOC limit"
  FAIL=1
fi
if [ "$MAX_PACKAGE_LOC_OBSERVED" -gt "$MAX_PACKAGE_LOC" ]; then
  echo "FAIL: largest package exceeds LOC limit"
  FAIL=1
fi
if [ "$REQUIRE_GOFUMPT" -eq 1 ]; then
  if [ "$GOFUMPT_PENDING" -lt 0 ]; then
    echo "FAIL: gofumpt is required but missing"
    FAIL=1
  elif [ "$GOFUMPT_PENDING" -gt 0 ]; then
    echo "FAIL: gofumpt found files that need formatting"
    sed -n '1,20p' < <(printf '%s\n' "${GOFMT_PENDING_FILES[@]}")
    FAIL=1
  fi
fi
if [ "$REQUIRE_GOVET" -eq 1 ] && [ "$GO_VET_STATUS" != "pass" ]; then
  echo "FAIL: go vet failed"
  sed -n '1,40p' "$TMPDIR_BUDGET/go_vet.out"
  FAIL=1
fi
if [ "$REQUIRE_STATICCHECK" -eq 1 ]; then
  case "$STATICCHECK_STATUS" in
    pass) ;;
    missing)
      echo "FAIL: staticcheck is required but missing"
      FAIL=1
      ;;
    *)
      echo "FAIL: staticcheck failed"
      sed -n '1,40p' "$TMPDIR_BUDGET/staticcheck.out"
      FAIL=1
      ;;
  esac
fi
if [ "$REQUIRE_STRUCTURE_LINT" -eq 1 ]; then
  if [ "$STRUCTURE_ISSUES" -lt 0 ]; then
    echo "FAIL: golangci-lint is required but missing"
    FAIL=1
  elif [ "$STRUCTURE_ISSUES" -gt "$MAX_STRUCTURE_ISSUES" ]; then
    echo "FAIL: structural lint issue count exceeds limit"
    sed -n '1,40p' "$TMPDIR_BUDGET/structure.out"
    FAIL=1
  fi
fi

if [ "$FAIL" -eq 0 ]; then
  echo "STATUS=PASS"
  exit 0
fi

echo "STATUS=FAIL"
exit 1
