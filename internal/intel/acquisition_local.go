package intel

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

var acquisitionCriteria = map[string]string{
	AcquisitionServe:        "Use the supplied static passage; it already provides the requested information",
	AcquisitionDeclared:     "Read an available declared machine-readable source to obtain missing requested information",
	AcquisitionRender:       "Run page JavaScript to obtain requested information missing from the static passage",
	AcquisitionInsufficient: "Report insufficient evidence; the permitted acquisition routes cannot obtain the requested information",
}

func (o *localDecisionObserver) ObserveAcquisition(ctx context.Context, input AcquisitionInput) (result AcquisitionObservation, err error) {
	started := time.Now()
	result = AcquisitionObservation{DecisionExecution: o.execution(AcquisitionSpecification), ObservationOnly: true, Input: input}
	result.Input.Evidence = nil
	defer func() { result.ElapsedMS = time.Since(started).Milliseconds() }()
	result.UnassessedEvidenceCount = (DecisionInput{Evidence: input.Evidence}).EvidenceCount()
	if err := o.cfg.Validate(); err != nil {
		return result, err
	}
	if strings.TrimSpace(input.Objective) == "" {
		result.Status = "insufficient_input"
		return result, nil
	}
	criteria := map[string]string{}
	for _, action := range input.AllowedActions {
		label, ok := acquisitionCriteria[action]
		if !ok {
			return result, fmt.Errorf("unknown acquisition action %q", action)
		}
		if _, duplicate := criteria[action]; duplicate {
			return result, fmt.Errorf("duplicate acquisition action %q", action)
		}
		criteria[action] = label
	}
	if len(criteria) < 2 {
		result.Status = "not_applicable"
		result.Reason = "no permitted choice"
		return result, nil
	}
	ctx, cancel := contextWithTimeoutMS(ctx, o.cfg.TimeoutMS)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	// Metadata is serialized canonically as state text. Its admission cost is
	// reserved before evidence selection, preserving complete UTF-8 units.
	metadata := input
	metadata.Evidence = nil
	metadata.SourceHash = ""
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return result, err
	}
	prefix := string(encoded) + "\nStatic passage:\n"
	prefixJSON, _ := json.Marshal(prefix)
	var unit *DecisionEvidence
	passage := "No extractable static passage is available."
	if len(input.Evidence) > 0 {
		var reason string
		var similarity *float64
		unit, similarity, reason, err = o.selectEvidence(ctx, DecisionInput{Assertion: input.Objective, Evidence: input.Evidence}, len(prefixJSON)-2)
		if err != nil {
			return result, err
		}
		result.Reason = reason
		result.SelectionSimilarity = similarity
		if unit == nil {
			result.Status = reason
			return result, nil
		}
		result.Input.Evidence = []DecisionEvidence{*unit}
		result.SelectedEvidenceIDs = []string{unit.ID}
		passage = decisionEvidenceText(*unit)
	}
	state, _ := json.Marshal(prefix + passage)
	result.InputBytes = len(state) + len(input.Objective)
	if result.InputBytes > o.cfg.MaxInputBytes {
		result.Status = "input_budget_exceeded"
		return result, nil
	}
	question := decisionQuestion{Type: "choice", Instructions: "For this objective, choose the permitted acquisition route most likely to obtain the requested evidence with the least work: " + input.Objective + ". Use only the supplied state; unassessed passages may contain more information.", Criteria: criteria}
	err = o.observeChoice(ctx, &result.DecisionExecution, state, AcquisitionQuestion, question, input.SourceHash)
	if result.Status == "observed" {
		result.ProposedAction = result.Answers[AcquisitionQuestion].Choice
		if !slices.Contains(input.AllowedActions, result.ProposedAction) {
			return result, fmt.Errorf("model proposed an unavailable acquisition route")
		}
		if unit != nil {
			result.UnassessedEvidenceCount--
		}
	}
	result.ElapsedMS = time.Since(started).Milliseconds()
	return result, err
}
