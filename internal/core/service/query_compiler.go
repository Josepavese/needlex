package service

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/josepavese/needlex/internal/config"
)

const (
	QueryCompilerVersion = "query_compiler.v1"

	QueryPlanReasonSeedPresent            = "NX_PLAN_SEED_PRESENT"
	QueryPlanReasonSeedMissing            = "NX_PLAN_SEED_MISSING"
	QueryPlanReasonDefaultMode            = "NX_PLAN_DEFAULT_DISCOVERY_MODE"
	QueryPlanReasonUserMode               = "NX_PLAN_USER_DISCOVERY_MODE"
	QueryPlanReasonSeedlessDefaultWeb     = "NX_PLAN_SEEDLESS_DEFAULT_WEB"
	QueryPlanReasonBudgetApplied          = "NX_PLAN_BUDGET_APPLIED"
	QueryPlanReasonSelection              = "NX_PLAN_SELECTED_URL"
	QueryPlanReasonWebIR                  = "NX_PLAN_WEB_IR_SIGNAL"
	QueryPlanReasonWebIRSelection         = "NX_PLAN_WEB_IR_SELECTION"
	QueryPlanReasonDomainHintEvidence     = "NX_PLAN_DOMAIN_HINT_EVIDENCE"
	QueryPlanReasonGraphEvidence          = "NX_PLAN_GRAPH_EVIDENCE"
	QueryPlanReasonWebBootstrapFallback   = "NX_PLAN_WEB_BOOTSTRAP_FALLBACK"
	QueryPlanReasonLowCandidateSetRisk    = "NX_PLAN_LOW_CANDIDATE_SET_RISK"
	QueryPlanReasonAmbiguousSelectionRisk = "NX_PLAN_AMBIGUOUS_SELECTION_RISK"
	QueryPlanReasonStableRegionBias       = "NX_PLAN_STABLE_REGION_BIAS"
	QueryPlanReasonNoveltyBias            = "NX_PLAN_NOVELTY_BIAS"
	QueryPlanReasonDeltaRisk              = "NX_PLAN_DELTA_RISK"
	QueryPlanReasonQualityLatencyMode     = "NX_PLAN_QUALITY_LATENCY_MODE"
	QueryPlanReasonLanePolicy             = "NX_PLAN_LANE_POLICY"
	QueryPlanReasonExecutionAligned       = "NX_PLAN_EXECUTION_ALIGNED"
	QueryPlanReasonExecutionDrift         = "NX_PLAN_EXECUTION_DRIFT"
	QueryPlanReasonPlanDiffObserved       = "NX_PLAN_DIFF_OBSERVED"
	QueryPlanReasonRuntimeEffectsClean    = "NX_PLAN_RUNTIME_EFFECTS_CLEAN"
	QueryPlanReasonRuntimeEffectsDetected = "NX_PLAN_RUNTIME_EFFECTS_DETECTED"
	QueryPlanReasonIntentBoundary         = "NX_PLAN_INTENT_BOUNDARY"
	QueryPlanReasonExecutionBoundary      = "NX_PLAN_EXECUTION_BOUNDARY"
	QueryPlanReasonBudgetOutcomeOK        = "NX_PLAN_BUDGET_OUTCOME_OK"
	QueryPlanReasonBudgetOutcomeExceeded  = "NX_PLAN_BUDGET_OUTCOME_EXCEEDED"
)

type QueryCompiler struct {
	Version   string              `json:"version"`
	Decisions []QueryPlanDecision `json:"decisions"`
}

