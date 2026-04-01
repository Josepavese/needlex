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
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/proof"
)

type fakeSemanticAligner struct {
	suppressed bool
	reason     string
}

func (f fakeSemanticAligner) Align(context.Context, string, []intel.SemanticCandidate) (intel.SemanticAlignment, error) {
	return intel.SemanticAlignment{Suppressed: f.suppressed, Reason: f.reason}, nil
}

func (f fakeSemanticAligner) Score(_ context.Context, _ string, candidates []intel.SemanticCandidate) ([]intel.SemanticScore, error) {
	out := make([]intel.SemanticScore, 0, len(candidates))
	for _, candidate := range candidates {
		score := 0.10
		if f.suppressed {
			score = 0.92
		}
		out = append(out, intel.SemanticScore{ID: candidate.ID, Similarity: score})
	}
	return out, nil
}

func TestBuildResolveAmbiguityInputBuildsGroundedCandidates(t *testing.T) {
	input, ok := buildResolveAmbiguityInput("proof replay deterministic", core.WebIR{Signals: core.WebIRSignals{SubstrateClass: "generic_content"}}, []rankedSegment{
		{
			chunk: core.Chunk{
				ID:          "chk_1",
				Fingerprint: "fp_1",
				Text:        "Proof replay deterministic context.",
				HeadingPath: []string{"Overview"},
				Score:       0.82,
				Confidence:  0.76,
			},
			ir: segmentIREvidence{kindMatch: true, headingBacked: true},
		},
		{
			chunk: core.Chunk{
				ID:          "chk_2",
				Fingerprint: "fp_2",
				Text:        "Replay context for operators.",
				HeadingPath: []string{"Troubleshooting"},
				Score:       0.81,
				Confidence:  0.74,
			},
			ir: segmentIREvidence{embedded: true},
		},
	}, map[string]intel.Decision{
		"fp_1": {RiskFlags: []string{"high_ambiguity"}},
		"fp_2": {RiskFlags: []string{"coverage_gap"}},
	})
	if !ok {
		t.Fatal("expected ambiguity input to be built")
	}
	if err := input.Validate(); err != nil {
		t.Fatalf("validate ambiguity input: %v", err)
	}
	if len(input.Candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(input.Candidates))
	}
	if len(input.Candidates[0].IRSignals) == 0 {
		t.Fatal("expected IR signals on ambiguity candidate")
	}
}

func TestBuildResolveAmbiguityInputSkipsWhenOnlyOneCandidateIsActuallyAmbiguous(t *testing.T) {
	input, ok := buildResolveAmbiguityInput("service offering", core.WebIR{Signals: core.WebIRSignals{SubstrateClass: "generic_content"}}, []rankedSegment{
		{
			chunk: core.Chunk{
				ID:          "chk_1",
				Fingerprint: "fp_1",
				Text:        "Needle-X offers web development and digital marketing services.",
				HeadingPath: []string{"Services"},
				Score:       0.95,
				Confidence:  0.90,
			},
		},
		{
			chunk: core.Chunk{
				ID:          "chk_2",
				Fingerprint: "fp_2",
				Text:        "Contact the team for a project estimate.",
				HeadingPath: []string{"CTA"},
				Score:       0.80,
				Confidence:  0.72,
			},
		},
	}, map[string]intel.Decision{
		"fp_1": {Fingerprint: "fp_1", Lane: 0},
		"fp_2": {Fingerprint: "fp_2", Lane: 1, RiskFlags: []string{"high_ambiguity"}},
	})
	if ok {
		t.Fatalf("expected ambiguity route to stay off, got %#v", input)
	}
}

