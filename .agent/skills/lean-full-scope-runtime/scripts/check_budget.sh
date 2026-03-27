#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-.}"
MAX_PROD_LOC="${MAX_PROD_LOC:-8000}"
MAX_INTERNAL_PACKAGES="${MAX_INTERNAL_PACKAGES:-10}"
MAX_RUNTIME_DEPS="${MAX_RUNTIME_DEPS:-8}"
MAX_FILE_LOC="${MAX_FILE_LOC:-400}"

cd "$ROOT"

# Count production Go LOC (exclude tests/vendor/generated-like paths).
mapfile -t GO_FILES < <(find . -type f -name '*.go' \
  ! -name '*_test.go' \
  ! -path './vendor/*' \
  ! -path './.git/*' \
  ! -path './testdata/*' \
  ! -path './.agent/*' \
  ! -path './docs/*' \
  | sort)

PROD_LOC=0
MAX_FILE_LOC_OBSERVED=0
MAX_FILE_PATH=""

if [ "${#GO_FILES[@]}" -gt 0 ]; then
  for f in "${GO_FILES[@]}"; do
    lines=$(wc -l < "$f")
    PROD_LOC=$((PROD_LOC + lines))
    if [ "$lines" -gt "$MAX_FILE_LOC_OBSERVED" ]; then
      MAX_FILE_LOC_OBSERVED="$lines"
      MAX_FILE_PATH="$f"
    fi
  done
fi

# Count internal packages as first-level dirs under internal/.
if [ -d internal ]; then
  INTERNAL_PACKAGES=$(find internal -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ')
else
  INTERNAL_PACKAGES=0
fi

# Count direct runtime deps from go.mod (exclude indirect).
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

FAIL=0

echo "== Lean Budget Report =="
echo "ROOT=$PWD"
echo "PROD_LOC=$PROD_LOC (limit $MAX_PROD_LOC)"
echo "INTERNAL_PACKAGES=$INTERNAL_PACKAGES (limit $MAX_INTERNAL_PACKAGES)"
echo "RUNTIME_DEPS=$RUNTIME_DEPS (limit $MAX_RUNTIME_DEPS)"
echo "MAX_FILE_LOC_OBSERVED=$MAX_FILE_LOC_OBSERVED (limit $MAX_FILE_LOC)"
if [ -n "$MAX_FILE_PATH" ]; then
  echo "MAX_FILE_PATH=$MAX_FILE_PATH"
fi

if [ "$PROD_LOC" -gt "$MAX_PROD_LOC" ]; then
  echo "FAIL: production LOC exceeds limit"
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
if [ "$MAX_FILE_LOC_OBSERVED" -gt "$MAX_FILE_LOC" ]; then
  echo "FAIL: largest file exceeds LOC limit"
  FAIL=1
fi

if [ "$FAIL" -eq 0 ]; then
  echo "STATUS=PASS"
  exit 0
fi

echo "STATUS=FAIL"
exit 1
