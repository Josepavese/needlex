package service

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/josepavese/needlex/internal/intel"
)

// All advisory work follows final operational selection, including fallbacks.
func (s *Service) observeReadDecisions(ctx context.Context, req ReadRequest, response *ReadResponse) error {
	defer func() { response.acquisitionPage = nil }()
	if err := s.observeReadDecision(ctx, req.Assertion, response); err != nil {
		return err
	}
	if !req.ObserveAcquisition || response.acquisitionPage == nil {
		return nil
	}
	started := s.now()
	input, err := s.sourceResolver().AcquisitionInput(sourceResolutionRequest(req), *response.acquisitionPage, response.acquisitionBudget)
	result := intel.AcquisitionObservation{Input: input, ObservationOnly: true, DecisionExecution: intel.DecisionExecution{Status: "disabled", Specification: intel.AcquisitionSpecification, Reason: "local decision observation is not configured"}}
	result.Input.Evidence = nil
	result.UnassessedEvidenceCount = (intel.DecisionInput{Evidence: input.Evidence}).EvidenceCount()
	if err == nil && s.acquisitionObserver != nil {
		result, err = s.acquisitionObserver.ObserveAcquisition(ctx, input)
	}
	if err != nil || (result.Status == "observed" && !slices.Contains(input.AllowedActions, result.ProposedAction)) {
		result.Status, result.ProposedAction, result.Answers = "unavailable", "", nil
		result.Reason = "acquisition observation failed or proposed an unavailable route"
		if err != nil {
			result.Reason = err.Error()
		}
	}
	result.ObservationOnly = true
	result.BaselineAction = acquisitionBaseline(*response)
	result.ElapsedMS = s.now().Sub(started).Milliseconds()
	audit, encodeErr := json.Marshal(result)
	if encodeErr != nil {
		result.Status, result.ProposedAction, result.Answers = "unavailable", "", nil
		result.Reason = "invalid acquisition diagnostic"
		audit, _ = json.Marshal(result)
	}
	response.AgentContext.Acquisition = &result
	return s.appendDecisionAudit(response, "acquisition_observation", audit)
}

func acquisitionBaseline(response ReadResponse) string {
	action := intel.AcquisitionServe
	for _, stage := range response.Trace.Stages {
		if stage.Stage == "agent_readable" && stage.Metadata["selected"] == "true" {
			action = intel.AcquisitionDeclared
			if declaration := stage.Metadata["selected_declared_by"]; declaration != "link_header" && declaration != "html_link" {
				action = intel.AcquisitionConventional
			}
		}
		if stage.Stage == "render" && stage.Metadata["rendered"] == "true" {
			return intel.AcquisitionRender
		}
	}
	return action
}
