package intel

import "context"

// DecisionModel exposes task contracts; model judgments never authorize actions.
// Both observers borrow the same local runtime, HTTP boundary and bounded memo.
type DecisionModel interface {
	DecisionObserver
	AcquisitionObserver
}

type AcquisitionObserver interface {
	ObserveAcquisition(context.Context, AcquisitionInput) (AcquisitionObservation, error)
}

const (
	AcquisitionSpecification = "acquisition-route-v1"
	AcquisitionQuestion      = "acquisition"
	AcquisitionServe         = "serve_static"
	AcquisitionDeclared      = "read_declared"
	AcquisitionRender        = "render"
	AcquisitionInsufficient  = "insufficient"
	// AcquisitionConventional records non-direct selected-source provenance,
	// including indirect links; it is never a proposed action.
	AcquisitionConventional = "read_conventional"
)

type AcquisitionSource struct {
	URL        string `json:"url"`
	Kind       string `json:"kind"`
	DeclaredBy string `json:"declared_by"`
}

// AcquisitionInput contains only the original HTTP surface and permitted routes,
// never later rendered content or an oracle label. Evidence units are whole.
type AcquisitionInput struct {
	SourcePartial     bool                `json:"source_partial"`
	HasScriptElements bool                `json:"has_script_elements"`
	Objective         string              `json:"objective"`
	URL               string              `json:"url"`
	ContentType       string              `json:"content_type"`
	SourceClass       string              `json:"source_class,omitempty"`
	SourceHash        string              `json:"source_hash,omitempty"`
	AllowedActions    []string            `json:"allowed_actions"`
	DeclaredSources   []AcquisitionSource `json:"declared_sources,omitempty"`
	Evidence          []DecisionEvidence  `json:"evidence,omitempty"`
}

type AcquisitionObservation struct {
	DecisionExecution
	Input                   AcquisitionInput `json:"input"`
	SelectedEvidenceIDs     []string         `json:"selected_evidence_ids,omitempty"`
	SelectionSimilarity     *float64         `json:"selection_similarity,omitempty"`
	UnassessedEvidenceCount int              `json:"unassessed_evidence_count"`
	ProposedAction          string           `json:"proposed_action,omitempty"`
	BaselineAction          string           `json:"baseline_action,omitempty"`
	ObservationOnly         bool             `json:"observation_only"`
}
