package intel

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
)

const (
	SchemaTaskV1  = "slm-task.v1"
	SchemaPatchV1 = "slm-patch.v1"
)

type AmbiguityCandidate struct {
	ChunkID     string   `json:"chunk_id"`
	Fingerprint string   `json:"fingerprint"`
	Text        string   `json:"text"`
	HeadingPath []string `json:"heading_path,omitempty"`
	Score       float64  `json:"score"`
	Confidence  float64  `json:"confidence"`
	RiskFlags   []string `json:"risk_flags,omitempty"`
	IRSignals   []string `json:"ir_signals,omitempty"`
}

type ResolveAmbiguityInput struct {
	Objective    string               `json:"objective"`
	FailureClass string               `json:"failure_class"`
	Candidates   []AmbiguityCandidate `json:"candidates"`
}

type ResolveAmbiguityPatch struct {
	SelectedChunkIDs []string `json:"selected_chunk_ids"`
	RejectedChunkIDs []string `json:"rejected_chunk_ids,omitempty"`
	DecisionReason   string   `json:"decision_reason"`
	Confidence       float64  `json:"confidence"`
}

func (i ResolveAmbiguityInput) Validate() error {
	errs := []error{}
	if strings.TrimSpace(i.Objective) == "" {
		errs = append(errs, fmt.Errorf("objective must not be empty"))
	}
	if strings.TrimSpace(i.FailureClass) == "" {
		errs = append(errs, fmt.Errorf("failure_class must not be empty"))
	}
	if len(i.Candidates) < 2 {
		errs = append(errs, fmt.Errorf("candidates must contain at least 2 items"))
	}
	seenChunkIDs := map[string]struct{}{}
	for idx, candidate := range i.Candidates {
		if strings.TrimSpace(candidate.ChunkID) == "" {
			errs = append(errs, fmt.Errorf("candidates[%d].chunk_id must not be empty", idx))
		}
		if strings.TrimSpace(candidate.Fingerprint) == "" {
			errs = append(errs, fmt.Errorf("candidates[%d].fingerprint must not be empty", idx))
		}
		if strings.TrimSpace(candidate.Text) == "" {
			errs = append(errs, fmt.Errorf("candidates[%d].text must not be empty", idx))
		}
		if candidate.Score < 0 || candidate.Score > 1 {
			errs = append(errs, fmt.Errorf("candidates[%d].score must be between 0 and 1", idx))
		}
		if candidate.Confidence < 0 || candidate.Confidence > 1 {
			errs = append(errs, fmt.Errorf("candidates[%d].confidence must be between 0 and 1", idx))
		}
		if _, ok := seenChunkIDs[candidate.ChunkID]; ok {
			errs = append(errs, fmt.Errorf("candidates[%d].chunk_id %q duplicated", idx, candidate.ChunkID))
		}
		seenChunkIDs[candidate.ChunkID] = struct{}{}
	}
	return joinErrors(errs...)
}

func (i ResolveAmbiguityInput) Request(trace ModelTraceContext) (ModelRequest, error) {
	return i.RequestWithRoute(TaskRoute{
		Task:         TaskResolveAmbiguity,
		FailureClass: FailureClassAmbiguityConflict,
		ModelClass:   ModelClassMicroSolver,
		Budget: TaskBudget{
			MaxInputTokens:  512,
			MaxOutputTokens: 128,
			TimeoutMS:       150,
		},
		ReasonCode: ReasonAmbiguityTriggered,
	}, trace)
}

func (i ResolveAmbiguityInput) RequestWithRoute(route TaskRoute, trace ModelTraceContext) (ModelRequest, error) {
	if err := i.Validate(); err != nil {
		return ModelRequest{}, err
	}
	if err := route.Validate(); err != nil {
		return ModelRequest{}, err
	}
	if route.Task != TaskResolveAmbiguity {
		return ModelRequest{}, fmt.Errorf("task route %q does not match resolve ambiguity", route.Task)
	}
	compacted := compactResolveAmbiguityInput(i, route.Budget.MaxInputTokens)
	return ModelRequest{
		Task:            route.Task,
		ModelClass:      route.ModelClass,
		MaxInputTokens:  route.Budget.MaxInputTokens,
		MaxOutputTokens: route.Budget.MaxOutputTokens,
		TimeoutMS:       route.Budget.TimeoutMS,
		SchemaName:      SchemaTaskV1,
		Input: map[string]any{
			"objective":     compacted.Objective,
			"failure_class": compacted.FailureClass,
			"candidates":    ambiguityCandidatesPayload(compacted.Candidates),
		},
		Trace: trace,
	}, nil
}

func ValidateResolveAmbiguityPatch(input ResolveAmbiguityInput, patch ResolveAmbiguityPatch, minConfidence float64) error {
	if err := input.Validate(); err != nil {
		return err
	}
	errs := []error{}
	if len(patch.SelectedChunkIDs) == 0 {
		errs = append(errs, fmt.Errorf("selected_chunk_ids must not be empty"))
	}
	if strings.TrimSpace(patch.DecisionReason) == "" {
		errs = append(errs, fmt.Errorf("decision_reason must not be empty"))
	}
	if patch.Confidence < minConfidence || patch.Confidence > 1 {
		errs = append(errs, fmt.Errorf("confidence must be between %.2f and 1", minConfidence))
	}
	allowed := map[string]struct{}{}
	for _, candidate := range input.Candidates {
		allowed[candidate.ChunkID] = struct{}{}
	}
	selectedSeen := map[string]struct{}{}
	for _, chunkID := range patch.SelectedChunkIDs {
		if _, ok := allowed[chunkID]; !ok {
			errs = append(errs, fmt.Errorf("selected chunk_id %q is not part of ambiguity input", chunkID))
			continue
		}
		if _, ok := selectedSeen[chunkID]; ok {
			errs = append(errs, fmt.Errorf("selected chunk_id %q duplicated", chunkID))
		}
		selectedSeen[chunkID] = struct{}{}
	}
	for _, chunkID := range patch.RejectedChunkIDs {
		if _, ok := allowed[chunkID]; !ok {
			errs = append(errs, fmt.Errorf("rejected chunk_id %q is not part of ambiguity input", chunkID))
			continue
		}
		if _, ok := selectedSeen[chunkID]; ok {
			errs = append(errs, fmt.Errorf("chunk_id %q cannot be both selected and rejected", chunkID))
		}
	}
	return joinErrors(errs...)
}

func ambiguityCandidatesPayload(candidates []AmbiguityCandidate) []any {
	out := make([]any, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, map[string]any{
			"chunk_id":     candidate.ChunkID,
			"fingerprint":  candidate.Fingerprint,
			"text":         candidate.Text,
			"heading_path": append([]string{}, candidate.HeadingPath...),
			"score":        candidate.Score,
			"confidence":   candidate.Confidence,
			"risk_flags":   append([]string{}, candidate.RiskFlags...),
			"ir_signals":   append([]string{}, candidate.IRSignals...),
		})
	}
	return out
}

func CandidateIRSignals(raw []string) []string {
	clean := map[string]struct{}{}
	for _, value := range raw {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		clean[value] = struct{}{}
	}
	return slices.Sorted(maps.Keys(clean))
}

func ParseResolveAmbiguityPatch(raw string) (ResolveAmbiguityPatch, error) {
	var patch ResolveAmbiguityPatch
	if err := json.Unmarshal([]byte(raw), &patch); err != nil {
		return ResolveAmbiguityPatch{}, fmt.Errorf("decode resolve ambiguity patch: %w", err)
	}
	return patch, nil
}
