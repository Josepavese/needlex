package transport

import (
	"fmt"
	"io"

	"github.com/josepavese/needlex/internal/intel"
)

type compactAcquisition struct {
	Status                  string   `json:"status"`
	ProposedAction          string   `json:"proposed_action,omitempty"`
	BaselineAction          string   `json:"baseline_action"`
	AllowedActions          []string `json:"allowed_actions"`
	Calibration             string   `json:"calibration"`
	ObservationOnly         bool     `json:"observation_only"`
	UnassessedEvidenceCount int      `json:"unassessed_evidence_count"`
	Reason                  string   `json:"reason,omitempty"`
}

func compactAcquisitionFor(observation *intel.AcquisitionObservation) *compactAcquisition {
	if observation == nil {
		return nil
	}
	return &compactAcquisition{Status: observation.Status, ProposedAction: observation.ProposedAction, BaselineAction: observation.BaselineAction, AllowedActions: append([]string{}, observation.Input.AllowedActions...), Calibration: observation.Calibration, ObservationOnly: true, UnassessedEvidenceCount: observation.UnassessedEvidenceCount, Reason: observation.Reason}
}

func renderAcquisitionText(w io.Writer, observation *compactAcquisition) {
	if observation != nil {
		fmt.Fprintf(w, "Acquisition observation: %s; proposed=%s actual=%s (%s, advisory only)\n", observation.Status, observation.ProposedAction, observation.BaselineAction, observation.Calibration)
	}
}
