package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/core"
	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/core/queryflow"
	"github.com/josepavese/needlex/internal/core/queryplan"
	"github.com/josepavese/needlex/internal/core/queryreview"
	"github.com/josepavese/needlex/internal/intel"
)

type postReadSemanticTestAligner struct {
	scores map[string]queryreview.Score
}

func (a postReadSemanticTestAligner) Align(context.Context, string, []intel.SemanticCandidate) (intel.SemanticAlignment, error) {
	return intel.SemanticAlignment{}, nil
}

func (a postReadSemanticTestAligner) Score(_ context.Context, objective string, candidates []intel.SemanticCandidate) ([]intel.SemanticScore, error) {
	out := make([]intel.SemanticScore, 0, len(candidates))
	for _, candidate := range candidates {
		score := a.scores[strings.TrimSpace(candidate.ID)]
		similarity := 0.18
		switch strings.TrimSpace(objective) {
		case hostRootOriginProfile():
			similarity = score.Origin
		case hostRootDerivativeProfile():
			similarity = score.Derivative
		default:
			if strings.Contains(objective, "source-owner identity") && score.Entity > 0 {
				similarity = score.Entity
			} else if score.Goal > 0 {
				similarity = score.Goal
			}
		}
		out = append(out, intel.SemanticScore{ID: candidate.ID, Similarity: similarity})
	}
	return out, nil
}

func TestQueryBuildsPlanAndResultPack(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, testHTML)
	}))
	defer server.Close()

	svc := newSemanticService(t, server.Client())

	resp, err := svc.Query(context.Background(), QueryRequest{
		Goal:        "proof replay deterministic",
		SeedURL:     server.URL,
		DomainHints: []string{server.URL},
		Profile:     core.ProfileTiny,
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if resp.Plan.Goal != "proof replay deterministic" {
		t.Fatalf("unexpected plan goal %q", resp.Plan.Goal)
	}
	if resp.ResultPack.Query != "proof replay deterministic" {
		t.Fatalf("expected result pack query to mirror goal, got %q", resp.ResultPack.Query)
	}
	if resp.Plan.SelectedURL != server.URL {
		t.Fatalf("expected selected url to default to seed, got %q", resp.Plan.SelectedURL)
	}
	if resp.Plan.Compiler.Version != QueryCompilerVersion {
		t.Fatalf("expected compiler version %q, got %q", QueryCompilerVersion, resp.Plan.Compiler.Version)
	}
	if len(resp.Plan.DomainHints) == 0 {
		t.Fatal("expected domain hints in plan")
	}
	if len(resp.Plan.Compiler.Decisions) < 3 {
		t.Fatalf("expected compiler decisions, got %d", len(resp.Plan.Compiler.Decisions))
	}
	webIRDecision := requireCompilerDecision(t, resp.Plan.Compiler.Decisions, QueryPlanReasonWebIR, nil)
	if webIRDecision.Metadata["node_count"] == "" || webIRDecision.Metadata["embedded_node_count"] == "" || webIRDecision.Metadata["heading_ratio"] == "" || webIRDecision.Metadata["short_text_ratio"] == "" || webIRDecision.Metadata["dominant_signal"] == "" {
		t.Fatalf("expected rich web ir metadata, got %#v", webIRDecision.Metadata)
	}
	requireCompilerDecision(t, resp.Plan.Compiler.Decisions, QueryPlanReasonQualityLatencyMode, nil)
	requireCompilerDecision(t, resp.Plan.Compiler.Decisions, QueryPlanReasonLanePolicy, nil)
	requireCompilerDecision(t, resp.Plan.Compiler.Decisions, QueryPlanReasonExecutionAligned, nil)
	planDiff := requireCompilerDecision(t, resp.Plan.Compiler.Decisions, QueryPlanReasonPlanDiffObserved, nil)
	if planDiff.Metadata["added_stage_count"] == "" {
		t.Fatalf("expected plan diff metadata in %#v", planDiff)
	}
	if !hasCompilerDecision(resp.Plan.Compiler.Decisions, QueryPlanReasonRuntimeEffectsClean) && !hasCompilerDecision(resp.Plan.Compiler.Decisions, QueryPlanReasonRuntimeEffectsDetected) {
		t.Fatalf("expected runtime effects decision in %#v", resp.Plan.Compiler.Decisions)
	}
	requireCompilerDecision(t, resp.Plan.Compiler.Decisions, QueryPlanReasonIntentBoundary, nil)
	requireCompilerDecision(t, resp.Plan.Compiler.Decisions, QueryPlanReasonExecutionBoundary, nil)
	if !hasCompilerDecision(resp.Plan.Compiler.Decisions, QueryPlanReasonBudgetOutcomeOK) && !hasCompilerDecision(resp.Plan.Compiler.Decisions, QueryPlanReasonBudgetOutcomeExceeded) {
		t.Fatalf("expected budget outcome decision in %#v", resp.Plan.Compiler.Decisions)
	}
	if resp.WebIR.Version != core.WebIRVersion {
		t.Fatalf("expected web_ir version %q, got %q", core.WebIRVersion, resp.WebIR.Version)
	}
	if resp.TraceID == "" {
		t.Fatal("expected trace id")
	}
	if len(resp.AgentContext.Chunks) == 0 {
		t.Fatal("expected agent context chunks")
	}
	if resp.AgentContext.URL != server.URL {
		t.Fatalf("expected agent context url %q, got %q", server.URL, resp.AgentContext.URL)
	}
	if resp.AgentContext.Chunks[0].SourceURL == "" || resp.AgentContext.Chunks[0].SourceSelector == "" || resp.AgentContext.Chunks[0].ProofRef == "" {
		t.Fatalf("expected inline provenance in agent context, got %#v", resp.AgentContext.Chunks[0])
	}
}

func TestApplyDiscoveryToPlanPublishesRankedCandidateURLs(t *testing.T) {
	svc := newSemanticService(t, nil)
	plan, _, _ := svc.buildQueryPlan(QueryRequest{Goal: "ranked candidates", DiscoveryMode: QueryDiscoveryWeb}, core.ProfileTiny, "", QueryDiscoveryWeb)

	err := svc.applyDiscoveryToPlan(&plan, QueryRequest{Goal: "ranked candidates"}, QueryDiscoveryWeb, queryDiscoveryResult{
		selected: "https://high.example/docs",
		candidates: []DiscoverCandidate{
			{URL: "https://low.example/docs", Score: 0.8},
			{URL: "https://high.example/docs", Score: 1.7},
			{URL: "https://mid.example/docs", Score: 1.2},
		},
	})
	if err != nil {
		t.Fatalf("apply discovery: %v", err)
	}
	want := []string{"https://high.example/docs", "https://mid.example/docs", "https://low.example/docs"}
	if fmt.Sprint(plan.CandidateURLs) != fmt.Sprint(want) {
		t.Fatalf("expected ranked candidate urls %v, got %v", want, plan.CandidateURLs)
	}
}

func TestReadQuerySelectedCandidateFallsBackFromUnsupportedContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/asset":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = fmt.Fprint(w, "binary")
		case "/docs":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, testHTML)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := newSemanticService(t, server.Client())
	plan, _, _ := svc.buildQueryPlan(QueryRequest{Goal: "runtime fallback", DiscoveryMode: QueryDiscoveryWeb}, core.ProfileTiny, "", QueryDiscoveryWeb)
	plan.SelectedURL = server.URL + "/asset"
	plan.CandidateURLs = []string{server.URL + "/asset", server.URL + "/docs"}
	plan.CandidateDiagnostics = []queryreview.Diagnostic{
		{URL: server.URL + "/asset", Score: 1.2},
		{URL: server.URL + "/docs", Score: 1.1},
	}

	resp, err := svc.readQuerySelectedCandidate(context.Background(), QueryRequest{Goal: "runtime fallback"}, core.ProfileTiny, QueryDiscoveryWeb, &plan, nil)
	if err != nil {
		t.Fatalf("expected fallback read to succeed: %v", err)
	}
	if plan.SelectedURL != server.URL+"/docs" || resp.Document.FinalURL != server.URL+"/docs" {
		t.Fatalf("expected fallback docs selection, plan=%q final=%q", plan.SelectedURL, resp.Document.FinalURL)
	}
	foundDecision := false
	for _, decision := range plan.Compiler.Decisions {
		if decision.Stage == "select.candidate_runtime_fallback" && decision.Choice == server.URL+"/docs" && decision.Metadata["runtime_error_class"] == "unsupported_content_type" {
			foundDecision = true
			break
		}
	}
	if !foundDecision {
		t.Fatalf("expected runtime fallback decision, got %#v", plan.Compiler.Decisions)
	}
}

