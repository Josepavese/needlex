package service

import (
	"fmt"
	"strings"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/pipeline"
)

const (
	intelOutcomeAccepted = "accepted"
	intelOutcomeRejected = "rejected"
	intelOutcomeNoop     = "noop"
	intelOutcomeError    = "error"
	intelOutcomeSkipped  = "skipped"
)

type plannedIntelTask struct {
	route           intel.TaskRoute
	request         intel.ModelRequest
	fingerprints    []string
	ambiguityInput  *intel.ResolveAmbiguityInput
	worthinessInput *intel.EmbeddedWorthinessInput
	embeddedInput   *intel.InterpretEmbeddedStateInput
	candidateChunks []string
}

type executedIntelTask struct {
	route            intel.TaskRoute
	outcome          string
	validatorOutcome string
	invocation       core.ModelInvocation
	affectedChunkIDs []string
}

func buildResolveAmbiguityInput(objective string, webIR core.WebIR, candidates []rankedSegment, decisions map[string]intel.Decision) (intel.ResolveAmbiguityInput, bool) {
	if len(candidates) < 2 {
		return intel.ResolveAmbiguityInput{}, false
	}
	out := intel.ResolveAmbiguityInput{
		Objective:    strings.TrimSpace(objective),
		FailureClass: intel.FailureClassAmbiguityConflict,
		Candidates:   make([]intel.AmbiguityCandidate, 0, len(candidates)),
	}
	if ambiguityRoutingSuppressed(out.Objective, webIR, candidates, decisions) {
		return intel.ResolveAmbiguityInput{}, false
	}
	ambiguousCandidates := 0
	for _, item := range candidates {
		decision := decisions[item.chunk.Fingerprint]
		if !candidateNeedsAmbiguityRoute(webIR, item, decision) {
			continue
		}
		ambiguousCandidates++
		out.Candidates = append(out.Candidates, intel.AmbiguityCandidate{
			ChunkID:     item.chunk.ID,
			Fingerprint: item.chunk.Fingerprint,
			Text:        item.chunk.Text,
			HeadingPath: append([]string{}, item.chunk.HeadingPath...),
			Score:       item.chunk.Score,
			Confidence:  item.chunk.Confidence,
			RiskFlags:   append([]string{}, decision.RiskFlags...),
			IRSignals:   candidateIRSignalsFromEvidence(item.ir),
		})
	}
	if ambiguousCandidates < 2 {
		return intel.ResolveAmbiguityInput{}, false
	}
	if err := out.Validate(); err != nil {
		return intel.ResolveAmbiguityInput{}, false
	}
	return out, true
}

func ambiguityRoutingSuppressed(objective string, webIR core.WebIR, candidates []rankedSegment, decisions map[string]intel.Decision) bool {
	if coverageDominanceSuppressed(objective, candidates, decisions) {
		return true
	}
	switch strings.TrimSpace(webIR.Signals.SubstrateClass) {
	case "embedded_app_payload":
		return true
	case "theme_heavy_wordpress":
		return ambiguityAlreadyResolved(objective, candidates, decisions, 0.75)
	default:
		return ambiguityAlreadyResolved(objective, candidates, decisions, 1.0)
	}
}

func coverageDominanceSuppressed(objective string, candidates []rankedSegment, decisions map[string]intel.Decision) bool {
	objectiveTokens := normalizedObjectiveTokens(objective)
	if len(objectiveTokens) == 0 || len(candidates) < 2 {
		return false
	}
	dominant := candidates[0]
	dominantCoverage := objectiveCoverageForChunk(dominant.chunk.Text, dominant.chunk.HeadingPath, objectiveTokens)
	if dominantCoverage < 1 {
		return false
	}
	if dominant.chunk.Confidence < 0.84 || dominant.chunk.Score < 0.86 {
		return false
	}
	if !hasStructuralEvidence(dominant.ir) {
		return false
	}
	for _, candidate := range candidates[1:] {
		coverage := objectiveCoverageForChunk(candidate.chunk.Text, candidate.chunk.HeadingPath, objectiveTokens)
		if coverage > dominantCoverage {
			return false
		}
		if coverage == dominantCoverage && candidate.chunk.Score >= dominant.chunk.Score-0.03 && candidate.chunk.Confidence >= dominant.chunk.Confidence-0.03 {
			return false
		}
	}
	return true
}

