#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-.}"
cd "$ROOT"

BASELINE_FILE="${BUDGET_BASELINE_FILE:-governance/budget.env}"
if [ -f "$BASELINE_FILE" ]; then
  # shellcheck disable=SC1090
  source "$BASELINE_FILE"
fi

HARD_MAX_PROD_LOC="${HARD_MAX_PROD_LOC:-26000}"
TARGET_MAX_PROD_LOC="${TARGET_MAX_PROD_LOC:-23500}"
HARD_MAX_INTERNAL_PACKAGES="${HARD_MAX_INTERNAL_PACKAGES:-14}"
TARGET_MAX_INTERNAL_PACKAGES="${TARGET_MAX_INTERNAL_PACKAGES:-11}"
HARD_MAX_RUNTIME_DEPS="${HARD_MAX_RUNTIME_DEPS:-4}"
TARGET_MAX_RUNTIME_DEPS="${TARGET_MAX_RUNTIME_DEPS:-2}"
HARD_MAX_FILE_LOC="${HARD_MAX_FILE_LOC:-1800}"
TARGET_MAX_FILE_LOC="${TARGET_MAX_FILE_LOC:-1400}"
HARD_MAX_AVG_FILE_LOC="${HARD_MAX_AVG_FILE_LOC:-235}"
TARGET_MAX_AVG_FILE_LOC="${TARGET_MAX_AVG_FILE_LOC:-210}"
HARD_MAX_FILES_OVER_300="${HARD_MAX_FILES_OVER_300:-30}"
TARGET_MAX_FILES_OVER_300="${TARGET_MAX_FILES_OVER_300:-22}"
HARD_MAX_FILES_OVER_350="${HARD_MAX_FILES_OVER_350:-22}"
TARGET_MAX_FILES_OVER_350="${TARGET_MAX_FILES_OVER_350:-16}"
HARD_MAX_PACKAGE_LOC="${HARD_MAX_PACKAGE_LOC:-6200}"
TARGET_MAX_PACKAGE_LOC="${TARGET_MAX_PACKAGE_LOC:-5000}"
HARD_MAX_STRUCTURE_ISSUES="${HARD_MAX_STRUCTURE_ISSUES:-0}"
TARGET_MAX_STRUCTURE_ISSUES="${TARGET_MAX_STRUCTURE_ISSUES:-0}"
REQUIRE_GOFUMPT="${REQUIRE_GOFUMPT:-1}"
REQUIRE_GOVET="${REQUIRE_GOVET:-1}"
REQUIRE_STATICCHECK="${REQUIRE_STATICCHECK:-1}"
REQUIRE_STRUCTURE_LINT="${REQUIRE_STRUCTURE_LINT:-1}"
REQUIRE_ADVISORY_LINT="${REQUIRE_ADVISORY_LINT:-1}"

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
  if [ "$lines" -gt 300 ]; then FILES_OVER_300=$((FILES_OVER_300 + 1)); fi
  if [ "$lines" -gt 350 ]; then FILES_OVER_350=$((FILES_OVER_350 + 1)); fi
  if [ "$lines" -gt "$MAX_FILE_LOC_OBSERVED" ]; then
    MAX_FILE_LOC_OBSERVED="$lines"
    MAX_FILE_PATH="$f"
  fi
done

GO_FILE_COUNT="${#GO_FILES[@]}"
AVG_FILE_LOC=0
if [ "$GO_FILE_COUNT" -gt 0 ]; then AVG_FILE_LOC=$((PROD_LOC / GO_FILE_COUNT)); fi

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
    in_block { if ($0 !~ /\/\/ indirect/) count++; next }
    /^require / { if ($0 !~ /\/\/ indirect/) count++ }
    END { print count+0 }
  ' go.mod)
else
  RUNTIME_DEPS=0
fi