func TestReadQuerySelectedCandidateRetriesSelectedWithResilientFetchBeforeFallback(t *testing.T) {
	primaryHits := 0
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryHits++
		if primaryHits == 1 {
			http.Error(w, "blocked", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Primary docs</title></head><body><h1>Primary docs</h1><p>Recovered through resilient fetch.</p></body></html>`)
	}))
	defer primary.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, testHTML)
	}))
	defer fallback.Close()

	svc := newSemanticService(t, primary.Client())
	plan, _, _ := svc.buildQueryPlan(QueryRequest{Goal: "runtime fallback", DiscoveryMode: QueryDiscoveryWeb}, core.ProfileTiny, "", QueryDiscoveryWeb)
	plan.SelectedURL = primary.URL
	plan.CandidateURLs = []string{primary.URL, fallback.URL}

	resp, err := svc.readQuerySelectedCandidate(context.Background(), QueryRequest{Goal: "runtime fallback", FetchProfile: "standard", FetchRetryProfile: "standard"}, core.ProfileTiny, QueryDiscoveryWeb, &plan, nil)
	if err != nil {
		t.Fatalf("expected resilient selected retry to succeed: %v", err)
	}
	if plan.SelectedURL != primary.URL || resp.Document.FinalURL != primary.URL {
		t.Fatalf("expected selected URL to remain primary, plan=%q final=%q", plan.SelectedURL, resp.Document.FinalURL)
	}
	if primaryHits != 2 {
		t.Fatalf("expected primary to be retried before fallback, got %d hits", primaryHits)
	}
}

func TestReadQuerySelectedCandidateFallsBackFromTLSError(t *testing.T) {
	badTLS := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, testHTML)
	}))
	defer badTLS.Close()
	docs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, testHTML)
	}))
	defer docs.Close()

	svc := newSemanticService(t, nil)
	plan, _, _ := svc.buildQueryPlan(QueryRequest{Goal: "runtime fallback", DiscoveryMode: QueryDiscoveryWeb}, core.ProfileTiny, "", QueryDiscoveryWeb)
	plan.SelectedURL = badTLS.URL
	plan.CandidateURLs = []string{badTLS.URL, docs.URL}

	resp, err := svc.readQuerySelectedCandidate(context.Background(), QueryRequest{Goal: "runtime fallback"}, core.ProfileTiny, QueryDiscoveryWeb, &plan, nil)
	if err != nil {
		t.Fatalf("expected TLS fallback read to succeed: %v", err)
	}
	if plan.SelectedURL != docs.URL || resp.Document.FinalURL != docs.URL {
		t.Fatalf("expected fallback docs selection, plan=%q final=%q", plan.SelectedURL, resp.Document.FinalURL)
	}
	decision := requireCompilerDecision(t, plan.Compiler.Decisions, QueryPlanReasonSelection, func(decision QueryPlanDecision) bool {
		return decision.Stage == "select.candidate_runtime_fallback"
	})
	if decision.Metadata["runtime_error_class"] != "tls_certificate" {
		t.Fatalf("expected tls_certificate fallback metadata, got %#v", decision.Metadata)
	}
}

func TestReadQuerySelectedCandidateFallsBackAfterPostReadSemanticSourceValidation(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/derivative":
			_, _ = fmt.Fprint(w, `<html><head><title>Derivative automation surface</title></head><body><article><h1>Derivative automation surface</h1><p>Managed execution surface describing another browser automation project.</p></article></body></html>`)
		case "/source":
			_, _ = fmt.Fprint(w, `<html><head><title>Source automation project</title></head><body><article><h1>Source automation project</h1><p>Primary maintained project documentation and release surface.</p></article></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	sourceURL := serverURL + "/source"
	derivativeURL := serverURL + "/derivative"
	svc := newSemanticService(t, server.Client())
	svc.semantic = postReadSemanticTestAligner{scores: map[string]queryreview.Score{
		derivativeURL: {Goal: 0.74, Entity: 0.30, Origin: 0.16, Derivative: 0.54},
		sourceURL:     {Goal: 0.70, Entity: 0.70, Origin: 0.62, Derivative: 0.18},
	}}
	candidates := []DiscoverCandidate{
		{
			URL:   derivativeURL,
			Score: 2.00,
			Reason: []string{
				"semantic_goal_alignment",
				"semantic_topic_without_identity_penalty",
				"semantic_derivative_surface_penalty",
			},
			Metadata: map[string]string{
				"semantic_family_intent_identity":   "0.480",
				"semantic_family_intent_topic":      "0.620",
				"semantic_family_intent_merit":      "0.660",
				"semantic_family_intent_origin":     "0.050",
				"semantic_family_intent_derivative": "0.180",
				"semantic_origin_alignment":         "0.050",
				"semantic_derivative_alignment":     "0.180",
			},
		},
		{
			URL:   sourceURL,
			Score: 1.60,
			Reason: []string{
				"semantic_goal_alignment",
				"semantic_provider_consensus",
			},
			Metadata: map[string]string{
				"semantic_family_intent_identity":   "0.504",
				"semantic_family_intent_topic":      "0.590",
				"semantic_family_intent_merit":      "0.682",
				"semantic_family_intent_origin":     "0.220",
				"semantic_family_intent_derivative": "0.080",
				"semantic_origin_alignment":         "0.220",
				"semantic_derivative_alignment":     "0.080",
			},
		},
	}
	plan, _, _ := svc.buildQueryPlan(QueryRequest{Goal: "browser automation project", DiscoveryMode: QueryDiscoveryWeb}, core.ProfileTiny, "", QueryDiscoveryWeb)
	plan.SelectedURL = derivativeURL
	plan.CandidateURLs = []string{derivativeURL, sourceURL}
	plan.CandidateDiagnostics = queryCandidateDiagnostics(candidates)

	resp, err := svc.readQuerySelectedCandidate(context.Background(), QueryRequest{Goal: "browser automation project"}, core.ProfileTiny, QueryDiscoveryWeb, &plan, candidates)
	if err != nil {
		t.Fatalf("expected post-read semantic fallback to succeed: %v", err)
	}
	if plan.SelectedURL != sourceURL || resp.Document.FinalURL != sourceURL {
		t.Fatalf("expected source selection after post-read validation, plan=%q final=%q", plan.SelectedURL, resp.Document.FinalURL)
	}
	decision := requireCompilerDecision(t, plan.Compiler.Decisions, QueryPlanReasonSelection, func(decision QueryPlanDecision) bool {
		return decision.Stage == "select.post_read_semantic_fallback"
	})
	if decision.Metadata["validation_surface"] != "post_read_embedding_source_role" {
		t.Fatalf("expected post-read validation metadata, got %#v", decision.Metadata)
	}
}

func TestReadQuerySelectedCandidateKeepsStrongPostReadSourceSelection(t *testing.T) {
	hits := map[string]int{}
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path]++
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/source":
			_, _ = fmt.Fprint(w, `<html><head><title>Source automation project</title></head><body><article><h1>Source automation project</h1><p>Primary maintained project documentation and release surface.</p></article></body></html>`)
		case "/derivative":
			_, _ = fmt.Fprint(w, `<html><head><title>Derivative automation surface</title></head><body><article><h1>Derivative automation surface</h1><p>Managed execution surface describing another browser automation project.</p></article></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	sourceURL := serverURL + "/source"
	derivativeURL := serverURL + "/derivative"
	svc := newSemanticService(t, server.Client())
	svc.semantic = postReadSemanticTestAligner{scores: map[string]queryreview.Score{
		sourceURL:     {Goal: 0.70, Entity: 0.70, Origin: 0.62, Derivative: 0.18},
		derivativeURL: {Goal: 0.74, Entity: 0.30, Origin: 0.16, Derivative: 0.54},
	}}
	candidates := []DiscoverCandidate{
		{
			URL:   sourceURL,
			Score: 2.00,
			Reason: []string{
				"semantic_goal_alignment",
				"semantic_provenance_identity",
			},
			Metadata: map[string]string{
				"semantic_family_intent_identity":   "0.640",
				"semantic_family_intent_topic":      "0.620",
				"semantic_family_intent_merit":      "0.820",
				"semantic_family_intent_origin":     "0.500",
				"semantic_family_intent_derivative": "0.050",
				"semantic_provenance_identity":      "0.640",
				"semantic_origin_alignment":         "0.500",
				"semantic_derivative_alignment":     "0.050",
			},
		},
		{
			URL:   derivativeURL,
			Score: 1.80,
			Reason: []string{
				"semantic_goal_alignment",
				"semantic_derivative_surface_penalty",
			},
			Metadata: map[string]string{
				"semantic_family_intent_identity":   "0.480",
				"semantic_family_intent_topic":      "0.620",
				"semantic_family_intent_merit":      "0.660",
				"semantic_family_intent_origin":     "0.050",
				"semantic_family_intent_derivative": "0.180",
				"semantic_origin_alignment":         "0.050",
				"semantic_derivative_alignment":     "0.180",
			},
		},
	}
	plan, _, _ := svc.buildQueryPlan(QueryRequest{Goal: "browser automation project", DiscoveryMode: QueryDiscoveryWeb}, core.ProfileTiny, "", QueryDiscoveryWeb)
	plan.SelectedURL = sourceURL
	plan.CandidateURLs = []string{sourceURL, derivativeURL}
	plan.CandidateDiagnostics = queryCandidateDiagnostics(candidates)

	resp, err := svc.readQuerySelectedCandidate(context.Background(), QueryRequest{Goal: "browser automation project"}, core.ProfileTiny, QueryDiscoveryWeb, &plan, candidates)
	if err != nil {
		t.Fatalf("expected selected read to succeed: %v", err)
	}
	if plan.SelectedURL != sourceURL || resp.Document.FinalURL != sourceURL {
		t.Fatalf("expected strong source selection to remain, plan=%q final=%q", plan.SelectedURL, resp.Document.FinalURL)
	}
	if hits["/derivative"] != 0 {
		t.Fatalf("expected derivative challenger not to be read, got hits=%d", hits["/derivative"])
	}
	for _, decision := range plan.Compiler.Decisions {
		if decision.Stage == "select.post_read_semantic_fallback" {
			t.Fatalf("did not expect post-read fallback decision, got %#v", plan.Compiler.Decisions)
		}
	}
}

func TestReadQuerySelectedCandidateUsesPreReadSemanticFamilyEvidenceFallback(t *testing.T) {
	hits := map[string]int{}
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path]++
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/derivative":
			_, _ = fmt.Fprint(w, `<html><head><title>Derivative automation surface</title></head><body><article><h1>Derivative automation surface</h1><p>Managed execution surface describing another browser automation project.</p></article></body></html>`)
		case "/source":
			_, _ = fmt.Fprint(w, `<html><head><title>Source automation project</title></head><body><article><h1>Source automation project</h1><p>Primary maintained project documentation and release surface.</p></article></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	sourceURL := serverURL + "/source"
	derivativeURL := serverURL + "/derivative"
	svc := newSemanticService(t, server.Client())
	svc.semantic = postReadSemanticTestAligner{scores: map[string]queryreview.Score{
		sourceURL: {Goal: 0.70, Entity: 0.70, Origin: 0.62, Derivative: 0.18},
	}}
	candidates := []DiscoverCandidate{
		{
			URL:   derivativeURL,
			Score: 2.00,
			Reason: []string{
				"semantic_goal_alignment",
				"semantic_topic_without_identity_penalty",
				"semantic_derivative_surface_penalty",
			},
			Metadata: map[string]string{
				"semantic_family_intent_identity":   "0.480",
				"semantic_family_intent_topic":      "0.620",
				"semantic_family_intent_merit":      "0.660",
				"semantic_family_intent_origin":     "0.050",
				"semantic_family_intent_derivative": "0.180",
				"semantic_origin_alignment":         "0.050",
				"semantic_derivative_alignment":     "0.180",
			},
		},
		{
			URL:   sourceURL,
			Score: 1.60,
			Reason: []string{
				"semantic_goal_alignment",
				"semantic_family_evidence_mass",
			},
			Metadata: map[string]string{
				"semantic_family_intent_identity":     "0.525",
				"semantic_family_intent_topic":        "0.590",
				"semantic_family_intent_merit":        "0.682",
				"semantic_family_intent_origin":       "0.220",
				"semantic_family_intent_derivative":   "0.080",
				"semantic_family_intent_provenance":   "2",
				"semantic_family_evidence_support":    "0.320",
				"semantic_family_evidence_strong":     "2",
				"semantic_family_evidence_provenance": "1",
				"semantic_origin_alignment":           "0.220",
				"semantic_derivative_alignment":       "0.080",
			},
		},
	}
	plan, _, _ := svc.buildQueryPlan(QueryRequest{Goal: "browser automation project", DiscoveryMode: QueryDiscoveryWeb}, core.ProfileTiny, "", QueryDiscoveryWeb)
	plan.SelectedURL = derivativeURL
	plan.CandidateURLs = []string{derivativeURL, sourceURL}
	plan.CandidateDiagnostics = queryCandidateDiagnostics(candidates)

	resp, err := svc.readQuerySelectedCandidate(context.Background(), QueryRequest{Goal: "browser automation project"}, core.ProfileTiny, QueryDiscoveryWeb, &plan, candidates)
	if err != nil {
		t.Fatalf("expected pre-read semantic fallback to succeed: %v", err)
	}
	if plan.SelectedURL != sourceURL || resp.Document.FinalURL != sourceURL {
		t.Fatalf("expected source selection from pre-read validation, plan=%q final=%q", plan.SelectedURL, resp.Document.FinalURL)
	}
	if hits["/derivative"] != 0 {
		t.Fatalf("expected derivative selected candidate not to be read, got hits=%d", hits["/derivative"])
	}
	requireCompilerDecision(t, plan.Compiler.Decisions, QueryPlanReasonSelection, func(decision QueryPlanDecision) bool {
		return decision.Stage == "select.pre_read_semantic_fallback"
	})
}

func TestQueryPreReadSemanticFallbackDoesNotPromoteRedundantWithoutFamilyEvidence(t *testing.T) {
	selectedURL := "https://source.example/docs"
	redundantURL := "https://mirror.example/docs"
	candidates := []DiscoverCandidate{
		{
			URL:   selectedURL,
			Score: 6.50,
			Reason: []string{
				"semantic_goal_alignment",
				"semantic_derivative_surface_penalty",
				"candidate_cluster_representative",
				"semantic_provenance_identity",
			},
			Metadata: map[string]string{
				"semantic_family_intent_identity":   "0.270",
				"semantic_family_intent_topic":      "0.810",
				"semantic_family_intent_merit":      "0.480",
				"semantic_family_intent_origin":     "0.046",
				"semantic_family_intent_derivative": "0.234",
				"semantic_family_intent_provenance": "2",
				"semantic_origin_similarity":        "0.120",
				"semantic_derivative_similarity":    "0.234",
			},
		},
		{
			URL:   redundantURL,
			Score: 6.20,
			Reason: []string{
				"semantic_goal_alignment",
				"semantic_derivative_surface_penalty",
				"candidate_cluster_redundant",
				"semantic_provenance_identity",
			},
			Metadata: map[string]string{
				"semantic_family_intent_identity":   "0.400",
				"semantic_family_intent_topic":      "0.830",
				"semantic_family_intent_merit":      "0.620",
				"semantic_family_intent_origin":     "0.047",
				"semantic_family_intent_derivative": "0.231",
				"semantic_family_intent_provenance": "2",
				"semantic_origin_similarity":        "0.120",
				"semantic_derivative_similarity":    "0.231",
			},
		},
	}
	plan := QueryPlan{
		SelectedURL:          selectedURL,
		CandidateURLs:        []string{selectedURL, redundantURL},
		CandidateDiagnostics: queryCandidateDiagnostics(candidates),
	}

	if fallback, ok := queryreview.PreReadFallbackCandidate("", QueryDiscoveryWeb, QueryDiscoveryWeb, queryReviewPlan(&plan), candidates); ok {
		t.Fatalf("did not expect redundant candidate without family evidence to win, got %#v", fallback)
	}
}

func TestQueryPreReadSemanticFallbackAllowsProvenanceIdentityAdvantage(t *testing.T) {
	selected := queryreview.Diagnostic{
		URL:                            "https://aggregate.example/pricing",
		SemanticFamilyIntentIdentity:   0.34,
		SemanticFamilyIntentMerit:      0.50,
		SemanticFamilyIntentOrigin:     0.03,
		SemanticFamilyIntentDerivative: 0.18,
		Reasons: []string{
			"semantic_derivative_surface_penalty",
		},
	}
	challenger := queryreview.Diagnostic{
		URL:                            "https://source.example/pricing",
		SemanticFamilyIntentIdentity:   0.47,
		SemanticFamilyIntentMerit:      0.66,
		SemanticFamilyIntentOrigin:     0.04,
		SemanticFamilyIntentDerivative: 0.22,
		Reasons: []string{
			"semantic_provenance_identity",
			"candidate_cluster_redundant",
		},
	}

	if !queryreview.PreReadChallengerBeatsSelected(selected, challenger) {
		t.Fatal("expected provenance identity advantage to allow pre-read semantic fallback")
	}
}

func TestQueryPreReadSemanticFallbackAllowsDerivativeRelief(t *testing.T) {
	selected := queryreview.Diagnostic{
		URL:                            "https://docs.example/topic",
		SemanticFamilyIntentIdentity:   0.54,
		SemanticFamilyIntentMerit:      0.64,
		SemanticFamilyIntentDerivative: 0.22,
		Reasons: []string{
			"semantic_derivative_surface_penalty",
		},
	}
	challenger := queryreview.Diagnostic{
		URL:                            "https://source.example/",
		SemanticFamilyIntentIdentity:   0.53,
		SemanticFamilyIntentMerit:      0.69,
		SemanticFamilyIntentDerivative: 0.16,
		Reasons: []string{
			"host_root_identity_probe",
		},
	}

	if !queryreview.PreReadChallengerBeatsSelected(selected, challenger) {
		t.Fatal("expected derivative relief to allow pre-read semantic fallback")
	}
}

func TestQueryCompilerRecordsForcedLanePolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, testHTML)
	}))
	defer server.Close()

	svc := newSemanticService(t, server.Client())
	resp, err := svc.Query(context.Background(), QueryRequest{
		Goal:      "proof replay deterministic",
		SeedURL:   server.URL,
		Profile:   core.ProfileTiny,
		ForceLane: 2,
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	requireCompilerDecision(t, resp.Plan.Compiler.Decisions, QueryPlanReasonLanePolicy, func(decision QueryPlanDecision) bool {
		return decision.Choice == "forced_lane" && decision.Metadata["force_lane"] == "2"
	})
}

func TestQueryCompilerRecordsExecutionDriftOnRedirect(t *testing.T) {
	var redirectedURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.Redirect(w, r, redirectedURL, http.StatusFound)
		case "/final":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, testHTML)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	redirectedURL = server.URL + "/final"

	svc := newSemanticService(t, server.Client())
	resp, err := svc.Query(context.Background(), QueryRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       server.URL,
		Profile:       core.ProfileTiny,
		DiscoveryMode: QueryDiscoveryOff,
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	drift := requireCompilerDecision(t, resp.Plan.Compiler.Decisions, QueryPlanReasonExecutionDrift, func(decision QueryPlanDecision) bool {
		return decision.Choice == "drift"
	})
	if drift.Metadata["planned_url"] != server.URL || drift.Metadata["final_url"] != redirectedURL {
		t.Fatalf("unexpected drift metadata %#v", drift.Metadata)
	}
}

func TestQueryCompilerRecordsRuntimeEffectsDetected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, testHTML)
	}))
	defer server.Close()

	svc := newTestService(t, testConfig(), server.Client())
	resp, err := svc.Query(context.Background(), QueryRequest{
		Goal:      "proof replay deterministic",
		SeedURL:   server.URL,
		Profile:   core.ProfileTiny,
		ForceLane: 2,
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	runtimeEffects := requireCompilerDecision(t, resp.Plan.Compiler.Decisions, QueryPlanReasonRuntimeEffectsDetected, func(decision QueryPlanDecision) bool {
		return decision.Stage == "verify.runtime_effects"
	})
	if runtimeEffects.Metadata["escalation_count"] == "" {
		t.Fatalf("expected escalation_count metadata, got %#v", runtimeEffects.Metadata)
	}
}

func TestQueryCompilerValidateRejectsIntentAfterExecutionStage(t *testing.T) {
	compiler := QueryCompiler{
		Version: QueryCompilerVersion,
		Decisions: []QueryPlanDecision{
			{Stage: "input.seed", Choice: "present", ReasonCode: QueryPlanReasonSeedPresent},
			{Stage: "observe.web_ir", Choice: "web_ir_observed", ReasonCode: QueryPlanReasonWebIR},
			{Stage: "select.candidate", Choice: "https://example.com", ReasonCode: QueryPlanReasonSelection},
		},
	}
	if err := compiler.Validate(); err == nil {
		t.Fatal("expected invalid stage order to fail validation")
	}
}

func TestQueryDiscoversHigherSignalCandidate(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprintf(w, `<html><head><title>Home</title></head><body><article><h1>Home</h1><p>Index page.</p><a href="%s/docs/replay-guide">Replay Guide</a></article></body></html>`, serverURL)
		case "/docs/replay-guide":
			_, _ = fmt.Fprint(w, `<html><head><title>Replay Guide</title></head><body><article><h1>Replay Guide</h1><p>Proof replay deterministic context for operators.</p></article></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	svc := newSemanticService(t, server.Client())

	resp, err := svc.Query(context.Background(), QueryRequest{
		Goal:    "proof replay deterministic",
		SeedURL: server.URL,
		Profile: core.ProfileTiny,
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if resp.Plan.SelectedURL != server.URL+"/docs/replay-guide" {
		t.Fatalf("expected discovered docs page, got %q", resp.Plan.SelectedURL)
	}
	if resp.Document.FinalURL != server.URL+"/docs/replay-guide" {
		t.Fatalf("expected query to read selected page, got %q", resp.Document.FinalURL)
	}
	if len(resp.Plan.CandidateURLs) < 2 {
		t.Fatalf("expected discovery candidates, got %#v", resp.Plan.CandidateURLs)
	}
}

func TestQueryDiscoveryOffKeepsSeedURL(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprintf(w, `<html><head><title>Home</title></head><body><article><h1>Home</h1><p>Seed page.</p><a href="%s/docs/replay-guide">Replay Guide</a></article></body></html>`, serverURL)
		case "/docs/replay-guide":
			_, _ = fmt.Fprint(w, `<html><head><title>Replay Guide</title></head><body><article><h1>Replay Guide</h1><p>Proof replay deterministic context for operators.</p></article></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	svc := newTestService(t, testConfig(), server.Client())

	resp, err := svc.Query(context.Background(), QueryRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       server.URL,
		Profile:       core.ProfileTiny,
		DiscoveryMode: QueryDiscoveryOff,
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if resp.Plan.SelectedURL != server.URL {
		t.Fatalf("expected selected url to remain seed, got %q", resp.Plan.SelectedURL)
	}
	if len(resp.Plan.CandidateURLs) != 1 || resp.Plan.CandidateURLs[0] != server.URL {
		t.Fatalf("expected only seed candidate, got %#v", resp.Plan.CandidateURLs)
	}
	requireCompilerDecision(t, resp.Plan.Compiler.Decisions, QueryPlanReasonLowCandidateSetRisk, nil)
}

func TestQueryWebSearchUsesCrossSiteDiscovery(t *testing.T) {
	docsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Replay Guide</title></head><body><article><h1>Replay Guide</h1><p>Proof replay deterministic context for operators.</p></article></body></html>`)
	}))
	defer docsServer.Close()

	blogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Blog</title></head><body><article><h1>Blog</h1><p>Company updates.</p></article></body></html>`)
	}))
	defer blogServer.Close()

	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s">Company Blog</a><a class="result__a" href="%s">Replay Guide</a></body></html>`, blogServer.URL, docsServer.URL)
	}))
	defer searchServer.Close()

	svc := newSemanticService(t, searchServer.Client())
	svc.SetWebDiscoverBaseURL(searchServer.URL)

	resp, err := svc.Query(context.Background(), QueryRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       "https://seed.example/root",
		Profile:       core.ProfileTiny,
		DiscoveryMode: QueryDiscoveryWeb,
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if resp.Plan.SelectedURL != docsServer.URL {
		t.Fatalf("expected web search to select docs candidate, got %q", resp.Plan.SelectedURL)
	}
	if resp.Plan.DiscoveryProvider == "" {
		t.Fatal("expected discovery provider to be recorded")
	}
	requireCompilerDecision(t, resp.Plan.Compiler.Decisions, QueryPlanReasonWebIRSelection, nil)
	requireCompilerDecision(t, resp.Plan.Compiler.Decisions, QueryPlanReasonWebBootstrapFallback, nil)
}

func TestQueryCompilerAddsStableFingerprintEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, testHTML)
	}))
	defer server.Close()

	svc := newTestService(t, testConfig(), server.Client())
	resp, err := svc.Query(context.Background(), QueryRequest{
		Goal:        "proof replay deterministic",
		SeedURL:     server.URL,
		Profile:     core.ProfileTiny,
		SeedTraceID: "trace_prev",
		SeedStable:  1.0,
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	stable := requireCompilerDecision(t, resp.Plan.Compiler.Decisions, QueryPlanReasonStableRegionBias, nil)
	if stable.Metadata["latest_trace_id"] != "trace_prev" {
		t.Fatalf("expected latest_trace_id metadata, got %#v", stable.Metadata)
	}
}

func TestQueryCompilerAddsNoveltyAndDeltaRiskEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, testHTML)
	}))
	defer server.Close()

	svc := newTestService(t, testConfig(), server.Client())
	resp, err := svc.Query(context.Background(), QueryRequest{
		Goal:        "proof replay deterministic",
		SeedURL:     server.URL,
		Profile:     core.ProfileTiny,
		SeedTraceID: "trace_prev",
		SeedStable:  0.25,
		SeedNovelty: 0.75,
		SeedChanged: true,
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	requireCompilerDecision(t, resp.Plan.Compiler.Decisions, QueryPlanReasonNoveltyBias, nil)
	requireCompilerDecision(t, resp.Plan.Compiler.Decisions, QueryPlanReasonDeltaRisk, nil)
}

func TestRerankQueryCandidatesWithFingerprintEvidence(t *testing.T) {
	tests := []struct {
		name       string
		candidates []discoverycore.Candidate
		seedURL    string
		evidence   QueryFingerprintEvidence
		loader     func(string) (QueryFingerprintEvidence, bool)
		topURL     string
		reasonURL  string
		reason     string
		traceID    string
	}{
		{
			name: "penalizes stable seed",
			candidates: []discoverycore.Candidate{
				{URL: "https://seed.example", Score: 1.00, Reason: []string{"seed_fallback"}},
				{URL: "https://seed.example/docs", Score: 0.95, Reason: []string{"structure_hint"}},
			},
			seedURL:   "https://seed.example",
			evidence:  QueryFingerprintEvidence{TraceID: "trace_seed", Stable: 1.0},
			topURL:    "https://seed.example/docs",
			reasonURL: "https://seed.example",
			reason:    "stable_seed_penalty",
		},
		{
			name: "boosts novel seed",
			candidates: []discoverycore.Candidate{
				{URL: "https://seed.example", Score: 0.95, Reason: []string{"seed_fallback"}},
				{URL: "https://seed.example/docs", Score: 1.00, Reason: []string{"structure_hint"}},
			},
			seedURL:   "https://seed.example",
			evidence:  QueryFingerprintEvidence{TraceID: "trace_seed", Stable: 0.25, Novelty: 0.75, Changed: true},
			topURL:    "https://seed.example",
			reasonURL: "https://seed.example",
			reason:    "novel_seed_bias",
		},
		{
			name: "boosts known novel candidate",
			candidates: []discoverycore.Candidate{
				{URL: "https://seed.example", Score: 1.00, Reason: []string{"seed_fallback"}},
				{URL: "https://seed.example/docs", Score: 0.95, Reason: []string{"structure_hint"}},
			},
			seedURL: "https://seed.example",
			loader: func(url string) (QueryFingerprintEvidence, bool) {
				if url != "https://seed.example/docs" {
					return QueryFingerprintEvidence{}, false
				}
				return QueryFingerprintEvidence{TraceID: "trace_docs", Stable: 0.10, Novelty: 0.90, Changed: true}, true
			},
			topURL:    "https://seed.example/docs",
			reasonURL: "https://seed.example/docs",
			reason:    "novel_candidate_bias",
			traceID:   "trace_docs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidates := queryflow.RerankCandidatesWithFingerprintEvidence(tt.candidates, tt.seedURL, tt.evidence, tt.loader)
			if candidates[0].URL != tt.topURL {
				t.Fatalf("unexpected top candidate %#v", candidates)
			}
			for _, candidate := range candidates {
				if candidate.URL != tt.reasonURL {
					continue
				}
				if !containsReason(candidate.Reason, tt.reason) {
					t.Fatalf("expected %s reason, got %#v", tt.reason, candidate.Reason)
				}
				if tt.traceID != "" && candidate.Metadata["candidate_latest_trace_id"] != tt.traceID {
					t.Fatalf("expected candidate trace metadata, got %#v", candidate.Metadata)
				}
				return
			}
			t.Fatalf("expected reason url %q in %#v", tt.reasonURL, candidates)
		})
	}
}