func TestBuildResolveAmbiguityInputSkipsWhenObjectiveAlreadyResolved(t *testing.T) {
	input, ok := buildResolveAmbiguityInput("web server platform summary", core.WebIR{Signals: core.WebIRSignals{SubstrateClass: "generic_content"}}, []rankedSegment{
		{
			chunk: core.Chunk{
				ID:          "chk_1",
				Fingerprint: "fp_1",
				Text:        "Nginx is a web server platform used as a proxy and load balancer.",
				HeadingPath: []string{"Overview"},
				Score:       0.92,
				Confidence:  0.88,
			},
		},
		{
			chunk: core.Chunk{
				ID:          "chk_2",
				Fingerprint: "fp_2",
				Text:        "Documentation and downloads.",
				HeadingPath: []string{"Resources"},
				Score:       0.83,
				Confidence:  0.83,
			},
		},
	}, map[string]intel.Decision{
		"fp_1": {Fingerprint: "fp_1", Lane: 1, RiskFlags: []string{"short_segment"}},
		"fp_2": {Fingerprint: "fp_2", Lane: 0},
	})
	if ok {
		t.Fatalf("expected structurally resolved anchor to keep ambiguity route off, got %#v", input)
	}
}

func TestBuildResolveAmbiguityInputSkipsForEmbeddedAppPayloadSubstrate(t *testing.T) {
	input, ok := buildResolveAmbiguityInput("company profile", core.WebIR{Signals: core.WebIRSignals{SubstrateClass: "embedded_app_payload"}}, []rankedSegment{
		{chunk: core.Chunk{ID: "chk_1", Fingerprint: "fp_1", Text: "Embedded business catalog snapshot.", HeadingPath: []string{"Catalog"}, Score: 0.72, Confidence: 0.71}},
		{chunk: core.Chunk{ID: "chk_2", Fingerprint: "fp_2", Text: "Business geometry and categories.", HeadingPath: []string{"Catalog"}, Score: 0.70, Confidence: 0.70}},
	}, map[string]intel.Decision{
		"fp_1": {Fingerprint: "fp_1", Lane: 1, RiskFlags: []string{"high_ambiguity"}},
		"fp_2": {Fingerprint: "fp_2", Lane: 1, RiskFlags: []string{"coverage_gap"}},
	})
	if ok {
		t.Fatalf("expected embedded_app_payload to suppress ambiguity route, got %#v", input)
	}
}

func TestBuildResolveAmbiguityInputSkipsForThemeHeavyWordPressWhenCoverageIsGood(t *testing.T) {
	input, ok := buildResolveAmbiguityInput("digital marketing websites", core.WebIR{Signals: core.WebIRSignals{SubstrateClass: "theme_heavy_wordpress"}}, []rankedSegment{
		{chunk: core.Chunk{ID: "chk_1", Fingerprint: "fp_1", Text: "We build websites and provide digital marketing for small businesses.", HeadingPath: []string{"Services"}, Score: 0.90, Confidence: 0.87}},
		{chunk: core.Chunk{ID: "chk_2", Fingerprint: "fp_2", Text: "Contact the studio today.", HeadingPath: []string{"CTA"}, Score: 0.80, Confidence: 0.77}},
	}, map[string]intel.Decision{
		"fp_1": {Fingerprint: "fp_1", Lane: 1, RiskFlags: []string{"short_segment"}},
		"fp_2": {Fingerprint: "fp_2", Lane: 1, RiskFlags: []string{"short_segment"}},
	})
	if ok {
		t.Fatalf("expected theme_heavy_wordpress to suppress ambiguity route when the structure is already strong, got %#v", input)
	}
}

func TestBuildResolveAmbiguityInputSkipsForStructurallyDominantCandidate(t *testing.T) {
	input, ok := buildResolveAmbiguityInput("web server platform summary", core.WebIR{Signals: core.WebIRSignals{SubstrateClass: "generic_content"}}, []rankedSegment{
		{chunk: core.Chunk{ID: "chk_1", Fingerprint: "fp_1", Text: "Nginx is a web server platform summary for operators and developers.", HeadingPath: []string{"Overview"}, Score: 0.95, Confidence: 0.92}, ir: segmentIREvidence{headingBacked: true}},
		{chunk: core.Chunk{ID: "chk_2", Fingerprint: "fp_2", Text: "Downloads, docs, and resources.", HeadingPath: []string{"Resources"}, Score: 0.79, Confidence: 0.78}, ir: segmentIREvidence{}},
	}, map[string]intel.Decision{
		"fp_1": {Fingerprint: "fp_1", Lane: 1, RiskFlags: []string{"selection_reused"}},
		"fp_2": {Fingerprint: "fp_2", Lane: 1, RiskFlags: []string{"short_segment"}},
	})
	if ok {
		t.Fatalf("expected structurally dominant candidate to suppress ambiguity route, got %#v", input)
	}
}