GOFUMPT_PENDING=0
GO_VET_STATUS="skipped"
STATICCHECK_STATUS="skipped"
STRUCTURE_ISSUES=0
STRUCTURE_LINT_STATUS="skipped"
ADVISORY_ISSUES=0
ADVISORY_LINT_STATUS="skipped"

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
  if go vet ./... >"$TMPDIR_BUDGET/go_vet.out" 2>&1; then GO_VET_STATUS="pass"; else GO_VET_STATUS="fail"; fi
fi

if [ "$REQUIRE_STATICCHECK" -eq 1 ]; then
  if command -v staticcheck >/dev/null 2>&1; then
    if staticcheck ./... >"$TMPDIR_BUDGET/staticcheck.out" 2>&1; then STATICCHECK_STATUS="pass"; else STATICCHECK_STATUS="fail"; fi
  else
    STATICCHECK_STATUS="missing"
  fi
fi

if [ "$REQUIRE_STRUCTURE_LINT" -eq 1 ]; then
  if command -v golangci-lint >/dev/null 2>&1; then
    set +e
    golangci-lint run --config .golangci.yml ./internal/... ./cmd/... ./schemas/... --output.text.path "$TMPDIR_BUDGET/structure.out" >/dev/null 2>&1
    gc_status=$?
    set -e
    if [ "$gc_status" -eq 0 ]; then
      STRUCTURE_LINT_STATUS="pass"
      STRUCTURE_ISSUES=0
    else
      STRUCTURE_ISSUES=$(grep -c '^[^[:space:]].*:[0-9]\+:' "$TMPDIR_BUDGET/structure.out" || true)
      if [ "$STRUCTURE_ISSUES" -gt 0 ]; then
        STRUCTURE_LINT_STATUS="issues"
      else
        STRUCTURE_LINT_STATUS="error"
      fi
    fi
  else
    STRUCTURE_LINT_STATUS="missing"
    STRUCTURE_ISSUES=-1
  fi
fi

if [ "$REQUIRE_ADVISORY_LINT" -eq 1 ]; then
  if command -v golangci-lint >/dev/null 2>&1; then
    set +e
    golangci-lint run --config governance/golangci.advisory.yml ./... --output.text.path "$TMPDIR_BUDGET/advisory.out" >/dev/null 2>&1
    advisory_status=$?
    set -e
    if [ "$advisory_status" -eq 0 ]; then
      ADVISORY_LINT_STATUS="pass"
    else
      if [ -f "$TMPDIR_BUDGET/advisory.out" ]; then
        ADVISORY_ISSUES=$(grep -c '^[^[:space:]].*:[0-9]\+:' "$TMPDIR_BUDGET/advisory.out" || true)
      else
        ADVISORY_ISSUES=0
      fi
      if [ "${ADVISORY_ISSUES:-0}" -gt 0 ]; then
        ADVISORY_LINT_STATUS="issues"
      else
        ADVISORY_LINT_STATUS="error"
      fi
    fi
  else
    ADVISORY_LINT_STATUS="missing"
  fi
fi

FAIL=0
WARN=0
warn_if_over() {
  local value="$1" target="$2" message="$3"
  if [ "$value" -gt "$target" ]; then
    echo "WARN: $message"
    WARN=1
  fi
}

