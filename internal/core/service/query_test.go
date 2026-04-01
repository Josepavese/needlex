package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/core/queryflow"
	"github.com/josepavese/needlex/internal/core/queryplan"
)

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

	svc := newTestService(t, config.Defaults(), server.Client())
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

	svc := newTestService(t, config.Defaults(), server.Client())

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

	svc := newTestService(t, config.Defaults(), server.Client())
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

	svc := newTestService(t, config.Defaults(), server.Client())
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
				{URL: "https://seed.example/docs", Score: 0.95, Reason: []string{"path_hint"}},
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
				{URL: "https://seed.example/docs", Score: 1.00, Reason: []string{"path_hint"}},
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
				{URL: "https://seed.example/docs", Score: 0.95, Reason: []string{"path_hint"}},
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

	svc := newTestService(t, config.Defaults(), searchServer.Client())
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
	svc := newTestService(t, config.Defaults(), nil)
	_, err := svc.Query(context.Background(), QueryRequest{SeedURL: "https://example.com"})
	if err == nil {
		t.Fatal("expected missing goal to fail")
	}
}

func TestQueryRequiresSeedWhenDiscoveryOff(t *testing.T) {
	svc := newTestService(t, config.Defaults(), nil)
	_, err := svc.Query(context.Background(), QueryRequest{
		Goal:          "proof replay deterministic",
		DiscoveryMode: QueryDiscoveryOff,
	})
	if err == nil {
		t.Fatal("expected missing seed to fail when discovery off")
	}
}

func TestQueryWithoutSeedUsesWebDiscovery(t *testing.T) {
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

	svc := newTestService(t, config.Defaults(), searchServer.Client())
	svc.SetWebDiscoverBaseURL(searchServer.URL)

	resp, err := svc.Query(context.Background(), QueryRequest{
		Goal:    "proof replay deterministic",
		Profile: core.ProfileTiny,
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if resp.Plan.DiscoveryMode != QueryDiscoveryWeb {
		t.Fatalf("expected default no-seed discovery mode to switch to web, got %q", resp.Plan.DiscoveryMode)
	}
	requireCompilerDecision(t, resp.Plan.Compiler.Decisions, QueryPlanReasonSeedlessDefaultWeb, nil)
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

	svc := newTestService(t, config.Defaults(), searchServer.Client())
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

	cfg := config.Defaults()
	cfg.Models.Backend = "openai-compatible"
	cfg.Models.BaseURL = modelServer.URL
	cfg.Models.Router = "gemma3:1b-it-q8_0"
	cfg.Models.MicroTimeoutMS = 1500
	cfg.Semantic.Enabled = false
	svc := newTestService(t, cfg, pageServer.Client())
	svc.SetWebDiscoverBaseURL(searchServer.URL)

	resp, err := svc.Query(context.Background(), QueryRequest{Goal: "ASD Charly Brown dance school Alessandria"})
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

func hasCompilerDecision(decisions []QueryPlanDecision, reason string) bool {
	for _, decision := range decisions {
		if decision.ReasonCode == reason {
			return true
		}
	}
	return false
}
