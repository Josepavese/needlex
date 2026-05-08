package semanticcalibrate

import (
	"testing"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
)

func TestApplyPromotesSemanticEvidence(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{URL: "https://weak.example", Score: 1.00, Metadata: map[string]string{"semantic_derivative_alignment": "0.700"}},
		{URL: "https://strong.example", Score: 0.96, Metadata: map[string]string{"late_interaction_score": "0.800", "semantic_origin_alignment": "0.900", "semantic_quorum_provider_count": "2"}},
	}
	got := Apply(candidates, DefaultModel())
	if got[0].URL != "https://strong.example" {
		t.Fatalf("expected semantic calibrator to promote stronger evidence, got %#v", got)
	}
	if got[0].Metadata["semantic_calibration_model"] == "" {
		t.Fatalf("expected calibration metadata, got %#v", got[0].Metadata)
	}
}
