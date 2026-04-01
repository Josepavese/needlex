package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/intel"
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
	candidateChunks []string
}

type executedIntelTask struct {
	route            intel.TaskRoute
	outcome          string
	validatorOutcome string
	invocation       core.ModelInvocation
	affectedChunkIDs []string
}

func (s *Service) buildPlannedIntelTasks(ctx context.Context, req ReadRequest, webIR core.WebIR, selected []rankedSegment, decisions map[string]intel.Decision, trace intel.ModelTraceContext) ([]plannedIntelTask, error) {
	plans := []plannedIntelTask{}

	if ambiguityInput, ok := buildResolveAmbiguityInput(req.Objective, webIR, selected, decisions); ok {
		if suppressed, reason := s.semanticSuppressesAmbiguity(ctx, req.Objective, ambiguityInput); suppressed {
			for _, candidate := range ambiguityInput.Candidates {
				decision := decisions[candidate.Fingerprint]
				decision.RiskFlags = append(decision.RiskFlags, "semantic_gate_"+reason)
				decisions[candidate.Fingerprint] = decision
			}
		} else if route, ok := intel.PlanResolveAmbiguityTask(s.cfg, ambiguityInput); ok {
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
			plans = append(plans, plannedIntelTask{route: route, request: request, fingerprints: fingerprints, ambiguityInput: &copyInput, candidateChunks: chunkIDs})
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
