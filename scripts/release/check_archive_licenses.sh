#!/usr/bin/env bash
# Reject a native release archive that does not ship the project license and
# third-party notices. Used by scripts/release/build_release.sh and by CI
# before a release asset is published.
set -euo pipefail

archive="${1:-}"
if [[ -z "${archive}" || ! -f "${archive}" ]]; then
  echo "usage: $0 <needlex_*.(tar.gz|zip)>" >&2
  exit 2
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

case "${archive}" in
  *.tar.gz|*.tgz)
    tar -tzf "${archive}" > "${tmp_dir}/contents.txt"
    ;;
  *.zip)
    unzip -Z1 "${archive}" > "${tmp_dir}/contents.txt"
    ;;
  *)
    echo "unsupported archive format: ${archive}" >&2
    exit 2
    ;;
esac

for required in LICENSE THIRD_PARTY_NOTICES.txt; do
  if ! grep -qx "${required}" "${tmp_dir}/contents.txt"; then
    echo "FAIL: ${archive} is missing ${required}" >&2
    exit 1
  fi
done

case "${archive}" in
  *.tar.gz|*.tgz) tar -xzf "${archive}" -C "${tmp_dir}" LICENSE THIRD_PARTY_NOTICES.txt ;;
  *.zip) unzip -q "${archive}" LICENSE THIRD_PARTY_NOTICES.txt -d "${tmp_dir}" ;;
esac

if ! grep -q "SPDX-License-Identifier: MIT" "${tmp_dir}/LICENSE"; then
  echo "FAIL: ${archive} LICENSE does not declare the expected SPDX license" >&2
  exit 1
fi

if ! grep -q "Josè Pavese" "${tmp_dir}/LICENSE"; then
  echo "FAIL: ${archive} LICENSE is missing the copyright holder" >&2
  exit 1
fi

if ! grep -qi "third-party notices" "${tmp_dir}/THIRD_PARTY_NOTICES.txt"; then
  echo "FAIL: ${archive} THIRD_PARTY_NOTICES.txt is not a notices file" >&2
  exit 1
fi

module_count="$(grep -cE '^- [A-Za-z0-9._~/-]+ [A-Za-z0-9._+~/-]+$' "${tmp_dir}/THIRD_PARTY_NOTICES.txt" || true)"
if [[ "${module_count}" -lt 10 ]]; then
  echo "FAIL: ${archive} THIRD_PARTY_NOTICES.txt lists only ${module_count} modules" >&2
  exit 1
fi

echo "OK: ${archive} ships LICENSE and THIRD_PARTY_NOTICES.txt (${module_count} modules)"
