package transport

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// needlexRepoSlug is the public repository the installer and the shipped agent
// skill are published from.
const needlexRepoSlug = "Josepavese/needlex"

// doctorAgentSkill reports whether a host-agent copy of the shipped skill is
// present and whether its version marker matches the running build. An installed
// skill is a snapshot: hosts read it from disk and never refresh it on their
// own, so drift here means the agent is following older guidance than the tool
// it is calling.
type doctorAgentSkill struct {
	Checked          bool   `json:"checked"`
	Installed        bool   `json:"installed"`
	Path             string `json:"path,omitempty"`
	InstalledVersion string `json:"installed_version,omitempty"`
	ExpectedVersion  string `json:"expected_version,omitempty"`
	Stale            bool   `json:"stale"`
	RefreshCommand   string `json:"refresh_command,omitempty"`
}

var skillVersionPattern = regexp.MustCompile(`(?m)^version:[ \t]*([^ \t\r\n]+)`)

func probeAgentSkill(expectedVersion string, candidates []string) doctorAgentSkill {
	report := doctorAgentSkill{ExpectedVersion: normalizeSkillVersion(expectedVersion)}
	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		report.Installed = true
		report.Path = candidate
		if match := skillVersionPattern.FindSubmatch(data); len(match) == 2 {
			report.InstalledVersion = normalizeSkillVersion(string(match[1]))
		}
		break
	}
	if !report.Installed {
		return report
	}
	report.Checked = report.ExpectedVersion != "" && report.ExpectedVersion != "dev"
	if !report.Checked || report.InstalledVersion == report.ExpectedVersion {
		return report
	}
	report.Stale = true
	report.RefreshCommand = skillRefreshCommand(report.Path)
	return report
}

func normalizeSkillVersion(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(value), "v")
}

// agentSkillCandidatePaths returns known host-agent install locations for the
// shipped skill, most specific first.
func agentSkillCandidatePaths() []string {
	roots := []string{}
	if codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME")); codexHome != "" {
		roots = append(roots, codexHome)
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		roots = append(roots, filepath.Join(home, ".codex"))
	}
	paths := make([]string, 0, len(roots))
	for _, root := range roots {
		paths = append(paths, filepath.Join(root, "skills", "needlex-web-retrieval", "SKILL.md"))
	}
	return paths
}

func skillRefreshCommand(skillPath string) string {
	installer := filepath.Join(filepath.Dir(filepath.Dir(skillPath)), ".system", "skill-installer", "scripts", "install-skill-from-github.py")
	if _, err := os.Stat(installer); err != nil {
		return fmt.Sprintf("reinstall from https://github.com/%s/tree/main/skills/needlex-web-retrieval", needlexRepoSlug)
	}
	return fmt.Sprintf("python3 %s --repo %s --path skills/needlex-web-retrieval", installer, needlexRepoSlug)
}

func agentSkillWarnings(skill doctorAgentSkill) []string {
	if !skill.Stale {
		return nil
	}
	message := fmt.Sprintf("installed agent skill declares no version, this build ships %s", skill.ExpectedVersion)
	if skill.InstalledVersion != "" {
		message = fmt.Sprintf("installed agent skill documents %s, this build ships %s", skill.InstalledVersion, skill.ExpectedVersion)
	}
	if skill.RefreshCommand != "" {
		message += "; refresh with: " + skill.RefreshCommand
	}
	return []string{message}
}
