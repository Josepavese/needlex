#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-.}"
cd "$ROOT"

scan_paths=(internal benchmarks scripts README.md docs schemas skills AGENTS.md)

bad_terms=(
  "URL""TokenText"
  "Host""TokenText"
  "Score""Candidates"
  "Score""URL"
  "native""SemanticVector"
  "sparse""Cosine"
  "Native""Semantic"
  "Native""TextEmbedder"
  "native""TextEmbedding"
  "native""Embedding"
  "Dense""Semantic""Model"
  "cfg\\.Semantic\\.""Model"
  "Semantic\\.""Model"
  "semantic\\.""model"
  "semantic\\.""backend"
  "embedding""_backend"
  "memory\\.embedding""_model"
  "NEEDLEX_SEMANTIC_""MODEL"
  "NEEDLEX_SEMANTIC_""ENABLED"
  "NEEDLEX_MEMORY_EMBEDDING""_MODEL"
  "needlex-dense-""embedding-v1"
  "native Needle-X semantic ""vectorizer"
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

grep -q 'DenseSemanticAligner' internal/intel/dense_semantic.go || {
  echo "FAIL: dense semantic aligner entrypoint missing"
  exit 1
}

grep -q 'DenseHTTPTextEmbedder' internal/intel/embedder.go || {
  echo "FAIL: dense HTTP embedder entrypoint missing"
  exit 1
}

echo "SEMANTIC_GUARD_STATUS=pass"
