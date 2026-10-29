package transport

import (
	"fmt"
	"io"

	"github.com/josepavese/needlex/internal/intel"
)

// Compact scope stays explicit: a model verdict concerns selected units only.
type compactDecision struct {
	Assertion               string   `json:"assertion"`
	Status                  string   `json:"status"`
	Verdict                 string   `json:"verdict,omitempty"`
	EvidenceIDs             []string `json:"evidence_ids,omitempty"`
	UnassessedEvidenceCount int      `json:"unassessed_evidence_count"`
	Calibration             string   `json:"calibration"`
	Reason                  string   `json:"reason,omitempty"`
}

func compactDecisionFor(observation *intel.DecisionObservation) *compactDecision {
	if observation == nil {
		return nil
	}
	return &compactDecision{observation.Assertion, observation.Status, observation.Answers["support"].Choice, append([]string{}, observation.EvidenceIDs...), observation.UnassessedEvidenceCount, observation.Calibration, observation.Reason}
}

func renderDecisionText(w io.Writer, observation *compactDecision) {
	if observation != nil {
		fmt.Fprintf(w, "Advisory assertion: %s\nDecision: %s %s (%s); unassessed units: %d\n", observation.Assertion, observation.Status, observation.Verdict, observation.Calibration, observation.UnassessedEvidenceCount)
	}
}
