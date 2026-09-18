#!/usr/bin/env bash
# Enforces the workstation footprint baseline in governance/workstation.env.
#
# Two families of checks:
#   - repository footprint: deterministic, runs everywhere including CI
#   - device footprint: free space and local caches, runs only on a workstation
#
# A device check is never silenced to make a gate green. Reclaim the cache or
# remove the residue instead.
set -euo pipefail

ROOT="${1:-.}"
cd "$ROOT"

BASELINE_FILE="${WORKSTATION_BASELINE_FILE:-governance/workstation.env}"
if [ -f "$BASELINE_FILE" ]; then
  # shellcheck disable=SC1090
  source "$BASELINE_FILE"
fi

MIN_FREE_GB_HARD="${MIN_FREE_GB_HARD:-3}"
MIN_FREE_GB_WARN="${MIN_FREE_GB_WARN:-5}"
MAX_GO_BUILD_CACHE_GB="${MAX_GO_BUILD_CACHE_GB:-3}"
MAX_LARGE_FILE_MB="${MAX_LARGE_FILE_MB:-10}"
MAX_DIST_MB="${MAX_DIST_MB:-200}"
MAX_IMPROVEMENTS_MB="${MAX_IMPROVEMENTS_MB:-12}"
MAX_LOCAL_STATE_MB="${MAX_LOCAL_STATE_MB:-128}"

fail=0
note() { echo "NOTE: $*"; }
warn() { echo "WARN: $*"; }
bad() {
  echo "FAIL: $*"
  fail=1
}

mb_of() { du -sm "$1" 2>/dev/null | awk '{print $1}'; }

device_checks_enabled() {
  [ "${NEEDLEX_SKIP_DEVICE_CHECKS:-0}" = "1" ] && return 1
  [ -n "${CI:-}" ] && return 1
  return 0
}

# --- repository footprint -----------------------------------------------------

largest_files="$(find . -path ./.git -prune -o -path ./dist -prune -o -type f -size "+${MAX_LARGE_FILE_MB}M" -print 2>/dev/null || true)"
if [ -n "$largest_files" ]; then
  bad "files larger than ${MAX_LARGE_FILE_MB} MB in the working tree:"
  printf '  %s\n' $largest_files
fi

if [ -d improvements ]; then
  improvements_mb="$(mb_of improvements)"
  if [ "${improvements_mb:-0}" -gt "$MAX_IMPROVEMENTS_MB" ]; then
    bad "improvements/ holds ${improvements_mb} MB (limit ${MAX_IMPROVEMENTS_MB} MB): benchmark artifacts stay out of the working tree"
  fi
fi

if [ -d dist ]; then
  dist_mb="$(mb_of dist)"
  if [ "${dist_mb:-0}" -gt "$MAX_DIST_MB" ]; then
    bad "dist/ holds ${dist_mb} MB (limit ${MAX_DIST_MB} MB): remove the build output once the release is published"
  else
    note "dist/ holds ${dist_mb} MB of release build output; remove it once published"
  fi
fi

stray_archives="$(find . -path ./.git -prune -o -path ./dist -prune -o -type f \( -name '*.tar.gz' -o -name '*.zip' -o -name '*.tgz' \) -print 2>/dev/null || true)"
if [ -n "$stray_archives" ]; then
  bad "archives outside dist/:"
  printf '  %s\n' $stray_archives
fi

# --- local residue ------------------------------------------------------------

# Small helper directories under /tmp are not the problem; accumulated local
# state and abandoned build trees are.
MIN_RESIDUE_MB=8

residue_mb=0
residue_what=""
for path in .needlex /tmp/needlex-* /tmp/nx[0-9]*; do
  [ -e "$path" ] || continue
  path_mb="$(mb_of "$path")"
  [ "${path_mb:-0}" -ge "$MIN_RESIDUE_MB" ] || continue
  residue_mb=$((residue_mb + path_mb))
  residue_what="${residue_what} ${path}(${path_mb}MB)"
done
if [ "$residue_mb" -gt "$MAX_LOCAL_STATE_MB" ]; then
  bad "local residue holds ${residue_mb} MB (limit ${MAX_LOCAL_STATE_MB} MB):${residue_what}"
elif [ "$residue_mb" -gt 0 ]; then
  note "local residue holds ${residue_mb} MB:${residue_what}"
fi

# --- device footprint (workstation only) --------------------------------------

if device_checks_enabled; then
  free_gb="$(df -Pk . | awk 'NR==2 {print int($4/1048576)}')"
  if [ "$free_gb" -lt "$MIN_FREE_GB_HARD" ]; then
    bad "free space is ${free_gb} GB (hard floor ${MIN_FREE_GB_HARD} GB): reclaim before running builds, tests or releases"
  elif [ "$free_gb" -lt "$MIN_FREE_GB_WARN" ]; then
    warn "free space is ${free_gb} GB (warn floor ${MIN_FREE_GB_WARN} GB): reclaim regenerable caches before heavy work"
  else
    echo "SPACE_FREE_GB=${free_gb}"
  fi

  build_cache_dir="$(go env GOCACHE 2>/dev/null || true)"
  if [ -n "$build_cache_dir" ] && [ -d "$build_cache_dir" ]; then
    build_cache_mb="$(mb_of "$build_cache_dir")"
    build_cache_gb="$(awk -v mb="$build_cache_mb" 'BEGIN {printf "%.1f", mb/1024}')"
    if awk -v gb="$build_cache_gb" -v limit="$MAX_GO_BUILD_CACHE_GB" 'BEGIN {exit !(gb >= limit)}'; then
      warn "Go build cache holds ${build_cache_gb} GB (limit ${MAX_GO_BUILD_CACHE_GB} GB): reclaim with 'go clean -cache'"
    else
      echo "SPACE_GO_BUILD_CACHE_GB=${build_cache_gb}"
    fi
  fi

  if [ -n "${NEEDLEX_HOME:-}" ] && [ -d "${NEEDLEX_HOME}" ]; then
    echo "SPACE_PAL_MB=$(mb_of "${NEEDLEX_HOME}")"
  fi
else
  note "device checks skipped (CI or NEEDLEX_SKIP_DEVICE_CHECKS=1)"
fi

if [ "$fail" -ne 0 ]; then
  echo "FAIL: workstation footprint exceeds governance/workstation.env"
  exit 1
fi

echo "WORKSTATION_STATUS=pass"