func hasStructuralEvidence(ir segmentIREvidence) bool {
	return ir.headingBacked || ir.kindMatch || ir.embedded
}

func ambiguityAlreadyResolved(objective string, candidates []rankedSegment, decisions map[string]intel.Decision, minCoverage float64) bool {
	objectiveTokens := normalizedObjectiveTokens(objective)
	if len(objectiveTokens) == 0 {
		return false
	}
	for _, item := range candidates {
		decision := decisions[item.chunk.Fingerprint]
		if objectiveCoverageForChunk(item.chunk.Text, item.chunk.HeadingPath, objectiveTokens) < minCoverage {
			continue
		}
		if item.chunk.Confidence < 0.82 || item.chunk.Score < 0.82 {
			continue
		}
		if hasStrictAmbiguityRisk(decision.RiskFlags) {
			continue
		}
		return true
	}
	return false
}

func candidateNeedsAmbiguityRoute(webIR core.WebIR, item rankedSegment, decision intel.Decision) bool {
	if strings.TrimSpace(webIR.Signals.SubstrateClass) == "theme_heavy_wordpress" && !hasStrictAmbiguityRisk(decision.RiskFlags) {
		return false
	}
	if decision.Lane >= 1 && hasStrictAmbiguityRisk(decision.RiskFlags) {
		return true
	}
	if item.chunk.Confidence < 0.80 {
		return true
	}
	if item.chunk.Score < 0.84 {
		return true
	}
	for _, flag := range decision.RiskFlags {
		switch strings.TrimSpace(flag) {
		case "high_ambiguity", "coverage_gap", "short_text", "low_confidence":
			return true
		}
	}
	return false
}

func hasStrictAmbiguityRisk(flags []string) bool {
	for _, flag := range flags {
		switch strings.TrimSpace(flag) {
		case "high_ambiguity", "coverage_gap":
			return true
		}
	}
	return false
}

