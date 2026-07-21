package queryplan

import (
	"testing"

	"github.com/josepavese/needlex/internal/config"
)

func TestQueryCompilerLifecycleValidatesRequiredStages(t *testing.T) {
	cfg := config.Defaults()
	base := BuildQueryCompiler("", "web_search", "web_search", "find protocol authentication flow", "standard", 0, cfg.Budget, cfg.Runtime)
	candidates := []Candidate{
		{URL: "https://example.com/docs/auth", Score: 1.2, Reason: []string{"semantic_goal_alignment"}, Metadata: map[string]string{"semantic_goal_similarity": "0.710"}},
		{URL: "https://example.com/blog", Score: 0.7, Reason: []string{"structure_hint"}},
	}
	plan := FinalizeQueryCompiler(base, "", "web_search", "brave", candidates[0].URL, candidates)
	plan = AnnotateQueryCompilerWithIntentBoundary(plan)
	plan = AnnotateQueryCompilerWithWebIR(plan, 42, 2, 0.21, 0.12)
	plan = AnnotateQueryCompilerWithExecution(plan, candidates[0].URL, candidates[0].URL, cfg.Runtime.LaneMax)
	plan = AnnotateQueryCompilerWithBudgetOutcome(plan, cfg.Budget.MaxLatencyMS, 100, cfg.Runtime.LaneMax, cfg.Runtime.LaneMax)
	plan = AnnotateQueryCompilerWithRuntimeEffects(plan, 0, 0, 0)
	plan = AnnotateQueryCompilerWithExecutionBoundary(plan)
	plan = AnnotateQueryCompilerWithPlanDiff(base, plan)

	if err := plan.Validate(); err != nil {
		t.Fatalf("expected valid compiler lifecycle: %v", err)
	}
	if !hasDecisionStage(plan.Decisions, "resolve.discovery_fallback") {
		t.Fatal("expected public web bootstrap fallback to be inspectable")
	}
}

func TestBuildQueryCompilerExplainsExperimentalWebSearchOptIn(t *testing.T) {
	cfg := config.Defaults()
	plan := BuildQueryCompiler("", "web_search", "web_search", "trova documentazione autorevole", "standard", 0, cfg.Budget, cfg.Runtime)
	if got := plan.Decisions[0].ReasonCode; got != QueryPlanReasonSeedMissing {
		t.Fatalf("seed reason got %q want %q", got, QueryPlanReasonSeedMissing)
	}
	if got := plan.Decisions[1].ReasonCode; got != QueryPlanReasonExperimentalWebOptIn {
		t.Fatalf("discovery reason got %q want %q", got, QueryPlanReasonExperimentalWebOptIn)
	}
}

func TestQueryCompilerRejectsNonExecutionAfterExecution(t *testing.T) {
	plan := QueryCompiler{Version: QueryCompilerVersion, Decisions: []QueryPlanDecision{
		{Stage: "input.seed", Choice: "missing", ReasonCode: QueryPlanReasonSeedMissing},
		{Stage: "observe.web_ir", Choice: "web_ir_observed", ReasonCode: QueryPlanReasonWebIR},
		{Stage: "select.candidate", Choice: "https://example.com", ReasonCode: QueryPlanReasonSelection},
	}}
	if err := plan.Validate(); err == nil {
		t.Fatal("expected invalid execution-stage ordering")
	}
}

func TestAnnotateQueryCompilerWithRewriteCarriesSemanticHints(t *testing.T) {
	plan := QueryCompiler{Version: QueryCompilerVersion}
	plan = AnnotateQueryCompilerWithRewrite(plan, []string{"credential handoff during session creation"}, "Agent Client Protocol", []string{"protocol"}, []string{"authentication"}, 0.82)
	got := plan.Decisions[len(plan.Decisions)-1]
	if got.Stage != "plan.query_rewrite" || got.Metadata["canonical_entity"] != "Agent Client Protocol" {
		t.Fatalf("unexpected rewrite annotation: %+v", got)
	}
	if got.Metadata["confidence"] != "0.820" {
		t.Fatalf("expected confidence metadata, got %+v", got.Metadata)
	}
}
