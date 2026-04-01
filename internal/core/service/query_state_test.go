package service

import (
	"context"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/memory"
	"github.com/josepavese/needlex/internal/proof"
	"github.com/josepavese/needlex/internal/store"
)

func TestSelectGraphExpansionMatchesFiltersLowScore(t *testing.T) {
	matches := []store.DomainMatch{
		{Domain: "low.example", Score: 2.4, Reason: []string{"outbound_transition"}},
		{Domain: "high.example", Score: 2.7, Reason: []string{"outbound_transition"}},
		{Domain: "inbound-only.example", Score: 3.0, Reason: []string{"inbound_transition"}},
		{Domain: "very-high.example", Score: 4.1, Reason: []string{"inbound_transition"}},
	}
	filtered := selectGraphExpansionMatches(matches)
	if len(filtered) != 3 {
		t.Fatalf("expected 3 filtered matches, got %d", len(filtered))
	}
	if containsString(domainsFromGraphMatches(filtered), "inbound-only.example") {
		t.Fatalf("did not expect inbound-only match in %#v", filtered)
	}
	if !containsString(domainsFromGraphMatches(filtered), "very-high.example") {
		t.Fatalf("expected very-high.example in %#v", filtered)
	}
}

func TestPrepareQueryRequestWithLocalStateGraphExpansionNeedsConfidence(t *testing.T) {
	root := t.TempDir()
	graph := store.NewDomainGraphStore(root)

	if _, _, err := graph.Observe("https://seed.example/root", "https://expansion.example/docs", "query_discovery"); err != nil {
		t.Fatalf("seed graph once: %v", err)
	}
	first := PrepareQueryRequestWithLocalState(root, QueryRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       "https://seed.example/root",
		DiscoveryMode: QueryDiscoverySameSite,
	}, config.Defaults(), intel.NoopSemanticAligner{})
	if containsString(first.DomainHints, "expansion.example") {
		t.Fatalf("did not expect low-confidence expansion hint, got %#v", first.DomainHints)
	}

	if _, _, err := graph.Observe("https://seed.example/root", "https://expansion.example/docs", "query_discovery"); err != nil {
		t.Fatalf("seed graph twice: %v", err)
	}
	second := PrepareQueryRequestWithLocalState(root, QueryRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       "https://seed.example/root",
		DiscoveryMode: QueryDiscoverySameSite,
	}, config.Defaults(), intel.NoopSemanticAligner{})
	if !containsString(second.DomainHints, "expansion.example") {
		t.Fatalf("expected confident expansion hint, got %#v", second.DomainHints)
	}
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func TestObserveQueryResponseWithLocalStatePersistsFingerprintGraph(t *testing.T) {
	root := t.TempDir()
	req := QueryRequest{}
	resp := QueryResponse{
		Document: core.Document{
			URL:       "https://example.com/docs",
			FinalURL:  "https://example.com/docs",
			FetchMode: core.FetchModeHTTP,
		},
		ResultPack: core.ResultPack{
			Profile:    core.ProfileStandard,
			Chunks:     []core.Chunk{{ID: "chk_1", DocID: "doc_1", Text: "Compact context.", HeadingPath: []string{"Docs"}, Score: 0.9, Fingerprint: "fp_1", Confidence: 0.95}},
			CostReport: core.CostReport{LanePath: []int{0}},
		},
		Plan:       QueryPlan{SelectedURL: "https://example.com/docs", CandidateURLs: []string{"https://example.com/docs"}},
		Trace:      proof.RunTrace{Stages: []proof.StageSnapshot{{Stage: "pack", StartedAt: time.Unix(1700000000, 0).UTC(), CompletedAt: time.Unix(1700000001, 0).UTC(), OutputHash: "hash"}}},
		TraceID:    "trace_query",
		CostReport: core.CostReport{LanePath: []int{0}},
	}

	ObserveQueryResponseWithLocalState(root, req, resp)

	graph, err := store.NewFingerprintGraphStore(root).Load("https://example.com/docs")
	if err != nil {
		t.Fatalf("load fingerprint graph: %v", err)
	}
	if graph.LatestTraceID != "trace_query" {
		t.Fatalf("expected latest trace trace_query, got %q", graph.LatestTraceID)
	}
}

