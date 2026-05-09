package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/josepavese/needlex/internal/intel"
)

type pageIntentTestAligner struct {
	target          string
	candidateScores map[string]float64
}

func (a pageIntentTestAligner) Align(context.Context, string, []intel.SemanticCandidate) (intel.SemanticAlignment, error) {
	return intel.SemanticAlignment{}, nil
}

func (a pageIntentTestAligner) Score(_ context.Context, objective string, candidates []intel.SemanticCandidate) ([]intel.SemanticScore, error) {
	out := make([]intel.SemanticScore, 0, len(candidates))
	targetText := targetKindArchetypeText(a.target)
	for _, candidate := range candidates {
		score := 0.10
		switch {
		case targetKindFromPrototypeID(candidate.ID) == a.target:
			score = 0.92
		case strings.TrimSpace(objective) == targetText:
			if value, ok := a.candidateScores[candidate.ID]; ok {
				score = value
			} else {
				score = 0.04
			}
		}
		out = append(out, intel.SemanticScore{ID: candidate.ID, Similarity: score})
	}
	return out, nil
}

func TestTargetKindDoesNotPromoteBroadTargetsWhenSemanticUnavailable(t *testing.T) {
	cfg := testConfig()
	svc := newTestService(t, cfg, nil)
	svc.semantic = noScoreSemanticAligner{}
	candidates := []DiscoverCandidate{
		{URL: "https://example.com/products/cloud", Score: 10},
		{URL: "https://example.com/", Score: 1},
	}

	got := svc.applyTargetKindRerank(context.Background(), "official main home page broad identity overview", candidates)
	if got[0].URL != candidates[0].URL {
		t.Fatalf("expected disabled semantic target-kind promotion to leave order unchanged, got %#v", got)
	}
	for _, candidate := range got {
		if candidate.Metadata["target_kind"] != "" {
			t.Fatalf("expected no target_kind metadata when semantic is disabled, got %#v", got)
		}
	}
}

func TestTargetKindPromotesOrganizationAboutBySemanticPageIntent(t *testing.T) {
	cfg := testConfig()
	svc := newTestService(t, cfg, nil)
	svc.semantic = pageIntentTestAligner{
		target: targetKindOrganizationAbout,
		candidateScores: map[string]float64{
			"https://example.com/solutions/gitops": 0.12,
			"https://example.com/about":            0.94,
		},
	}
	candidates := []DiscoverCandidate{
		{
			URL:   "https://example.com/solutions/gitops",
			Label: "Capability page",
			Score: 2.05,
			Metadata: map[string]string{
				"page_title":        "Capability surface",
				"web_ir_context":    "Product workflow and deployment option.",
				"resource_class":    "html_like",
				"host_root_title":   "Example Platform",
				"host_root_context": "Example Platform product and service entry.",
			},
		},
		{
			URL:   "https://example.com/about",
			Label: "Institutional profile",
			Score: 1.20,
			Metadata: map[string]string{
				"page_title":        "Institutional profile",
				"web_ir_context":    "Organization mission governance accountability and team identity.",
				"resource_class":    "html_like",
				"host_root_title":   "Example Platform",
				"host_root_context": "Example Platform product and service entry.",
			},
		},
	}

	got := svc.applyTargetKindRerank(context.Background(), "understand the organization identity and accountability", candidates)
	if got[0].URL != "https://example.com/about" {
		t.Fatalf("expected semantic organization-about intent to win, got %#v", got)
	}
	if !containsReason(got[0].Reason, "target_kind_semantic_alignment") {
		t.Fatalf("expected semantic target-kind reason, got %#v", got[0].Reason)
	}
	if got[0].Metadata["target_kind"] != targetKindOrganizationAbout {
		t.Fatalf("expected target kind metadata, got %#v", got[0].Metadata)
	}
	if got[0].Metadata["target_kind_candidate_semantic"] == "" {
		t.Fatalf("expected candidate semantic metadata, got %#v", got[0].Metadata)
	}
}

func TestPreferredRecoveredMemoryURLUsesSemanticCanonicalHome(t *testing.T) {
	seed := "https://sqlite.org/whentouse.html"
	recovered := DiscoverResponse{
		SelectedURL: "https://sqlite.org",
		Candidates: []DiscoverCandidate{
			{
				URL:   "https://sqlite.org",
				Score: 2.4,
				Metadata: map[string]string{
					"target_kind": targetKindCanonicalHome,
				},
			},
			{URL: seed, Score: 2.2},
		},
	}

	profile := targetKindProfile{Kind: targetKindCanonicalHome, Similarity: 0.80, Margin: 0.20}
	if got := preferredRecoveredMemoryURL(seed, recovered, profile); got != "https://sqlite.org" {
		t.Fatalf("expected semantic canonical home recovery, got %q", got)
	}
}

