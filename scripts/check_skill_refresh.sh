#!/usr/bin/env bash
# Verifies the installer's host-agent-skill refresh: it backs up the installed copy,
# installs the new one, and restores the backup when the refresh fails. Runs the real
# function extracted from install/install.sh against a throwaway CODEX_HOME.
set -euo pipefail

ROOT="${1:-.}"
cd "$ROOT"

INSTALLER="install/install.sh"
if [ ! -f "$INSTALLER" ]; then
  exit 0
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

harness="$TMP/refresh.sh"
{
  echo '#!/usr/bin/env bash'
  echo 'set -euo pipefail'
  echo 'REPO="Josepavese/needlex"'
  echo 'SKIP_SKILL_REFRESH="${NEEDLEX_INSTALL_SKIP_SKILL_REFRESH:-0}"'
  sed -n '/^refresh_host_agent_skill() {/,/^}$/p' "$INSTALLER"
  echo 'refresh_host_agent_skill'
} >"$harness"
if ! grep -q '^refresh_host_agent_skill$' "$harness"; then
  echo "FAIL: refresh_host_agent_skill not found in $INSTALLER"
  exit 1
fi

staged="$TMP/staged-skill"
codex="$TMP/codex"
mkdir -p "$staged" "$codex/skills/.system/skill-installer/scripts" "$codex/skills/needlex-web-retrieval"
printf -- '---\nname: needlex-web-retrieval\nversion: v9.9.9\n---\n\n# staged\n' >"$staged/SKILL.md"
printf -- '---\nname: needlex-web-retrieval\nversion: v1.0.0\n---\n\n# installed\n' >"$codex/skills/needlex-web-retrieval/SKILL.md"
cat >"$codex/skills/.system/skill-installer/scripts/install-skill-from-github.py" <<'PY'
import os, shutil, sys

home = os.environ["CODEX_HOME"]
dest = os.path.join(home, "skills", "needlex-web-retrieval")
if os.environ.get("FAKE_FAIL") == "1":
    os.makedirs(dest, exist_ok=True)
    open(os.path.join(dest, "PARTIAL"), "w").write("x")
    sys.exit(1)
shutil.rmtree(dest, ignore_errors=True)
shutil.copytree(os.environ["FAKE_SKILL_SRC"], dest)
PY

installed_version() {
  sed -n 's/^version:[[:space:]]*//p' "$codex/skills/needlex-web-retrieval/SKILL.md" | head -n1 | tr -d '[:space:]'
}

export CODEX_HOME="$codex" FAKE_SKILL_SRC="$staged"
fail=0

if ! bash "$harness" >/dev/null; then
  echo "FAIL: successful refresh returned non-zero"
  fail=1
elif [ "$(installed_version)" != "v9.9.9" ]; then
  echo "FAIL: refresh did not install the staged skill (got $(installed_version))"
  fail=1
elif [ -z "$(ls "$codex/skill-backups" 2>/dev/null)" ]; then
  echo "FAIL: refresh did not back up the previous skill copy"
  fail=1
elif [ "$(sed -n 's/^version:[[:space:]]*//p' "$codex"/skill-backups/*/SKILL.md | tr -d '[:space:]')" != "v1.0.0" ]; then
  echo "FAIL: backup does not hold the previous skill copy"
  fail=1
fi

printf -- '---\nname: needlex-web-retrieval\nversion: v1.0.0\n---\n\n# installed\n' >"$codex/skills/needlex-web-retrieval/SKILL.md"
rm -rf "$codex/skill-backups"
if ! FAKE_FAIL=1 bash "$harness" >/dev/null 2>&1; then
  echo "FAIL: failed refresh must not fail the install"
  fail=1
elif [ "$(installed_version)" != "v1.0.0" ]; then
  echo "FAIL: failed refresh did not restore the previous copy (got $(installed_version))"
  fail=1
elif [ -e "$codex/skills/needlex-web-retrieval/PARTIAL" ]; then
  echo "FAIL: failed refresh left partial output in place"
  fail=1
fi

if ! NEEDLEX_INSTALL_SKIP_SKILL_REFRESH=1 bash "$harness" >/dev/null || [ "$(installed_version)" != "v1.0.0" ]; then
  echo "FAIL: NEEDLEX_INSTALL_SKIP_SKILL_REFRESH=1 did not skip the refresh"
  fail=1
fi

if ! CODEX_HOME="$TMP/absent" bash "$harness" >/dev/null; then
  echo "FAIL: refresh must be a no-op when no host skill is installed"
  fail=1
elif [ -d "$TMP/absent/skills/needlex-web-retrieval" ]; then
  echo "FAIL: refresh installed a skill that was not previously present"
  fail=1
fi

exit "$fail"