func TestPrepareQueryRequestWithLocalStateLoadsFingerprintEvidence(t *testing.T) {
	root := t.TempDir()
	_, _, err := store.NewFingerprintGraphStore(root).Observe("https://example.com/docs", "trace_1", []core.Chunk{
		{ID: "chk_1", DocID: "doc_1", Text: "Compact context.", HeadingPath: []string{"Docs"}, Score: 0.9, Fingerprint: "fp_1", Confidence: 0.95},
	})
	if err != nil {
		t.Fatalf("seed fingerprint graph: %v", err)
	}
	req := PrepareQueryRequestWithLocalState(root, QueryRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       "https://example.com/docs",
		DiscoveryMode: QueryDiscoverySameSite,
	}, config.Defaults(), intel.NoopSemanticAligner{})
	if req.SeedTraceID != "trace_1" {
		t.Fatalf("expected seed trace trace_1, got %#v", req)
	}
	if !req.SeedChanged {
		t.Fatalf("expected seed changed evidence, got %#v", req)
	}
	if req.SeedNovelty <= 0 {
		t.Fatalf("expected positive novelty, got %#v", req)
	}
}

func TestNewFingerprintEvidenceLoaderLoadsGraphEvidence(t *testing.T) {
	root := t.TempDir()
	_, _, err := store.NewFingerprintGraphStore(root).Observe("https://example.com/docs", "trace_1", []core.Chunk{
		{ID: "chk_1", DocID: "doc_1", Text: "Compact context.", HeadingPath: []string{"Docs"}, Score: 0.9, Fingerprint: "fp_1", Confidence: 0.95},
	})
	if err != nil {
		t.Fatalf("seed fingerprint graph: %v", err)
	}
	evidence, ok := NewFingerprintEvidenceLoader(root)("https://example.com/docs")
	if !ok {
		t.Fatal("expected fingerprint evidence")
	}
	if evidence.TraceID != "trace_1" {
		t.Fatalf("expected trace_1, got %#v", evidence)
	}
}

func TestPrepareQueryRequestWithLocalStateLoadsMemoryCandidates(t *testing.T) {
	root := t.TempDir()
	semantic := newDiscoverSemanticServer()
	defer semantic.Close()

	cfg := config.Defaults()
	enableDiscoverSemantic(&cfg, semantic.URL)
	cfg.Memory.Enabled = true

	store := memory.NewSQLiteStore(root, cfg.Memory.Path)
	svc := memory.NewService(cfg.Memory, store, intel.NewTextEmbedder(cfg, semantic.Client()))
	err := svc.Observe(context.Background(), memory.Observation{
		Document: core.Document{
			URL:       "https://playwright.dev/docs/intro",
			FinalURL:  "https://playwright.dev/docs/intro",
			Title:     "Installation | Playwright",
			FetchMode: core.FetchModeHTTP,
		},
		ResultPack: core.ResultPack{
			Profile: core.ProfileStandard,
			Chunks: []core.Chunk{
				{
					ID:          "chk_install",
					DocID:       "doc_intro",
					Text:        "Install Playwright and then run the installation command to download browser binaries.",
					HeadingPath: []string{"Installation"},
					Score:       0.94,
					Confidence:  0.96,
				},
			},
			Links: []string{"https://playwright.dev/docs/intro"},
		},
		ProofRecords: []proof.ProofRecord{{ID: "proof_install"}},
		TraceID:      "trace_install",
		SourceKind:   "read",
	})
	if err != nil {
		t.Fatalf("observe memory: %v", err)
	}

	req := PrepareQueryRequestWithLocalState(root, QueryRequest{
		Goal:          "playwright installation",
		DiscoveryMode: QueryDiscoveryWeb,
	}, cfg, intel.NoopSemanticAligner{})

	if len(req.MemoryCandidates) == 0 {
		t.Fatalf("expected memory candidates, got %#v", req)
	}
	if req.MemoryCandidates[0].URL != "https://playwright.dev/docs/intro" {
		t.Fatalf("expected top memory candidate to load from memory, got %#v", req.MemoryCandidates)
	}
	if !containsString(req.DomainHints, "playwright.dev") {
		t.Fatalf("expected domain hints to include playwright.dev, got %#v", req.DomainHints)
	}
	if req.SeedURL != "" {
		t.Fatalf("expected seed url to remain empty when only memory candidates are loaded, got %#v", req)
	}
}