echo "== Governance Budget Report =="
echo "ROOT=$PWD"
echo "BASELINE_FILE=$BASELINE_FILE"
echo "PROD_LOC=$PROD_LOC (hard $HARD_MAX_PROD_LOC, target $TARGET_MAX_PROD_LOC)"
echo "GO_FILE_COUNT=$GO_FILE_COUNT"
echo "AVG_FILE_LOC=$AVG_FILE_LOC (hard $HARD_MAX_AVG_FILE_LOC, target $TARGET_MAX_AVG_FILE_LOC)"
echo "INTERNAL_PACKAGES=$INTERNAL_PACKAGES (hard $HARD_MAX_INTERNAL_PACKAGES, target $TARGET_MAX_INTERNAL_PACKAGES)"
echo "RUNTIME_DEPS=$RUNTIME_DEPS (hard $HARD_MAX_RUNTIME_DEPS, target $TARGET_MAX_RUNTIME_DEPS)"
echo "FILES_OVER_300=$FILES_OVER_300 (hard $HARD_MAX_FILES_OVER_300, target $TARGET_MAX_FILES_OVER_300)"
echo "FILES_OVER_350=$FILES_OVER_350 (hard $HARD_MAX_FILES_OVER_350, target $TARGET_MAX_FILES_OVER_350)"
echo "MAX_FILE_LOC_OBSERVED=$MAX_FILE_LOC_OBSERVED (hard $HARD_MAX_FILE_LOC, target $TARGET_MAX_FILE_LOC)"
[ -n "$MAX_FILE_PATH" ] && echo "MAX_FILE_PATH=$MAX_FILE_PATH"
echo "MAX_PACKAGE_LOC_OBSERVED=$MAX_PACKAGE_LOC_OBSERVED (hard $HARD_MAX_PACKAGE_LOC, target $TARGET_MAX_PACKAGE_LOC)"
[ -n "$MAX_PACKAGE_PATH" ] && echo "MAX_PACKAGE_PATH=$MAX_PACKAGE_PATH"
if [ "$REQUIRE_GOFUMPT" -eq 1 ]; then
  if [ "$GOFUMPT_PENDING" -ge 0 ]; then echo "GOFUMPT_PENDING=$GOFUMPT_PENDING (hard 0)"; else echo "GOFUMPT_PENDING=missing_tool"; fi
fi
if [ "$REQUIRE_GOVET" -eq 1 ]; then echo "GO_VET_STATUS=$GO_VET_STATUS"; fi
if [ "$REQUIRE_STATICCHECK" -eq 1 ]; then echo "STATICCHECK_STATUS=$STATICCHECK_STATUS"; fi
if [ "$REQUIRE_STRUCTURE_LINT" -eq 1 ]; then
  echo "STRUCTURE_LINT_STATUS=$STRUCTURE_LINT_STATUS"
  if [ "$STRUCTURE_ISSUES" -ge 0 ]; then echo "STRUCTURE_ISSUES=$STRUCTURE_ISSUES (hard $HARD_MAX_STRUCTURE_ISSUES, target $TARGET_MAX_STRUCTURE_ISSUES)"; fi
fi
if [ "$REQUIRE_ADVISORY_LINT" -eq 1 ]; then
  echo "ADVISORY_LINT_STATUS=$ADVISORY_LINT_STATUS"
  echo "ADVISORY_ISSUES=$ADVISORY_ISSUES"
fi

if [ "$PROD_LOC" -gt "$HARD_MAX_PROD_LOC" ]; then echo "FAIL: production LOC exceeds hard limit"; FAIL=1; fi
if [ "$AVG_FILE_LOC" -gt "$HARD_MAX_AVG_FILE_LOC" ]; then echo "FAIL: average file size exceeds hard limit"; FAIL=1; fi
if [ "$INTERNAL_PACKAGES" -gt "$HARD_MAX_INTERNAL_PACKAGES" ]; then echo "FAIL: internal package count exceeds hard limit"; FAIL=1; fi
if [ "$RUNTIME_DEPS" -gt "$HARD_MAX_RUNTIME_DEPS" ]; then echo "FAIL: runtime dependency count exceeds hard limit"; FAIL=1; fi
if [ "$FILES_OVER_300" -gt "$HARD_MAX_FILES_OVER_300" ]; then echo "FAIL: too many files exceed 300 LOC"; FAIL=1; fi
if [ "$FILES_OVER_350" -gt "$HARD_MAX_FILES_OVER_350" ]; then echo "FAIL: too many files exceed 350 LOC"; FAIL=1; fi
if [ "$MAX_FILE_LOC_OBSERVED" -gt "$HARD_MAX_FILE_LOC" ]; then echo "FAIL: largest file exceeds hard LOC limit"; FAIL=1; fi
if [ "$MAX_PACKAGE_LOC_OBSERVED" -gt "$HARD_MAX_PACKAGE_LOC" ]; then echo "FAIL: largest package exceeds hard LOC limit"; FAIL=1; fi