func TestBuildResolveAmbiguityInputKeepsRouteWhenTwoFullCoverageCandidatesAreClose(t *testing.T) {
	input, ok := buildResolveAmbiguityInput("web server platform summary", core.WebIR{Signals: core.WebIRSignals{SubstrateClass: "generic_content"}}, []rankedSegment{
		{chunk: core.Chunk{ID: "chk_1", Fingerprint: "fp_1", Text: "Nginx is a web server platform summary for operators and developers.", HeadingPath: []string{"Overview"}, Score: 0.90, Confidence: 0.87}, ir: segmentIREvidence{headingBacked: true}},
		{chunk: core.Chunk{ID: "chk_2", Fingerprint: "fp_2", Text: "Nginx is a web server platform summary with deployment and proxy focus.", HeadingPath: []string{"Overview"}, Score: 0.89, Confidence: 0.86}, ir: segmentIREvidence{headingBacked: true}},
	}, map[string]intel.Decision{
		"fp_1": {Fingerprint: "fp_1", Lane: 1, RiskFlags: []string{"high_ambiguity"}},
		"fp_2": {Fingerprint: "fp_2", Lane: 1, RiskFlags: []string{"coverage_gap"}},
	})
	if !ok {
		t.Fatalf("expected ambiguity route to remain on when two full-coverage candidates are equally strong, got %#v", input)
	}
}

func TestBuildResolveAmbiguityInputKeepsRouteForCrossLingualMismatchUntilSemanticGateRuns(t *testing.T) {
	input, ok := buildResolveAmbiguityInput("service offering", core.WebIR{Signals: core.WebIRSignals{SubstrateClass: "generic_content"}}, []rankedSegment{
		{chunk: core.Chunk{ID: "chk_1", Fingerprint: "fp_1", Text: "Aiutiamo aziende e startup a crescere attraverso il digitale.", HeadingPath: []string{"Diamo forma alle tue idee"}, Score: 1.0, Confidence: 0.93}},
		{chunk: core.Chunk{ID: "chk_2", Fingerprint: "fp_2", Text: "Realizziamo siti web e marketing digitale per il business.", HeadingPath: []string{"Servizi"}, Score: 0.99, Confidence: 0.93}},
	}, map[string]intel.Decision{
		"fp_1": {Fingerprint: "fp_1", Lane: 1, RiskFlags: []string{"coverage_gap"}},
		"fp_2": {Fingerprint: "fp_2", Lane: 1, RiskFlags: []string{"coverage_gap"}},
	})
	if !ok {
		t.Fatalf("expected ambiguity route to stay available until semantic gate runs, got %#v", input)
	}
}

func TestBuildResolveAmbiguityInputSkipsForCoverageAnchor(t *testing.T) {
	input, ok := buildResolveAmbiguityInput("database engine summary", core.WebIR{Signals: core.WebIRSignals{SubstrateClass: "generic_content"}}, []rankedSegment{
		{chunk: core.Chunk{ID: "chk_1", Fingerprint: "fp_1", Text: "SQLite is a database engine library used in embedded and local applications.", HeadingPath: []string{"Overview"}, Score: 0.96, Confidence: 0.91}},
		{chunk: core.Chunk{ID: "chk_2", Fingerprint: "fp_2", Text: "Documentation, download, and support resources.", HeadingPath: []string{"Resources"}, Score: 0.89, Confidence: 0.89}},
	}, map[string]intel.Decision{
		"fp_1": {Fingerprint: "fp_1", Lane: 1, RiskFlags: []string{"selection_reused"}},
		"fp_2": {Fingerprint: "fp_2", Lane: 1, RiskFlags: []string{"coverage_gap", "selection_recomputed"}},
	})
	if ok {
		t.Fatalf("expected coverage anchor to suppress ambiguity route, got %#v", input)
	}
}