func TestPreferredRecoveredMemoryURLPreservesNonRootMemorySeedForWeakHomeIntent(t *testing.T) {
	seed := "https://svelte.dev/docs/svelte/overview"
	recovered := DiscoverResponse{
		SelectedURL: "https://svelte.dev/",
		Candidates: []DiscoverCandidate{
			{
				URL:   "https://svelte.dev/",
				Score: 2.4,
				Metadata: map[string]string{
					"target_kind": targetKindCanonicalHome,
				},
			},
			{URL: seed, Score: 2.2},
		},
	}

	profile := targetKindProfile{Kind: targetKindCanonicalHome, Similarity: 0.48, Margin: 0.04}
	if got := preferredRecoveredMemoryURL(seed, recovered, profile); got != seed {
		t.Fatalf("expected weak home intent to preserve non-root memory seed, got %q", got)
	}
}

func TestPreferredRecoveredMemoryURLKeepsRootSeedSelection(t *testing.T) {
	seed := "https://fastapi.tiangolo.com/"
	recovered := DiscoverResponse{
		SelectedURL: seed,
		Candidates: []DiscoverCandidate{
			{URL: seed, Score: 1.4},
			{URL: "https://fastapi.tiangolo.com/about/", Score: 1.2},
		},
	}

	profile := targetKindProfile{Kind: targetKindCanonicalHome, Similarity: 0.80, Margin: 0.20}
	if got := preferredRecoveredMemoryURL(seed, recovered, profile); got != seed {
		t.Fatalf("expected root seed recovery to keep discovery selection, got %q", got)
	}
}

func TestPreferredRecoveredMemoryURLAllowsSpecificRecoveryForNonHomeIntent(t *testing.T) {
	seed := "https://playwright.dev/"
	recovered := DiscoverResponse{
		SelectedURL: seed,
		Candidates: []DiscoverCandidate{
			{URL: seed, Score: 1.4},
			{URL: "https://playwright.dev/docs/intro", Score: 1.2},
		},
	}

	profile := targetKindProfile{Kind: targetKindLearningPath, Similarity: 0.60, Margin: 0.12}
	if got := preferredRecoveredMemoryURL(seed, recovered, profile); got != "https://playwright.dev/docs/intro" {
		t.Fatalf("expected non-home intent to allow specific same-site recovery, got %q", got)
	}
}

func TestDiscoverWebAppliesTargetKindResolverToSeedlessResults(t *testing.T) {
	var siteURL string
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprint(w, `<html><head><title>Example Platform</title></head><body><h1>Example Platform</h1></body></html>`)
		case "/solutions/gitops":
			_, _ = fmt.Fprint(w, `<html><head><title>Capability surface</title></head><body><article><h1>Capability</h1><p>Product workflow and deployment option.</p></article></body></html>`)
		case "/about":
			_, _ = fmt.Fprint(w, `<html><head><title>Institutional profile</title></head><body><article><h1>Profile</h1><p>Organization mission governance accountability and team identity.</p></article></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer site.Close()
	siteURL = site.URL

	search := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s/solutions/gitops">Capability surface</a><a class="result__a" href="%s/about">Institutional profile</a></body></html>`, siteURL, siteURL)
	}))
	defer search.Close()

	cfg := testConfig()
	svc := newTestService(t, cfg, search.Client())
	svc.SetWebDiscoverBaseURL(search.URL)
	svc.semantic = pageIntentTestAligner{
		target: targetKindOrganizationAbout,
		candidateScores: map[string]float64{
			siteURL + "/solutions/gitops": 0.10,
			siteURL + "/about":            0.96,
		},
	}

	resp, err := svc.DiscoverWeb(context.Background(), DiscoverWebRequest{
		Goal:          "identify the organization profile and accountability surface",
		MaxCandidates: 5,
	})
	if err != nil {
		t.Fatalf("discover web failed: %v", err)
	}
	if resp.SelectedURL != siteURL+"/about" {
		t.Fatalf("expected web discovery target-kind resolver to select about surface, got %#v", resp)
	}
	if !containsReason(resp.Candidates[0].Reason, "target_kind_semantic_alignment") {
		t.Fatalf("expected target-kind resolver reason on selected candidate, got %#v", resp.Candidates[0].Reason)
	}
}
