package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
)

func TestQueryBuildsPlanAndResultPack(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, testHTML)
	}))
	defer server.Close()

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

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
	foundWebIRDecision := false
	foundQualityMode := false
	foundLanePolicy := false
	foundExecutionAlignment := false
	foundPlanDiff := false
	foundRuntimeEffects := false
	foundIntentBoundary := false
	foundExecutionBoundary := false
	foundBudgetOutcome := false
	for _, decision := range resp.Plan.Compiler.Decisions {
		if decision.ReasonCode == QueryPlanReasonWebIR {
			foundWebIRDecision = true
			if decision.Metadata["node_count"] == "" || decision.Metadata["embedded_node_count"] == "" || decision.Metadata["heading_ratio"] == "" || decision.Metadata["short_text_ratio"] == "" || decision.Metadata["dominant_signal"] == "" {
				t.Fatalf("expected rich web ir metadata, got %#v", decision.Metadata)
			}
		}
		if decision.ReasonCode == QueryPlanReasonQualityLatencyMode {
			foundQualityMode = true
		}
		if decision.ReasonCode == QueryPlanReasonLanePolicy {
			foundLanePolicy = true
		}
		if decision.ReasonCode == QueryPlanReasonExecutionAligned {
			foundExecutionAlignment = true
		}
		if decision.ReasonCode == QueryPlanReasonPlanDiffObserved {
			foundPlanDiff = true
			if decision.Metadata["added_stage_count"] == "" {
				t.Fatalf("expected plan diff metadata in %#v", decision)
			}
		}
		if decision.ReasonCode == QueryPlanReasonRuntimeEffectsClean || decision.ReasonCode == QueryPlanReasonRuntimeEffectsDetected {
			foundRuntimeEffects = true
		}
		if decision.ReasonCode == QueryPlanReasonIntentBoundary {
			foundIntentBoundary = true
		}
		if decision.ReasonCode == QueryPlanReasonExecutionBoundary {
			foundExecutionBoundary = true
		}
		if decision.ReasonCode == QueryPlanReasonBudgetOutcomeOK || decision.ReasonCode == QueryPlanReasonBudgetOutcomeExceeded {
			foundBudgetOutcome = true
		}
	}
	if !foundWebIRDecision {
		t.Fatalf("expected compiler reason %q in %#v", QueryPlanReasonWebIR, resp.Plan.Compiler.Decisions)
	}
	if !foundQualityMode {
		t.Fatalf("expected compiler reason %q in %#v", QueryPlanReasonQualityLatencyMode, resp.Plan.Compiler.Decisions)
	}
	if !foundLanePolicy {
		t.Fatalf("expected compiler reason %q in %#v", QueryPlanReasonLanePolicy, resp.Plan.Compiler.Decisions)
	}
	if !foundExecutionAlignment {
		t.Fatalf("expected compiler reason %q in %#v", QueryPlanReasonExecutionAligned, resp.Plan.Compiler.Decisions)
	}
	if !foundPlanDiff {
		t.Fatalf("expected compiler reason %q in %#v", QueryPlanReasonPlanDiffObserved, resp.Plan.Compiler.Decisions)
	}
	if !foundRuntimeEffects {
		t.Fatalf("expected runtime effects decision in %#v", resp.Plan.Compiler.Decisions)
	}
	if !foundIntentBoundary || !foundExecutionBoundary {
		t.Fatalf("expected intent/execution boundaries in %#v", resp.Plan.Compiler.Decisions)
	}
	if !foundBudgetOutcome {
		t.Fatalf("expected budget outcome decision in %#v", resp.Plan.Compiler.Decisions)
	}
	if resp.WebIR.Version != core.WebIRVersion {
		t.Fatalf("expected web_ir version %q, got %q", core.WebIRVersion, resp.WebIR.Version)
	}
	if resp.TraceID == "" {
		t.Fatal("expected trace id")
	}
}

func TestQueryCompilerRecordsForcedLanePolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, testHTML)
	}))
	defer server.Close()

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	resp, err := svc.Query(context.Background(), QueryRequest{
		Goal:      "proof replay deterministic",
		SeedURL:   server.URL,
		Profile:   core.ProfileTiny,
		ForceLane: 2,
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	found := false
	for _, decision := range resp.Plan.Compiler.Decisions {
		if decision.ReasonCode == QueryPlanReasonLanePolicy && decision.Choice == "forced_lane" && decision.Metadata["force_lane"] == "2" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected forced lane policy decision in %#v", resp.Plan.Compiler.Decisions)
	}
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

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	resp, err := svc.Query(context.Background(), QueryRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       server.URL,
		Profile:       core.ProfileTiny,
		DiscoveryMode: QueryDiscoveryOff,
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	found := false
	for _, decision := range resp.Plan.Compiler.Decisions {
		if decision.ReasonCode == QueryPlanReasonExecutionDrift && decision.Choice == "drift" {
			found = true
			if decision.Metadata["planned_url"] != server.URL || decision.Metadata["final_url"] != redirectedURL {
				t.Fatalf("unexpected drift metadata %#v", decision.Metadata)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected execution drift decision in %#v", resp.Plan.Compiler.Decisions)
	}
}

func TestQueryCompilerRecordsRuntimeEffectsDetected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, testHTML)
	}))
	defer server.Close()

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	resp, err := svc.Query(context.Background(), QueryRequest{
		Goal:      "proof replay deterministic",
		SeedURL:   server.URL,
		Profile:   core.ProfileTiny,
		ForceLane: 2,
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	found := false
	for _, decision := range resp.Plan.Compiler.Decisions {
		if decision.ReasonCode == QueryPlanReasonRuntimeEffectsDetected && decision.Stage == "verify.runtime_effects" {
			found = true
			if decision.Metadata["escalation_count"] == "" {
				t.Fatalf("expected escalation_count metadata, got %#v", decision.Metadata)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected runtime effects detected decision in %#v", resp.Plan.Compiler.Decisions)
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

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

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

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

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
	foundRisk := false
	for _, decision := range resp.Plan.Compiler.Decisions {
		if decision.ReasonCode == QueryPlanReasonLowCandidateSetRisk {
			foundRisk = true
			break
		}
	}
	if !foundRisk {
		t.Fatalf("expected compiler reason %q in %#v", QueryPlanReasonLowCandidateSetRisk, resp.Plan.Compiler.Decisions)
	}
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

	svc, err := New(config.Defaults(), searchServer.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}
	svc.webDiscoverBaseURL = searchServer.URL

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
	foundPlanningWebIR := false
	for _, decision := range resp.Plan.Compiler.Decisions {
		if decision.ReasonCode == QueryPlanReasonWebIRSelection {
			foundPlanningWebIR = true
			break
		}
	}
	if !foundPlanningWebIR {
		t.Fatalf("expected compiler reason %q in %#v", QueryPlanReasonWebIRSelection, resp.Plan.Compiler.Decisions)
	}
	foundFallback := false
	for _, decision := range resp.Plan.Compiler.Decisions {
		if decision.ReasonCode == QueryPlanReasonWebBootstrapFallback {
			foundFallback = true
			break
		}
	}
	if !foundFallback {
		t.Fatalf("expected compiler reason %q in %#v", QueryPlanReasonWebBootstrapFallback, resp.Plan.Compiler.Decisions)
	}
}

func TestQueryCompilerAddsStableFingerprintEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, testHTML)
	}))
	defer server.Close()

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
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
	found := false
	for _, decision := range resp.Plan.Compiler.Decisions {
		if decision.ReasonCode == QueryPlanReasonStableRegionBias {
			found = true
			if decision.Metadata["latest_trace_id"] != "trace_prev" {
				t.Fatalf("expected latest_trace_id metadata, got %#v", decision.Metadata)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected compiler reason %q in %#v", QueryPlanReasonStableRegionBias, resp.Plan.Compiler.Decisions)
	}
}

func TestQueryCompilerAddsNoveltyAndDeltaRiskEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, testHTML)
	}))
	defer server.Close()

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
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
	foundNovelty := false
	foundDeltaRisk := false
	for _, decision := range resp.Plan.Compiler.Decisions {
		if decision.ReasonCode == QueryPlanReasonNoveltyBias {
			foundNovelty = true
		}
		if decision.ReasonCode == QueryPlanReasonDeltaRisk {
			foundDeltaRisk = true
		}
	}
	if !foundNovelty {
		t.Fatalf("expected compiler reason %q in %#v", QueryPlanReasonNoveltyBias, resp.Plan.Compiler.Decisions)
	}
	if !foundDeltaRisk {
		t.Fatalf("expected compiler reason %q in %#v", QueryPlanReasonDeltaRisk, resp.Plan.Compiler.Decisions)
	}
}

func TestRerankQueryCandidatesWithFingerprintEvidencePenalizesStableSeed(t *testing.T) {
	candidates := rerankQueryCandidatesWithFingerprintEvidence([]DiscoverCandidate{
		{URL: "https://seed.example", Score: 1.00, Reason: []string{"seed_fallback"}},
		{URL: "https://seed.example/docs", Score: 0.95, Reason: []string{"path_hint"}},
	}, "https://seed.example", QueryFingerprintEvidence{TraceID: "trace_seed", Stable: 1.0}, nil)
	if candidates[0].URL != "https://seed.example/docs" {
		t.Fatalf("expected stable seed penalty to demote seed, got %#v", candidates)
	}
	if !containsReason(candidates[1].Reason, "stable_seed_penalty") {
		t.Fatalf("expected stable_seed_penalty reason, got %#v", candidates[1].Reason)
	}
}