func normalizedObjectiveTokens(objective string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, token := range strings.Fields(strings.ToLower(strings.TrimSpace(objective))) {
		token = strings.Trim(token, ".,:;!?()[]{}\"'")
		if len(token) < 3 {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	return out
}

func objectiveCoverageForChunk(text string, headingPath []string, objectiveTokens []string) float64 {
	if len(objectiveTokens) == 0 {
		return 1
	}
	haystack := strings.ToLower(strings.Join(headingPath, " ") + " " + strings.TrimSpace(text))
	matches := 0
	for _, token := range objectiveTokens {
		if strings.Contains(haystack, token) {
			matches++
		}
	}
	return float64(matches) / float64(len(objectiveTokens))
}

func buildInterpretEmbeddedStateInput(objective string, dom pipeline.SimplifiedDOM, webIR core.WebIR) (intel.InterpretEmbeddedStateInput, bool) {
	byPath := map[string]core.WebIRNode{}
	for _, node := range webIR.Nodes {
		byPath[node.Path] = node
	}

	input := intel.InterpretEmbeddedStateInput{
		Objective:        strings.TrimSpace(objective),
		FailureClass:     intel.FailureClassEmbeddedPayload,
		EmbeddedExcerpts: []intel.EmbeddedStateExcerpt{},
		VisibleEvidence:  []intel.EmbeddedVisibleEvidence{},
	}

	for _, node := range dom.Nodes {
		path := strings.TrimSpace(node.Path)
		text := strings.TrimSpace(node.Text)
		if path == "" || text == "" {
			continue
		}
		if strings.HasPrefix(path, "/embedded/") {
			input.EmbeddedExcerpts = append(input.EmbeddedExcerpts, intel.EmbeddedStateExcerpt{
				Path:        path,
				Text:        text,
				HeadingPath: inferHeadingPathForNode(path, dom, byPath),
			})
			continue
		}
		if len(input.VisibleEvidence) >= 3 {
			continue
		}
		input.VisibleEvidence = append(input.VisibleEvidence, intel.EmbeddedVisibleEvidence{
			Path: path,
			Text: text,
		})
	}

	if err := input.Validate(); err != nil {
		return intel.InterpretEmbeddedStateInput{}, false
	}
	return input, true
}

func candidateIRSignalsFromEvidence(e segmentIREvidence) []string {
	signals := []string{}
	if e.kindMatch {
		signals = append(signals, "kind_match")
	}
	if e.embedded {
		signals = append(signals, "embedded")
	}
	if e.headingBacked {
		signals = append(signals, "heading_backed")
	}
	if e.averageNodeDepth > 0 && e.averageNodeDepth <= 4 {
		signals = append(signals, "shallow_depth")
	}
	return intel.CandidateIRSignals(signals)
}

func inferHeadingPathForNode(path string, dom pipeline.SimplifiedDOM, byPath map[string]core.WebIRNode) []string {
	if path == "" || !strings.HasPrefix(path, "/embedded/") {
		return nil
	}
	out := []string{}
	for _, node := range dom.Nodes {
		if node.Kind != "heading" || strings.TrimSpace(node.Text) == "" {
			continue
		}
		if webNode, ok := byPath[node.Path]; ok && webNode.Depth > 0 {
			out = append(out, strings.TrimSpace(node.Text))
		}
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 && strings.TrimSpace(dom.Title) != "" {
		return []string{strings.TrimSpace(dom.Title)}
	}
	return out
}

func (s *Service) buildPlannedIntelTasks(req ReadRequest, dom pipeline.SimplifiedDOM, webIR core.WebIR, selected []rankedSegment, decisions map[string]intel.Decision, trace intel.ModelTraceContext) ([]plannedIntelTask, error) {
	plans := []plannedIntelTask{}

	if ambiguityInput, ok := buildResolveAmbiguityInput(req.Objective, webIR, selected, decisions); ok {
		if route, ok := intel.PlanResolveAmbiguityTask(s.cfg, ambiguityInput); ok {
			request, err := ambiguityInput.RequestWithRoute(route, trace)
			if err != nil {
				return nil, err
			}
			fingerprints := make([]string, 0, len(selected))
			chunkIDs := make([]string, 0, len(selected))
			for _, item := range selected {
				fingerprints = append(fingerprints, item.chunk.Fingerprint)
				chunkIDs = append(chunkIDs, item.chunk.ID)
			}
			copyInput := ambiguityInput
			plans = append(plans, plannedIntelTask{
				route:           route,
				request:         request,
				fingerprints:    fingerprints,
				ambiguityInput:  &copyInput,
				candidateChunks: chunkIDs,
			})
		}
	}

	if embeddedInput, ok := buildInterpretEmbeddedStateInput(req.Objective, dom, webIR); ok {
		if route, ok := intel.PlanEmbeddedWorthinessTask(s.cfg, intel.EmbeddedWorthinessInput{
			Objective:        embeddedInput.Objective,
			FailureClass:     intel.FailureClassEmbeddedWorthiness,
			EmbeddedExcerpts: append([]intel.EmbeddedStateExcerpt{}, embeddedInput.EmbeddedExcerpts...),
			VisibleEvidence:  append([]intel.EmbeddedVisibleEvidence{}, embeddedInput.VisibleEvidence...),
		}); ok {
			request, err := (intel.EmbeddedWorthinessInput{
				Objective:        embeddedInput.Objective,
				FailureClass:     intel.FailureClassEmbeddedWorthiness,
				EmbeddedExcerpts: append([]intel.EmbeddedStateExcerpt{}, embeddedInput.EmbeddedExcerpts...),
				VisibleEvidence:  append([]intel.EmbeddedVisibleEvidence{}, embeddedInput.VisibleEvidence...),
			}).RequestWithRoute(route, trace)
			if err != nil {
				return nil, err
			}
			fingerprints := []string{}
			for _, item := range selected {
				if item.ir.embedded {
					fingerprints = append(fingerprints, item.chunk.Fingerprint)
				}
			}
			if len(fingerprints) > 0 {
				copyInput := intel.EmbeddedWorthinessInput{
					Objective:        embeddedInput.Objective,
					FailureClass:     intel.FailureClassEmbeddedWorthiness,
					EmbeddedExcerpts: append([]intel.EmbeddedStateExcerpt{}, embeddedInput.EmbeddedExcerpts...),
					VisibleEvidence:  append([]intel.EmbeddedVisibleEvidence{}, embeddedInput.VisibleEvidence...),
				}
				plans = append(plans, plannedIntelTask{
					route:           route,
					request:         request,
					fingerprints:    fingerprints,
					worthinessInput: &copyInput,
				})
			}
		}
		if route, ok := intel.PlanInterpretEmbeddedStateTask(s.cfg, embeddedInput); ok {
			request, err := embeddedInput.RequestWithRoute(route, trace)
			if err != nil {
				return nil, err
			}
			fingerprints := []string{}
			for _, item := range selected {
				if item.ir.embedded {
					fingerprints = append(fingerprints, item.chunk.Fingerprint)
				}
			}
			if len(fingerprints) > 0 {
				copyInput := embeddedInput
				plans = append(plans, plannedIntelTask{
					route:         route,
					request:       request,
					fingerprints:  fingerprints,
					embeddedInput: &copyInput,
				})
			}
		}
	}

	return plans, nil
}

func annotateDecisionsWithPlannedTasks(decisions map[string]intel.Decision, plans []plannedIntelTask) {
	for _, plan := range plans {
		for _, fingerprint := range plan.fingerprints {
			decision := decisions[fingerprint]
			decision.TaskRoutes = append(decision.TaskRoutes, plan.route)
			decision.RiskFlags = append(decision.RiskFlags, fmt.Sprintf("task_%s_planned", plan.route.Task))
			decision.TransformChain = append(decision.TransformChain, fmt.Sprintf("intel:task:%s:v1", plan.route.Task))
			decisions[fingerprint] = decision
		}
	}
}

func intelTaskPlanMetadata(executions []executedIntelTask) map[string]string {
	if len(executions) == 0 {
		return map[string]string{"intel_task_route_count": "0"}
	}
	names := make([]string, 0, len(executions))
	failureClasses := make([]string, 0, len(executions))
	accepted := 0
	rejected := 0
	skipped := 0
	for _, exec := range executions {
		names = append(names, exec.route.Task)
		failureClasses = append(failureClasses, exec.route.FailureClass)
		switch exec.outcome {
		case intelOutcomeAccepted:
			accepted++
		case intelOutcomeRejected:
			rejected++
		case intelOutcomeSkipped:
			skipped++
		}
	}
	return map[string]string{
		"intel_task_route_count":       fmt.Sprintf("%d", len(executions)),
		"intel_task_names":             strings.Join(names, ","),
		"intel_task_failure_classes":   strings.Join(failureClasses, ","),
		"intel_task_accepted_count":    fmt.Sprintf("%d", accepted),
		"intel_task_rejected_count":    fmt.Sprintf("%d", rejected),
		"intel_task_skipped_count":     fmt.Sprintf("%d", skipped),
		"intel_task_intervention_live": fmt.Sprintf("%t", accepted > 0),
	}
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}
