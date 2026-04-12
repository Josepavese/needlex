package service

import (
	"context"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/proof"
	"github.com/josepavese/needlex/internal/store"
)

func TestPrepareReadRequestWithLocalStateAppliesGenome(t *testing.T) {
	root := t.TempDir()
	_, _, err := store.NewGenomeStore(root).Observe(store.GenomeObservation{
		URL:               "https://example.com/docs",
		ObservedLane:      2,
		PreferredProfile:  "tiny",
		FetchProfile:      "hardened",
		FetchRetryProfile: "hardened",
		PruningProfile:    "aggressive",
		RenderNeeded:      true,
	})
	if err != nil {
		t.Fatalf("seed genome: %v", err)
	}

	req := PrepareReadRequestWithLocalState(root, ReadRequest{URL: "https://example.com/docs"})
	if req.ForceLane != 2 {
		t.Fatalf("expected force lane 2, got %d", req.ForceLane)
	}
	if req.Profile != "tiny" {
		t.Fatalf("expected profile tiny, got %q", req.Profile)
	}
	if req.FetchProfile != "hardened" || req.FetchRetryProfile != "hardened" {
		t.Fatalf("expected fetch profiles from genome, got %q/%q", req.FetchProfile, req.FetchRetryProfile)
	}
	if req.PruningProfile != "aggressive" {
		t.Fatalf("expected pruning aggressive, got %q", req.PruningProfile)
	}
	if !req.RenderHint {
		t.Fatal("expected render hint true")
	}
}

func TestPrepareReadRequestWithLocalStateLoadsStableFingerprints(t *testing.T) {
	root := t.TempDir()
	_, _, err := store.NewFingerprintGraphStore(root).Observe("https://example.com/docs", "trace_1", []core.Chunk{
		{ID: "chk_1", DocID: "doc_1", Text: "Compact context.", HeadingPath: []string{"Docs"}, Score: 0.9, Fingerprint: "fp_1", Confidence: 0.95},
	})
	if err != nil {
		t.Fatalf("seed fingerprint graph: %v", err)
	}
	req := PrepareReadRequestWithLocalState(root, ReadRequest{URL: "https://example.com/docs"})
	if len(req.StableFingerprints) != 1 || req.StableFingerprints[0] != "fp_1" {
		t.Fatalf("expected stable fingerprint hint fp_1, got %#v", req.StableFingerprints)
	}
}

func TestObserveReadResponseWithLocalStatePersistsArtifacts(t *testing.T) {
	root := t.TempDir()
	req := ReadRequest{PruningProfile: "standard"}
	resp := ReadResponse{
		Document: core.Document{
			URL:       "https://example.com/docs",
			FinalURL:  "https://example.com/docs",
			Title:     "Example Docs",
			FetchMode: core.FetchModeHTTP,
		},
		ResultPack: core.ResultPack{
			Profile: core.ProfileStandard,
			CostReport: core.CostReport{
				LanePath: []int{0, 1},
			},
		},
		Trace: proof.RunTrace{
			Stages: []proof.StageSnapshot{
				{
					Stage: "pack",
					Metadata: map[string]string{
						"noise_level": "low",
						"page_type":   "docs",
					},
					StartedAt:   time.Unix(1700000000, 0).UTC(),
					CompletedAt: time.Unix(1700000001, 0).UTC(),
					OutputHash:  "hash",
				},
				{
					Stage: "acquire",
					Metadata: map[string]string{
						"fetch_profile": "browser_like",
						"retry_profile": "hardened",
					},
					StartedAt:   time.Unix(1700000000, 0).UTC(),
					CompletedAt: time.Unix(1700000001, 0).UTC(),
					OutputHash:  "hash2",
				},
			},
		},
	}

	ObserveReadResponseWithLocalState(root, req, resp)

	matches, err := store.NewCandidateStore(root).Search(context.Background(), "example docs", 1, fakeSemanticAligner{suppressed: true})
	if err != nil {
		t.Fatalf("search candidates: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected candidate persisted, got %d", len(matches))
	}
	genome, err := store.NewGenomeStore(root).LoadByURL("https://example.com/any")
	if err != nil {
		t.Fatalf("load genome: %v", err)
	}
	if genome.ForceLane != 1 {
		t.Fatalf("expected force lane 1, got %d", genome.ForceLane)
	}
	if genome.FetchProfile != "browser_like" || genome.FetchRetryProfile != "hardened" {
		t.Fatalf("expected fetch profiles persisted, got %+v", genome)
	}
}

func TestPrepareCrawlRequestWithLocalStateAppliesGenome(t *testing.T) {
	root := t.TempDir()
	_, _, err := store.NewGenomeStore(root).Observe(store.GenomeObservation{
		URL:               "https://example.com/root",
		ObservedLane:      1,
		PreferredProfile:  "tiny",
		FetchProfile:      "browser_like",
		FetchRetryProfile: "hardened",
		PruningProfile:    "forum",
		RenderNeeded:      true,
	})
	if err != nil {
		t.Fatalf("seed genome: %v", err)
	}

	req := PrepareCrawlRequestWithLocalState(root, CrawlRequest{SeedURL: "https://example.com/root"})
	if req.ForceLane != 1 {
		t.Fatalf("expected force lane 1, got %d", req.ForceLane)
	}
	if req.Profile != "tiny" {
		t.Fatalf("expected profile tiny, got %q", req.Profile)
	}
	if req.FetchProfile != "browser_like" || req.FetchRetryProfile != "hardened" {
		t.Fatalf("expected fetch profiles from genome, got %q/%q", req.FetchProfile, req.FetchRetryProfile)
	}
	if req.PruningProfile != "forum" {
		t.Fatalf("expected pruning forum, got %q", req.PruningProfile)
	}
	if !req.RenderHint {
		t.Fatal("expected render hint true")
	}
}

