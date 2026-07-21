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
			"resource_class":                      "html_like",
			"semantic_role":                       "custodian_record",
			"semantic_role_confidence":            "0.880",
			"semantic_role_intent":                "0.910",
			"semantic_origin_alignment":           "0.801",
			"semantic_derivative_alignment":       "0.042",
			"cluster_id":                          "c1",
			"cluster_size":                        "3",
			"late_interaction_score":              "0.733",
			"late_interaction_confidence":         "0.144",
			"semantic_evidence_similarity":        "0.830",
			"semantic_evidence_boost":             "0.620",
			"semantic_origin_similarity":          "0.710",
			"semantic_derivative_similarity":      "0.120",
			"semantic_community_similarity":       "0.080",
			"semantic_authority_boost":            "0.390",
			"semantic_authority_penalty":          "0.050",
			"semantic_community_penalty":          "0.020",
			"semantic_quorum_provider_count":      "2",
			"semantic_calibration_score":          "0.087",
			"semantic_provenance_identity":        "0.644",
			"semantic_provenance_topic":           "0.512",
			"semantic_provenance_boost":           "0.321",
			"semantic_provenance_penalty":         "0.011",
			"semantic_family_intent_score":        "0.812",
			"semantic_family_intent_identity":     "0.812",
			"semantic_family_intent_topic":        "0.604",
			"semantic_family_intent_merit":        "1.211",
			"semantic_family_intent_boost":        "0.184",
			"semantic_family_intent_origin":       "0.744",
			"semantic_family_intent_derivative":   "0.033",
			"semantic_family_intent_count":        "4",
			"semantic_family_intent_provenance":   "2",
			"semantic_family_evidence_count":      "3",
			"semantic_family_evidence_strong":     "2",
			"semantic_family_evidence_provenance": "1",
			"semantic_family_evidence_support":    "0.377",
			"semantic_near_tie_merit":             "0.512",
			"semantic_near_tie_boost":             "0.144",
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
		got.SemanticCalibrationScore != 0.087 ||
		got.SemanticProvenanceIdentity != 0.644 ||
		got.SemanticProvenanceTopic != 0.512 ||
		got.SemanticProvenanceBoost != 0.321 ||
		got.SemanticProvenancePenalty != 0.011 ||
		got.SemanticFamilyIntentScore != 0.812 ||
		got.SemanticFamilyIntentIdentity != 0.812 ||
		got.SemanticFamilyIntentTopic != 0.604 ||
		got.SemanticFamilyIntentMerit != 1.211 ||
		got.SemanticFamilyIntentBoost != 0.184 ||
		got.SemanticFamilyIntentOrigin != 0.744 ||
		got.SemanticFamilyIntentDerivative != 0.033 ||
		got.SemanticFamilyIntentCount != 4 ||
		got.SemanticFamilyIntentProvenance != 2 ||
		got.SemanticFamilyEvidenceCount != 3 ||
		got.SemanticFamilyEvidenceStrong != 2 ||
		got.SemanticFamilyProvenance != 1 ||
		got.SemanticFamilyEvidence != 0.377 ||
		got.SemanticNearTieMerit != 0.512 ||
		got.SemanticNearTieBoost != 0.144 {
		t.Fatalf("unexpected diagnostic: %#v", got)
	}
}
