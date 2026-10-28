package intel

import "context"

// DecisionObserver borrows a local runtime. Its output is advisory, never a
// replacement for dense embeddings, source proofs or transport state.
type DecisionObserver interface {
	Observe(context.Context, DecisionInput) (DecisionObservation, error)
}

type DecisionInput struct {
	Objective string             `json:"objective"`
	Evidence  []DecisionEvidence `json:"evidence"`
}

type DecisionEvidence struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type DecisionAnswer struct {
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities"`
}

type DecisionObservation struct {
	Status        string                    `json:"status"`
	Model         string                    `json:"model"`
	Revision      string                    `json:"revision"`
	Specification string                    `json:"specification"`
	Calibration   string                    `json:"calibration"`
	ElapsedMS     int64                     `json:"elapsed_ms"`
	InputBytes    int                       `json:"input_bytes"`
	EvidenceIDs   []string                  `json:"evidence_ids,omitempty"`
	Answers       map[string]DecisionAnswer `json:"answers,omitempty"`
	Reason        string                    `json:"reason,omitempty"`
}

const DecisionSpecification = "evidence-audit-v1"