func TestObserveCrawlResponseWithLocalStatePersistsGenome(t *testing.T) {
	root := t.TempDir()
	req := CrawlRequest{PruningProfile: "standard"}
	resp := CrawlResponse{
		Pages: []ReadResponse{
			{
				Document: core.Document{
					URL:       "https://example.com/a",
					FinalURL:  "https://example.com/a",
					Title:     "Page A",
					FetchMode: core.FetchModeHTTP,
				},
				ResultPack: core.ResultPack{
					Profile: core.ProfileStandard,
					CostReport: core.CostReport{
						LanePath: []int{0, 2},
					},
				},
				Trace: proof.RunTrace{
					Stages: []proof.StageSnapshot{
						{
							Stage: "pack",
							Metadata: map[string]string{
								"noise_level": "medium",
								"page_type":   "docs",
							},
							StartedAt:   time.Unix(1700000000, 0).UTC(),
							CompletedAt: time.Unix(1700000001, 0).UTC(),
							OutputHash:  "hash",
						},
					},
				},
			},
		},
	}

	ObserveCrawlResponseWithLocalState(root, req, resp)

	genome, err := store.NewGenomeStore(root).LoadByURL("https://example.com/x")
	if err != nil {
		t.Fatalf("load genome: %v", err)
	}
	if genome.ForceLane != 2 {
		t.Fatalf("expected force lane 2, got %d", genome.ForceLane)
	}
}

func TestObserveReadResponseWithLocalStatePersistsFingerprintGraph(t *testing.T) {
	root := t.TempDir()
	req := ReadRequest{}
	resp := ReadResponse{
		Document: core.Document{
			URL:       "https://example.com/docs",
			FinalURL:  "https://example.com/docs",
			Title:     "Example Docs",
			FetchMode: core.FetchModeHTTP,
		},
		ResultPack: core.ResultPack{
			Profile: core.ProfileStandard,
			Chunks: []core.Chunk{
				{
					ID:          "chk_1",
					DocID:       "doc_1",
					Text:        "Compact context.",
					HeadingPath: []string{"Docs"},
					Score:       0.9,
					Fingerprint: "fp_1",
					Confidence:  0.95,
				},
			},
			CostReport: core.CostReport{LanePath: []int{0}},
		},
		Trace: proof.RunTrace{TraceID: "trace_1", Stages: []proof.StageSnapshot{{Stage: "pack", StartedAt: time.Unix(1700000000, 0).UTC(), CompletedAt: time.Unix(1700000001, 0).UTC(), OutputHash: "hash"}}},
	}

	ObserveReadResponseWithLocalState(root, req, resp)

	graph, err := store.NewFingerprintGraphStore(root).Load("https://example.com/docs")
	if err != nil {
		t.Fatalf("load fingerprint graph: %v", err)
	}
	if graph.LatestTraceID != "trace_1" {
		t.Fatalf("expected latest trace trace_1, got %q", graph.LatestTraceID)
	}
	if len(graph.LatestNodes) != 1 || graph.LatestNodes[0].Fingerprint != "fp_1" {
		t.Fatalf("unexpected graph nodes %#v", graph.LatestNodes)
	}
}

func TestObserveCrawlResponseWithLocalStatePersistsFingerprintGraph(t *testing.T) {
	root := t.TempDir()
	req := CrawlRequest{}
	resp := CrawlResponse{
		Pages: []ReadResponse{{
			Document: core.Document{URL: "https://example.com/a", FinalURL: "https://example.com/a", FetchMode: core.FetchModeHTTP},
			ResultPack: core.ResultPack{
				Profile:    core.ProfileStandard,
				Chunks:     []core.Chunk{{ID: "chk_1", DocID: "doc_1", Text: "Compact context.", HeadingPath: []string{"Page A"}, Score: 0.9, Fingerprint: "fp_a", Confidence: 0.95}},
				CostReport: core.CostReport{LanePath: []int{0}},
			},
			Trace: proof.RunTrace{TraceID: "trace_a", Stages: []proof.StageSnapshot{{Stage: "pack", StartedAt: time.Unix(1700000000, 0).UTC(), CompletedAt: time.Unix(1700000001, 0).UTC(), OutputHash: "hash"}}},
		}},
	}

	ObserveCrawlResponseWithLocalState(root, req, resp)

	graph, err := store.NewFingerprintGraphStore(root).Load("https://example.com/a")
	if err != nil {
		t.Fatalf("load fingerprint graph: %v", err)
	}
	if graph.LatestTraceID != "trace_a" {
		t.Fatalf("expected latest trace trace_a, got %q", graph.LatestTraceID)
	}
}
