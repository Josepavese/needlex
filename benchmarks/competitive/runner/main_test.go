package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAdapters(t *testing.T) {
	adapters, err := resolveAdapters([]string{"needlex", "jina", "firecrawl", "tavily", "exa", "brave-search", "vercel-browser-agent"})
	if err != nil {
		t.Fatalf("resolveAdapters failed: %v", err)
	}
	if len(adapters) != 7 {
		t.Fatalf("expected 7 adapters, got %d", len(adapters))
	}
}

func TestVercelBrowserAgentAvailabilityRequiresEndpoint(t *testing.T) {
	t.Setenv("VERCEL_BROWSER_AGENT_ENDPOINT", "")
	a := vercelBrowserAgentAdapter{}
	ok, reason := a.Availability()
	if ok || reason != "missing_env:VERCEL_BROWSER_AGENT_ENDPOINT" {
		t.Fatalf("unexpected availability ok=%v reason=%q", ok, reason)
	}
}

func TestSummarizeByCompetitor(t *testing.T) {
	adapters := []adapter{
		envSkippedAdapter{name: "firecrawl", category: "market_reference", envVar: "MISSING"},
		jinaAdapter{},
	}
	hop := 1
	claimSteps := 2
	rows := []caseResult{{
		ID: "case-1",
		Results: []competitorResult{
			{Competitor: "jina", Category: "simple_baseline", Configured: true, Supported: true, RuntimeOK: true, QualityPass: false, SelectedURLPass: false, ProofUsable: false, FactCoverage: 0.5, PacketBytes: 100, FailureClasses: []string{"wrong_selected_url"}},
			{Competitor: "firecrawl", Category: "market_reference", Configured: false, Skipped: true, SkipReason: "missing_env:FIRECRAWL_API_KEY"},
		},
	}, {
		ID: "case-2",
		Results: []competitorResult{
			{Competitor: "jina", Category: "simple_baseline", Configured: true, Supported: true, RuntimeOK: true, QualityPass: true, SelectedURLPass: true, ProofUsable: false, FactCoverage: 1, PacketBytes: 200, HopCountToTarget: &hop, ClaimToSourceSteps: &claimSteps},
		},
	}}
	summary := summarizeByCompetitor(rows, adapters)
	if len(summary) != 2 {
		t.Fatalf("expected 2 summary rows, got %d", len(summary))
	}
	if summary[0].Competitor != "firecrawl" || summary[0].SkippedCases != 1 {
		t.Fatalf("unexpected firecrawl summary %#v", summary[0])
	}
	if summary[1].Competitor != "jina" || summary[1].RuntimeSuccessRate != 1 {
		t.Fatalf("unexpected jina summary %#v", summary[1])
	}
	if summary[1].FactCoverageRate != 0.75 {
		t.Fatalf("unexpected fact coverage %#v", summary[1])
	}
	if summary[1].AvgHopCountToTarget != 1 {
		t.Fatalf("unexpected hop count %#v", summary[1])
	}
	if summary[1].PacketReductionVsBaseline != 0 {
		t.Fatalf("unexpected packet reduction %#v", summary[1])
	}
	if summary[1].ClaimToSourceCoverageRate != 0.5 {
		t.Fatalf("unexpected claim/source coverage %#v", summary[1])
	}
	if summary[1].AvgClaimToSourceSteps != 2 {
		t.Fatalf("unexpected claim/source steps %#v", summary[1])
	}
	if summary[1].AvgPostProcessingBurden != 0 {
		t.Fatalf("unexpected post-processing burden %#v", summary[1])
	}
}

func TestClassifyExecutionErrorCompetitive(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"context deadline exceeded", "network_timeout"},
		{"tls handshake failure", "network_tls_error"},
		{"connection refused", "network_connect_error"},
		{"boom", "runtime_error"},
	}
	for _, tt := range tests {
		if got := classifyExecutionError(tt.in); got != tt.want {
			t.Fatalf("classifyExecutionError(%q)=%q want %q", tt.in, got, tt.want)
		}
	}
}

