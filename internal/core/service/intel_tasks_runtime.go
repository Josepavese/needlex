package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/proof"
)

func (s *Service) executeIntelTasks(ctx context.Context, recorder *proof.Recorder, req ReadRequest, webIR core.WebIR, selected []rankedSegment, decisions map[string]intel.Decision, trace intel.ModelTraceContext) ([]rankedSegment, []executedIntelTask, error) {
	plans, err := s.buildPlannedIntelTasks(ctx, req, webIR, selected, decisions, trace)
	if err != nil {
		return nil, nil, err
	}
	annotateDecisionsWithPlannedTasks(decisions, plans)

	runtime := s.runtime
	if runtime == nil {
		runtime = intel.DefaultRuntime()
	}

	current := append([]rankedSegment{}, selected...)
	executions := []executedIntelTask{}
	for _, plan := range plans {
		exec := executedIntelTask{
			route:            plan.route,
			outcome:          intelOutcomeNoop,
			validatorOutcome: intelOutcomeNoop,
		}
		backend := strings.TrimSpace(s.cfg.Models.Backend)
		if allowed, reason := intel.TaskAllowedForBackend(backend, plan.route.Task); !allowed {
			exec = skippedIntelExecution(s.cfg, plan, backend, reason)
			annotateTaskOutcome(decisions, plan.fingerprints, exec.invocation, plan.route.Task, intelOutcomeSkipped)
			recorder.ModelIntervention("pack", plan.route.ReasonCode, "model task skipped by runtime selection", routeLane(plan.route.Task), traceEventData(exec), s.now().UTC())
			executions = append(executions, exec)
			continue
		}
		response, runErr := runtime.Run(ctx, plan.request)
		if runErr != nil {
			exec = failedIntelExecution(s.cfg, plan, backend, runErr)
			recorder.ModelIntervention("pack", plan.route.ReasonCode, "model task execution failed", routeLane(plan.route.Task), traceEventData(exec), s.now().UTC())
			executions = append(executions, exec)
			annotateTaskOutcome(decisions, plan.fingerprints, exec.invocation, plan.route.Task, intelOutcomeError)
			continue
		}

		switch plan.route.Task {
		case intel.TaskResolveAmbiguity:
			exec, current = applyResolveAmbiguityExecution(response, plan, current, decisions, backend, s.cfg.Policy.ThresholdConfidence)
		default:
			exec.outcome = intelOutcomeRejected
			exec.validatorOutcome = intelOutcomeRejected
		}
		recorder.ModelIntervention("pack", plan.route.ReasonCode, "model task executed", routeLane(plan.route.Task), traceEventData(exec), s.now().UTC())
		executions = append(executions, exec)
	}
	return current, executions, nil
}

func skippedIntelExecution(cfg config.Config, plan plannedIntelTask, backend, reason string) executedIntelTask {
	model := ""
	switch plan.route.ModelClass {
	case intel.ModelClassMicroSolver:
		model = cfg.Models.Router
	}
	return executedIntelTask{
		route:            plan.route,
		outcome:          intelOutcomeSkipped,
		validatorOutcome: intelOutcomeSkipped,
		invocation: core.ModelInvocation{
			Model:            nonEmpty(model, "unconfigured"),
			Backend:          nonEmpty(backend, intel.BackendNoop),
			Task:             plan.route.Task,
			Purpose:          "typed_patch_gate",
			PatchEffect:      reason,
			ValidatorOutcome: intelOutcomeSkipped,
		},
	}
}

func failedIntelExecution(cfg config.Config, plan plannedIntelTask, backend string, cause error) executedIntelTask {
	exec := skippedIntelExecution(cfg, plan, backend, "runtime_error")
	exec.outcome = intelOutcomeError
	exec.validatorOutcome = intelOutcomeError
	exec.invocation.Purpose = "typed_patch"
	exec.invocation.PatchEffect = "runtime_error"
	exec.invocation.ValidatorOutcome = intelOutcomeError
	exec.invocation.ValidatorMessage = trimDiagnostic(cause.Error())
	return exec
}

