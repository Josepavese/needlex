package semanticrank

import (
	"context"
	"testing"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/intel"
)

type stubSemantic struct {
	scores map[string]float64
}

func (s stubSemantic) Align(context.Context, string, []intel.SemanticCandidate) (intel.SemanticAlignment, error) {
	return intel.SemanticAlignment{}, nil
}

func (s stubSemantic) Score(_ context.Context, _ string, candidates []intel.SemanticCandidate) ([]intel.SemanticScore, error) {
	out := make([]intel.SemanticScore, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, intel.SemanticScore{ID: candidate.ID, Similarity: s.scores[candidate.ID]})
	}
	return out, nil
}

func TestRerankPromotesSemanticLateInteractionWinner(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{URL: "https://derived.example/a", Score: 1.00, Label: "Derivative", Metadata: map[string]string{"semantic_role": "derivative_representation", "source_context": "secondary commentary"}},
		{URL: "https://origin.example/b", Score: 0.95, Label: "Origin", Metadata: map[string]string{"semantic_role": "custodian_record", "source_context": "maintained authoritative record", "semantic_origin_alignment": "0.700"}},
	}
	spans, _ := candidateSpans(candidates)
	scores := map[string]float64{}
	for _, span := range spans {
		if span.Text == "Maintained authoritative record, documentation, reference, specification, policy, API contract, or canonical knowledge from the responsible family." {
			scores[span.ID] = 0.80
			continue
		}
		scores[span.ID] = 0.05
	}
	got := Reranker{Semantic: stubSemantic{scores: scores}, Config: DefaultConfig()}.Rerank(context.Background(), "authoritative source", candidates)
	if got[0].URL != "https://origin.example/b" {
		t.Fatalf("expected semantic late interaction to promote origin, got %#v", got)
	}
	if got[0].Metadata["late_interaction_score"] == "" {
		t.Fatalf("expected late interaction metadata, got %#v", got[0].Metadata)
	}
	if !contains(got[0].Reason, ReasonLateInteraction) {
		t.Fatalf("expected late interaction reason, got %#v", got[0].Reason)
	}
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