func TestQueryWebSearchExpandsLandingPageToSelectedChild(t *testing.T) {
	var portalURL string
	portalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprintf(w, `<html><head><title>Portal</title></head><body><article><h1>Portal</h1><p>Overview.</p><a href="%s/docs/replay-proof">Replay Proof Guide</a></article></body></html>`, portalURL)
		case "/docs/replay-proof":
			_, _ = fmt.Fprint(w, `<html><head><title>Replay Proof Guide</title></head><body><article><h1>Replay Proof Guide</h1><p>Proof replay deterministic context for operators.</p></article></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer portalServer.Close()
	portalURL = portalServer.URL

	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s">Official Site</a></body></html>`, portalServer.URL)
	}))
	defer searchServer.Close()

	svc := newTestService(t, testConfig(), searchServer.Client())
	svc.SetWebDiscoverBaseURL(searchServer.URL)

	resp, err := svc.Query(context.Background(), QueryRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       "https://seed.example/root",
		Profile:       core.ProfileTiny,
		DiscoveryMode: QueryDiscoveryWeb,
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if resp.Plan.SelectedURL != portalServer.URL+"/docs/replay-proof" {
		t.Fatalf("expected expanded docs child to be selected, got %q", resp.Plan.SelectedURL)
	}
	if resp.Document.FinalURL != portalServer.URL+"/docs/replay-proof" {
		t.Fatalf("expected selected child page to be read, got %q", resp.Document.FinalURL)
	}
}

