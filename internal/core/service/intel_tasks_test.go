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
	"github.com/josepavese/needlex/internal/pipeline"
	"github.com/josepavese/needlex/internal/proof"
)

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
		t.Fatalf("expected ambiguity route to stay off for fully resolved objective, got %#v", input)
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
		t.Fatalf("expected theme_heavy_wordpress to suppress ambiguity route when objective is already covered, got %#v", input)
	}
}

func TestBuildResolveAmbiguityInputSkipsForCoverageDominantCandidate(t *testing.T) {
	input, ok := buildResolveAmbiguityInput("web server platform summary", core.WebIR{Signals: core.WebIRSignals{SubstrateClass: "generic_content"}}, []rankedSegment{
		{chunk: core.Chunk{ID: "chk_1", Fingerprint: "fp_1", Text: "Nginx is a web server platform summary for operators and developers.", HeadingPath: []string{"Overview"}, Score: 0.91, Confidence: 0.88}, ir: segmentIREvidence{headingBacked: true}},
		{chunk: core.Chunk{ID: "chk_2", Fingerprint: "fp_2", Text: "Downloads, docs, and resources.", HeadingPath: []string{"Resources"}, Score: 0.79, Confidence: 0.78}, ir: segmentIREvidence{headingBacked: true}},
	}, map[string]intel.Decision{
		"fp_1": {Fingerprint: "fp_1", Lane: 1, RiskFlags: []string{"high_ambiguity"}},
		"fp_2": {Fingerprint: "fp_2", Lane: 1, RiskFlags: []string{"short_segment"}},
	})
	if ok {
		t.Fatalf("expected coverage-dominant candidate to suppress ambiguity route, got %#v", input)
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

func TestBuildInterpretEmbeddedStateInputCollectsEmbeddedAndVisibleEvidence(t *testing.T) {
	dom := pipeline.SimplifiedDOM{
		Title: "Needle Runtime",
		Nodes: []pipeline.SimplifiedNode{
			{Path: "/article[1]/h1[1]", Kind: "heading", Text: "Needle Runtime", Depth: 2},
			{Path: "/article[1]/p[1]", Kind: "paragraph", Text: "Visible company summary.", Depth: 2},
			{Path: "/embedded/script[1]/text[1]", Kind: "paragraph", Text: `{"company":"Needle-X"}`, Depth: 3},
		},
	}
	webIR := buildWebIR(dom)

	input, ok := buildInterpretEmbeddedStateInput("company profile", dom, webIR)
	if !ok {
		t.Fatal("expected embedded state input to be built")
	}
	if err := input.Validate(); err != nil {
		t.Fatalf("validate embedded input: %v", err)
	}
	if len(input.EmbeddedExcerpts) != 1 {
		t.Fatalf("expected 1 embedded excerpt, got %d", len(input.EmbeddedExcerpts))
	}
	if len(input.VisibleEvidence) == 0 {
		t.Fatal("expected visible corroboration evidence")
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

	out, executions, err := svc.executeIntelTasks(context.Background(), rec, ReadRequest{Objective: "proof replay context"}, pipeline.SimplifiedDOM{}, core.WebIR{}, selected, decisions, intel.ModelTraceContext{RunID: "run_1", TraceID: "trace_1"})
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

func TestExecuteIntelTasksSkipsInterpretEmbeddedWhenTaskIsHoldout(t *testing.T) {
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"normalized_fields\":[{\"name\":\"company\",\"value\":\"Needle-X\",\"source_paths\":[\"/embedded/invalid\"]}],\"confidence\":0.92}"}}],"usage":{"prompt_tokens":41,"completion_tokens":13}}`)
	}))
	defer modelServer.Close()

	cfg := config.Defaults()
	cfg.Models.Backend = "openai-compatible"
	cfg.Models.BaseURL = modelServer.URL
	cfg.Models.Extractor = "qwen-embedded"

	svc, err := New(cfg, modelServer.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	dom := pipeline.SimplifiedDOM{
		Title: "Needle Runtime",
		Nodes: []pipeline.SimplifiedNode{
			{Path: "/article[1]/h1[1]", Kind: "heading", Text: "Needle Runtime"},
			{Path: "/article[1]/p[1]", Kind: "paragraph", Text: "Release update."},
			{Path: "/embedded/script[1]/text[1]", Kind: "paragraph", Text: `{"release_notes":["Delta export now keeps selector proof snapshots","Faster replay diff for changed pages"],"company":"Needle-X"}`},
		},
	}
	webIR := buildWebIR(dom)
	selected := []rankedSegment{
		{
			chunk: core.Chunk{ID: "chk_1", Fingerprint: "fp_embedded", Text: `{"release_notes":["Delta export now keeps selector proof snapshots","Faster replay diff for changed pages"],"company":"Needle-X"}`, HeadingPath: []string{"Needle Runtime"}, Score: 0.88, Confidence: 0.80},
			ir:    segmentIREvidence{embedded: true},
		},
	}
	decisions := map[string]intel.Decision{
		"fp_embedded": {Fingerprint: "fp_embedded", Lane: 2},
	}
	rec := proof.NewRecorder("run_1", "trace_1", svc.now())

	out, executions, err := svc.executeIntelTasks(context.Background(), rec, ReadRequest{Objective: "company profile"}, dom, webIR, selected, decisions, intel.ModelTraceContext{RunID: "run_1", TraceID: "trace_1"})
	if err != nil {
		t.Fatalf("execute intel tasks: %v", err)
	}
	if len(executions) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(executions))
	}
	if executions[0].outcome != intelOutcomeSkipped {
		t.Fatalf("expected skipped outcome, got %q", executions[0].outcome)
	}
	if out[0].chunk.Text != `{"release_notes":["Delta export now keeps selector proof snapshots","Faster replay diff for changed pages"],"company":"Needle-X"}` {
		t.Fatalf("expected embedded text to remain unchanged, got %q", out[0].chunk.Text)
	}
	if got := decisions["fp_embedded"].ModelInvocations; len(got) == 0 || got[0].ValidatorOutcome != intelOutcomeSkipped {
		t.Fatalf("expected skipped invocation on embedded fingerprint, got %#v", got)
	}
}

func TestExecuteIntelTasksSkipsRuntimeErrorPathForHoldoutTask(t *testing.T) {
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "backend exploded", http.StatusBadGateway)
	}))
	defer modelServer.Close()

	cfg := config.Defaults()
	cfg.Models.Backend = "openai-compatible"
	cfg.Models.BaseURL = modelServer.URL
	cfg.Models.Extractor = "qwen-embedded"

	svc, err := New(cfg, modelServer.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	dom := pipeline.SimplifiedDOM{
		Title: "Needle Runtime",
		Nodes: []pipeline.SimplifiedNode{
			{Path: "/article[1]/h1[1]", Kind: "heading", Text: "Needle Runtime"},
			{Path: "/article[1]/p[1]", Kind: "paragraph", Text: "Release update."},
			{Path: "/embedded/script[1]/text[1]", Kind: "paragraph", Text: `{"release_notes":["Delta export now keeps selector proof snapshots","Faster replay diff for changed pages"],"company":"Needle-X"}`},
		},
	}
	webIR := buildWebIR(dom)
	selected := []rankedSegment{
		{
			chunk: core.Chunk{ID: "chk_1", Fingerprint: "fp_embedded", Text: `{"release_notes":["Delta export now keeps selector proof snapshots","Faster replay diff for changed pages"],"company":"Needle-X"}`, HeadingPath: []string{"Needle Runtime"}, Score: 0.88, Confidence: 0.80},
			ir:    segmentIREvidence{embedded: true},
		},
	}
	decisions := map[string]intel.Decision{
		"fp_embedded": {Fingerprint: "fp_embedded", Lane: 2},
	}
	rec := proof.NewRecorder("run_1", "trace_1", svc.now())

	_, executions, err := svc.executeIntelTasks(context.Background(), rec, ReadRequest{Objective: "company profile"}, dom, webIR, selected, decisions, intel.ModelTraceContext{RunID: "run_1", TraceID: "trace_1"})
	if err != nil {
		t.Fatalf("execute intel tasks: %v", err)
	}
	if len(executions) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(executions))
	}
	if executions[0].outcome != intelOutcomeSkipped {
		t.Fatalf("expected skipped outcome, got %q", executions[0].outcome)
	}
	got := decisions["fp_embedded"].ModelInvocations
	if len(got) == 0 {
		t.Fatal("expected skipped invocation to be recorded")
	}
	if got[0].ValidatorOutcome != intelOutcomeSkipped {
		t.Fatalf("expected validator outcome error, got %#v", got[0])
	}
	if got[0].PatchEffect != "not_benchmark_proven" {
		t.Fatalf("expected holdout skip effect, got %#v", got[0])
	}
}

func TestExecuteIntelTasksSkipsEmbeddedWorthinessWhenTaskIsHoldout(t *testing.T) {
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"should_interpret_embedded\":false,\"selected_paths\":[],\"decision_reason\":\"visible DOM already covers the objective\",\"confidence\":0.93}"}}],"usage":{"prompt_tokens":33,"completion_tokens":11}}`)
	}))
	defer modelServer.Close()

	cfg := config.Defaults()
	cfg.Models.Backend = "openai-compatible"
	cfg.Models.BaseURL = modelServer.URL
	cfg.Models.Extractor = "readerlm-v2"

	svc, err := New(cfg, modelServer.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	dom := pipeline.SimplifiedDOM{
		Title: "Needle Runtime",
		Nodes: []pipeline.SimplifiedNode{
			{Path: "/article[1]/h1[1]", Kind: "heading", Text: "Needle Runtime"},
			{Path: "/article[1]/p[1]", Kind: "paragraph", Text: "Company profile and services overview for Needle-X."},
			{Path: "/embedded/script[1]/text[1]", Kind: "paragraph", Text: `{"cta":"Contact us"}`},
		},
	}
	webIR := buildWebIR(dom)
	selected := []rankedSegment{
		{
			chunk: core.Chunk{ID: "chk_1", Fingerprint: "fp_embedded", Text: `{"cta":"Contact us"}`, HeadingPath: []string{"Needle Runtime"}, Score: 0.88, Confidence: 0.80},
			ir:    segmentIREvidence{embedded: true},
		},
	}
	decisions := map[string]intel.Decision{
		"fp_embedded": {Fingerprint: "fp_embedded", Lane: 2},
	}
	rec := proof.NewRecorder("run_1", "trace_1", svc.now())

	out, executions, err := svc.executeIntelTasks(context.Background(), rec, ReadRequest{Objective: "company profile"}, dom, webIR, selected, decisions, intel.ModelTraceContext{RunID: "run_1", TraceID: "trace_1"})
	if err != nil {
		t.Fatalf("execute intel tasks: %v", err)
	}
	if len(executions) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(executions))
	}
	if executions[0].outcome != intelOutcomeSkipped {
		t.Fatalf("expected skipped worthiness outcome, got %q", executions[0].outcome)
	}
	if executions[0].invocation.PatchEffect != "not_benchmark_proven" {
		t.Fatalf("expected holdout skip effect, got %q", executions[0].invocation.PatchEffect)
	}
	if out[0].chunk.Text != `{"cta":"Contact us"}` {
		t.Fatalf("expected embedded text unchanged, got %q", out[0].chunk.Text)
	}
	got := decisions["fp_embedded"].ModelInvocations
	if len(got) == 0 || got[0].ValidatorOutcome != intelOutcomeSkipped {
		t.Fatalf("expected skipped worthiness invocation, got %#v", got)
	}
}
