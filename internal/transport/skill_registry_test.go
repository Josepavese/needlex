package transport

import (
	"os"
	"regexp"
	"testing"
)

func TestNeedleXSkillMentionsKnownMCPTools(t *testing.T) {
	data, err := os.ReadFile("../../skills/needlex-web-retrieval/SKILL.md")
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}
	known := map[string]struct{}{}
	for _, tool := range mcpTools() {
		known[tool.Name] = struct{}{}
	}
	matches := regexp.MustCompile(`\b(?:web|memory|analytics)_[a-z_]+\b`).FindAllString(string(data), -1)
	if len(matches) == 0 {
		t.Fatal("expected skill to mention MCP tools")
	}
	for _, name := range matches {
		if _, ok := known[name]; !ok {
			t.Fatalf("skill mentions unknown MCP tool %q", name)
		}
	}
}
