package service

import "testing"

func TestQueryCandidateDiagnosticsExposeSemanticRoleMetadata(t *testing.T) {
	diagnostics := queryCandidateDiagnostics([]DiscoverCandidate{{
		URL:   "https://source.example/record",
		Score: 1.23,
		Reason: []string{
			"semantic_custodian_alignment",
		},
		Metadata: map[string]string{
			"resource_class":                 "html_like",
			"semantic_role":                  "custodian_record",
			"semantic_role_confidence":       "0.880",
			"semantic_role_intent":           "0.910",
			"semantic_origin_alignment":      "0.801",
			"semantic_derivative_alignment":  "0.042",
			"cluster_id":                     "c1",
			"cluster_size":                   "3",
			"late_interaction_score":         "0.733",
			"late_interaction_confidence":    "0.144",
			"semantic_evidence_similarity":   "0.830",
			"semantic_evidence_boost":        "0.620",
			"semantic_origin_similarity":     "0.710",
			"semantic_derivative_similarity": "0.120",
			"semantic_community_similarity":  "0.080",
			"semantic_authority_boost":       "0.390",
			"semantic_authority_penalty":     "0.050",
			"semantic_community_penalty":     "0.020",
			"semantic_quorum_provider_count": "2",
			"semantic_calibration_score":     "0.087",
		},
	}})
	if len(diagnostics) != 1 {
		t.Fatalf("expected one diagnostic, got %d", len(diagnostics))
	}
	got := diagnostics[0]
	if got.SemanticRole != "custodian_record" ||
		got.SemanticOriginAlignment != 0.801 ||
		got.ClusterSize != 3 ||
		got.LateInteractionScore != 0.733 ||
		got.SemanticEvidenceSimilarity != 0.830 ||
		got.SemanticEvidenceBoost != 0.620 ||
		got.SemanticOriginSimilarity != 0.710 ||
		got.SemanticDerivativeSimilarity != 0.120 ||
		got.SemanticCommunitySimilarity != 0.080 ||
		got.SemanticAuthorityBoost != 0.390 ||
		got.SemanticAuthorityPenalty != 0.050 ||
		got.SemanticCommunityPenalty != 0.020 ||
		got.SemanticQuorumProviderCount != 2 ||
		got.SemanticCalibrationScore != 0.087 {
		t.Fatalf("unexpected diagnostic: %#v", got)
	}
}
