package intel

import "context"

// DecisionObserver borrows a local runtime. Its output is advisory, never a
// replacement for dense embeddings, source proofs or transport state.
type DecisionObserver interface {
	Observe(context.Context, DecisionInput) (DecisionObservation, error)
}

type DecisionInput struct {
	Assertion string             `json:"assertion"`
	Evidence  []DecisionEvidence `json:"evidence"`
}

type DecisionEvidence struct {
	Heading []string `json:"heading,omitempty"`
	ID      string   `json:"id"`
	Text    string   `json:"text"`
}

type DecisionAnswer struct {
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities"`
}

// DecisionExecution is the common diagnostic contract of one local choice.
// Task-specific observations add scope; it never grants authority to act.
type DecisionExecution struct {
	RuntimeMS     int64                     `json:"runtime_ms,omitempty"`
	CacheHit      bool                      `json:"cache_hit,omitempty"`
	Status        string                    `json:"status"`
	Model         string                    `json:"model"`
	Revision      string                    `json:"revision"`
	Specification string                    `json:"specification"`
	Calibration   string                    `json:"calibration"`
	ElapsedMS     int64                     `json:"elapsed_ms"`
	InputBytes    int                       `json:"input_bytes"`
	Answers       map[string]DecisionAnswer `json:"answers,omitempty"`
	Reason        string                    `json:"reason,omitempty"`
}

type DecisionObservation struct {
	DecisionExecution
	Assertion               string   `json:"assertion"`
	UnassessedEvidenceCount int      `json:"unassessed_evidence_count"`
	SelectionSimilarity     *float64 `json:"selection_similarity,omitempty"`
	EvidenceIDs             []string `json:"evidence_ids,omitempty"`
}

const DecisionSpecification = "claim-support-v1"

// EvidenceCount counts attributable units, not repeated copies of an identical ID.
func (input DecisionInput) EvidenceCount() int {
	seen := map[string]bool{}
	for _, evidence := range input.Evidence {
		seen[evidence.ID] = true
	}
	return len(seen)
}
