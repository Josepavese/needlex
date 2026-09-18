package transport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestSkill(t *testing.T, dir, version string) string {
	t.Helper()
	skillDir := filepath.Join(dir, "skills", "needlex-web-retrieval")
	if err := os.MkdirAll(skillDir, 0o755); err != nil { //nolint:gosec
		t.Fatalf("create skill dir: %v", err)
	}
	path := filepath.Join(skillDir, "SKILL.md")
	content := "---\nname: needlex-web-retrieval\nversion: " + version + "\ndescription: test fixture\n---\n\n# Test\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { //nolint:gosec
		t.Fatalf("write skill: %v", err)
	}
	return path
}

func TestProbeAgentSkillReportsStaleInstalledCopy(t *testing.T) {
	path := writeTestSkill(t, t.TempDir(), "v0.1.33")
	report := probeAgentSkill("v0.1.36", []string{path})

	if !report.Installed || !report.Checked || !report.Stale {
		t.Fatalf("expected stale installed copy, got %#v", report)
	}
	if report.InstalledVersion != "0.1.33" || report.ExpectedVersion != "0.1.36" {
		t.Fatalf("expected normalized reported versions, got %#v", report)
	}
	if !strings.Contains(report.RefreshCommand, needlexRepoSlug) {
		t.Fatalf("expected refresh command with repo slug, got %q", report.RefreshCommand)
	}
}

func TestProbeAgentSkillAcceptsMatchingVersion(t *testing.T) {
	path := writeTestSkill(t, t.TempDir(), "0.1.36")
	report := probeAgentSkill("v0.1.36", []string{path})

	if !report.Checked || report.Stale {
		t.Fatalf("expected matching version to stay current, got %#v", report)
	}
	if report.RefreshCommand != "" {
		t.Fatalf("expected no refresh command for a current copy, got %q", report.RefreshCommand)
	}
}

func TestProbeAgentSkillSkipsComparisonForDevBuilds(t *testing.T) {
	path := writeTestSkill(t, t.TempDir(), "v0.1.33")
	report := probeAgentSkill("dev", []string{path})

	if report.Checked || report.Stale {
		t.Fatalf("expected dev builds to skip the comparison, got %#v", report)
	}
	if !report.Installed || report.InstalledVersion != "0.1.33" {
		t.Fatalf("expected installed copy to stay reported, got %#v", report)
	}
}

func TestProbeAgentSkillReportsInstalledCopyWithoutVersionMarker(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "needlex-web-retrieval")
	if err := os.MkdirAll(skillDir, 0o755); err != nil { //nolint:gosec
		t.Fatalf("create skill dir: %v", err)
	}
	path := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(path, []byte("---\nname: needlex-web-retrieval\ndescription: test fixture\n---\n\n# Test\n"), 0o644); err != nil { //nolint:gosec
		t.Fatalf("write skill: %v", err)
	}

	report := probeAgentSkill("v0.1.36", []string{path})
	if !report.Checked || !report.Stale || report.InstalledVersion != "" {
		t.Fatalf("expected an unversioned installed copy to be reported stale, got %#v", report)
	}
}

func TestProbeAgentSkillReportsMissingInstall(t *testing.T) {
	report := probeAgentSkill("v0.1.36", []string{filepath.Join(t.TempDir(), "skills", "needlex-web-retrieval", "SKILL.md")})

	if report.Installed || report.Checked || report.Stale {
		t.Fatalf("expected no install to be reported, got %#v", report)
	}
}

func TestProbeAgentSkillUsesHostInstallerWhenPresent(t *testing.T) {
	dir := t.TempDir()
	path := writeTestSkill(t, dir, "v0.1.33")
	installerDir := filepath.Join(dir, "skills", ".system", "skill-installer", "scripts")
	if err := os.MkdirAll(installerDir, 0o755); err != nil { //nolint:gosec
		t.Fatalf("create installer dir: %v", err)
	}
	installer := filepath.Join(installerDir, "install-skill-from-github.py")
	if err := os.WriteFile(installer, []byte("#!/usr/bin/env python3\n"), 0o755); err != nil { //nolint:gosec
		t.Fatalf("write installer: %v", err)
	}

	report := probeAgentSkill("v0.1.36", []string{path})
	if !strings.Contains(report.RefreshCommand, installer) {
		t.Fatalf("expected host installer in refresh command, got %q", report.RefreshCommand)
	}
}

func TestAgentSkillWarningsSurfaceStaleCopy(t *testing.T) {
	stale := doctorAgentSkill{Stale: true, InstalledVersion: "0.1.33", ExpectedVersion: "0.1.36", RefreshCommand: "refresh-now"}
	warnings := agentSkillWarnings(stale)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "documents 0.1.33") || !strings.Contains(warnings[0], "ships 0.1.36") || !strings.Contains(warnings[0], "refresh-now") {
		t.Fatalf("expected a drift warning naming both versions and the refresh command, got %#v", warnings)
	}
	if got := agentSkillWarnings(doctorAgentSkill{}); got != nil {
		t.Fatalf("expected no warnings for a current copy, got %#v", got)
	}
	unversioned := agentSkillWarnings(doctorAgentSkill{Stale: true, ExpectedVersion: "0.1.36"})
	if len(unversioned) != 1 || !strings.Contains(unversioned[0], "declares no version") {
		t.Fatalf("expected an unversioned copy to be described as such, got %#v", unversioned)
	}
}