func TestQueryRejectsMissingGoal(t *testing.T) {
	svc := newTestService(t, testConfig(), nil)
	_, err := svc.Query(context.Background(), QueryRequest{SeedURL: "https://example.com"})
	if err == nil {
		t.Fatal("expected missing goal to fail")
	}
}

func TestQueryRequiresSeedWhenDiscoveryOff(t *testing.T) {
	svc := newTestService(t, testConfig(), nil)
	_, err := svc.Query(context.Background(), QueryRequest{
		Goal:          "proof replay deterministic",
		DiscoveryMode: QueryDiscoveryOff,
	})
	if err == nil {
		t.Fatal("expected missing seed to fail when discovery off")
	}
}

func TestQueryWithoutSeedRequiresExplicitWebSearch(t *testing.T) {
	svc := newTestService(t, testConfig(), nil)
	_, err := svc.Query(context.Background(), QueryRequest{Goal: "proof replay deterministic"})
	if err == nil || !strings.Contains(err.Error(), "seedless discovery is experimental") || !strings.Contains(err.Error(), "explicit discovery_mode=web_search") {
		t.Fatalf("expected explicit experimental opt-in error, got %v", err)
	}
}

func TestQueryWithoutSeedUsesExplicitWebDiscovery(t *testing.T) {
	docsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Replay Guide</title></head><body><article><h1>Replay Guide</h1><p>Proof replay deterministic context for operators.</p></article></body></html>`)
	}))
	defer docsServer.Close()

	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s">Replay Guide</a></body></html>`, docsServer.URL)
	}))
	defer searchServer.Close()

	svc := newTestService(t, testConfig(), searchServer.Client())
	svc.SetWebDiscoverBaseURL(searchServer.URL)

	resp, err := svc.Query(context.Background(), QueryRequest{
		Goal:          "proof replay deterministic",
		Profile:       core.ProfileTiny,
		DiscoveryMode: QueryDiscoveryWeb,
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if resp.Plan.DiscoveryMode != QueryDiscoveryWeb {
		t.Fatalf("expected explicit no-seed discovery mode to remain web, got %q", resp.Plan.DiscoveryMode)
	}
	requireCompilerDecision(t, resp.Plan.Compiler.Decisions, QueryPlanReasonExperimentalWebOptIn, nil)
	if resp.Plan.SeedURL != "" {
		t.Fatalf("expected empty seed in plan, got %q", resp.Plan.SeedURL)
	}
	if resp.Plan.SelectedURL != docsServer.URL {
		t.Fatalf("expected selected url from web discovery, got %q", resp.Plan.SelectedURL)
	}
	if len(resp.Plan.CandidateURLs) == 0 {
		t.Fatal("expected candidate urls from web discovery")
	}
}

func TestQueryRecordsGraphEvidenceForCrossDomainHintSelection(t *testing.T) {
	docsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Replay Guide</title></head><body><article><h1>Replay Guide</h1><p>Proof replay deterministic context for operators.</p></article></body></html>`)
	}))
	defer docsServer.Close()

	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s">Replay Guide</a></body></html>`, docsServer.URL)
	}))
	defer searchServer.Close()

	svc := newTestService(t, testConfig(), searchServer.Client())
	svc.SetWebDiscoverBaseURL(searchServer.URL)

	resp, err := svc.Query(context.Background(), QueryRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       "https://seed.example/root",
		Profile:       core.ProfileTiny,
		DiscoveryMode: QueryDiscoveryWeb,
		DomainHints:   []string{"seed.example", hostFromURLString(docsServer.URL)},
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	requireCompilerDecision(t, resp.Plan.Compiler.Decisions, QueryPlanReasonGraphEvidence, nil)
}

