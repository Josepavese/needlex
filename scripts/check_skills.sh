#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-.}"
cd "$ROOT"

if [ ! -d skills ]; then
  exit 0
fi

fail=0
while IFS= read -r -d '' skill; do
  first_line="$(sed -n '1p' "$skill")"
  if [ "$first_line" != "---" ]; then
    echo "FAIL: $skill missing frontmatter delimiter"
    fail=1
  fi
  if ! grep -Eq '^name:[[:space:]]*[^[:space:]]+' "$skill"; then
    echo "FAIL: $skill missing name frontmatter"
    fail=1
  fi
  if ! grep -Eq '^description:[[:space:]]*.+' "$skill"; then
    echo "FAIL: $skill missing description frontmatter"
    fail=1
  fi
  if ! grep -Eq '^# ' "$skill"; then
    echo "FAIL: $skill missing top-level heading"
    fail=1
  fi
done < <(find skills -mindepth 2 -maxdepth 2 -name SKILL.md -print0)

exit "$fail"
