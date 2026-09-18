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

grep -q 'Render and Application-Data Delivery Principles' AGENTS.md || {
  echo "FAIL: AGENTS.md must declare render application-data delivery doctrine"
  exit 1
}

grep -q 'Degradation is reported, never silent' AGENTS.md || {
  echo "FAIL: AGENTS.md must require reported render degradation"
  exit 1
}

grep -q 'content_source' skills/needlex-web-retrieval/SKILL.md || {
  echo "FAIL: shipped skill must document signals.content_source"
  exit 1
}

grep -q 'content_source' docs/agent-answer-packet.md || {
  echo "FAIL: packet contract must document signals.content_source"
  exit 1
}

grep -q 'Installed agent guidance must be able to go stale visibly' AGENTS.md || {
  echo "FAIL: AGENTS.md must require visible drift for installed agent guidance"
  exit 1
}

grep -Eq '^version:[[:space:]]*v?[0-9]+\.[0-9]+\.[0-9]+' skills/needlex-web-retrieval/SKILL.md || {
  echo "FAIL: shipped skill must declare the released contract version"
  exit 1
}

grep -q 'Workstation and Artifact Footprint' AGENTS.md || {
  echo "FAIL: AGENTS.md must declare workstation footprint doctrine"
  exit 1
}

grep -q 'space-constrained' AGENTS.md || {
  echo "FAIL: AGENTS.md must state that the development device is space-constrained"
  exit 1
}

grep -q '^MIN_FREE_GB_HARD=' governance/workstation.env || {
  echo "FAIL: governance/workstation.env must define the device space policy"
  exit 1
}

echo "SEMANTIC_GUARD_STATUS=pass"