func applyResolveAmbiguityExecution(response intel.ModelResponse, plan plannedIntelTask, current []rankedSegment, decisions map[string]intel.Decision, backend string, minConfidence float64) (executedIntelTask, []rankedSegment) {
	exec := executedIntelTask{
		route:            plan.route,
		outcome:          intelOutcomeRejected,
		validatorOutcome: intelOutcomeRejected,
		invocation:       modelInvocationFromResponse(response, backend, "typed_patch"),
	}
	if response.FinishReason == intel.ModelFinishReasonNoop {
		exec.outcome = intelOutcomeNoop
		exec.validatorOutcome = intelOutcomeNoop
		return exec, current
	}
	patch, err := intel.ParseResolveAmbiguityPatch(response.OutputJSON)
	if err != nil {
		exec.invocation.ValidatorOutcome = intelOutcomeRejected
		exec.invocation.PatchEffect = "selection_rejected"
		exec.invocation.ValidatorMessage = trimDiagnostic(err.Error())
		annotateTaskOutcome(decisions, plan.fingerprints, exec.invocation, plan.route.Task, intelOutcomeRejected)
		return exec, current
	}
	if err := intel.ValidateResolveAmbiguityPatch(*plan.ambiguityInput, patch, minConfidence); err != nil {
		exec.invocation.ValidatorOutcome = intelOutcomeRejected
		exec.invocation.PatchEffect = "selection_rejected"
		exec.invocation.ValidatorMessage = trimDiagnostic(err.Error())
		annotateTaskOutcome(decisions, plan.fingerprints, exec.invocation, plan.route.Task, intelOutcomeRejected)
		return exec, current
	}

	selectedSet := map[string]struct{}{}
	for _, chunkID := range patch.SelectedChunkIDs {
		selectedSet[chunkID] = struct{}{}
	}
	candidateSet := map[string]struct{}{}
	for _, chunkID := range plan.candidateChunks {
		candidateSet[chunkID] = struct{}{}
	}
	next := make([]rankedSegment, 0, len(current))
	for _, item := range current {
		if _, isCandidate := candidateSet[item.chunk.ID]; !isCandidate {
			next = append(next, item)
			continue
		}
		if _, keep := selectedSet[item.chunk.ID]; keep {
			next = append(next, item)
			exec.affectedChunkIDs = append(exec.affectedChunkIDs, item.chunk.ID)
		}
	}
	if len(next) == 0 {
		exec.invocation.ValidatorMessage = "no_candidate_survived_selection"
		return exec, current
	}
	exec.outcome = intelOutcomeAccepted
	exec.validatorOutcome = intelOutcomeAccepted
	exec.invocation.PatchID = prefixedHash("patch", plan.route.Task, response.OutputJSON)
	exec.invocation.PatchEffect = "selection_refined"
	exec.invocation.AffectedChunkIDs = append([]string{}, exec.affectedChunkIDs...)
	exec.invocation.ValidatorOutcome = intelOutcomeAccepted
	annotateTaskOutcome(decisions, plan.fingerprints, exec.invocation, plan.route.Task, intelOutcomeAccepted)
	return exec, next
}

func annotateTaskOutcome(decisions map[string]intel.Decision, fingerprints []string, invocation core.ModelInvocation, task, outcome string) {
	for _, fingerprint := range fingerprints {
		decision := decisions[fingerprint]
		decision.ModelInvocations = append(decision.ModelInvocations, invocation)
		decision.RiskFlags = append(decision.RiskFlags, fmt.Sprintf("intel_patch_%s", outcome))
		decision.TransformChain = append(decision.TransformChain, fmt.Sprintf("intel:patch:%s:%s:v1", task, outcome))
		decisions[fingerprint] = decision
	}
}

func modelInvocationFromResponse(response intel.ModelResponse, backend, purpose string) core.ModelInvocation {
	return core.ModelInvocation{
		Model:        response.Model,
		Backend:      strings.TrimSpace(backend),
		Task:         response.Task,
		Purpose:      purpose,
		TokensIn:     response.TokensIn,
		TokensOut:    response.TokensOut,
		LatencyMS:    response.LatencyMS,
		PatchPreview: trimDiagnostic(response.OutputJSON),
	}
}

func routeLane(task string) int {
	switch task {
	case intel.TaskResolveAmbiguity:
		return 1
	default:
		return 0
	}
}

func traceEventData(exec executedIntelTask) map[string]string {
	data := map[string]string{
		"task":              exec.route.Task,
		"failure_class":     exec.route.FailureClass,
		"backend":           nonEmpty(exec.invocation.Backend, "noop"),
		"model":             nonEmpty(exec.invocation.Model, "noop-runtime"),
		"validator_outcome": exec.validatorOutcome,
		"patch_outcome":     exec.outcome,
		"tokens_in":         fmt.Sprintf("%d", exec.invocation.TokensIn),
		"tokens_out":        fmt.Sprintf("%d", exec.invocation.TokensOut),
		"latency_ms":        fmt.Sprintf("%d", exec.invocation.LatencyMS),
	}
	if exec.invocation.PatchID != "" {
		data["patch_id"] = exec.invocation.PatchID
	}
	if exec.invocation.PatchEffect != "" {
		data["patch_effect"] = exec.invocation.PatchEffect
	}
	if exec.invocation.ValidatorMessage != "" {
		data["validator_message"] = exec.invocation.ValidatorMessage
	}
	if len(exec.affectedChunkIDs) > 0 {
		data["affected_chunk_count"] = fmt.Sprintf("%d", len(exec.affectedChunkIDs))
	}
	return data
}

func trimDiagnostic(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if len(value) <= 180 {
		return value
	}
	return value[:177] + "..."
}
