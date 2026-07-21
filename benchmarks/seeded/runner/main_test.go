package main

import (
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestSummarize(t *testing.T) {
	rows := []caseResult{
		{Family: "docs", RuntimeOK: true, QualityPass: true, Pass: true, SelectedURLPass: true, ProofUsable: true, LatencyMS: 100, PacketBytes: 1000},
		{Family: "docs", RuntimeOK: true, QualityPass: false, Pass: false, SelectedURLPass: false, ProofUsable: true, LatencyMS: 200, PacketBytes: 2000, FailureClasses: []string{"wrong_selected_url"}},
		{Family: "corporate", RuntimeOK: false, QualityPass: false, Pass: false, SelectedURLPass: false, ProofUsable: false, LatencyMS: 0, PacketBytes: 0, FailureClasses: []string{"network_timeout"}},
	}

	s := summarize(rows)
	if s.CaseCount != 3 {
		t.Fatalf("expected 3 cases, got %d", s.CaseCount)
	}
	if s.FailureClassCounts["wrong_selected_url"] != 1 {
		t.Fatalf("expected wrong_selected_url count 1, got %#v", s.FailureClassCounts)
	}
	if s.FailureClassCounts["network_timeout"] != 1 {
		t.Fatalf("expected network_timeout count 1, got %#v", s.FailureClassCounts)
	}
	if s.RuntimeSuccessRate != 2.0/3.0 {
		t.Fatalf("expected runtime success rate 2/3, got %v", s.RuntimeSuccessRate)
	}
	if s.QualityPassRate != 1.0/3.0 {
		t.Fatalf("expected quality pass rate 1/3, got %v", s.QualityPassRate)
	}
	if len(s.FamilyBreakdown) != 2 {
		t.Fatalf("expected 2 family breakdown rows, got %d", len(s.FamilyBreakdown))
	}
}

func TestClassifyExecutionError(t *testing.T) {
	tests := []struct {
		errText string
		want    string
	}{
		{"read failed: context deadline exceeded", "network_timeout"},
		{"fetch page: tls handshake failure", "network_tls_error"},
		{"dial tcp: connection refused", "network_connect_error"},
		{"something else", "runtime_error"},
	}
	for _, tt := range tests {
		if got := classifyExecutionError(tt.errText); got != tt.want {
			t.Fatalf("classifyExecutionError(%q)=%q want %q", tt.errText, got, tt.want)
		}
	}
}

func TestSameCanonicalURLAcceptsAgentReadableMarkdownEquivalent(t *testing.T) {
	if !sameCanonicalURL("https://react.dev/reference/react.md", "https://react.dev/reference/react") {
		t.Fatal("expected markdown agent-readable URL to match source document")
	}
	if !sameCanonicalURL("https://docs.z.ai/devpack/overview.md", "https://docs.z.ai/devpack/overview") {
		t.Fatal("expected markdown agent-readable URL to match extensionless document")
	}
	if sameCanonicalURL("https://docs.python.org/3/library/json.md", "https://docs.python.org/3/library/asyncio.html") {
		t.Fatal("unexpected match across different documents")
	}
}

func TestDefaultCorpusV2HasHundredUniqueCases(t *testing.T) {
	corpusPath := filepath.Join("..", "..", "corpora", "seeded-corpus-v2.json")
	c, err := loadCorpus(corpusPath)
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
	if c.Version != "seeded-corpus-v2" {
		t.Fatalf("expected seeded-corpus-v2, got %q", c.Version)
	}
	if len(c.Cases) != 100 {
		t.Fatalf("expected 100 cases, got %d", len(c.Cases))
	}

	ids := map[string]struct{}{}
	families := map[string]int{}
	for _, item := range c.Cases {
		if strings.TrimSpace(item.ID) == "" {
			t.Fatalf("empty case id: %#v", item)
		}
		if _, exists := ids[item.ID]; exists {
			t.Fatalf("duplicate case id %q", item.ID)
		}
		ids[item.ID] = struct{}{}
		families[item.Family]++
		assertBenchmarkURL(t, "seed_url", item.ID, item.SeedURL)
		assertBenchmarkURL(t, "expected_url", item.ID, item.ExpectedURL)
		if strings.TrimSpace(item.ExpectedDomain) == "" {
			t.Fatalf("case %s has empty expected_domain", item.ID)
		}
		if item.TaskType == "same_site_query_routing" && strings.TrimSpace(item.Goal) == "" {
			t.Fatalf("case %s is same-site routing without goal", item.ID)
		}
		if item.MustExposeProof != true {
			t.Fatalf("case %s must require proof", item.ID)
		}
	}
	for _, family := range []string{"same_site_query", "docs", "multilingual", "corporate", "classic_homepage"} {
		if families[family] == 0 {
			t.Fatalf("expected at least one %s case, got %#v", family, families)
		}
	}
}

func assertBenchmarkURL(t *testing.T, field, id, raw string) {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("case %s invalid %s %q: %v", id, field, raw, err)
	}
	if parsed.Scheme != "https" || strings.TrimSpace(parsed.Host) == "" {
		t.Fatalf("case %s invalid %s %q", id, field, raw)
	}
}
