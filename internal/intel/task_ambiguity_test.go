package intel

import (
	"strings"
	"testing"
)

func TestResolveAmbiguityInputRequestBuildsValidModelRequest(t *testing.T) {
	input := ResolveAmbiguityInput{
		Objective:    "proof replay deterministic",
		FailureClass: "ambiguity_conflict",
		Candidates: []AmbiguityCandidate{
			{ChunkID: "chk_1", Fingerprint: "fp_1", Text: "Proof replay.", Score: 0.82, Confidence: 0.76},
			{ChunkID: "chk_2", Fingerprint: "fp_2", Text: "Deterministic context.", Score: 0.81, Confidence: 0.75},
		},
	}

	req, err := input.Request(ModelTraceContext{TraceID: "trace_1"})
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("request should validate: %v", err)
	}
	if req.Task != TaskResolveAmbiguity {
		t.Fatalf("expected task %q, got %q", TaskResolveAmbiguity, req.Task)
	}
}

func TestValidateResolveAmbiguityPatchRejectsUnknownChunk(t *testing.T) {
	input := ResolveAmbiguityInput{
		Objective:    "proof replay deterministic",
		FailureClass: "ambiguity_conflict",
		Candidates: []AmbiguityCandidate{
			{ChunkID: "chk_1", Fingerprint: "fp_1", Text: "Proof replay.", Score: 0.82, Confidence: 0.76},
			{ChunkID: "chk_2", Fingerprint: "fp_2", Text: "Deterministic context.", Score: 0.81, Confidence: 0.75},
		},
	}

	err := ValidateResolveAmbiguityPatch(input, ResolveAmbiguityPatch{
		SelectedChunkIDs: []string{"chk_404"},
		DecisionReason:   "choose best candidate",
		Confidence:       0.9,
	}, 0.6)
	if err == nil {
		t.Fatal("expected unknown chunk to fail validation")
	}
}

func TestValidateResolveAmbiguityPatchAcceptsConsistentPatch(t *testing.T) {
	input := ResolveAmbiguityInput{
		Objective:    "proof replay deterministic",
		FailureClass: "ambiguity_conflict",
		Candidates: []AmbiguityCandidate{
			{ChunkID: "chk_1", Fingerprint: "fp_1", Text: "Proof replay.", Score: 0.82, Confidence: 0.76},
			{ChunkID: "chk_2", Fingerprint: "fp_2", Text: "Deterministic context.", Score: 0.81, Confidence: 0.75},
		},
	}

	err := ValidateResolveAmbiguityPatch(input, ResolveAmbiguityPatch{
		SelectedChunkIDs: []string{"chk_1"},
		RejectedChunkIDs: []string{"chk_2"},
		DecisionReason:   "better objective coverage",
		Confidence:       0.9,
	}, 0.6)
	if err != nil {
		t.Fatalf("expected patch to validate, got %v", err)
	}
}

func TestResolveAmbiguityRequestCompactsCandidatesToBudget(t *testing.T) {
	input := ResolveAmbiguityInput{
		Objective:    "proof replay deterministic",
		FailureClass: FailureClassAmbiguityConflict,
		Candidates: []AmbiguityCandidate{
			{ChunkID: "chk_1", Fingerprint: "fp_1", Text: strings.Repeat("alpha ", 80), HeadingPath: []string{"A", "B", "C"}, Score: 0.82, Confidence: 0.76},
			{ChunkID: "chk_2", Fingerprint: "fp_2", Text: strings.Repeat("beta ", 80), HeadingPath: []string{"A", "B", "C"}, Score: 0.81, Confidence: 0.75},
		},
	}
	req, err := input.RequestWithRoute(TaskRoute{
		Task:         TaskResolveAmbiguity,
		FailureClass: FailureClassAmbiguityConflict,
		ModelClass:   ModelClassMicroSolver,
		Budget:       TaskBudget{MaxInputTokens: 64, MaxOutputTokens: 32, TimeoutMS: 1000},
		ReasonCode:   ReasonAmbiguityTriggered,
	}, ModelTraceContext{TraceID: "trace_1"})
	if err != nil {
		t.Fatalf("build compacted request: %v", err)
	}
	candidates := req.Input["candidates"].([]any)
	first := candidates[0].(map[string]any)
	if len(strings.Fields(first["text"].(string))) >= 80 {
		t.Fatalf("expected first candidate to be compacted, got %q", first["text"])
	}
	if got := first["heading_path"].([]string); len(got) != 2 {
		t.Fatalf("expected heading path clamp to 2, got %#v", got)
	}
}