func TestQueryWebSearchLocalSubstrateDoesNotEmitBootstrapFallback(t *testing.T) {
	var seedURL string
	seedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprintf(w, `<html><head><title>Portal</title></head><body><article><a href="%s/docs/replay">Replay Guide</a></article></body></html>`, seedURL)
		case "/docs/replay":
			_, _ = fmt.Fprint(w, `<html><head><title>Replay Guide</title></head><body><article><h1>Replay Guide</h1><p>Proof replay deterministic context.</p></article></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer seedServer.Close()
	seedURL = seedServer.URL

	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><body><a class="result__a" href="https://external.example/replay">Replay</a></body></html>`)
	}))
	defer searchServer.Close()

	svc := newSemanticService(t, seedServer.Client())
	svc.SetWebDiscoverBaseURL(searchServer.URL)

	resp, err := svc.Query(context.Background(), QueryRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       seedServer.URL,
		Profile:       core.ProfileTiny,
		DiscoveryMode: QueryDiscoveryWeb,
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if resp.Plan.DiscoveryProvider != "local_same_site" {
		t.Fatalf("expected local_same_site provider, got %q", resp.Plan.DiscoveryProvider)
	}
	forbidCompilerDecision(t, resp.Plan.Compiler.Decisions, QueryPlanReasonWebBootstrapFallback)
}

