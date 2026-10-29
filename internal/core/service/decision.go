package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/proof"
)

// observeReadDecision audits only finalized operational work and continues its trace.
func (s *Service) observeReadDecision(ctx context.Context, assertion string, response *ReadResponse) error {
	if strings.TrimSpace(assertion) == "" {
		return nil
	}
	input := intel.DecisionInput{Assertion: assertion}
	for _, chunk := range response.ResultPack.Chunks {
		input.Evidence = append(input.Evidence, intel.DecisionEvidence{ID: chunk.ID, Heading: chunk.HeadingPath, Text: chunk.Text})
	}
	started := s.now()
	result := intel.DecisionObservation{Assertion: assertion, UnassessedEvidenceCount: input.EvidenceCount(), DecisionExecution: intel.DecisionExecution{Status: "disabled", Specification: intel.DecisionSpecification, Reason: "local decision observation is not configured"}}
	var err error
	if s.decisionObserver != nil {
		result, err = s.decisionObserver.Observe(ctx, input)
	}
	result.ElapsedMS = s.now().Sub(started).Milliseconds()
	if err != nil {
		result.Status = "unavailable"
		result.Reason = err.Error()
	}
	// Persist the typed audit with the existing trace so observations survive the
	// request. It never changes content, confidence or proofs.
	audit, encodeErr := json.Marshal(result)
	if encodeErr != nil {
		result.Status = "unavailable"
		result.Answers = nil
		result.Reason = "invalid decision diagnostic"
		audit, _ = json.Marshal(result)
	}
	response.AgentContext.Decision = &result
	return s.appendDecisionAudit(response, "decision_observation", audit)
}

func (s *Service) appendDecisionAudit(response *ReadResponse, stage string, audit []byte) error {
	recorder := proof.NewRecorder(response.Trace.RunID, response.Trace.TraceID, response.Trace.FinishedAt)
	recorder.ModelIntervention(stage, "advisory_only", "local decision audit; no retrieval action", 0, map[string]string{"observation": string(audit)}, s.now().UTC())
	auditTrace := recorder.Finish(s.now().UTC())
	response.Trace.Events = append(response.Trace.Events, auditTrace.Events...)
	response.Trace.FinishedAt = auditTrace.FinishedAt
	replay, err := response.Trace.ReplayReport()
	if err != nil {
		return err
	}
	response.Replay = replay
	response.ResultPack.CostReport = buildCostReport(response.Trace, response.ResultPack.CostReport.LanePath)
	return response.Validate()
}