warn_if_over "$PROD_LOC" "$TARGET_MAX_PROD_LOC" "production LOC exceeds target"
warn_if_over "$AVG_FILE_LOC" "$TARGET_MAX_AVG_FILE_LOC" "average file size exceeds target"
warn_if_over "$INTERNAL_PACKAGES" "$TARGET_MAX_INTERNAL_PACKAGES" "internal package count exceeds target"
warn_if_over "$RUNTIME_DEPS" "$TARGET_MAX_RUNTIME_DEPS" "runtime dependency count exceeds target"
warn_if_over "$FILES_OVER_300" "$TARGET_MAX_FILES_OVER_300" "too many files exceed 300 LOC target"
warn_if_over "$FILES_OVER_350" "$TARGET_MAX_FILES_OVER_350" "too many files exceed 350 LOC target"
warn_if_over "$MAX_FILE_LOC_OBSERVED" "$TARGET_MAX_FILE_LOC" "largest file exceeds target LOC"
warn_if_over "$MAX_PACKAGE_LOC_OBSERVED" "$TARGET_MAX_PACKAGE_LOC" "largest package exceeds target LOC"
warn_if_over "$STRUCTURE_ISSUES" "$TARGET_MAX_STRUCTURE_ISSUES" "structural lint issues exceed target"

if [ "$REQUIRE_GOFUMPT" -eq 1 ]; then
  if [ "$GOFUMPT_PENDING" -lt 0 ]; then echo "FAIL: gofumpt is required but missing"; FAIL=1
  elif [ "$GOFUMPT_PENDING" -gt 0 ]; then echo "FAIL: gofumpt found files that need formatting"; sed -n '1,20p' < <(printf '%s\n' "${GOFMT_PENDING_FILES[@]}"); FAIL=1; fi
fi
if [ "$REQUIRE_GOVET" -eq 1 ] && [ "$GO_VET_STATUS" != "pass" ]; then echo "FAIL: go vet failed"; sed -n '1,40p' "$TMPDIR_BUDGET/go_vet.out"; FAIL=1; fi
if [ "$REQUIRE_STATICCHECK" -eq 1 ]; then
  case "$STATICCHECK_STATUS" in
    pass) ;;
    missing) echo "FAIL: staticcheck is required but missing"; FAIL=1 ;;
    *) echo "FAIL: staticcheck failed"; sed -n '1,40p' "$TMPDIR_BUDGET/staticcheck.out"; FAIL=1 ;;
  esac
fi
if [ "$REQUIRE_STRUCTURE_LINT" -eq 1 ]; then
  case "$STRUCTURE_LINT_STATUS" in
    pass) ;;
    missing) echo "FAIL: golangci-lint is required but missing"; FAIL=1 ;;
    error) echo "FAIL: structural lint execution failed"; sed -n '1,40p' "$TMPDIR_BUDGET/structure.out"; FAIL=1 ;;
    issues)
      if [ "$STRUCTURE_ISSUES" -gt "$HARD_MAX_STRUCTURE_ISSUES" ]; then
        echo "FAIL: structural lint issue count exceeds hard limit"
        sed -n '1,40p' "$TMPDIR_BUDGET/structure.out"
        FAIL=1
      fi
      ;;
  esac
fi
if [ "$REQUIRE_ADVISORY_LINT" -eq 1 ] && [ "$ADVISORY_LINT_STATUS" = "issues" ]; then
  echo "== Advisory Lint Sample =="
  sed -n '1,20p' "$TMPDIR_BUDGET/advisory.out"
fi

if [ "$FAIL" -ne 0 ]; then
  echo "STATUS=FAIL"
  exit 1
fi
if [ "$WARN" -ne 0 ]; then
  echo "STATUS=PASS_WITH_WARNINGS"
else
  echo "STATUS=PASS"
fi