type QueryPlanDecision struct {
	Stage      string            `json:"stage"`
	Choice     string            `json:"choice"`
	ReasonCode string            `json:"reason_code"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

func (d QueryPlanDecision) Validate() error {
	if strings.TrimSpace(d.Stage) == "" {
		return fmt.Errorf("decision.stage must not be empty")
	}
	if strings.TrimSpace(d.Choice) == "" {
		return fmt.Errorf("decision.choice must not be empty")
	}
	if strings.TrimSpace(d.ReasonCode) == "" {
		return fmt.Errorf("decision.reason_code must not be empty")
	}
	return nil
}

func (c QueryCompiler) Validate() error {
	if strings.TrimSpace(c.Version) == "" {
		return fmt.Errorf("version must not be empty")
	}
	if c.Version != QueryCompilerVersion {
		return fmt.Errorf("version must be %q", QueryCompilerVersion)
	}
	if len(c.Decisions) == 0 {
		return fmt.Errorf("decisions must not be empty")
	}
	for i, decision := range c.Decisions {
		if err := decision.Validate(); err != nil {
			return fmt.Errorf("decisions[%d]: %w", i, err)
		}
		if i > 0 && isExecutionStage(decision.Stage) && !isExecutionStage(c.Decisions[i-1].Stage) {
			for _, later := range c.Decisions[i+1:] {
				if !isExecutionStage(later.Stage) {
					return fmt.Errorf("decisions stage order invalid: %q appears after execution stage", later.Stage)
				}
			}
			break
		}
	}
	for _, stage := range []string{
		"input.seed", "resolve.discovery_mode", "apply.budget", "select.candidate", "select.discovery_strategy",
		"plan.intent_boundary", "verify.execution_alignment", "verify.budget_outcome", "verify.runtime_effects", "verify.execution_boundary", "verify.plan_diff",
	} {
		if !hasDecisionStage(c.Decisions, stage) {
			return fmt.Errorf("decisions missing required stage %q", stage)
		}
	}
	return nil
}

func buildQueryCompiler(seedURL, requestedMode, resolvedMode, goal, profile string, forceLane int, budget config.BudgetConfig, runtime config.RuntimeConfig) QueryCompiler {
	seedChoice, seedReason := "present", QueryPlanReasonSeedPresent
	if strings.TrimSpace(seedURL) == "" {
		seedChoice, seedReason = "missing", QueryPlanReasonSeedMissing
	}
	qualityChoice := "quality_biased"
	if profile == "tiny" || budget.MaxLatencyMS <= 5000 {
		qualityChoice = "latency_constrained"
	}
	laneChoice := "auto_lane_balanced"
	laneMeta := map[string]string{}
	if forceLane > 0 {
		laneChoice = "forced_lane"
		laneMeta["force_lane"] = strconv.Itoa(forceLane)
	} else if profile == "tiny" {
		laneChoice = "auto_lane_low"
	} else if profile == "deep" {
		laneChoice = "auto_lane_high"
	}
	return QueryCompiler{Version: QueryCompilerVersion, Decisions: []QueryPlanDecision{
		{Stage: "input.seed", Choice: seedChoice, ReasonCode: seedReason},
		{Stage: "resolve.discovery_mode", Choice: resolvedMode, ReasonCode: resolveDiscoveryReason(seedURL, requestedMode, resolvedMode), Metadata: map[string]string{"requested_mode": requestedMode}},
		{Stage: "apply.budget", Choice: "runtime_budget", ReasonCode: QueryPlanReasonBudgetApplied, Metadata: map[string]string{"max_tokens": strconv.Itoa(budget.MaxTokens), "max_latency_ms": strconv.FormatInt(budget.MaxLatencyMS, 10), "max_pages": strconv.Itoa(runtime.MaxPages), "max_depth": strconv.Itoa(runtime.MaxDepth)}},
		{Stage: "plan.quality_latency_mode", Choice: qualityChoice, ReasonCode: QueryPlanReasonQualityLatencyMode, Metadata: map[string]string{"goal_terms": strconv.Itoa(len(strings.Fields(strings.TrimSpace(goal)))), "profile": profile, "max_latency_ms": strconv.FormatInt(budget.MaxLatencyMS, 10)}},
		{Stage: "plan.lane_policy", Choice: laneChoice, ReasonCode: QueryPlanReasonLanePolicy, Metadata: laneMeta},
	}}
}

func finalizeQueryCompiler(plan QueryCompiler, seedURL, discoveryMode, provider, selectedURL string, candidates []DiscoverCandidate) QueryCompiler {
	metadata := map[string]string{
		"candidate_count": strconv.Itoa(len(candidates)),
	}
	if strings.TrimSpace(provider) != "" {
		metadata["provider"] = provider
	}

	selected := discoveryCandidateByURL(candidates, selectedURL)
	if strings.TrimSpace(selected.URL) != "" {
		metadata["selected_score"] = strconv.FormatFloat(selected.Score, 'f', 3, 64)
	}
	plan.Decisions = append(plan.Decisions,
		QueryPlanDecision{Stage: "select.candidate", Choice: selectedURL, ReasonCode: QueryPlanReasonSelection, Metadata: metadata},
		QueryPlanDecision{Stage: "select.discovery_strategy", Choice: discoveryMode, ReasonCode: QueryPlanReasonSelection},
	)
	return annotateQueryCompilerWithSelectionContext(plan, seedURL, discoveryMode, provider, selected, candidates)
}

func annotateQueryCompilerWithSelectionContext(plan QueryCompiler, seedURL, discoveryMode, provider string, selected DiscoverCandidate, candidates []DiscoverCandidate) QueryCompiler {
	if discoveryMode == QueryDiscoveryWeb && strings.TrimSpace(provider) != "" && !strings.HasPrefix(strings.TrimSpace(provider), "local_") {
		plan.Decisions = append(plan.Decisions, QueryPlanDecision{Stage: "resolve.discovery_fallback", Choice: provider, ReasonCode: QueryPlanReasonWebBootstrapFallback, Metadata: map[string]string{"strategy": discoveryMode}})
	}
	if len(candidates) <= 1 {
		plan.Decisions = append(plan.Decisions, QueryPlanDecision{Stage: "gate.selection_risk", Choice: "low_candidate_set", ReasonCode: QueryPlanReasonLowCandidateSetRisk, Metadata: map[string]string{"candidate_count": strconv.Itoa(len(candidates))}})
	} else if selectionDelta := selectionScoreDelta(candidates); selectionDelta <= 0.25 {
		plan.Decisions = append(plan.Decisions, QueryPlanDecision{Stage: "gate.selection_risk", Choice: "ambiguous_top_candidates", ReasonCode: QueryPlanReasonAmbiguousSelectionRisk, Metadata: map[string]string{"top_delta": strconv.FormatFloat(selectionDelta, 'f', 3, 64)}})
	}
	if len(selected.Reason) > 0 && slices.Contains(selected.Reason, "domain_hint_match") {
		reasonCode, seedHost, selectedHost := QueryPlanReasonDomainHintEvidence, hostFromURLString(seedURL), hostFromURLString(selected.URL)
		if selectedHost != "" && seedHost != "" && selectedHost != seedHost {
			reasonCode = QueryPlanReasonGraphEvidence
		}
		plan.Decisions = append(plan.Decisions, QueryPlanDecision{Stage: "select.graph_evidence", Choice: selected.URL, ReasonCode: reasonCode, Metadata: map[string]string{"selected_host": selectedHost, "seed_host": seedHost}})
	}
	return plan
}

func annotateQueryCompilerWithWebIR(plan QueryCompiler, webIRNodeCount, embeddedNodeCount int, headingRatio, shortTextRatio float64) QueryCompiler {
	metadata := map[string]string{
		"node_count":          strconv.Itoa(webIRNodeCount),
		"embedded_node_count": strconv.Itoa(embeddedNodeCount),
		"heading_ratio":       strconv.FormatFloat(headingRatio, 'f', 3, 64),
		"short_text_ratio":    strconv.FormatFloat(shortTextRatio, 'f', 3, 64),
	}
	if embeddedNodeCount > 0 {
		metadata["dominant_signal"] = "embedded_nodes"
	} else if headingRatio >= 0.15 {
		metadata["dominant_signal"] = "heading_backed"
	} else if shortTextRatio >= 0.80 {
		metadata["dominant_signal"] = "short_text_heavy"
	} else {
		metadata["dominant_signal"] = "balanced"
	}
	plan.Decisions = append(plan.Decisions, QueryPlanDecision{
		Stage:      "observe.web_ir",
		Choice:     "web_ir_observed",
		ReasonCode: QueryPlanReasonWebIR,
		Metadata:   metadata,
	})
	return plan
}

func annotateQueryCompilerWithFingerprintEvidence(plan QueryCompiler, seedURL, traceID string, stableRatio, noveltyRatio float64, changed bool) QueryCompiler {
	if strings.TrimSpace(seedURL) == "" || strings.TrimSpace(traceID) == "" {
		return plan
	}
	metadata := map[string]string{
		"latest_trace_id": traceID,
		"stable_ratio":    strconv.FormatFloat(stableRatio, 'f', 3, 64),
		"novelty_ratio":   strconv.FormatFloat(noveltyRatio, 'f', 3, 64),
	}
	if stableRatio >= 0.80 && !changed {
		plan.Decisions = append(plan.Decisions, QueryPlanDecision{Stage: "plan.change_evidence", Choice: "stable_seed_region", ReasonCode: QueryPlanReasonStableRegionBias, Metadata: metadata})
		return plan
	}
	if noveltyRatio > 0 {
		plan.Decisions = append(plan.Decisions, QueryPlanDecision{Stage: "plan.change_evidence", Choice: "novel_seed_region", ReasonCode: QueryPlanReasonNoveltyBias, Metadata: metadata})
	}
	if changed {
		plan.Decisions = append(plan.Decisions, QueryPlanDecision{Stage: "gate.delta_risk", Choice: "seed_changed_since_last_trace", ReasonCode: QueryPlanReasonDeltaRisk, Metadata: metadata})
	}
	return plan
}

func annotateQueryCompilerWithPlanningWebIR(plan QueryCompiler, candidate DiscoverCandidate) QueryCompiler {
	if len(candidate.Metadata) == 0 {
		return plan
	}
	nodeCount := strings.TrimSpace(candidate.Metadata["web_ir_node_count"])
	embeddedCount := strings.TrimSpace(candidate.Metadata["web_ir_embedded_node_count"])
	if nodeCount == "" && embeddedCount == "" {
		return plan
	}
	plan.Decisions = append(plan.Decisions, QueryPlanDecision{Stage: "select.web_ir_evidence", Choice: candidate.URL, ReasonCode: QueryPlanReasonWebIRSelection, Metadata: map[string]string{"web_ir_node_count": nodeCount, "web_ir_embedded_node_count": embeddedCount, "web_ir_heading_ratio": candidate.Metadata["web_ir_heading_ratio"], "web_ir_short_text_ratio": candidate.Metadata["web_ir_short_text_ratio"]}})
	return plan
}

func annotateQueryCompilerWithExecution(plan QueryCompiler, plannedURL, finalURL string, lanePath []int) QueryCompiler {
	choice, reason := "aligned", QueryPlanReasonExecutionAligned
	if strings.TrimSpace(plannedURL) != "" && strings.TrimSpace(finalURL) != "" && strings.TrimSpace(plannedURL) != strings.TrimSpace(finalURL) {
		choice, reason = "drift", QueryPlanReasonExecutionDrift
	}
	plan.Decisions = append(plan.Decisions, QueryPlanDecision{Stage: "verify.execution_alignment", Choice: choice, ReasonCode: reason, Metadata: map[string]string{"planned_url": strings.TrimSpace(plannedURL), "final_url": strings.TrimSpace(finalURL), "observed_lane_max": strconv.Itoa(maxLane(lanePath))}})
	return plan
}

func annotateQueryCompilerWithPlanDiff(base, final QueryCompiler) QueryCompiler {
	baseStages := map[string]struct{}{}
	for _, decision := range base.Decisions {
		baseStages[decision.Stage] = struct{}{}
	}
	added := make([]string, 0, len(final.Decisions))
	for _, decision := range final.Decisions {
		if _, ok := baseStages[decision.Stage]; ok {
			continue
		}
		baseStages[decision.Stage] = struct{}{}
		added = append(added, decision.Stage)
	}
	choice := "expanded"
	if len(added) == 0 {
		choice = "unchanged"
	}
	final.Decisions = append(final.Decisions, QueryPlanDecision{Stage: "verify.plan_diff", Choice: choice, ReasonCode: QueryPlanReasonPlanDiffObserved, Metadata: map[string]string{"base_decisions": strconv.Itoa(len(base.Decisions)), "final_decisions": strconv.Itoa(len(final.Decisions)), "added_stage_count": strconv.Itoa(len(added)), "added_stages": strings.Join(added, ",")}})
	return final
}

func annotateQueryCompilerWithRuntimeEffects(plan QueryCompiler, escalations, budgetWarnings, errors int) QueryCompiler {
	choice, reason := "clean", QueryPlanReasonRuntimeEffectsClean
	if escalations > 0 || budgetWarnings > 0 || errors > 0 {
		choice, reason = "side_effects_detected", QueryPlanReasonRuntimeEffectsDetected
	}
	plan.Decisions = append(plan.Decisions, QueryPlanDecision{Stage: "verify.runtime_effects", Choice: choice, ReasonCode: reason, Metadata: map[string]string{"escalation_count": strconv.Itoa(escalations), "budget_warning_count": strconv.Itoa(budgetWarnings), "error_count": strconv.Itoa(errors)}})
	return plan
}

func annotateQueryCompilerWithIntentBoundary(plan QueryCompiler) QueryCompiler {
	plan.Decisions = append(plan.Decisions, QueryPlanDecision{Stage: "plan.intent_boundary", Choice: "planning_complete", ReasonCode: QueryPlanReasonIntentBoundary, Metadata: map[string]string{"planned_stage_count": strconv.Itoa(len(plan.Decisions))}})
	return plan
}

func annotateQueryCompilerWithExecutionBoundary(plan QueryCompiler) QueryCompiler {
	execCount := 0
	for _, decision := range plan.Decisions {
		if isExecutionStage(decision.Stage) {
			execCount++
		}
	}
	plan.Decisions = append(plan.Decisions, QueryPlanDecision{Stage: "verify.execution_boundary", Choice: "execution_observed", ReasonCode: QueryPlanReasonExecutionBoundary, Metadata: map[string]string{"execution_stage_count": strconv.Itoa(execCount)}})
	return plan
}

func annotateQueryCompilerWithBudgetOutcome(plan QueryCompiler, budgetMaxLatencyMS, observedLatencyMS int64, laneMax, observedLane int) QueryCompiler {
	choice, reason := "within_budget", QueryPlanReasonBudgetOutcomeOK
	if observedLatencyMS > budgetMaxLatencyMS || observedLane > laneMax {
		choice, reason = "exceeded_budget", QueryPlanReasonBudgetOutcomeExceeded
	}
	plan.Decisions = append(plan.Decisions, QueryPlanDecision{Stage: "verify.budget_outcome", Choice: choice, ReasonCode: reason, Metadata: map[string]string{"max_latency_ms": strconv.FormatInt(budgetMaxLatencyMS, 10), "observed_latency_ms": strconv.FormatInt(observedLatencyMS, 10), "lane_max": strconv.Itoa(laneMax), "observed_lane_max": strconv.Itoa(observedLane)}})
	return plan
}

func isExecutionStage(stage string) bool {
	return strings.HasPrefix(stage, "observe.") || strings.HasPrefix(stage, "verify.")
}

func hasDecisionStage(decisions []QueryPlanDecision, stage string) bool {
	for _, decision := range decisions {
		if decision.Stage == stage {
			return true
		}
	}
	return false
}

func resolveDiscoveryReason(seedURL, requestedMode, resolvedMode string) string {
	if strings.TrimSpace(requestedMode) == "" {
		if strings.TrimSpace(seedURL) == "" && resolvedMode == QueryDiscoveryWeb {
			return QueryPlanReasonSeedlessDefaultWeb
		}
		return QueryPlanReasonDefaultMode
	}
	return QueryPlanReasonUserMode
}

func selectionScoreDelta(candidates []DiscoverCandidate) float64 {
	if len(candidates) < 2 {
		return 0
	}
	first := candidates[0].Score
	second := candidates[1].Score
	if first < second {
		first, second = second, first
	}
	return first - second
}