func TestQuerySeedlessWebSearchUsesRewriteQueries(t *testing.T) {
	pageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>ASD Charly Brown</title></head><body><article><h1>ASD Charly Brown</h1><p>Scuola di danza a Cassine.</p></article></body></html>`)
	}))
	defer pageServer.Close()

	searchHits := []string{}
	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searchHits = append(searchHits, r.URL.Query().Get("q"))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if strings.Contains(r.URL.Query().Get("q"), `"ASD Charly Brown"`) {
			_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s/asd-charly-brown">ASD Charly Brown</a></body></html>`, pageServer.URL)
			return
		}
		_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s/other-a">Other Dance School</a><a class="result__a" href="%s/other-b">Cassine Events</a></body></html>`, pageServer.URL, pageServer.URL)
	}))
	defer searchServer.Close()

	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		content := `{"search_queries":["\"ASD Charly Brown\" Alessandria","\"ASD Charly Brown\" scuola di danza"],"canonical_entity":"ASD Charly Brown","locality_hints":["alessandria"],"category_hints":["scuola di danza"],"confidence":0.92}`
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"finish_reason": "stop", "message": map[string]any{"content": content}}}, "usage": map[string]any{"prompt_tokens": 32, "completion_tokens": 24}})
	}))
	defer modelServer.Close()

	cfg := testConfig()
	cfg.Models.Backend = "openai-compatible"
	cfg.Models.BaseURL = modelServer.URL
	cfg.Models.Router = "gemma3:1b-it-q8_0"
	cfg.Models.MicroTimeoutMS = 1500
	enableDiscoverSemantic(&cfg, "")
	svc := newTestService(t, cfg, pageServer.Client())
	svc.SetWebDiscoverBaseURL(searchServer.URL)

	resp, err := svc.Query(context.Background(), QueryRequest{Goal: "ASD Charly Brown dance school Alessandria", DiscoveryMode: QueryDiscoveryWeb})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if resp.Plan.SelectedURL != pageServer.URL+"/asd-charly-brown" {
		t.Fatalf("unexpected selected url %q", resp.Plan.SelectedURL)
	}
	if len(searchHits) < 2 {
		t.Fatalf("expected multiple rewritten search hits, got %#v", searchHits)
	}
	rewrite := requireCompilerDecision(t, resp.Plan.Compiler.Decisions, queryplan.QueryPlanReasonRewrite, nil)
	if rewrite.Metadata["query_count"] == "" {
		t.Fatalf("missing rewrite metadata %#v", rewrite.Metadata)
	}
}