func TestExecuteIntelTasksAcceptsAmbiguityPatch(t *testing.T) {
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"selected_chunk_ids\":[\"chk_2\"],\"rejected_chunk_ids\":[\"chk_1\"],\"decision_reason\":\"second candidate is more grounded\",\"confidence\":0.91}"}}],"usage":{"prompt_tokens":33,"completion_tokens":11}}`)
	}))
	defer modelServer.Close()

	cfg := config.Defaults()
	cfg.Models.Backend = "openai-compatible"
	cfg.Models.BaseURL = modelServer.URL
	cfg.Models.Router = "qwen-ambiguity"

	svc, err := New(cfg, modelServer.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	selected := []rankedSegment{
		{chunk: core.Chunk{ID: "chk_1", Fingerprint: "fp_1", Text: "Primary proof replay context.", HeadingPath: []string{"Overview"}, Score: 0.81, Confidence: 0.79}},
		{chunk: core.Chunk{ID: "chk_2", Fingerprint: "fp_2", Text: "Secondary proof replay context with stronger operator detail.", HeadingPath: []string{"Overview"}, Score: 0.80, Confidence: 0.79}},
	}
	decisions := map[string]intel.Decision{
		"fp_1": {Fingerprint: "fp_1", Lane: 1},
		"fp_2": {Fingerprint: "fp_2", Lane: 1},
	}
	rec := proof.NewRecorder("run_1", "trace_1", svc.now())

	out, executions, err := svc.executeIntelTasks(context.Background(), rec, ReadRequest{Objective: "proof replay context"}, core.WebIR{}, selected, decisions, intel.ModelTraceContext{RunID: "run_1", TraceID: "trace_1"})
	if err != nil {
		t.Fatalf("execute intel tasks: %v", err)
	}
	if len(executions) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(executions))
	}
	if executions[0].outcome != intelOutcomeAccepted {
		t.Fatalf("expected accepted outcome, got %q", executions[0].outcome)
	}
	if len(out) != 1 || out[0].chunk.ID != "chk_2" {
		t.Fatalf("expected ambiguity patch to keep chk_2, got %#v", out)
	}
	if got := decisions["fp_2"].ModelInvocations; len(got) == 0 || got[0].ValidatorOutcome != intelOutcomeAccepted {
		t.Fatalf("expected accepted invocation on fp_2, got %#v", got)
	}
}

func TestBuildPlannedIntelTasksSuppressesAmbiguityWhenSemanticGateFires(t *testing.T) {
	cfg := config.Defaults()
	cfg.Semantic.Enabled = true
	cfg.Semantic.Model = "embed-x"
	svc, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.semantic = fakeSemanticAligner{suppressed: true, reason: "cross_lingual_alignment"}
	plans, err := svc.buildPlannedIntelTasks(context.Background(), ReadRequest{Objective: "service offering"}, core.WebIR{Signals: core.WebIRSignals{SubstrateClass: "generic_content"}}, []rankedSegment{
		{chunk: core.Chunk{ID: "chk_1", Fingerprint: "fp_1", Text: "Aiutiamo aziende a crescere attraverso il digitale.", HeadingPath: []string{"Diamo forma alle tue idee"}, Score: 0.99, Confidence: 0.93}},
		{chunk: core.Chunk{ID: "chk_2", Fingerprint: "fp_2", Text: "Servizi di sviluppo siti web e marketing.", HeadingPath: []string{"Servizi"}, Score: 0.98, Confidence: 0.93}},
	}, map[string]intel.Decision{
		"fp_1": {Fingerprint: "fp_1", Lane: 1, RiskFlags: []string{"coverage_gap"}},
		"fp_2": {Fingerprint: "fp_2", Lane: 1, RiskFlags: []string{"coverage_gap"}},
	}, intel.ModelTraceContext{RunID: "run_1", TraceID: "trace_1"})
	if err != nil {
		t.Fatalf("build planned tasks: %v", err)
	}
	if len(plans) != 0 {
		t.Fatalf("expected semantic gate to suppress ambiguity plan, got %#v", plans)
	}
}