func TestPrepareQueryRequestWithLocalStateDoesNotAutoSeedOverMemoryCandidates(t *testing.T) {
	root := t.TempDir()
	semantic := newDiscoverSemanticServer()
	defer semantic.Close()

	cfg := config.Defaults()
	enableDiscoverSemantic(&cfg, semantic.URL)
	cfg.Memory.Enabled = true

	memStore := memory.NewSQLiteStore(root, cfg.Memory.Path)
	memSvc := memory.NewService(cfg.Memory, memStore, intel.NewTextEmbedder(cfg, semantic.Client()))
	if err := memSvc.Observe(context.Background(), memory.Observation{
		Document: core.Document{
			URL:       "https://playwright.dev/docs/intro",
			FinalURL:  "https://playwright.dev/docs/intro",
			Title:     "Installation | Playwright",
			FetchMode: core.FetchModeHTTP,
		},
		ResultPack: core.ResultPack{
			Profile: core.ProfileStandard,
			Chunks:  []core.Chunk{{ID: "chk_mem", DocID: "doc_mem", Text: "Install Playwright and browser binaries.", HeadingPath: []string{"Installation"}, Score: 0.9, Confidence: 0.95}},
		},
		ProofRecords: []proof.ProofRecord{{ID: "proof_mem"}},
		TraceID:      "trace_mem",
		SourceKind:   "read",
	}); err != nil {
		t.Fatalf("observe memory: %v", err)
	}

	candidateStore := store.NewCandidateStore(root)
	if _, _, err := candidateStore.Observe(store.CandidateObservation{
		URL:    "https://candidate.example.com/landing",
		Title:  "Playwright Installation Mirror",
		Source: "query_discovery",
	}); err != nil {
		t.Fatalf("seed candidate store: %v", err)
	}

	req := PrepareQueryRequestWithLocalState(root, QueryRequest{
		Goal:          "playwright installation",
		DiscoveryMode: QueryDiscoveryWeb,
	}, cfg, intel.NewSemanticAligner(cfg, semantic.Client()))

	if len(req.MemoryCandidates) == 0 {
		t.Fatalf("expected memory candidates, got %#v", req)
	}
	if req.SeedURL != "" {
		t.Fatalf("expected candidate store not to auto-seed over memory candidates, got %#v", req)
	}
}

func TestRunQueryDiscoveryPrefersMemoryCandidatesForSeedlessWeb(t *testing.T) {
	svc := newTestService(t, config.Defaults(), nil)

	result, err := svc.runQueryDiscovery(context.Background(), QueryRequest{
		Goal: "playwright installation",
		MemoryCandidates: []DiscoverCandidate{
			{
				URL:    "https://playwright.dev/docs/intro",
				Label:  "Installation",
				Score:  2.4,
				Reason: []string{"semantic_goal_alignment", "local_memory_hit"},
			},
		},
	}, QueryDiscoveryWeb, QueryFingerprintEvidence{})
	if err != nil {
		t.Fatalf("run query discovery: %v", err)
	}
	if result.provider != "discovery_memory" {
		t.Fatalf("expected discovery_memory provider, got %#v", result)
	}
	if result.selected != "https://playwright.dev/docs/intro" {
		t.Fatalf("expected selected memory candidate, got %#v", result)
	}
	if len(result.candidates) != 1 || result.candidates[0].URL != "https://playwright.dev/docs/intro" {
		t.Fatalf("expected only memory-backed candidate, got %#v", result.candidates)
	}
}
