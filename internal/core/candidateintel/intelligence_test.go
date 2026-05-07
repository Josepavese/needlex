package candidateintel

import (
	"context"
	"strings"
	"testing"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/intel"
)

func TestApplyPromotesSemanticCustodianRoleOverDerivativeSurface(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{
			URL:   "https://secondary.example/collected/entity-record",
			Label: "Collected entity record",
			Score: 1.00,
			Metadata: map[string]string{
				"resource_class":  discoverycore.ResourceClassHTMLLike,
				"source_context":  "secondary representation commentary collection explaining another source",
				"host_root_title": "Collected explanations",
			},
		},
		{
			URL:   "https://custodian.example/reference/entity",
			Label: "Maintained entity reference",
			Score: 0.96,
			Metadata: map[string]string{
				"resource_class":  discoverycore.ResourceClassHTMLLike,
				"source_context":  "responsible custodian maintained record primary reference provenance",
				"host_root_title": "Entity custodian",
			},
		},
	}

	ranked := Apply(context.Background(), roleSemanticAligner{}, "find the maintained primary reference for the entity", candidates)
	if ranked[0].URL != "https://custodian.example/reference/entity" {
		t.Fatalf("expected semantic custodian candidate first, got %q", ranked[0].URL)
	}
	if ranked[0].Metadata["semantic_role"] == "" {
		t.Fatalf("expected semantic role metadata on promoted candidate: %#v", ranked[0].Metadata)
	}
	if !hasReason(ranked[0].Reason, "semantic_custodian_alignment") {
		t.Fatalf("expected semantic custodian reason, got %#v", ranked[0].Reason)
	}
}

func TestWindowKeepsExpandedSemanticCandidatePool(t *testing.T) {
	candidates := make([]discoverycore.Candidate, 0, 8)
	for i := 0; i < 8; i++ {
		candidates = append(candidates, discoverycore.Candidate{
			URL:   "https://candidate.example/" + string(rune('a'+i)),
			Score: 1.00 - float64(i)*0.04,
		})
	}
	if got := Window(candidates); got != 8 {
		t.Fatalf("expected semantic window to include 8 candidates, got %d", got)
	}
}

func TestWindowSkipsClearStructuralLeaderWithoutProvenanceConflict(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{URL: "https://example.com/root", Score: 4.8},
		{URL: "https://example.com/docs", Score: 4.1},
		{URL: "https://example.com/other", Score: 3.4},
	}
	if got := Window(candidates); got != 0 {
		t.Fatalf("expected clear structural leader to skip semantic review, got %d", got)
	}
}

func TestWindowKeepsSemanticReviewForProvenanceConflict(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{URL: "https://related.example/", Score: 4.8, Reason: []string{"same_family_canonical_root"}},
		{URL: "https://origin.example/docs", Score: 3.7, Reason: []string{"host_root_identity_probe"}},
		{URL: "https://example.com/other", Score: 3.4},
	}
	if got := Window(candidates); got != 3 {
		t.Fatalf("expected provenance conflict to trigger semantic review, got %d", got)
	}
}

type roleSemanticAligner struct{}

func (roleSemanticAligner) Align(context.Context, string, []intel.SemanticCandidate) (intel.SemanticAlignment, error) {
	return intel.SemanticAlignment{}, nil
}

func (roleSemanticAligner) Score(_ context.Context, objective string, candidates []intel.SemanticCandidate) ([]intel.SemanticScore, error) {
	out := make([]intel.SemanticScore, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, intel.SemanticScore{ID: candidate.ID, Similarity: semanticRoleTestScore(objective, candidate)})
	}
	return out, nil
}

func semanticRoleTestScore(objective string, candidate intel.SemanticCandidate) float64 {
	objectiveRole := roleClassForTest(objective)
	candidateRole := roleClassForTest(candidate.Text + " " + candidate.ID)
	switch {
	case candidate.ID == roleCustodianOrigin || candidate.ID == roleCustodianRecord:
		if objectiveRole == roleCustodianRecord {
			return 0.90
		}
		return 0.18
	case candidate.ID == roleDerivative || candidate.ID == roleSocialContext:
		if objectiveRole == roleDerivative {
			return 0.92
		}
		return 0.12
	case candidate.ID == roleDistributionNode:
		if objectiveRole == roleDistributionNode {
			return 0.90
		}
		return 0.08
	case objectiveRole == candidateRole && objectiveRole != "":
		return 0.88
	case objectiveRole == roleCustodianRecord && candidateRole == roleDerivative:
		return 0.05
	case objectiveRole == roleDerivative && candidateRole == roleCustodianRecord:
		return 0.10
	default:
		return 0.34
	}
}

func roleClassForTest(text string) string {
	text = strings.ToLower(text)
	switch {
	case strings.Contains(text, "secondary") || strings.Contains(text, "commentary") || strings.Contains(text, "collection"):
		return roleDerivative
	case strings.Contains(text, "custodian") || strings.Contains(text, "maintained") || strings.Contains(text, "primary") || strings.Contains(text, "reference"):
		return roleCustodianRecord
	case strings.Contains(text, "distribution"):
		return roleDistributionNode
	default:
		return ""
	}
}

func hasReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