func TestQuerySeedlessSkipsRewriteWhenBudgetIsLow(t *testing.T) {
	pageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Budgeted result</title></head><body><article><h1>Budgeted result</h1><p>Seedless query should keep first discovery when the rewrite budget is low.</p></article></body></html>`)
	}))
	defer pageServer.Close()

	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s/first">First result</a><a class="result__a" href="%s/second">Second result</a></body></html>`, pageServer.URL, pageServer.URL)
	}))
	defer searchServer.Close()

	modelHits := 0
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelHits++
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"finish_reason": "stop", "message": map[string]any{"content": `{"search_queries":["refined query"],"canonical_entity":"Budgeted","confidence":0.9}`}}}})
	}))
	defer modelServer.Close()

	cfg := testConfig()
	cfg.Models.Backend = "openai-compatible"
	cfg.Models.BaseURL = modelServer.URL
	cfg.Models.Router = "gemma3:1b-it-q8_0"
	cfg.Models.MicroTimeoutMS = 500
	svc := newTestService(t, cfg, pageServer.Client())
	svc.SetWebDiscoverBaseURL(searchServer.URL)

	ctx, cancel := context.WithTimeout(context.Background(), seedlessRewriteMinTimeLeft-time.Second)
	defer cancel()
	resp, err := svc.Query(ctx, QueryRequest{Goal: "ambiguous budgeted result", DiscoveryMode: QueryDiscoveryWeb})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if modelHits != 0 {
		t.Fatalf("expected rewrite model to be skipped under low budget, got %d hits", modelHits)
	}
	if resp.Plan.SelectedURL == "" {
		t.Fatal("expected first discovery selection to be preserved")
	}
	if hasCompilerDecision(resp.Plan.Compiler.Decisions, queryplan.QueryPlanReasonRewrite) {
		t.Fatalf("did not expect rewrite decision under low budget")
	}
}

func TestQueryDiscoveryOffGuidesOnSeed404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	svc := newTestService(t, testConfig(), server.Client())
	_, err := svc.Query(context.Background(), QueryRequest{
		Goal:          "Read the initialize method",
		SeedURL:       server.URL + "/missing-page",
		DiscoveryMode: QueryDiscoveryOff,
	})
	if err == nil {
		t.Fatal("expected query to fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "seed_url returned 404") || !strings.Contains(msg, "same_site_links") || !strings.Contains(msg, "web_read") || strings.Contains(msg, "web_search") {
		t.Fatalf("unexpected guided error: %q", msg)
	}
}

func hasCompilerDecision(decisions []QueryPlanDecision, reason string) bool {
	for _, decision := range decisions {
		if decision.ReasonCode == reason {
			return true
		}
	}
	return false
}
