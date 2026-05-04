#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-.}"
cd "$ROOT"

scan_paths=(
  AGENTS.md
  README.md
  benchmarks
  docs
  internal
  schemas
  skills
)

bad_terms=(
  "lexi""cal"
  "key""word"
  "[Ll]it""eral"
  "URL""TokenText"
  "Host""TokenText"
  "Score""Candidates"
  "Score""URL"
  "native""SemanticVector"
  "sparse""Cosine"
  "domain""_hint_match"
  "path""_hint"
  "goal""_label_alignment"
  "to""k:"
)

pattern="$(IFS='|'; echo "${bad_terms[*]}")"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

existing_paths=()
for path in "${scan_paths[@]}"; do
  if [ -e "$path" ]; then
    existing_paths+=("$path")
  fi
done

set +e
if command -v rg >/dev/null 2>&1; then
  rg -n --hidden \
    --glob '!docs/assets/**' \
    --glob '!docs/archive/**/node_modules/**' \
    --glob '!**/*.png' \
    --glob '!**/*.jpg' \
    --glob '!**/*.jpeg' \
    --glob '!**/*.webp' \
    --glob '!**/*.gif' \
    --glob '!**/*.ico' \
    --glob '!**/*.zip' \
    --glob '!**/*.tar.gz' \
    -e "$pattern" "${existing_paths[@]}" >"$tmp"
  search_status=$?
else
  grep -RInE \
    --exclude='*.png' \
    --exclude='*.jpg' \
    --exclude='*.jpeg' \
    --exclude='*.webp' \
    --exclude='*.gif' \
    --exclude='*.ico' \
    --exclude='*.zip' \
    --exclude='*.tar.gz' \
    --exclude-dir='assets' \
    --exclude-dir='node_modules' \
    "$pattern" "${existing_paths[@]}" >"$tmp"
  search_status=$?
fi
set -e

if [ "$search_status" -eq 0 ]; then
  echo "FAIL: semantic guard found banned surface-form retrieval residues"
  cat "$tmp"
  exit 1
fi

if [ "$search_status" -gt 1 ]; then
  echo "FAIL: semantic guard search failed"
  cat "$tmp"
  exit 1
fi

grep -q 'Embeddings-first, semantic-first' AGENTS.md || {
  echo "FAIL: AGENTS.md must declare embeddings-first semantics"
  exit 1
}

grep -q 'semantic/embedding alignment' AGENTS.md || {
  echo "FAIL: AGENTS.md must require text ranking through semantic/embedding alignment"
  exit 1
}

grep -q 'ScoreStructuralCandidates' internal/core/discovery/types.go || {
  echo "FAIL: discovery structural prior entrypoint missing"
  exit 1
}

grep -q 'nativeEmbeddingFeatures' internal/intel/native_semantic.go || {
  echo "FAIL: native embedding fallback entrypoint missing"
  exit 1
}

echo "SEMANTIC_GUARD_STATUS=pass"