func TestClassifyQualityFailuresSkipsProofWhenNotComparable(t *testing.T) {
	res := competitorResult{
		SelectedURLPass: true,
		SummaryPresent:  true,
		ChunkCount:      1,
		FactCoverage:    1,
		ProofUsable:     false,
	}
	item := seededCase{MustExposeProof: true}
	fails := classifyQualityFailures(res, item, false)
	if len(fails) != 0 {
		t.Fatalf("expected no failures, got %#v", fails)
	}
}

func TestEvaluateFactCoverage(t *testing.T) {
	text := "Python packaging uses pip for installing packages."
	got, covered, missing := evaluateFactCoverage(text, []string{"pip", "installing packages"})
	if got != 1 {
		t.Fatalf("expected full coverage, got %v", got)
	}
	if len(covered) != 2 || len(missing) != 0 {
		t.Fatalf("unexpected coverage covered=%v missing=%v", covered, missing)
	}
}

func TestEstimateHopCountToTarget(t *testing.T) {
	item := seededCase{
		TaskType:    "same_site_query_routing",
		SeedURL:     "https://example.com",
		ExpectedURL: "https://example.com/docs/intro",
	}
	hops := estimateHopCountToTarget(item, true)
	if hops == nil || *hops != 1 {
		t.Fatalf("unexpected hops %#v", hops)
	}
}

func TestEstimateClaimToSourceSteps(t *testing.T) {
	steps := estimateClaimToSourceSteps(competitorResult{SummaryPresent: true, ProofUsable: true})
	if steps == nil || *steps != 1 {
		t.Fatalf("unexpected proof-backed steps %#v", steps)
	}
	steps = estimateClaimToSourceSteps(competitorResult{SummaryPresent: true, ProofUsable: false})
	if steps == nil || *steps != 2 {
		t.Fatalf("unexpected non-proof steps %#v", steps)
	}
}

func TestEstimatePostProcessingBurden(t *testing.T) {
	item := seededCase{TaskType: "same_site_query_routing", MustExposeProof: true}
	got := estimatePostProcessingBurden(item, competitorResult{
		SummaryPresent:  true,
		ChunkCount:      1,
		PacketBytes:     20000,
		SelectionWhy:    nil,
		ProofUsable:     false,
		SelectedURLPass: false,
	})
	if got != 4 {
		t.Fatalf("unexpected burden %d", got)
	}
}

func TestRunCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.json")
	cache := runCache{
		Entries: map[string]competitorResult{
			"abc": {Competitor: "firecrawl", QualityPass: true},
		},
	}
	if err := saveRunCache(path, cache); err != nil {
		t.Fatalf("saveRunCache failed: %v", err)
	}
	got, err := loadRunCache(path)
	if err != nil {
		t.Fatalf("loadRunCache failed: %v", err)
	}
	if !got.Entries["abc"].QualityPass {
		t.Fatalf("unexpected cache %#v", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected cache file: %v", err)
	}
}

func TestCacheableCompetitor(t *testing.T) {
	if !cacheableCompetitor("firecrawl") || cacheableCompetitor("needlex") {
		t.Fatalf("unexpected cacheable competitor behavior")
	}
}

func TestVercelBrowserAgentRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"selected_url":"https://docs.z.ai/devpack/overview",
			"summary":"supports coding tools and lists supported models or plans",
			"chunks":[{"text":"supports coding tools and lists supported models or plans"}],
			"latency_ms":321
		}`))
	}))
	defer srv.Close()

	t.Setenv("VERCEL_BROWSER_AGENT_ENDPOINT", srv.URL)
	a := vercelBrowserAgentAdapter{}
	res := a.Run(t.Context(), seededCase{
		ID:               "zai-coding-plan",
		SeedURL:          "https://docs.z.ai/guides/overview/quick-start",
		TaskType:         "same_site_query_routing",
		Goal:             "coding plan",
		ExpectedURL:      "https://docs.z.ai/devpack/overview",
		ExpectedDomain:   "docs.z.ai",
		MustContainFacts: []string{"supports coding tools", "lists supported models or plans"},
		MustExposeProof:  false,
	})
	if !res.RuntimeOK || !res.SelectedURLPass || !res.QualityPass {
		t.Fatalf("unexpected result %#v", res)
	}
	if res.FactCoverage != 1 {
		t.Fatalf("unexpected fact coverage %#v", res)
	}
}