func TestRerankQueryCandidatesWithFingerprintEvidenceBoostsNovelSeed(t *testing.T) {
	candidates := rerankQueryCandidatesWithFingerprintEvidence([]DiscoverCandidate{
		{URL: "https://seed.example", Score: 0.95, Reason: []string{"seed_fallback"}},
		{URL: "https://seed.example/docs", Score: 1.00, Reason: []string{"path_hint"}},
	}, "https://seed.example", QueryFingerprintEvidence{TraceID: "trace_seed", Stable: 0.25, Novelty: 0.75, Changed: true}, nil)
	if candidates[0].URL != "https://seed.example" {
		t.Fatalf("expected novel seed bias to keep seed on top, got %#v", candidates)
	}
	if !containsReason(candidates[0].Reason, "novel_seed_bias") {
		t.Fatalf("expected novel_seed_bias reason, got %#v", candidates[0].Reason)
	}
}

func TestRerankQueryCandidatesWithFingerprintEvidenceBoostsKnownNovelCandidate(t *testing.T) {
	candidates := rerankQueryCandidatesWithFingerprintEvidence([]DiscoverCandidate{
		{URL: "https://seed.example", Score: 1.00, Reason: []string{"seed_fallback"}},
		{URL: "https://seed.example/docs", Score: 0.95, Reason: []string{"path_hint"}},
	}, "https://seed.example", QueryFingerprintEvidence{}, func(url string) (QueryFingerprintEvidence, bool) {
		if url != "https://seed.example/docs" {
			return QueryFingerprintEvidence{}, false
		}
		return QueryFingerprintEvidence{TraceID: "trace_docs", Stable: 0.10, Novelty: 0.90, Changed: true}, true
	})
	if candidates[0].URL != "https://seed.example/docs" {
		t.Fatalf("expected novel candidate bias to promote docs candidate, got %#v", candidates)
	}
	if !containsReason(candidates[0].Reason, "novel_candidate_bias") {
		t.Fatalf("expected novel_candidate_bias reason, got %#v", candidates[0].Reason)
	}
	if candidates[0].Metadata["candidate_latest_trace_id"] != "trace_docs" {
		t.Fatalf("expected candidate trace metadata, got %#v", candidates[0].Metadata)
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

	svc, err := New(config.Defaults(), searchServer.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}
	svc.webDiscoverBaseURL = searchServer.URL

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
	svc, err := New(config.Defaults(), nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = svc.Query(context.Background(), QueryRequest{SeedURL: "https://example.com"})
	if err == nil {
		t.Fatal("expected missing goal to fail")
	}
}

func TestQueryRequiresSeedWhenDiscoveryOff(t *testing.T) {
	svc, err := New(config.Defaults(), nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = svc.Query(context.Background(), QueryRequest{
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

	svc, err := New(config.Defaults(), searchServer.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}
	svc.webDiscoverBaseURL = searchServer.URL

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
	foundSeedlessReason := false
	for _, decision := range resp.Plan.Compiler.Decisions {
		if decision.ReasonCode == QueryPlanReasonSeedlessDefaultWeb {
			foundSeedlessReason = true
			break
		}
	}
	if !foundSeedlessReason {
		t.Fatalf("expected compiler reason %q in %#v", QueryPlanReasonSeedlessDefaultWeb, resp.Plan.Compiler.Decisions)
	}
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

	svc, err := New(config.Defaults(), searchServer.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}
	svc.webDiscoverBaseURL = searchServer.URL

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
	foundGraphEvidence := false
	for _, decision := range resp.Plan.Compiler.Decisions {
		if decision.ReasonCode == QueryPlanReasonGraphEvidence {
			foundGraphEvidence = true
			break
		}
	}
	if !foundGraphEvidence {
		t.Fatalf("expected compiler reason %q in %#v", QueryPlanReasonGraphEvidence, resp.Plan.Compiler.Decisions)
	}
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

	svc, err := New(config.Defaults(), seedServer.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.webDiscoverBaseURL = searchServer.URL

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
	for _, decision := range resp.Plan.Compiler.Decisions {
		if decision.ReasonCode == QueryPlanReasonWebBootstrapFallback {
			t.Fatalf("did not expect bootstrap fallback reason when local substrate resolves, got %#v", resp.Plan.Compiler.Decisions)
		}
	}
}
